package formatters

import (
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/go-quicktest/qt"

	"github.com/Rethunk-Tech/claude-format-hooks/internal/config"
)

func TestWriteFormattedPreservesExistingMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows doesn't model POSIX executable bits")
	}
	path := filepath.Join(t.TempDir(), "f")
	qt.Assert(t, qt.IsNil(os.WriteFile(path, []byte("old"), 0o755))) //nolint:gosec // test fixture

	qt.Assert(t, qt.IsNil(writeFormatted(path, []byte("old"), []byte("new"), 0o644)))

	info, err := os.Stat(path)
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.Equals(info.Mode().Perm(), os.FileMode(0o755)), qt.Commentf("existing mode must survive a reformat"))
}

func TestWriteFormattedFallsBackToDefaultModeWhenFileIsGone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "never-existed")

	qt.Assert(t, qt.IsNil(writeFormatted(path, nil, []byte("new"), 0o644)))

	info, err := os.Stat(path)
	qt.Assert(t, qt.IsNil(err))
	if runtime.GOOS != "windows" {
		qt.Check(t, qt.Equals(info.Mode().Perm(), os.FileMode(0o644)))
	}
}

func TestWriteFormattedFollowsSymlinkToTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows symlink creation requires elevated privileges")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	link := filepath.Join(dir, "link")
	qt.Assert(t, qt.IsNil(os.WriteFile(target, []byte("old"), 0o644))) //nolint:gosec // test fixture
	qt.Assert(t, qt.IsNil(os.Symlink(target, link)))

	qt.Assert(t, qt.IsNil(writeFormatted(link, []byte("old"), []byte("new"), 0o644)))

	got, err := os.ReadFile(target) //nolint:gosec // path is the temp dir this test just wrote
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.Equals(string(got), "new"))
	info, err := os.Lstat(link)
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.Equals(info.Mode()&os.ModeSymlink, os.ModeSymlink), qt.Commentf("the symlink node must remain intact"))
}

func TestWriteFormattedDanglingSymlinkNoOp(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows symlink creation requires elevated privileges")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "missing-target")
	link := filepath.Join(dir, "link")
	qt.Assert(t, qt.IsNil(os.Symlink(target, link)))

	qt.Assert(t, qt.IsNil(writeFormatted(link, []byte("old"), []byte("new"), 0o644)))

	_, readErr := os.ReadFile(link) //nolint:gosec // path is the temp dir this test just created
	qt.Check(t, qt.IsTrue(os.IsNotExist(readErr)), qt.Commentf("reading through a dangling symlink must fail"))
	gotTarget, err := os.Readlink(link)
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.Equals(gotTarget, target), qt.Commentf("a dangling symlink target must remain unchanged"))
	_, err = os.Stat(target)
	qt.Check(t, qt.IsTrue(os.IsNotExist(err)), qt.Commentf("a dangling symlink must not create its missing target"))
	info, err := os.Lstat(link)
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.Equals(info.Mode()&os.ModeSymlink, os.ModeSymlink), qt.Commentf("the symlink node must remain intact"))
}

func TestWriteFormattedSkipsStaleSource(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f")
	qt.Assert(t, qt.IsNil(os.WriteFile(path, []byte("newer"), 0o644))) //nolint:gosec // test fixture

	qt.Assert(t, qt.IsNil(writeFormatted(path, []byte("old"), []byte("formatted"), 0o644)))

	got, err := os.ReadFile(path) //nolint:gosec // path is the temp dir this test just wrote
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.Equals(string(got), "newer"), qt.Commentf("a newer on-disk source must not be overwritten"))
}

func TestWriteFormattedReturnsEvalSymlinksError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not model POSIX path resolution errors consistently")
	}
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	qt.Assert(t, qt.IsNil(os.WriteFile(blocker, []byte("not a directory"), 0o644))) //nolint:gosec // test fixture

	err := writeFormatted(filepath.Join(blocker, "target"), []byte("old"), []byte("new"), 0o644)

	qt.Check(t, qt.IsNotNil(err))
}

func TestWriteFormattedReturnsCreateTempError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not model POSIX directory permissions")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "f")
	qt.Assert(t, qt.IsNil(os.WriteFile(path, []byte("old"), 0o644))) //nolint:gosec // test fixture
	qt.Assert(t, qt.IsNil(os.Chmod(dir, 0o555)))
	t.Cleanup(func() {
		_ = os.Chmod(dir, 0o700)
	})
	probe, probeErr := os.CreateTemp(dir, ".chmod-probe-*")
	if probeErr == nil {
		_ = probe.Close()
		_ = os.Remove(probe.Name())
		t.Skip("directory still writable after chmod 0555")
	}

	err := writeFormatted(path, []byte("old"), []byte("new"), 0o644)

	qt.Check(t, qt.IsNotNil(err))
}

func TestWriteFormattedReturnsWriteError(t *testing.T) {
	runWithFileSizeLimit(t, func() {
		err := writeFormatted(filepath.Join(t.TempDir(), "f"), nil, []byte("new"), 0o644)

		qt.Check(t, qt.IsNotNil(err))
	})
}

func TestWriteFormattedReturnsChmodError(t *testing.T) {
	original := writeFormattedChmod
	writeFormattedChmod = func(*os.File, os.FileMode) error {
		return os.ErrPermission
	}
	t.Cleanup(func() {
		writeFormattedChmod = original
	})

	dir := t.TempDir()
	err := writeFormatted(filepath.Join(dir, "f"), nil, []byte("new"), 0o644)

	qt.Check(t, qt.IsNotNil(err))
	entries, readErr := os.ReadDir(dir)
	qt.Assert(t, qt.IsNil(readErr))
	qt.Check(t, qt.HasLen(entries, 0), qt.Commentf("failed chmod must remove the temporary file"))
}

func TestWriteFormattedReturnsCloseError(t *testing.T) {
	original := writeFormattedClose
	writeFormattedClose = func(file *os.File) error {
		if err := original(file); err != nil {
			return err
		}
		return os.ErrPermission
	}
	t.Cleanup(func() {
		writeFormattedClose = original
	})

	dir := t.TempDir()
	err := writeFormatted(filepath.Join(dir, "f"), nil, []byte("new"), 0o644)

	qt.Check(t, qt.IsNotNil(err))
	entries, readErr := os.ReadDir(dir)
	qt.Assert(t, qt.IsNil(readErr))
	qt.Check(t, qt.HasLen(entries, 0), qt.Commentf("failed close must remove the temporary file"))
}

func TestWriteFormattedReturnsReadFileErrorForDirectoryTarget(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	qt.Assert(t, qt.IsNil(os.Mkdir(target, 0o755)))

	err := writeFormatted(target, []byte("old"), []byte("new"), 0o644)

	qt.Check(t, qt.IsNotNil(err))
	entries, readErr := os.ReadDir(dir)
	qt.Assert(t, qt.IsNil(readErr))
	qt.Check(t, qt.Equals(len(entries), 1), qt.Commentf("a failed read must remove the temporary file"))
}

func TestWriteFormattedIgnoresMissingTargetAfterFormatting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing")

	qt.Assert(t, qt.IsNil(writeFormatted(path, []byte("old"), []byte("new"), 0o644)))

	_, err := os.Stat(path)
	qt.Check(t, qt.IsTrue(os.IsNotExist(err)))
}

func TestWriteFormattedReturnsRenameErrorForDirectoryTarget(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	qt.Assert(t, qt.IsNil(os.Mkdir(target, 0o755)))

	err := writeFormatted(target, nil, []byte("new"), 0o644)

	qt.Check(t, qt.IsNotNil(err))
	entries, readErr := os.ReadDir(dir)
	qt.Assert(t, qt.IsNil(readErr))
	qt.Check(t, qt.Equals(len(entries), 1), qt.Commentf("a failed rename must remove the temporary file"))
}

func TestWriteIfMissingReturnsCreateTempError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not model POSIX directory permissions")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.jsonc")
	qt.Assert(t, qt.IsNil(os.Chmod(dir, 0o555)))
	t.Cleanup(func() {
		_ = os.Chmod(dir, 0o700)
	})
	probe, probeErr := os.CreateTemp(dir, ".chmod-probe-*")
	if probeErr == nil {
		_ = probe.Close()
		_ = os.Remove(probe.Name())
		t.Skip("directory still writable after chmod 0555")
	}

	err := writeIfMissing(path, []byte("content"))

	qt.Check(t, qt.IsNotNil(err))
}

func TestWriteIfMissingReturnsLinkErrorForDanglingSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows symlink creation requires elevated privileges")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.jsonc")
	target := filepath.Join(dir, "missing")
	qt.Assert(t, qt.IsNil(os.Symlink(target, path)))

	err := writeIfMissing(path, []byte("content"))

	qt.Check(t, qt.IsNotNil(err))
	gotTarget, readErr := os.Readlink(path)
	qt.Assert(t, qt.IsNil(readErr))
	qt.Check(t, qt.Equals(gotTarget, target))
}

func TestWriteIfMissingReturnsWriteError(t *testing.T) {
	runWithFileSizeLimit(t, func() {
		err := writeIfMissing(filepath.Join(t.TempDir(), "config.jsonc"), []byte("content"))

		qt.Check(t, qt.IsNotNil(err))
	})
}

func runWithFileSizeLimit(t *testing.T, fn func()) {
	t.Helper()
	if os.Getenv("FORMAT_DISPATCH_FILE_SIZE_HELPER") == "1" {
		withFileSizeLimit(t, fn)
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run", "^"+t.Name()+"$")
	cmd.Env = append(os.Environ(), "FORMAT_DISPATCH_FILE_SIZE_HELPER=1")
	output, err := cmd.CombinedOutput()
	qt.Assert(t, qt.IsNil(err), qt.Commentf("file-size helper output: %s", output))
}

func withFileSizeLimit(t *testing.T, fn func()) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("Linux prlimit is required to exercise a real write failure")
	}

	pid := strconv.Itoa(os.Getpid())
	output, err := exec.Command("prlimit", "--pid", pid, "--fsize").Output()
	if err != nil {
		t.Skipf("prlimit is unavailable: %v", err)
	}
	fields := strings.Fields(string(output))
	if len(fields) < 3 {
		t.Fatalf("unexpected prlimit output: %q", output)
	}
	soft, hard := fields[len(fields)-3], fields[len(fields)-2]

	sigxfsz := syscall.Signal(25)
	signal.Ignore(sigxfsz)
	defer signal.Reset(sigxfsz)
	if err := exec.Command("prlimit", "--pid", pid, "--fsize=0:"+hard).Run(); err != nil {
		t.Fatalf("set file-size limit: %v", err)
	}
	defer func() {
		if err := exec.Command("prlimit", "--pid", pid, "--fsize="+soft+":"+hard).Run(); err != nil {
			t.Errorf("restore file-size limit: %v", err)
		}
	}()

	fn()
}

func TestShellFormatterPreservesExecutableBit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows doesn't model POSIX executable bits")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "t.sh")
	src := "#!/bin/sh\nif true; then\necho hi\nfi\n"
	qt.Assert(t, qt.IsNil(os.WriteFile(path, []byte(src), 0o755))) //nolint:gosec // test fixture

	res := NewShell(config.Default()).Format(t.Context(), dir, path)
	qt.Assert(t, qt.IsNil(res.Err))

	info, err := os.Stat(path)
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.Equals(info.Mode().Perm(), os.FileMode(0o755)), qt.Commentf("reformatting must not drop the executable bit"))
}

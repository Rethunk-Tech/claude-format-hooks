package formatters

import (
	"os"
	"path/filepath"
	"runtime"
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

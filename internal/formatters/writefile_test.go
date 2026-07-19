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

	qt.Assert(t, qt.IsNil(writeFormatted(path, []byte("new"), 0o644)))

	info, err := os.Stat(path)
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.Equals(info.Mode().Perm(), os.FileMode(0o755)), qt.Commentf("existing mode must survive a reformat"))
}

func TestWriteFormattedFallsBackToDefaultModeWhenFileIsGone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "never-existed")

	qt.Assert(t, qt.IsNil(writeFormatted(path, []byte("new"), 0o644)))

	info, err := os.Stat(path)
	qt.Assert(t, qt.IsNil(err))
	if runtime.GOOS != "windows" {
		qt.Check(t, qt.Equals(info.Mode().Perm(), os.FileMode(0o644)))
	}
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

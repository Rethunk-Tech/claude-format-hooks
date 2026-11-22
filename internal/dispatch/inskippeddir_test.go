package dispatch

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInSkippedDirIgnoresFilenameSegment(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if !InSkippedDir(dir, "build/ok.sh", nil) {
		t.Error(`InSkippedDir("build/ok.sh") = false, want true: directory segments still skip`)
	}
	for _, rel := range []string{"build", "out", "dist", "target", "coverage", "scripts/build"} {
		if d := filepath.Dir(rel); d != "." {
			if err := os.MkdirAll(d, 0o700); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.WriteFile(rel, []byte("#!/bin/sh\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if InSkippedDir(dir, rel, nil) {
			t.Errorf("InSkippedDir(%q) = true, want false for a file named after a skip dir", rel)
		}
	}
}

// --check node_modules with no cwd still has to prune: a last segment that
// names a skip directory is skipped when it is a directory or is missing.
func TestInSkippedDirLastSegmentDirectoryOrMissing(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	if !InSkippedDir(dir, "node_modules", nil) {
		t.Error(`InSkippedDir("node_modules") = false, want true for a missing skip directory`)
	}
	if err := os.Mkdir("node_modules", 0o700); err != nil {
		t.Fatal(err)
	}
	if !InSkippedDir(dir, "node_modules", nil) {
		t.Error(`InSkippedDir("node_modules") = false, want true for an existing skip directory`)
	}
	if err := os.Mkdir("generated", 0o700); err != nil {
		t.Fatal(err)
	}
	if !InSkippedDir(dir, "generated", []string{"generated"}) {
		t.Error(`InSkippedDir("generated") = false, want true for a configured skip directory`)
	}
}

// A file whose basename is a skip-dir name must be judged against the
// project tree, not the process cwd. Stat("build") from elsewhere is a
// miss, and a miss used to skip -- so --check from outside the project
// reported 0 files and exited 0.
func TestInSkippedDirLastSegmentFileWhenCwdDiffers(t *testing.T) {
	proj := t.TempDir()
	if err := os.WriteFile(filepath.Join(proj, "build"), []byte("#!/bin/sh\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	if InSkippedDir(proj, "build", nil) {
		t.Error(`InSkippedDir("build") = true, want false for a skip-dir-named file when cwd is not the project`)
	}
}

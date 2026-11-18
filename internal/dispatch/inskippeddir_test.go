package dispatch

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInSkippedDirIgnoresFilenameSegment(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if !InSkippedDir("build/ok.sh", nil) {
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
		if InSkippedDir(rel, nil) {
			t.Errorf("InSkippedDir(%q) = true, want false for a file named after a skip dir", rel)
		}
	}
}

// --check node_modules with no cwd still has to prune: a last segment that
// names a skip directory is skipped when it is a directory or is missing.
func TestInSkippedDirLastSegmentDirectoryOrMissing(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	if !InSkippedDir("node_modules", nil) {
		t.Error(`InSkippedDir("node_modules") = false, want true for a missing skip directory`)
	}
	if err := os.Mkdir("node_modules", 0o700); err != nil {
		t.Fatal(err)
	}
	if !InSkippedDir("node_modules", nil) {
		t.Error(`InSkippedDir("node_modules") = false, want true for an existing skip directory`)
	}
	if err := os.Mkdir("generated", 0o700); err != nil {
		t.Fatal(err)
	}
	if !InSkippedDir("generated", []string{"generated"}) {
		t.Error(`InSkippedDir("generated") = false, want true for a configured skip directory`)
	}
}

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

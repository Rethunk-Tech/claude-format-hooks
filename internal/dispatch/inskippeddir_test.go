package dispatch

import "testing"

func TestInSkippedDirIgnoresFilenameSegment(t *testing.T) {
	t.Parallel()
	for _, rel := range []string{"build", "out", "dist", "target", "coverage", "scripts/build"} {
		if InSkippedDir(rel, nil) {
			t.Errorf("InSkippedDir(%q) = true, want false for a file named after a skip dir", rel)
		}
	}
	if !InSkippedDir("build/ok.sh", nil) {
		t.Error(`InSkippedDir("build/ok.sh") = false, want true: directory segments still skip`)
	}
}

package main

import (
	"os"
	"testing"
)

func skipIfCwdSurvivesRemoval(t *testing.T, dir string) {
	t.Helper()
	if err := os.RemoveAll(dir); err != nil {
		t.Fatalf("remove current directory: %v", err)
	}
	if cwd, err := os.Getwd(); err == nil {
		t.Skipf("removing current directory leaves Getwd usable: %s", cwd)
	}
}

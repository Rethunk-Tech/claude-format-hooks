package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/go-quicktest/qt"
)

// fakeToolEnv, when set, turns this test binary into a stub formatter. A
// copy of the binary installed by fakeTool sees it in its inherited
// environment and behaves as the value says instead of running tests.
const fakeToolEnv = "FORMAT_DISPATCH_FAKE_TOOL"

func TestMain(m *testing.M) {
	if os.Getenv(fakeToolEnv) == "fail" {
		for i := 1; i <= 20; i++ {
			fmt.Printf("line %d\n", i)
		}
		os.Exit(1)
	}
	os.Exit(m.Run())
}

// fakeTool installs a copy of this test binary in dir as an executable
// named name. A copied binary rather than a #!/bin/sh script is what lets
// the stub resolve and run on Windows, where LookPath only accepts PATHEXT
// extensions and a shell script cannot execute.
func fakeTool(t *testing.T, dir, name string) {
	t.Helper()
	self, err := os.Executable()
	qt.Assert(t, qt.IsNil(err))
	bin, err := os.ReadFile(self) //nolint:gosec // self is the current test executable
	qt.Assert(t, qt.IsNil(err))
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	qt.Assert(t, qt.IsNil(os.WriteFile(filepath.Join(dir, name), bin, 0o755)))
}

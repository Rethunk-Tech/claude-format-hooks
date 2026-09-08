package installer

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/go-quicktest/qt"
)

// fakeBun puts a stub `bun` on PATH so provisioning can be exercised
// without touching the network or the operator's real global install.
// script is the body of the stub.
func fakeBun(t *testing.T, script string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake shell-script tools are POSIX-shell only")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "bun")
	qt.Assert(t, qt.IsNil(os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\n"), 0o755)))
	t.Setenv("PATH", dir)
}

func TestProvisionToolsIsANoOpWithoutBun(t *testing.T) {
	// A machine with no bun is a supported configuration -- those formatters
	// are simply skipped at format time -- so --install must not complain.
	t.Setenv("PATH", t.TempDir())
	qt.Check(t, qt.IsNil(ProvisionTools(io.Discard)))
}

func TestProvisionToolsSurvivesAFailedInstall(t *testing.T) {
	// The settings.json wiring already succeeded by this point. A registry
	// outage must not turn that into a failed install.
	fakeBun(t, `echo "registry unreachable" >&2; exit 1`)
	qt.Check(t, qt.IsNil(ProvisionTools(io.Discard)))
}

func TestFirstLineTruncatesAtTheNewline(t *testing.T) {
	qt.Check(t, qt.Equals(string(firstLine([]byte("first\nsecond\nthird"))), "first"))
	qt.Check(t, qt.Equals(string(firstLine([]byte("only"))), "only"))
	qt.Check(t, qt.Equals(string(firstLine(nil)), ""))
}

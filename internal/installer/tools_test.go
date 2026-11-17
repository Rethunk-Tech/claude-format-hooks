package installer

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
	qt.Assert(t, qt.IsNil(os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\n"), 0o755))) //nolint:gosec // test bun fixture must be executable
	t.Setenv("PATH", dir)
}

func TestProvisionToolsIsANoOpWithoutBun(t *testing.T) {
	// A machine with no bun is a supported configuration -- those formatters
	// are simply skipped at format time -- so --install must not complain.
	t.Setenv("PATH", t.TempDir())
	var buf bytes.Buffer
	qt.Check(t, qt.IsNil(ProvisionTools(&buf)))
	qt.Check(t, qt.Equals(buf.String(), ""), qt.Commentf("absent bun must not announce an install"))
}

func TestProvisionToolsAnnouncesInstallWhenBunExists(t *testing.T) {
	fakeBun(t, `exit 0`)
	var buf bytes.Buffer
	qt.Check(t, qt.IsNil(ProvisionTools(&buf)))
	qt.Check(t, qt.StringContains(buf.String(), "Installing formatters globally"))
}

func TestProvisionToolsSurvivesAFailedInstall(t *testing.T) {
	// The settings.json wiring already succeeded by this point. A registry
	// outage must not turn that into a failed install.
	fakeBun(t, `echo "registry unreachable" >&2; exit 1`)
	var buf bytes.Buffer
	qt.Check(t, qt.IsNil(ProvisionTools(&buf)))
	qt.Check(t, qt.StringContains(buf.String(), "skipped"))
	qt.Check(t, qt.StringContains(buf.String(), "registry unreachable"))
}

func TestProvisionToolsFailedInstallWithEmptyOutputOmitsToolStderr(t *testing.T) {
	fakeBun(t, `exit 1`)
	var buf bytes.Buffer
	qt.Check(t, qt.IsNil(ProvisionTools(&buf)))
	qt.Check(t, qt.StringContains(buf.String(), "skipped"))
	qt.Check(t, qt.Equals(strings.Count(buf.String(), "\n"), 2), qt.Commentf("%q", buf.String()))
}

func TestFirstLineTruncatesAtTheNewline(t *testing.T) {
	qt.Check(t, qt.Equals(string(firstLine([]byte("first\nsecond\nthird"))), "first"))
	qt.Check(t, qt.Equals(string(firstLine([]byte("only"))), "only"))
	qt.Check(t, qt.Equals(string(firstLine(nil)), ""))
}

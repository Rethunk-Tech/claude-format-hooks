package installer

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"

	"github.com/go-quicktest/qt"
)

// releaseConfig points upgradeWithConfig at a test release server serving
// this runtime's asset names.
func releaseConfig(server *httptest.Server) upgradeConfig {
	return upgradeConfig{
		client:     server.Client(),
		apiBaseURL: server.URL,
		goos:       runtime.GOOS,
		goarch:     runtime.GOARCH,
	}
}

// installedBinary writes content as the hook binary inside a fresh temp
// directory and returns its path.
func installedBinary(t *testing.T, content []byte, perm os.FileMode) string {
	t.Helper()
	target := HookBinaryPath(t.TempDir())
	qt.Assert(t, qt.IsNil(os.WriteFile(target, content, perm)))
	return target
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path) //nolint:gosec // path is a temp file the test wrote
	qt.Assert(t, qt.IsNil(err))
	return raw
}

// assertFileIs checks a file's whole content, naming what it was meant to
// prove when it does not match.
func assertFileIs(t *testing.T, path string, want []byte, why string) {
	t.Helper()
	qt.Check(t, qt.DeepEquals(readFile(t, path), want), qt.Commentf("%s", why))
}

// assertPerm checks a file's permission bits, skipping on Windows where
// they are not meaningful.
func assertPerm(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return
	}
	info, err := os.Stat(path)
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.Equals(info.Mode().Perm(), want))
}

// assertNoScratchLeftBehind checks the atomic replacement's temp sibling is
// gone, whichever way the upgrade ended.
func assertNoScratchLeftBehind(t *testing.T, target string) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Dir(target))
	qt.Assert(t, qt.IsNil(err))
	for _, entry := range entries {
		qt.Check(t, qt.Not(qt.Equals(filepath.Ext(entry.Name()), ".tmp")),
			qt.Commentf("leftover scratch file %q", entry.Name()))
	}
}

// assertRequests checks how many times the test release server was asked
// for one asset.
func assertRequests(t *testing.T, what string, counter *atomic.Int32, want int32) {
	t.Helper()
	qt.Check(t, qt.Equals(counter.Load(), want), qt.Commentf("%s requests", what))
}

package formatters

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/go-quicktest/qt"

	"github.com/Rethunk-Tech/claude-format-hooks/internal/diskcache"
)

func TestLookPathFindsRealBinary(t *testing.T) {
	isolateDiskCache(t)
	writeFakeTool(t, "findable-tool", "exit 0")

	path, err := lookPath("findable-tool")
	qt.Check(t, qt.IsNil(err))
	qt.Check(t, qt.Not(qt.Equals(path, "")))
}

func TestLookPathCachesMissAndPersistsAMarker(t *testing.T) {
	isolateDiskCache(t)
	clearPath(t)

	_, err := lookPath("still-missing-tool")
	qt.Assert(t, qt.IsNotNil(err))

	dir, ok := diskcache.Dir()
	qt.Assert(t, qt.IsTrue(ok))
	_, statErr := os.Stat(filepath.Join(dir, "missing-still-missing-tool"))
	qt.Check(t, qt.IsNil(statErr), qt.Commentf("a miss should persist a marker to disk"))
}

func TestLookPathFreshMissMasksASubsequentInstall(t *testing.T) {
	isolateDiskCache(t)
	clearPath(t)

	_, err := lookPath("was-missing-then-installed")
	qt.Assert(t, qt.IsNotNil(err))

	// A working binary of the same name lands on PATH — a fresh negative
	// cache entry (well within the TTL) must still mask it.
	writeFakeTool(t, "was-missing-then-installed", "exit 0")
	_, err = lookPath("was-missing-then-installed")
	qt.Check(t, qt.IsNotNil(err), qt.Commentf("fresh negative cache entry should short-circuit the recheck"))
}

func TestLookPathMissesAreIndependentByBinaryName(t *testing.T) {
	isolateDiskCache(t)
	clearPath(t)

	_, err := lookPath("missing-first-binary")
	qt.Assert(t, qt.IsNotNil(err))

	writeFakeTool(t, "available-second-binary", "exit 0")
	_, err = lookPath("available-second-binary")
	qt.Check(t, qt.IsNil(err), qt.Commentf("a miss for one binary must not mask another binary"))
}

func TestLookPathRechecksAfterTTLExpires(t *testing.T) {
	isolateDiskCache(t)
	writeFakeTool(t, "was-missing-now-stale", "exit 0")

	dir, ok := diskcache.Dir()
	qt.Assert(t, qt.IsTrue(ok))
	path := filepath.Join(dir, "missing-was-missing-now-stale")
	qt.Assert(t, qt.IsNil(os.MkdirAll(dir, 0o700)))
	stale := time.Now().Add(-2 * binPathCacheTTL).Unix()
	qt.Assert(t, qt.IsNil(os.WriteFile(path, []byte(strconv.FormatInt(stale, 10)+"\n"), 0o600))) //nolint:gosec // test fixture

	got, err := lookPath("was-missing-now-stale")
	qt.Check(t, qt.IsNil(err), qt.Commentf("a stale marker must not mask a now-installed binary"))
	qt.Check(t, qt.Not(qt.Equals(got, "")))
}

func TestLookPathSuccessClearsAStaleMarker(t *testing.T) {
	isolateDiskCache(t)
	writeFakeTool(t, "clears-its-marker", "exit 0")

	dir, ok := diskcache.Dir()
	qt.Assert(t, qt.IsTrue(ok))
	path := filepath.Join(dir, "missing-clears-its-marker")
	qt.Assert(t, qt.IsNil(os.MkdirAll(dir, 0o700)))
	qt.Assert(t, qt.IsNil(os.WriteFile(path, []byte("123\n"), 0o600))) //nolint:gosec // test fixture, deliberately stale/bogus

	_, err := lookPath("clears-its-marker")
	qt.Assert(t, qt.IsNil(err))

	_, statErr := os.Stat(path)
	qt.Check(t, qt.IsTrue(os.IsNotExist(statErr)), qt.Commentf("a successful lookup should clear any marker left behind"))
}

func TestLookPathDegradesGracefullyWithNoCacheDirAvailable(t *testing.T) {
	t.Setenv("CLAUDE_FORMAT_HOOKS_CACHE", "")
	// Force os.UserCacheDir to fail on every platform it supports:
	// Windows reads %LocalAppData%, everything else reads
	// $XDG_CACHE_HOME then $HOME.
	t.Setenv("LocalAppData", "")
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("HOME", "")

	_, ok := diskcache.Dir()
	qt.Assert(t, qt.IsFalse(ok), qt.Commentf("test environment must be able to force this for the assertions below to be meaningful"))

	clearPath(t)
	_, err := lookPath("no-cache-dir-tool")
	qt.Check(t, qt.IsNotNil(err), qt.Commentf("an uncached lookup must still correctly report a miss"))

	writeFakeTool(t, "no-cache-dir-tool", "exit 0")
	path, err := lookPath("no-cache-dir-tool")
	qt.Check(t, qt.IsNil(err), qt.Commentf("with no cache available, every call must fall back to a real, uncached exec.LookPath"))
	qt.Check(t, qt.Not(qt.Equals(path, "")))
}

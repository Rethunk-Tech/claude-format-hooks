package diskcache

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/go-quicktest/qt"
)

func TestDirHonorsEnvOverride(t *testing.T) {
	want := t.TempDir()
	t.Setenv("CLAUDE_FORMAT_HOOKS_CACHE", want)

	dir, ok := Dir()
	qt.Check(t, qt.IsTrue(ok))
	qt.Check(t, qt.Equals(dir, want))
}

func TestDirFallsBackToOSCacheDirWhenUnset(t *testing.T) {
	t.Setenv("CLAUDE_FORMAT_HOOKS_CACHE", "")

	dir, ok := Dir()
	qt.Assert(t, qt.IsTrue(ok), qt.Commentf("the real test environment is expected to have a usable OS cache dir"))
	qt.Check(t, qt.Equals(filepath.Base(dir), "claude-format-hooks"))
}

func TestDirDegradesGracefullyWithNoneAvailable(t *testing.T) {
	t.Setenv("CLAUDE_FORMAT_HOOKS_CACHE", "")
	// Force os.UserCacheDir to fail on every platform it supports:
	// Windows reads %LocalAppData%, everything else reads
	// $XDG_CACHE_HOME then $HOME.
	t.Setenv("LocalAppData", "")
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("HOME", "")

	_, ok := Dir()
	qt.Check(t, qt.IsFalse(ok))
}

func TestKeyIsDeterministicAndFilesystemSafe(t *testing.T) {
	a := Key("ns", "/some/path", "extra")
	b := Key("ns", "/some/path", "extra")
	qt.Check(t, qt.Equals(a, b), qt.Commentf("same inputs must hash to the same key"))

	c := Key("ns", "/some/other/path", "extra")
	qt.Check(t, qt.Not(qt.Equals(a, c)), qt.Commentf("different inputs should (almost always) hash differently"))

	qt.Check(t, qt.Equals(filepath.Base(a), a), qt.Commentf("key must not contain path separators"))
}

func TestGetSetRoundTrip(t *testing.T) {
	dir := t.TempDir()
	Set(dir, "k", "hello world")

	value, ok := Get(dir, "k", time.Hour)
	qt.Check(t, qt.IsTrue(ok))
	qt.Check(t, qt.Equals(value, "hello world"))
}

func TestGetSetRoundTripPreservesEmbeddedNewlines(t *testing.T) {
	dir := t.TempDir()
	Set(dir, "k", "line one\nline two")

	value, ok := Get(dir, "k", time.Hour)
	qt.Check(t, qt.IsTrue(ok))
	qt.Check(t, qt.Equals(value, "line one\nline two"))
}

func TestGetMissesWhenNothingCached(t *testing.T) {
	dir := t.TempDir()
	_, ok := Get(dir, "never-set", time.Hour)
	qt.Check(t, qt.IsFalse(ok))
}

func TestGetExpiresAfterMaxAge(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "k")
	stale := time.Now().Add(-time.Hour).Unix()
	qt.Assert(t, qt.IsNil(os.WriteFile(path, []byte(strconv.FormatInt(stale, 10)+"\nvalue"), 0o600))) //nolint:gosec // test fixture

	_, ok := Get(dir, "k", time.Minute)
	qt.Check(t, qt.IsFalse(ok), qt.Commentf("an hour-old entry must not survive a one-minute TTL"))
	_, err := os.Stat(path)
	qt.Check(t, qt.IsTrue(os.IsNotExist(err)), qt.Commentf("expired entries should be removed after they are observed"))
}

func TestGetPrunesExpiredEntriesInTheSameNamespace(t *testing.T) {
	dir := t.TempDir()
	freshKey := Key("ns", "fresh")
	staleKey := Key("ns", "stale")
	otherKey := Key("other", "stale")
	stale := strconv.FormatInt(time.Now().Add(-time.Hour).Unix(), 10) + "\nvalue"

	Set(dir, freshKey, "fresh")
	qt.Assert(t, qt.IsNil(os.WriteFile(filepath.Join(dir, staleKey), []byte(stale), 0o600))) //nolint:gosec // test fixture
	qt.Assert(t, qt.IsNil(os.WriteFile(filepath.Join(dir, otherKey), []byte(stale), 0o600))) //nolint:gosec // test fixture

	_, ok := Get(dir, freshKey, time.Minute)
	qt.Assert(t, qt.IsTrue(ok))

	_, err := os.Stat(filepath.Join(dir, staleKey))
	qt.Check(t, qt.IsTrue(os.IsNotExist(err)), qt.Commentf("expired entries in the requested namespace should be pruned"))
	_, err = os.Stat(filepath.Join(dir, otherKey))
	qt.Check(t, qt.IsNil(err), qt.Commentf("pruning must not cross namespace boundaries"))
}

func TestGetPrunesHyphenatedBinaryNamesAsOneNamespace(t *testing.T) {
	dir := t.TempDir()
	freshKey := "missing-current-binary"
	staleKey := "missing-old-binary"
	otherKey := "other-old-binary"
	stale := strconv.FormatInt(time.Now().Add(-time.Hour).Unix(), 10) + "\nvalue"

	Set(dir, freshKey, "fresh")
	qt.Assert(t, qt.IsNil(os.WriteFile(filepath.Join(dir, staleKey), []byte(stale), 0o600))) //nolint:gosec // test fixture
	qt.Assert(t, qt.IsNil(os.WriteFile(filepath.Join(dir, otherKey), []byte(stale), 0o600))) //nolint:gosec // test fixture

	_, ok := Get(dir, freshKey, time.Minute)
	qt.Assert(t, qt.IsTrue(ok))

	_, err := os.Stat(filepath.Join(dir, staleKey))
	qt.Check(t, qt.IsTrue(os.IsNotExist(err)), qt.Commentf("binary names after the missing- namespace must share pruning"))
	_, err = os.Stat(filepath.Join(dir, otherKey))
	qt.Check(t, qt.IsNil(err), qt.Commentf("pruning must not cross namespace boundaries"))
}

func TestGetTreatsEntryWithNoNewlineAsAMiss(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "k")
	qt.Assert(t, qt.IsNil(os.WriteFile(path, []byte("no-newline-at-all"), 0o600))) //nolint:gosec // test fixture

	_, ok := Get(dir, "k", time.Hour)
	qt.Check(t, qt.IsFalse(ok))
}

func TestGetTreatsMalformedEntryAsAMiss(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "k")
	qt.Assert(t, qt.IsNil(os.WriteFile(path, []byte("not-a-timestamp\nvalue"), 0o600))) //nolint:gosec // test fixture

	_, ok := Get(dir, "k", time.Hour)
	qt.Check(t, qt.IsFalse(ok))
}

func TestRemoveDeletesEntry(t *testing.T) {
	dir := t.TempDir()
	Set(dir, "k", "value")

	Remove(dir, "k")

	_, ok := Get(dir, "k", time.Hour)
	qt.Check(t, qt.IsFalse(ok))
}

func TestRemoveOfMissingEntryIsNoop(t *testing.T) {
	dir := t.TempDir()
	Remove(dir, "never-existed") // must not panic or error
}

func TestSetSwallowsWriteFailure(t *testing.T) {
	// A path that can't be created as a directory (its parent is a
	// regular file, not a directory) makes MkdirAll fail — Set must not
	// panic or otherwise surface that.
	parent := filepath.Join(t.TempDir(), "not-a-dir")
	qt.Assert(t, qt.IsNil(os.WriteFile(parent, []byte("x"), 0o600))) //nolint:gosec // test fixture
	Set(filepath.Join(parent, "sub"), "k", "value")
}

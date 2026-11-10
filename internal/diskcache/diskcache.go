// Package diskcache is a small disk-backed key/value cache with TTL-based
// expiry. format-dispatch is invoked as a fresh, short-lived process on
// every single file write (see AGENTS.md's cold-start rationale) — an
// in-memory cache wouldn't survive between writes, so a result that's
// expensive to recompute every invocation but rarely changes during a
// session (is this binary installed, where's the nearest biome.json,
// what does .editorconfig resolve to for this file) has to live in a
// small file on disk instead. Shared by internal/formatters and
// internal/config, neither of which may import the other in the
// direction this package would require.
package diskcache

import (
	"bytes"
	"hash/fnv"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// prunedNamespaces records the namespaces this process has already swept, so
// a run that reads the same namespace many times sweeps it once.
//
// A process marker alone is not enough: format-dispatch is a fresh process
// per file write, so "once per process" is still once per hook invocation --
// every one of them an os.ReadDir plus a read of every entry in the
// namespace. Measured with 5000 fresh entries and nothing prunable, that was
// 70-80ms per invocation against 173us on an empty cache, and a normal
// monorepo session (one entry per file formatted) reaches those numbers. The
// sweep interval below makes the cost amortized instead.
var prunedNamespaces sync.Map

// pruneInterval is how often any process may sweep a namespace. Expired
// entries linger up to this much longer than their TTL, which costs disk
// space but never a wrong answer: Get re-checks the timestamp of the entry it
// reads and treats a stale one as a miss whether or not a sweep ran.
const pruneInterval = 5 * time.Minute

// pruneStampName records when the cache directory was last swept. Dotted so
// it cannot collide with an entry: every namespace prefix is a bare word.
const pruneStampName = ".prune-stamp"

// Dir resolves the directory cache entries are stored under:
// $CLAUDE_FORMAT_HOOKS_CACHE if set, else the OS's own cache directory
// (respecting $XDG_CACHE_HOME on Linux, %LocalAppData% on Windows, etc.)
// under a claude-format-hooks subdirectory. ok is false if neither is
// available (e.g. no $HOME) — callers must degrade to doing the
// uncached work rather than fail, matching every other config-loading
// path in this hook.
func Dir() (dir string, ok bool) {
	if dir := os.Getenv("CLAUDE_FORMAT_HOOKS_CACHE"); dir != "" {
		return dir, true
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return "", false
	}
	return filepath.Join(base, "claude-format-hooks"), true
}

// Key derives a filesystem-safe cache filename from namespace and parts
// (e.g. an absolute path) that may contain characters unsafe as a raw
// filename. Hashed with a fast, non-cryptographic hash — this buckets
// cache entries, it isn't a security boundary.
func Key(namespace string, parts ...string) string {
	h := fnv.New64a()
	for _, p := range parts {
		_, _ = h.Write([]byte(p))
		_, _ = h.Write([]byte{0})
	}
	return namespace + "-" + strconv.FormatUint(h.Sum64(), 16)
}

// Get reads the value cached under key within dir, if a cache entry
// exists and is still within maxAge.
func Get(dir, key string, maxAge time.Duration) (value string, ok bool) {
	pruneOnce(dir, cacheNamespacePrefix(key), maxAge)

	path := filepath.Join(dir, key)
	raw, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return "", false
	}
	value, stamped, ok := parseEntry(raw)
	if !ok {
		return "", false
	}
	if time.Since(stamped) >= maxAge {
		_ = os.Remove(path)
		return "", false
	}
	return value, true
}

// parseEntry splits a cache entry into its value and timestamp, reporting
// ok=false for an entry it cannot read. Callers set policy from there: a
// read treats it as a miss, while the sweep leaves it alone rather than
// removing a file it does not understand.
func parseEntry(raw []byte) (value string, stamped time.Time, ok bool) {
	timestamp, payload, found := bytes.Cut(raw, []byte{'\n'})
	if !found {
		return "", time.Time{}, false
	}
	ts, err := strconv.ParseInt(string(timestamp), 10, 64)
	if err != nil {
		return "", time.Time{}, false
	}
	return string(payload), time.Unix(ts, 0), true
}

func cacheNamespacePrefix(key string) string {
	i := strings.IndexByte(key, '-')
	if i <= 0 {
		return ""
	}
	return key[:i+1]
}

// pruneOnce sweeps a namespace at most once per process and once per
// pruneInterval across processes. A cache miss must not pay for a full sweep
// of a directory that has grown one entry per formatted file.
func pruneOnce(dir, namespacePrefix string, maxAge time.Duration) {
	if namespacePrefix == "" {
		return
	}
	if _, swept := prunedNamespaces.LoadOrStore(dir+"\x00"+namespacePrefix, struct{}{}); swept {
		return
	}
	if !pruneDue(dir) {
		return
	}
	prune(dir, namespacePrefix, maxAge)
	writePruneStamp(dir)
}

// pruneDue reports whether the cache directory's last sweep is older than
// pruneInterval, or unknown. A missing or unreadable stamp counts as due so a
// cache that never records a sweep still collects expired entries.
func pruneDue(dir string) bool {
	raw, err := os.ReadFile(filepath.Clean(filepath.Join(dir, pruneStampName)))
	if err != nil {
		return true
	}
	stamped, err := strconv.ParseInt(string(bytes.TrimSpace(raw)), 10, 64)
	if err != nil {
		return true
	}
	return time.Since(time.Unix(stamped, 0)) >= pruneInterval
}

// writePruneStamp records now as the last sweep time, best effort and through
// a rename so a concurrent reader never sees a half-written timestamp.
func writePruneStamp(dir string) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	tmp, err := os.CreateTemp(dir, "."+pruneStampName+"-*")
	if err != nil {
		return
	}
	tmpName := tmp.Name()
	if _, err := tmp.WriteString(strconv.FormatInt(time.Now().Unix(), 10)); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return
	}
	if err := os.Rename(tmpName, filepath.Join(dir, pruneStampName)); err != nil {
		_ = os.Remove(tmpName)
	}
}

// prune removes expired entries under namespacePrefix. Cache cleanup is best
// effort: a failed read or remove must never block the uncached operation.
func prune(dir, namespacePrefix string, maxAge time.Duration) {
	if namespacePrefix == "" {
		return
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), namespacePrefix) {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		if !cacheEntryExpired(path, maxAge) {
			continue
		}
		// Set can refresh the path after the first read. Recheck immediately
		// before Remove so a fresh cache value is not swept away.
		if !cacheEntryExpired(path, maxAge) {
			continue
		}
		_ = os.Remove(path)
	}
}

func cacheEntryExpired(path string, maxAge time.Duration) bool {
	raw, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return false
	}
	_, stamped, ok := parseEntry(raw)
	return ok && time.Since(stamped) >= maxAge
}

// Set records value under key within dir, timestamped now. A failure to
// write is swallowed — the cache is a pure performance optimization,
// never something the cached work should be blocked on.
func Set(dir, key, value string) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	data := strconv.FormatInt(time.Now().Unix(), 10) + "\n" + value

	// Written through a temp file and renamed rather than straight to the
	// key: --check formats many files at once, so two processes or two of
	// its workers can Set the same key concurrently. A partial write would
	// be read back as a malformed entry -- which parseEntry rejects, so the
	// cost is a recomputed miss rather than a wrong answer, but rename is
	// one syscall and removes the case entirely. CreateTemp already makes
	// the file 0600, and its dotted name keeps it out of prune's
	// namespace-prefix sweep.
	tmp, err := os.CreateTemp(dir, "."+key+"-*")
	if err != nil {
		return
	}
	tmpName := tmp.Name()
	if _, err := tmp.WriteString(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return
	}
	if err := os.Rename(tmpName, filepath.Join(dir, key)); err != nil {
		_ = os.Remove(tmpName)
	}
}

// Remove deletes any cached entry for key within dir.
func Remove(dir, key string) {
	_ = os.Remove(filepath.Join(dir, key))
}

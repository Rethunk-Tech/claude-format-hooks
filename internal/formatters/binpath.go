package formatters

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// binPathCacheTTL is how long a "binary not found" result is trusted
// before lookPath re-walks $PATH for it. Each format-dispatch invocation
// is a fresh, short-lived process (see AGENTS.md's cold-start
// rationale) — an in-memory cache wouldn't survive between file writes,
// so a miss is cached to disk instead. Long enough to absorb a burst of
// many file writes (a large multi-file edit can fire tens to hundreds of
// hook invocations in quick succession) without re-walking $PATH for
// every single one; short enough that installing the missing tool
// mid-session is picked up within the same session, no restart required.
const binPathCacheTTL = 30 * time.Second

// lookPath is exec.LookPath, but a miss is remembered on disk for
// binPathCacheTTL so a burst of invocations for a tool that isn't
// installed doesn't re-walk $PATH on every single call. A hit is never
// cached — LookPath is cheap once the binary exists, and caching a
// resolved path risks running a since-removed or since-upgraded binary
// under a stale path.
func lookPath(name string) (string, error) {
	if since, ok := readMissingMarker(name); ok && time.Since(since) < binPathCacheTTL {
		return "", &exec.Error{Name: name, Err: exec.ErrNotFound}
	}

	path, err := exec.LookPath(name)
	if err != nil {
		writeMissingMarker(name)
		return "", err
	}
	removeMissingMarker(name)
	return path, nil
}

// cacheDir resolves the directory markers are stored under:
// $CLAUDE_FORMAT_HOOKS_CACHE if set, else the OS's own cache directory
// (respecting $XDG_CACHE_HOME on Linux, %LocalAppData% on Windows, etc.)
// under a claude-format-hooks subdirectory. ok is false if neither is
// available (e.g. no $HOME) — callers must degrade to an uncached
// lookup rather than fail, matching every other config-loading path in
// this hook.
func cacheDir() (dir string, ok bool) {
	if dir := os.Getenv("CLAUDE_FORMAT_HOOKS_CACHE"); dir != "" {
		return dir, true
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return "", false
	}
	return filepath.Join(base, "claude-format-hooks"), true
}

// markerPath resolves the on-disk marker file recording name's last
// known "missing" check.
func markerPath(name string) (path string, ok bool) {
	dir, ok := cacheDir()
	if !ok {
		return "", false
	}
	return filepath.Join(dir, "missing-"+name), true
}

// readMissingMarker reports the time name was last found missing, if a
// marker exists and is readable.
func readMissingMarker(name string) (time.Time, bool) {
	path, ok := markerPath(name)
	if !ok {
		return time.Time{}, false
	}
	raw, err := os.ReadFile(path) //nolint:gosec // path is our own fixed cache dir joined with a hardcoded binary name, never user input
	if err != nil {
		return time.Time{}, false
	}
	sec, err := strconv.ParseInt(strings.TrimSpace(string(raw)), 10, 64)
	if err != nil {
		return time.Time{}, false
	}
	return time.Unix(sec, 0), true
}

// writeMissingMarker records that name was just found missing. A
// failure to write is swallowed — the cache is a pure performance
// optimization, never something a missing tool's diagnostic should be
// blocked on.
func writeMissingMarker(name string) {
	path, ok := markerPath(name)
	if !ok {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	_ = os.WriteFile(path, []byte(strconv.FormatInt(time.Now().Unix(), 10)), 0o600)
}

// removeMissingMarker clears any marker left from a past miss, so a
// stale record doesn't outlive the tool actually becoming available
// again.
func removeMissingMarker(name string) {
	path, ok := markerPath(name)
	if !ok {
		return
	}
	_ = os.Remove(path)
}

package formatters

import (
	"os/exec"
	"time"

	"github.com/Rethunk-Tech/claude-format-hooks/internal/diskcache"
)

// binPathCacheTTL is how long a "binary not found" result is trusted
// before lookPath re-walks $PATH for it. Long enough to absorb a burst
// of many file writes (a large multi-file edit can fire tens to
// hundreds of hook invocations in quick succession) without re-walking
// $PATH for every single one; short enough that installing the missing
// tool mid-session is picked up within the same session, no restart
// required.
const binPathCacheTTL = 30 * time.Second

// lookPath is exec.LookPath, but a miss is remembered on disk (via
// internal/diskcache) for binPathCacheTTL so a burst of invocations for
// a tool that isn't installed doesn't re-walk $PATH on every single
// call. A hit is never cached — LookPath is cheap once the binary
// exists, and caching a resolved path risks running a since-removed or
// since-upgraded binary under a stale path.
func lookPath(name string) (string, error) {
	dir, hasCache := diskcache.Dir()
	key := diskcache.Key("missing", name)

	if hasCache {
		if _, missing := diskcache.Get(dir, key, binPathCacheTTL); missing {
			return "", &exec.Error{Name: name, Err: exec.ErrNotFound}
		}
	}

	path, err := exec.LookPath(name)
	if err != nil {
		if hasCache {
			diskcache.Set(dir, key, "")
		}
		return "", err
	}
	if hasCache {
		diskcache.Remove(dir, key)
	}
	return path, nil
}

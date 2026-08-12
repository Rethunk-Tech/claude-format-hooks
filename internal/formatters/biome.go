package formatters

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/Rethunk-Tech/claude-format-hooks/internal/diskcache"
)

// biomeFormatter shells out to Biome's CLI for JS/TS/JSX/TSX/CSS/JSONC, plus
// GraphQL and JSON when their project-aware routers select it, preferring a
// provisioned `biome` binary and falling back to bunx. Biome (Rust) has no Go
// bindings, so this stays external.
//
// It walks up from the file to the nearest biome.json/biome.jsonc and runs
// from that directory, so monorepos with a nested config (e.g. a package
// under apps/*) pick up the right config instead of the repo root's. When no
// config exists anywhere under projectRoot, it still runs — from
// projectRoot, using biome's own built-in defaults — the same way the
// bunx-based formatters (prettier, taplo, markdownlint-cli2) format any
// project regardless of whether that project has opted into their config.
type biomeFormatter struct{}

// NewBiome returns the biomeFormatter for JS/TS/JSX/TSX/CSS/JSONC, GraphQL,
// and JSON when selected by their project-aware routers.
func NewBiome() Formatter { return biomeFormatter{} }

func (biomeFormatter) Name() string { return "biome" }

func (biomeFormatter) Format(ctx context.Context, projectRoot, abs string) Result {
	cfgDir := cachedFindUpward(filepath.Dir(abs), projectRoot, "biome.json", "biome.jsonc")
	if cfgDir == "" {
		cfgDir = projectRoot
	}

	return runPathOrBunx(
		ctx,
		cfgDir,
		"biome",
		[]string{"format", "--write", "--no-errors-on-unmatched", "--", abs},
		[]string{"@biomejs/biome", "format", "--write", "--no-errors-on-unmatched", "--", abs},
	)
}

// findUpwardCacheTTL is how long a resolved (or unresolved) config
// directory is trusted before cachedFindUpward re-walks the tree. The
// directory a biome.json/biome.jsonc lives in — or its absence entirely
// — practically never changes mid-session, so this is generous compared
// to binPathCacheTTL; a stale hit just costs one extra walk after expiry,
// never a wrong result.
const findUpwardCacheTTL = 30 * time.Second

// cachedFindUpward wraps findUpward with a short-TTL disk cache: each
// format-dispatch invocation is a fresh process (see AGENTS.md's
// cold-start rationale), and this walk otherwise repeats on every single
// file written under the same directory during a session — e.g. a large
// multi-file edit across one package. An empty result (no config found
// anywhere up to root) is cached too, which is the common case for a
// project with no biome.json at all.
func cachedFindUpward(dir, root string, names ...string) string {
	cacheDir, ok := diskcache.Dir()
	if !ok {
		return findUpward(dir, root, names...)
	}

	key := diskcache.Key("findupward", append([]string{root, dir}, names...)...)
	if cached, hit := diskcache.Get(cacheDir, key, findUpwardCacheTTL); hit {
		return cached
	}

	found := findUpward(dir, root, names...)
	diskcache.Set(cacheDir, key, found)
	return found
}

// findUpward walks from dir up to (and including) root looking for any of
// names. Returns the containing directory, or "" if none is found.
func findUpward(dir, root string, names ...string) string {
	root = filepath.Clean(root)
	for {
		if slices.ContainsFunc(names, func(n string) bool {
			_, err := os.Stat(filepath.Join(dir, n))
			return err == nil
		}) {
			return dir
		}
		if dir == root || dir == "/" || dir == "." {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

package formatters

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
)

// biomeFormatter shells out to `bunx @biomejs/biome check --write` for JS/TS/JSX/
// TSX/CSS/JSONC. Biome (Rust) has no Go bindings, so this stays external.
//
// It walks up from the file to the nearest biome.json/biome.jsonc and runs
// from that directory, so monorepos with a nested config (e.g. a package
// under apps/*) pick up the right config instead of the repo root's. When no
// config exists anywhere under projectRoot, it still runs — from
// projectRoot, using biome's own built-in defaults — the same way the
// bunx-based formatters (prettier, taplo, markdownlint-cli2) format any
// project regardless of whether that project has opted into their config.
type biomeFormatter struct{}

func NewBiome() Formatter { return biomeFormatter{} }

func (biomeFormatter) Name() string { return "biome" }

func (biomeFormatter) Format(ctx context.Context, projectRoot, abs string) Result {
	if _, err := exec.LookPath("bunx"); err != nil {
		return Result{Skipped: true}
	}

	cfgDir := findUpward(filepath.Dir(abs), projectRoot, "biome.json", "biome.jsonc")
	if cfgDir == "" {
		cfgDir = projectRoot
	}

	ok, diag := runExternal(ctx, cfgDir, "bunx", []string{"@biomejs/biome", "check", "--write", "--no-errors-on-unmatched", "--", abs})
	if !ok {
		return Result{Diagnostic: diag}
	}
	return Result{}
}

// findUpward walks from dir up to (and including) root looking for any of
// names. Returns the containing directory, or "" if none is found.
func findUpward(dir, root string, names ...string) string {
	root = filepath.Clean(root)
	for {
		for _, n := range names {
			if _, err := os.Stat(filepath.Join(dir, n)); err == nil {
				return dir
			}
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

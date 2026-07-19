// Command format-dispatch is a Claude Code PostToolUse hook for
// Write/Edit/NotebookEdit. It formats/lints the file that was just written,
// natively in-process where a formatting-fidelity-safe Go implementation
// exists (JSON, shell), and via each ecosystem's own tool otherwise
// (biome, markdownlint-cli2, taplo, prettier, sqlfluff).
//
// Design contract, unchanged from the hand-written hooks this replaces:
//   - Silent on success — nothing to show, saves tokens in the transcript.
//   - On failure, print a truncated (<=10 lines / 500 chars) diagnostic to
//     stderr so a broken fixer is still debuggable.
//   - Always exit 0. A PostToolUse hook runs after the tool already
//     succeeded; it must never be the reason a Write/Edit/NotebookEdit
//     call reports failure.
//   - An unsupported extension is an instant no-op: one filepath.Ext call
//     and one map lookup, nothing else — no stat, no exec.LookPath, no
//     subprocess.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Rethunk-Tech/claude-format-hooks/internal/config"
	"github.com/Rethunk-Tech/claude-format-hooks/internal/dispatch"
	"github.com/Rethunk-Tech/claude-format-hooks/internal/hookio"
	"github.com/Rethunk-Tech/claude-format-hooks/internal/installer"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--install" {
		os.Exit(runInstall(os.Args[2:]))
	}
	os.Exit(run(os.Stdin))
}

// runInstall wires format-dispatch into the installing user's
// settings.json, replacing install.sh's jq-based mutation of the same
// file. `--dry-run` (as the sole remaining argument) previews the change
// without writing it.
func runInstall(args []string) int {
	dryRun := len(args) > 0 && args[0] == "--dry-run"

	opts, err := installer.DefaultOptions()
	if err != nil {
		fmt.Fprintf(os.Stderr, "format-dispatch --install: %v\n", err)
		return 1
	}
	if err := installer.Install(opts, dryRun, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "format-dispatch --install: %v\n", err)
		return 1
	}
	return 0
}

// run contains all logic and always returns 0, except for a genuine
// inability to even read stdin (which should never happen under Claude
// Code, but exiting non-zero there is at least diagnosable rather than
// silently swallowed).
func run(stdin io.Reader) int {
	raw, err := io.ReadAll(stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "format-dispatch: read stdin: %v\n", err)
		return 1
	}

	path := hookio.Parse(raw).FilePath()
	if path == "" {
		return 0
	}

	cfg, cfgErr := config.Load(configPath())
	if cfgErr != nil {
		// A broken user config must never turn this into a blocking
		// hook — fall back to defaults and say why on stderr.
		fmt.Fprintf(os.Stderr, "format-dispatch: config: %v (using defaults)\n", cfgErr)
		cfg = config.Default()
	}
	registry := dispatch.NewRegistry(cfg)

	// Instant no-op path for unsupported (or user-disabled) extensions:
	// no filesystem access at all beyond the two cheap calls below.
	ext := filepath.Ext(path)
	if !registry.Supported(ext) {
		return 0
	}

	abs := path
	if !filepath.IsAbs(abs) {
		if a, err := filepath.Abs(abs); err == nil {
			abs = a
		}
	}

	if info, err := os.Stat(abs); err != nil || info.IsDir() {
		return 0
	}

	projectRoot := os.Getenv("CLAUDE_PROJECT_DIR")
	if projectRoot == "" {
		if wd, err := os.Getwd(); err == nil {
			projectRoot = wd
		} else {
			projectRoot = filepath.Dir(abs)
		}
	}

	if !within(abs, projectRoot) {
		return 0
	}
	rel, err := filepath.Rel(projectRoot, abs)
	if err != nil {
		return 0
	}
	if dispatch.InVendoredDir(rel) {
		return 0
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	result := registry.Dispatch(ctx, projectRoot, abs)

	if result.Err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", registry.Name(ext), result.Err)
		return 0
	}
	if result.Diagnostic != "" {
		fmt.Fprintf(os.Stderr, "%s: fixer failed\n%s\n", registry.Name(ext), result.Diagnostic)
		return 0
	}
	return 0
}

// configPath returns the installing user's config file location:
// $CLAUDE_FORMAT_HOOKS_CONFIG if set, else ~/.claude/claude-format-hooks.json.
func configPath() string {
	if p := os.Getenv("CLAUDE_FORMAT_HOOKS_CONFIG"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".claude-format-hooks.json"
	}
	return filepath.Join(home, ".claude", "claude-format-hooks.json")
}

// within reports whether abs is at or under root. Both are resolved
// through any symlinks first, so a symlink inside root pointing outside
// it (or vice versa) can't slip past a raw string-prefix comparison. If
// symlink resolution fails for a path (e.g. it doesn't exist yet), the
// unresolved path is used for that side.
func within(abs, root string) bool {
	root = resolveSymlinks(filepath.Clean(root))
	abs = resolveSymlinks(filepath.Clean(abs))
	if abs == root {
		return true
	}
	return strings.HasPrefix(abs, root+string(filepath.Separator))
}

// resolveSymlinks returns path with all symlinks resolved, or path
// unchanged if resolution fails (e.g. the path doesn't exist yet).
func resolveSymlinks(path string) string {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return path
	}
	return resolved
}

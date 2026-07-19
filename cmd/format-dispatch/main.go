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
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/Rethunk-Tech/claude-format-hooks/internal/config"
	"github.com/Rethunk-Tech/claude-format-hooks/internal/dispatch"
	"github.com/Rethunk-Tech/claude-format-hooks/internal/hookio"
	"github.com/Rethunk-Tech/claude-format-hooks/internal/installer"
)

// errFormatterTimeout is context.Cause(ctx) once the per-file timeout below
// fires, so a hung formatter's diagnostic says why instead of surfacing the
// generic context.DeadlineExceeded.
var errFormatterTimeout = errors.New("formatter timed out after 25s")

// projectConfigFile is a project-root dotfile (same schema as the user's
// own ~/.claude/claude-format-hooks.json) letting a project opt a specific
// formatter out for itself, layered the same way .editorconfig already is
// for indent settings.
const projectConfigFile = ".claude-format-hooks.json"

const usage = `format-dispatch is a Claude Code PostToolUse hook. Invoked with no
arguments, it reads a hook payload from stdin and formats the file it names.

Usage:
  format-dispatch                    read a PostToolUse payload from stdin (normal hook invocation)
  format-dispatch --install          wire this binary into ~/.claude/settings.json as a PostToolUse hook
  format-dispatch --uninstall        remove it from ~/.claude/settings.json
  format-dispatch --install --dry-run    preview the settings.json diff for either subcommand, without writing
  format-dispatch --version          print version and build info
  format-dispatch --help             show this help
`

func main() {
	if len(os.Args) > 1 {
		os.Exit(dispatchArgs(os.Args[1:]))
	}
	os.Exit(run(os.Stdin))
}

// dispatchArgs handles every non-empty os.Args[1:] form: the --install/
// --uninstall subcommands, --version, --help/-h, and (as a fallback) an
// unrecognized flag — printed as a usage error rather than silently falling
// through to run(stdin), which would otherwise block forever waiting on
// stdin in an interactive terminal.
func dispatchArgs(args []string) int {
	switch args[0] {
	case "--install":
		return runInstall(args[1:], false)
	case "--uninstall":
		return runInstall(args[1:], true)
	case "--version":
		fmt.Println(versionString())
		return 0
	case "--help", "-h":
		fmt.Print(usage)
		return 0
	default:
		fmt.Fprint(os.Stderr, usage)
		return 1
	}
}

// versionString reports the module version and VCS revision embedded by
// `go build` (Go 1.18+), so `--version` needs no ldflags or manual bump —
// it reflects whatever commit the binary was actually built from.
func versionString() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "format-dispatch: unknown version (no build info embedded)"
	}
	version := info.Main.Version
	var revision, dirty string
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value
		case "vcs.modified":
			if s.Value == "true" {
				dirty = "-dirty"
			}
		}
	}
	if revision == "" {
		return fmt.Sprintf("format-dispatch %s", version)
	}
	if len(revision) > 12 {
		revision = revision[:12]
	}
	return fmt.Sprintf("format-dispatch %s (%s%s)", version, revision, dirty)
}

// runInstall wires format-dispatch into (--install) or removes it from
// (--uninstall) the installing user's settings.json, replacing install.sh's
// jq-based mutation of the same file. `--dry-run` (as the sole remaining
// argument) previews the change without writing it; anything else is a
// usage error rather than a silently-ignored typo.
func runInstall(args []string, uninstall bool) int {
	label := "--install"
	action := installer.Install
	if uninstall {
		label = "--uninstall"
		action = installer.Uninstall
	}

	var dryRun bool
	switch len(args) {
	case 0:
	case 1:
		if args[0] != "--dry-run" {
			fmt.Fprintf(os.Stderr, "format-dispatch %s: unrecognized argument %q\n\n%s", label, args[0], usage)
			return 1
		}
		dryRun = true
	default:
		fmt.Fprintf(os.Stderr, "format-dispatch %s: unexpected arguments %q\n\n%s", label, args, usage)
		return 1
	}

	opts, err := installer.DefaultOptions()
	if err != nil {
		fmt.Fprintf(os.Stderr, "format-dispatch %s: %v\n", label, err)
		return 1
	}
	if err := action(opts, dryRun, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "format-dispatch %s: %v\n", label, err)
		return 1
	}
	return 0
}

// run contains all logic and always returns 0, except for a genuine
// inability to even read stdin (which should never happen under Claude
// Code, but exiting non-zero there is at least diagnosable rather than
// silently swallowed).
func run(stdin io.Reader) int {
	var logPath, logFormatter, logOutcome string
	defer func() { logInvocation(logPath, logFormatter, logOutcome) }()

	raw, err := io.ReadAll(stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "format-dispatch: read stdin: %v\n", err)
		logOutcome = "error: read stdin failed"
		return 1
	}

	logPath = hookio.Parse(raw).FilePath()
	path := logPath
	if path == "" {
		logOutcome = "skip: empty payload"
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
		logOutcome = "skip: unsupported extension"
		return 0
	}
	logFormatter = registry.Name(ext)

	abs := path
	if !filepath.IsAbs(abs) {
		if a, err := filepath.Abs(abs); err == nil {
			abs = a
		}
	}

	if info, err := os.Stat(abs); err != nil || info.IsDir() {
		logOutcome = "skip: stat failed or is a directory"
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
		logOutcome = "skip: outside project root"
		return 0
	}
	rel, err := filepath.Rel(projectRoot, abs)
	if err != nil {
		logOutcome = "skip: relative path error"
		return 0
	}
	if dispatch.InVendoredDir(rel) {
		logOutcome = "skip: vendored directory"
		return 0
	}

	// A project can opt a specific formatter out for itself (e.g. it
	// already runs its own pre-commit prettier with different rules)
	// without every operator changing their global config. Same schema,
	// same Load/IsDisabled as the user-level config; a malformed project
	// file is ignored (diagnostic to stderr) rather than blocking, for the
	// same reason a malformed user config falls back above.
	if projectCfg, err := config.Load(filepath.Join(projectRoot, projectConfigFile)); err != nil {
		fmt.Fprintf(os.Stderr, "format-dispatch: project config: %v (ignoring)\n", err)
	} else if projectCfg.IsDisabled(ext) {
		logOutcome = "skip: disabled by project config"
		return 0
	}

	ctx, cancel := context.WithTimeoutCause(context.Background(), 25*time.Second, errFormatterTimeout)
	defer cancel()

	result := registry.Dispatch(ctx, projectRoot, abs)

	if result.Err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", registry.Name(ext), result.Err)
		logOutcome = fmt.Sprintf("error: %v", result.Err)
		return 0
	}
	if result.Diagnostic != "" {
		fmt.Fprintf(os.Stderr, "%s: fixer failed\n%s\n", registry.Name(ext), result.Diagnostic)
		logOutcome = "fixer failed"
		return 0
	}
	if result.Skipped {
		logOutcome = "skip: formatter declined"
	} else {
		logOutcome = "ok"
	}
	return 0
}

// logInvocation appends one line to $CLAUDE_FORMAT_HOOKS_LOG, if set — an
// opt-in troubleshooting aid for "why didn't my file get formatted",
// silent (a no-op) otherwise, matching the hook's own silent-on-success
// contract. A failure to open or write the log is swallowed: logging must
// never be the reason a hook invocation fails. The file grows unbounded —
// meant for a short debugging session, not left on permanently.
func logInvocation(path, formatterName, outcome string) {
	logPath := os.Getenv("CLAUDE_FORMAT_HOOKS_LOG")
	if logPath == "" {
		return
	}
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) //nolint:gosec // logPath is an explicit opt-in env var, by design
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	_, _ = fmt.Fprintf(f, "%s path=%q formatter=%q outcome=%q\n",
		time.Now().UTC().Format(time.RFC3339), path, formatterName, outcome)
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

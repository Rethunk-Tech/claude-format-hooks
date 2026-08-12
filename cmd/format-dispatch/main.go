// Command format-dispatch is a Claude Code PostToolUse hook for
// Write/Edit/MultiEdit/NotebookEdit. It formats/lints the file that was just written,
// natively in-process where a formatting-fidelity-safe Go implementation
// exists (JSON, shell, Go), and via each ecosystem's own tool otherwise
// (biome, markdownlint-cli2, taplo, prettier, sqlfluff, ruff/black,
// rustfmt, terraform, buf).
//
// Design contract, unchanged from the hand-written hooks this replaces:
//   - Silent on success — nothing to show, saves tokens in the transcript.
//   - On failure, print a truncated (<=10 lines / 500 chars) diagnostic to
//     stderr so a broken fixer is still debuggable.
//   - Always exit 0. A PostToolUse hook runs after the tool already
//     succeeded; it must never be the reason a Write/Edit/MultiEdit/NotebookEdit
//     call reports failure.
//   - An unsupported extension is an instant no-op: one ResolveExtension
//     call and one map lookup, nothing else — no stat, no exec.LookPath, no
//     subprocess. Extensionless files get one bounded shebang peek.
package main

import (
	"context"
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

// formatterTimeout bounds a single formatter run. Formatting one file is
// sub-500ms work in practice, and the tools it shells out to are provisioned
// at install time rather than fetched on demand, so anything approaching
// this budget is hung rather than slow.
//
// It MUST stay below the `timeout` the installer writes into settings.json
// (installer.hookTimeout): Claude Code kills the process at that limit, so a
// budget above it can never fire and the operator gets a bare kill instead
// of errFormatterTimeout's explanation. The gap absorbs process startup and
// the write of the diagnostic itself.
const formatterTimeout = 4 * time.Second

// errFormatterTimeout is context.Cause(ctx) once formatterTimeout fires, so
// a hung formatter's diagnostic says why instead of surfacing the generic
// context.DeadlineExceeded.
var errFormatterTimeout = fmt.Errorf("formatter timed out after %s", formatterTimeout)

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
  format-dispatch --upgrade          download and replace this platform's latest release binary
  format-dispatch --upgrade --dry-run    preview the binary upgrade without writing
  format-dispatch --check PATH...    report files a formatter would change, without changing them (exit 1 if any)
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
	case "--upgrade":
		return runUpgrade(args[1:])
	case "--check":
		return runCheck(args[1:], os.Stdout, os.Stderr)
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
	return versionStringFrom(info)
}

// versionStringFrom formats info, split out from versionString so tests
// can exercise every branch (revision truncation, the dirty-worktree
// suffix, the no-VCS-metadata case) against a fabricated *debug.BuildInfo
// instead of whatever happens to be embedded in the test binary itself —
// debug.ReadBuildInfo has no other seam to control that from a test.
func versionStringFrom(info *debug.BuildInfo) string {
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

	// Provision after the wiring, and only for a real --install: the
	// settings.json change is what the operator asked for and must not be
	// gated on a network round trip. --dry-run promises to write nothing,
	// which includes not installing packages.
	if !uninstall && !dryRun {
		if err := installer.ProvisionTools(os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "format-dispatch %s: %v\n", label, err)
			return 1
		}
	}
	return 0
}

// runUpgrade downloads the latest release binary for this platform, verifies
// its checksum, and replaces the installed hook binary. It never rewrites
// settings.json. `--dry-run` previews the plan without writing.
func runUpgrade(args []string) int {
	const label = "--upgrade"
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
	if err := installer.Upgrade(opts, dryRun, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "format-dispatch %s: %v\n", label, err)
		return 1
	}
	return 0
}

// run contains all logic and always returns 0, except for a genuine
// inability to even read stdin (which should never happen under Claude
// Code, but exiting non-zero there is at least diagnosable rather than
// silently swallowed). It reads as a sequence of guard checks, each
// setting logOutcome before returning; resolveTarget, projectDisables,
// and buildRegistry hold the checks' actual logic so each is independently
// testable and this function stays a readable top-level flow.
func run(stdin io.Reader) int {
	start := time.Now()
	var logPath, logFormatter, logOutcome string
	defer func() { logInvocation(logPath, logFormatter, logOutcome, time.Since(start)) }()

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

	// Instant no-op path for an extension no formatter ever handles: no
	// filesystem access at all, not even a config load — KnownExtension
	// needs no Registry to answer.
	ext := dispatch.ResolveExtension(path)
	if ext == "" {
		if shebangExt, ok := shellShebangExt(path); ok {
			ext = shebangExt
		}
	}
	if !dispatch.KnownExtension(ext) {
		logOutcome = "skip: unsupported extension"
		return 0
	}

	registry, userCfg := buildRegistry()
	if !registry.Supported(ext) {
		logOutcome = "skip: disabled by config"
		return 0
	}
	logFormatter = registry.Name(ext)

	abs, projectRoot, skipReason := resolveTarget(path)
	if skipReason != "" {
		logOutcome = skipReason
		return 0
	}

	// A project can opt a specific formatter out for itself (e.g. it
	// already runs its own pre-commit prettier with different rules)
	// without every operator changing their global config.
	if disabled, projectCfg, err := projectDisables(projectRoot, ext, registry.Name(ext)); err != nil {
		fmt.Fprintf(os.Stderr, "format-dispatch: project config: %v (ignoring)\n", err)
	} else if disabled {
		logOutcome = "skip: disabled by project config"
		return 0
	} else {
		registry = registryWithProjectConfig(registry, userCfg, ext, projectCfg)
	}

	ctx, cancel := context.WithTimeoutCause(context.Background(), formatterTimeout, errFormatterTimeout)
	defer cancel()

	result := registry.Dispatch(ctx, projectRoot, abs, ext)

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

// buildRegistry loads the user-level config and builds the formatter
// registry from it. A malformed config must never turn this into a
// blocking hook — it falls back to defaults and says why on stderr.
func buildRegistry() (*dispatch.Registry, config.Config) {
	cfg, err := config.Load(configPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "format-dispatch: config: %v (using defaults)\n", err)
		cfg = config.Default()
	}
	return dispatch.NewRegistry(cfg), cfg
}

// resolveTarget resolves path to an absolute path and its project root,
// and applies every project-boundary guard shared by every dispatch-bound
// file: existence (and not-a-directory), containment within projectRoot,
// and exclusion from vendored directories. skipReason is empty on
// success; otherwise it's why run() should skip this file, suitable for
// the invocation log as-is.
func resolveTarget(path string) (abs, projectRoot, skipReason string) {
	abs = path
	if !filepath.IsAbs(abs) {
		if a, err := filepath.Abs(abs); err == nil {
			abs = a
		}
	}

	if info, err := os.Stat(abs); err != nil || info.IsDir() {
		return abs, "", "skip: stat failed or is a directory"
	}

	projectRoot = os.Getenv("CLAUDE_PROJECT_DIR")
	if projectRoot == "" {
		if wd, err := os.Getwd(); err == nil {
			projectRoot = wd
		} else {
			projectRoot = filepath.Dir(abs)
		}
	}

	if !within(abs, projectRoot) {
		return abs, projectRoot, "skip: outside project root"
	}
	rel, err := filepath.Rel(projectRoot, abs)
	if err != nil {
		return abs, projectRoot, "skip: relative path error"
	}
	if dispatch.InVendoredDir(rel) {
		return abs, projectRoot, "skip: vendored directory"
	}
	return abs, projectRoot, ""
}

// projectDisables reports whether ext is opted out for this project via
// projectConfigFile at projectRoot. Same schema, same Load and disabled-list
// checks as the user-level config. A malformed project config returns a
// non-nil err — the caller prints it and proceeds as if nothing were
// disabled, rather than blocking formatting.
func projectDisables(projectRoot, ext, formatterName string) (disabled bool, cfg config.Config, err error) {
	cfg, err = config.Load(filepath.Join(projectRoot, projectConfigFile))
	if err != nil {
		return false, config.Config{}, err
	}
	return cfg.IsDisabled(ext) || cfg.IsFormatterDisabled(formatterName), cfg, nil
}

// projectRebuildsJSONRegistry is true when project config forces a NewRegistry
// rebuild for .json, .graphql, or .gql — the json router Names itself "json"
// and the graphql router "prettier" while biome may run underneath, so a
// project-level "biome" disable is invisible to extension filtering alone.
// Keep --check's cache guard on this same predicate.
func projectRebuildsJSONRegistry(ext string, projectCfg config.Config) bool {
	if !strings.EqualFold(ext, ".json") &&
		!strings.EqualFold(ext, ".graphql") &&
		!strings.EqualFold(ext, ".gql") {
		return false
	}
	return projectCfg.IsFormatterDisabled("biome")
}

// registryWithProjectConfig applies project formatter opt-outs that affect a
// router's internal choice, while keeping extension-specific project opt-outs
// in projectDisables. jsonRouter.Name() is "json" and graphqlRouter.Name() is
// "prettier" even when either delegates to biome, so a project-level "biome"
// disable must be applied here (and in --check) rather than relying on
// registry name filtering alone. Any future router that delegates to another
// formatter name has the same requirement.
func registryWithProjectConfig(registry *dispatch.Registry, userCfg config.Config, ext string, projectCfg config.Config) *dispatch.Registry {
	if !projectRebuildsJSONRegistry(ext, projectCfg) {
		return registry
	}
	merged := userCfg
	merged.DisabledFormatters = append(append([]string(nil), userCfg.DisabledFormatters...), projectCfg.DisabledFormatters...)
	return dispatch.NewRegistry(merged)
}

// logInvocation appends one line to $CLAUDE_FORMAT_HOOKS_LOG, if set — an
// opt-in troubleshooting aid for "why didn't my file get formatted" (and
// "why is it slow", via duration), silent (a no-op) otherwise, matching
// the hook's own silent-on-success contract. A failure to open or write
// the log is swallowed: logging must never be the reason a hook
// invocation fails. The file grows unbounded — meant for a short
// debugging session, not left on permanently.
func logInvocation(path, formatterName, outcome string, duration time.Duration) {
	logPath := os.Getenv("CLAUDE_FORMAT_HOOKS_LOG")
	if logPath == "" {
		return
	}
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) //nolint:gosec // logPath is an explicit opt-in env var, by design
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	_, _ = fmt.Fprintf(f, "%s path=%q formatter=%q outcome=%q duration=%q\n",
		time.Now().UTC().Format(time.RFC3339), path, formatterName, outcome, duration.Round(time.Microsecond))
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

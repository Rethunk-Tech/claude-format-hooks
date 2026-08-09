// Package installer wires format-dispatch into ~/.claude/settings.json as
// a PostToolUse hook, natively in Go — the same settings.json mutation
// install.sh used to perform via jq, without the external jq dependency.
package installer

import (
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const (
	matcherAll   = "Write|Edit|NotebookEdit"
	matcherOld   = "Write|Edit"
	oldBiomeMark = "biome check --write"
	statusMsg    = "format-dispatch..."
	// hookTimeout is the seconds Claude Code allows this hook before killing
	// it. Formatting one file is sub-500ms work and the external tools are
	// provisioned at install time rather than fetched on demand, so a
	// generous budget buys nothing and only delays the operator's feedback
	// when something is genuinely hung. Must stay ABOVE the binary's own
	// formatterTimeout (cmd/format-dispatch/main.go) so that budget fires
	// first and explains itself, rather than the harness killing the process
	// with no diagnostic.
	hookTimeout = 5
)

// HookCommand is one entry in a PostToolUse matcher's "hooks" array.
type HookCommand struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	// omitempty matters here: this hook takes no arguments, so without it a
	// round-trip rewrote an existing `"args": []` as `"args": null`. That is
	// not cosmetic -- Claude Code re-reads settings.json after a write, and
	// the null was enough to change how it evaluated the file. Omitting the
	// key entirely is both what the schema expects for "no arguments" and
	// the only form that survives a rewrite unchanged.
	Args          []string `json:"args,omitempty"`
	Timeout       int      `json:"timeout,omitempty"`
	StatusMessage string   `json:"statusMessage,omitempty"`
}

// PostToolUseEntry is one matcher block under hooks.PostToolUse.
type PostToolUseEntry struct {
	Matcher string        `json:"matcher"`
	Hooks   []HookCommand `json:"hooks"`
}

// Options controls where Install/Uninstall read and write.
type Options struct {
	BinPath      string
	SettingsPath string
}

// DefaultOptions resolves the binary and settings paths the same way
// install.sh did: CLAUDE_HOOKS_BIN_DIR / CLAUDE_SETTINGS_FILE, falling
// back to ~/.claude/hooks and ~/.claude/settings.json.
func DefaultOptions() (Options, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Options{}, err
	}
	binDir := cmp.Or(os.Getenv("CLAUDE_HOOKS_BIN_DIR"), filepath.Join(home, ".claude", "hooks"))
	settingsPath := cmp.Or(os.Getenv("CLAUDE_SETTINGS_FILE"), filepath.Join(home, ".claude", "settings.json"))
	return Options{
		BinPath:      HookBinaryPath(binDir),
		SettingsPath: settingsPath,
	}, nil
}

// parseSettings reads settingsPath (a missing file is treated as `{}`) and
// decodes it down to its hooks.PostToolUse entries, preserving top-level
// and hooks.* key order via orderedMap so an unrelated key survives a
// round-trip untouched.
func parseSettings(settingsPath string) (before []byte, top, hooks *orderedMap, entries []PostToolUseEntry, err error) {
	before, err = os.ReadFile(settingsPath) //nolint:gosec // caller-controlled settings location (env override or fixed default)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, nil, nil, nil, err
		}
		before = []byte("{}")
	}

	top = newOrderedMap()
	if err := json.Unmarshal(before, top); err != nil {
		return nil, nil, nil, nil, fmt.Errorf("parse %s: %w", settingsPath, err)
	}

	hooks = newOrderedMap()
	if raw, ok := top.Get("hooks"); ok {
		if err := json.Unmarshal(raw, hooks); err != nil {
			return nil, nil, nil, nil, fmt.Errorf("parse %s: hooks: %w", settingsPath, err)
		}
	}

	if raw, ok := hooks.Get("PostToolUse"); ok {
		if err := json.Unmarshal(raw, &entries); err != nil {
			return nil, nil, nil, nil, fmt.Errorf("parse %s: hooks.PostToolUse: %w", settingsPath, err)
		}
	}
	return before, top, hooks, entries, nil
}

// renderSettings re-embeds entries as hooks.PostToolUse into top/hooks and
// serializes the result, indented, with every untouched key in its
// original position.
func renderSettings(top, hooks *orderedMap, entries []PostToolUseEntry) ([]byte, error) {
	ptuRaw, err := json.Marshal(entries)
	if err != nil {
		return nil, err
	}
	hooks.Set("PostToolUse", ptuRaw)

	hooksRaw, err := json.Marshal(hooks)
	if err != nil {
		return nil, err
	}
	top.Set("hooks", hooksRaw)

	afterCompact, err := json.MarshalIndent(top, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(afterCompact, '\n'), nil
}

// Wire reads the settings JSON at settingsPath and returns the document
// before and after wiring in our PostToolUse hook. Idempotent: a prior
// entry pointing at binPath, or the old narrow inline biome-only hook
// (matcher "Write|Edit" running a "biome check --write" command), is
// removed before the new entry is appended. Every other top-level key,
// every other hooks.* event, and every other PostToolUse entry is
// preserved untouched, in its original order.
func Wire(settingsPath, binPath string) (before, after []byte, err error) {
	before, top, hooks, entries, err := parseSettings(settingsPath)
	if err != nil {
		return nil, nil, err
	}

	kept := slices.DeleteFunc(entries, func(e PostToolUseEntry) bool { return !keepEntry(e, binPath) })
	kept = append(kept, PostToolUseEntry{
		Matcher: matcherAll,
		Hooks: []HookCommand{{
			Type:          "command",
			Command:       binPath,
			Args:          []string{},
			Timeout:       hookTimeout,
			StatusMessage: statusMsg,
		}},
	})

	after, err = renderSettings(top, hooks, kept)
	if err != nil {
		return nil, nil, err
	}
	return before, after, nil
}

// Unwire reads the settings JSON at settingsPath and returns the document
// before and after removing the PostToolUse entry pointing at binPath. A
// settings file with no such entry round-trips unchanged (aside from
// re-serialization). Every other top-level key, hooks.* event, and
// PostToolUse entry is preserved untouched, in its original order.
func Unwire(settingsPath, binPath string) (before, after []byte, err error) {
	before, top, hooks, entries, err := parseSettings(settingsPath)
	if err != nil {
		return nil, nil, err
	}

	kept := slices.DeleteFunc(entries, func(e PostToolUseEntry) bool { return hasBin(e, binPath) })

	after, err = renderSettings(top, hooks, kept)
	if err != nil {
		return nil, nil, err
	}
	return before, after, nil
}

// hasBin reports whether e has a hook command pointing at binPath.
func hasBin(e PostToolUseEntry, binPath string) bool {
	return slices.ContainsFunc(e.Hooks, func(h HookCommand) bool {
		return h.Command == binPath || isHookBinaryCommand(h.Command)
	})
}

func isHookBinaryCommand(command string) bool {
	switch filepath.Base(command) {
	case "format-dispatch", "format-dispatch.exe":
		return true
	default:
		return false
	}
}

// keepEntry reports whether an existing PostToolUse entry should survive
// Wire's rewrite: it must not already be a stale copy of our own binary,
// and it must not be the old narrow biome-only hook this installer
// replaces.
func keepEntry(e PostToolUseEntry, binPath string) bool {
	if hasBin(e, binPath) {
		return false
	}
	if e.Matcher != matcherOld {
		return true
	}
	return !slices.ContainsFunc(e.Hooks, func(h HookCommand) bool { return strings.Contains(h.Command, oldBiomeMark) })
}

// Install wires the PostToolUse hook into opts.SettingsPath. If dryRun,
// the settings file is left untouched and a diff is printed to out
// instead of being written.
func Install(opts Options, dryRun bool, out io.Writer) error {
	before, after, err := Wire(opts.SettingsPath, opts.BinPath)
	if err != nil {
		return err
	}
	return applyChange(opts, before, after, dryRun, out, "Wired PostToolUse hook into")
}

// Uninstall removes the PostToolUse hook from opts.SettingsPath. If
// dryRun, the settings file is left untouched and a diff is printed to
// out instead of being written.
func Uninstall(opts Options, dryRun bool, out io.Writer) error {
	before, after, err := Unwire(opts.SettingsPath, opts.BinPath)
	if err != nil {
		return err
	}
	return applyChange(opts, before, after, dryRun, out, "Removed PostToolUse hook from")
}

// applyChange previews or writes a settings.json mutation. verb is the
// past-tense description printed on a real write, e.g. "Wired ... into".
// Before a real write, the settings file's current on-disk content (if any)
// is copied to a sibling ".bak" file, overwriting any previous backup —
// one rolling backup of the last-known-good state, not a write history.
func applyChange(opts Options, before, after []byte, dryRun bool, out io.Writer, verb string) error {
	if dryRun {
		_, _ = fmt.Fprintln(out, "==> --dry-run: settings.json diff (not written):")
		if bytes.Equal(before, after) {
			_, _ = fmt.Fprintln(out, "(no changes)")
			return nil
		}
		_, _ = fmt.Fprintf(out, "--- %s\n%s", opts.SettingsPath, before)
		_, _ = fmt.Fprintf(out, "+++ %s\n%s", opts.SettingsPath, after)
		return nil
	}

	if bytes.Equal(before, after) {
		_, _ = fmt.Fprintf(out, "==> %s already reflects this state, nothing to do\n", opts.SettingsPath)
		return nil
	}

	backupPath := opts.SettingsPath + ".bak"
	wroteBackup := false
	if existing, err := os.ReadFile(opts.SettingsPath); err == nil { //nolint:gosec // caller-controlled settings location
		if err := writeAtomic(backupPath, existing, 0o600); err != nil {
			return fmt.Errorf("backup %s: %w", opts.SettingsPath, err)
		}
		wroteBackup = true
	} else if !os.IsNotExist(err) {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(opts.SettingsPath), 0o700); err != nil {
		return err
	}
	if err := writeAtomic(opts.SettingsPath, after, 0o600); err != nil {
		return err
	}
	if wroteBackup {
		_, _ = fmt.Fprintf(out, "==> %s %s (previous version backed up to %s)\n", verb, opts.SettingsPath, backupPath)
	} else {
		_, _ = fmt.Fprintf(out, "==> %s %s\n", verb, opts.SettingsPath)
	}
	return nil
}

// writeAtomic writes data to path via a temp file in the same directory
// followed by a rename, so a process killed mid-write (or a crash) can
// never leave path holding a truncated settings.json — the file the
// installing user's entire Claude Code hook configuration lives in, not
// just this one hook's entry. The temp file is written in path's own
// directory rather than the OS temp dir so the rename is guaranteed to
// stay on one filesystem.
func writeAtomic(path string, data []byte, perm os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil { //nolint:gosec // path is caller-controlled (settings location or its .bak sibling), by design
		return err
	}
	return os.Rename(tmp, path)
}

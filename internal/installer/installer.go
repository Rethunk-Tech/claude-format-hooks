// Package installer wires format-dispatch into ~/.claude/settings.json as
// a PostToolUse hook, natively in Go — the same settings.json mutation
// install.sh used to perform via jq, without the external jq dependency.
package installer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	matcherAll   = "Write|Edit|NotebookEdit"
	matcherOld   = "Write|Edit"
	oldBiomeMark = "biome check --write"
	statusMsg    = "format-dispatch..."
	hookTimeout  = 30
)

// HookCommand is one entry in a PostToolUse matcher's "hooks" array.
type HookCommand struct {
	Type          string   `json:"type"`
	Command       string   `json:"command"`
	Args          []string `json:"args"`
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
	binDir := os.Getenv("CLAUDE_HOOKS_BIN_DIR")
	if binDir == "" {
		binDir = filepath.Join(home, ".claude", "hooks")
	}
	settingsPath := os.Getenv("CLAUDE_SETTINGS_FILE")
	if settingsPath == "" {
		settingsPath = filepath.Join(home, ".claude", "settings.json")
	}
	return Options{
		BinPath:      filepath.Join(binDir, "format-dispatch"),
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

	kept := entries[:0:0]
	for _, e := range entries {
		if keepEntry(e, binPath) {
			kept = append(kept, e)
		}
	}
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

	kept := entries[:0:0]
	for _, e := range entries {
		if !hasBin(e, binPath) {
			kept = append(kept, e)
		}
	}

	after, err = renderSettings(top, hooks, kept)
	if err != nil {
		return nil, nil, err
	}
	return before, after, nil
}

// hasBin reports whether e has a hook command pointing at binPath.
func hasBin(e PostToolUseEntry, binPath string) bool {
	for _, h := range e.Hooks {
		if h.Command == binPath {
			return true
		}
	}
	return false
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
	for _, h := range e.Hooks {
		if strings.Contains(h.Command, oldBiomeMark) {
			return false
		}
	}
	return true
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
		if err := os.WriteFile(backupPath, existing, 0o600); err != nil {
			return fmt.Errorf("backup %s: %w", opts.SettingsPath, err)
		}
		wroteBackup = true
	} else if !os.IsNotExist(err) {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(opts.SettingsPath), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(opts.SettingsPath, after, 0o600); err != nil {
		return err
	}
	if wroteBackup {
		_, _ = fmt.Fprintf(out, "==> %s %s (previous version backed up to %s)\n", verb, opts.SettingsPath, backupPath)
	} else {
		_, _ = fmt.Fprintf(out, "==> %s %s\n", verb, opts.SettingsPath)
	}
	return nil
}

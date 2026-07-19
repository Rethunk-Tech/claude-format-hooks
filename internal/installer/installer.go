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

// Options controls where Install reads and writes.
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

// Wire reads the settings JSON at settingsPath (a missing file is treated
// as `{}`) and returns the document before and after wiring in our
// PostToolUse hook. Idempotent: a prior entry pointing at binPath, or the
// old narrow inline biome-only hook (matcher "Write|Edit" running a
// "biome check --write" command), is removed before the new entry is
// appended. Every other top-level key, every other hooks.* event, and
// every other PostToolUse entry is preserved untouched.
func Wire(settingsPath, binPath string) (before, after []byte, err error) {
	before, err = os.ReadFile(settingsPath) //nolint:gosec // caller-controlled settings location (env override or fixed default)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, nil, err
		}
		before = []byte("{}")
	}

	var top map[string]json.RawMessage
	if err := json.Unmarshal(before, &top); err != nil {
		return nil, nil, fmt.Errorf("parse %s: %w", settingsPath, err)
	}
	if top == nil {
		top = map[string]json.RawMessage{}
	}

	var hooks map[string]json.RawMessage
	if raw, ok := top["hooks"]; ok {
		if err := json.Unmarshal(raw, &hooks); err != nil {
			return nil, nil, fmt.Errorf("parse %s: hooks: %w", settingsPath, err)
		}
	}
	if hooks == nil {
		hooks = map[string]json.RawMessage{}
	}

	var entries []PostToolUseEntry
	if raw, ok := hooks["PostToolUse"]; ok {
		if err := json.Unmarshal(raw, &entries); err != nil {
			return nil, nil, fmt.Errorf("parse %s: hooks.PostToolUse: %w", settingsPath, err)
		}
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

	ptuRaw, err := json.Marshal(kept)
	if err != nil {
		return nil, nil, err
	}
	hooks["PostToolUse"] = ptuRaw

	hooksRaw, err := json.Marshal(hooks)
	if err != nil {
		return nil, nil, err
	}
	top["hooks"] = hooksRaw

	afterCompact, err := json.MarshalIndent(top, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	after = append(afterCompact, '\n')
	return before, after, nil
}

// keepEntry reports whether an existing PostToolUse entry should survive
// the rewrite: it must not already be a stale copy of our own binary, and
// it must not be the old narrow biome-only hook this installer replaces.
func keepEntry(e PostToolUseEntry, binPath string) bool {
	hasOwnBin := false
	hasOldBiome := false
	for _, h := range e.Hooks {
		if h.Command == binPath {
			hasOwnBin = true
		}
		if strings.Contains(h.Command, oldBiomeMark) {
			hasOldBiome = true
		}
	}
	if hasOwnBin {
		return false
	}
	if e.Matcher == matcherOld && hasOldBiome {
		return false
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

	if err := os.MkdirAll(filepath.Dir(opts.SettingsPath), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(opts.SettingsPath, after, 0o600); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "==> Wired PostToolUse hook into %s\n", opts.SettingsPath)
	return nil
}

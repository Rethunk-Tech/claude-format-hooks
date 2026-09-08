// Package installer wires format-dispatch into ~/.claude/settings.json as
// a PostToolUse hook, natively in Go — the settings.json mutation without
// an external jq dependency.
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
)

const (
	matcherAll = "Write|Edit|MultiEdit|NotebookEdit"
	statusMsg  = "format-dispatch..."
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
	BinPath         string
	SettingsPath    string
	CursorHooksPath string
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
	cursorHooksPath := cmp.Or(os.Getenv("CURSOR_HOOKS_FILE"), filepath.Join(home, ".cursor", "hooks.json"))
	return Options{
		BinPath:         HookBinaryPath(binDir),
		SettingsPath:    settingsPath,
		CursorHooksPath: cursorHooksPath,
	}, nil
}

// jsonObject is a decoded JSON object whose values stay as raw blobs, so a
// key this package never inspects survives a round-trip byte-for-byte.
// Rewriting sorts keys, which is encoding/json's behaviour for a map.
type jsonObject map[string]json.RawMessage

// parseHookDoc reads a hooks document (a missing file is treated as `{}`)
// and decodes it down to one event's entries, leaving every key it does not
// touch as a raw blob. exists distinguishes an absent file from an empty
// one, which is what tells WireCursor whether to add a version field.
func parseHookDoc[T any](path, event string) (before []byte, top, hooks jsonObject, entries []T, exists bool, err error) {
	before, err = os.ReadFile(path) //nolint:gosec // caller-controlled hooks location (env override or fixed default)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, nil, nil, nil, false, err
		}
		before = []byte("{}")
	} else {
		exists = true
	}

	top = jsonObject{}
	if err := json.Unmarshal(before, &top); err != nil {
		return nil, nil, nil, nil, false, fmt.Errorf("parse %s: %w", path, err)
	}

	hooks = jsonObject{}
	if raw, ok := top["hooks"]; ok {
		if err := json.Unmarshal(raw, &hooks); err != nil {
			return nil, nil, nil, nil, false, fmt.Errorf("parse %s: hooks: %w", path, err)
		}
	}

	if raw, ok := hooks[event]; ok {
		if err := json.Unmarshal(raw, &entries); err != nil {
			return nil, nil, nil, nil, false, fmt.Errorf("parse %s: hooks.%s: %w", path, event, err)
		}
	}
	return before, top, hooks, entries, exists, nil
}

const postToolUseEvent = "PostToolUse"

// renderHooks re-embeds entries as hooks.<event> into top/hooks and
// serializes the result, indented. An event left with no entries is removed rather than
// written as an empty array, and a hooks object emptied that way is removed
// too, so uninstalling restores the document it started from.
func renderHooks[T any](top, hooks jsonObject, event string, entries []T) ([]byte, error) {
	_, hooksExisted := top["hooks"]
	eventDeleted := false
	if len(entries) > 0 {
		eventRaw, err := json.Marshal(entries)
		if err != nil {
			return nil, err
		}
		hooks[event] = eventRaw
	} else {
		_, eventDeleted = hooks[event]
		delete(hooks, event)
	}

	// Drop an emptied hooks object, but leave a pre-existing empty one that
	// this call never touched: it is the operator's, not ours to tidy.
	if len(hooks) == 0 && (eventDeleted || !hooksExisted) {
		delete(top, "hooks")
	} else {
		hooksRaw, err := json.Marshal(hooks)
		if err != nil {
			return nil, err
		}
		top["hooks"] = hooksRaw
	}

	after, err := json.MarshalIndent(top, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(after, '\n'), nil
}

// Wire reads the settings JSON at settingsPath and returns the document
// before and after wiring in our PostToolUse hook. Idempotent: a prior entry
// pointing at binPath is removed before the new entry is appended. Every
// other top-level key, hooks.* event, and PostToolUse entry survives with its
// value byte-for-byte, though rewriting sorts the object's keys.
func Wire(settingsPath, binPath string) (before, after []byte, err error) {
	before, top, hooks, entries, _, err := parseHookDoc[PostToolUseEntry](settingsPath, postToolUseEvent)
	if err != nil {
		return nil, nil, err
	}

	kept := slices.DeleteFunc(entries, func(e PostToolUseEntry) bool { return hasBin(e, binPath) })
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

	after, err = renderHooks(top, hooks, postToolUseEvent, kept)
	if err != nil {
		return nil, nil, err
	}
	return before, after, nil
}

// Unwire reads the settings JSON at settingsPath and returns the document
// before and after removing the PostToolUse entry pointing at binPath. A
// settings file with no such entry round-trips unchanged (aside from
// re-serialization). Every other top-level key, hooks.* event, and
// PostToolUse entry survives with its value byte-for-byte, though rewriting
// sorts the object's keys.
func Unwire(settingsPath, binPath string) (before, after []byte, err error) {
	before, top, hooks, entries, _, err := parseHookDoc[PostToolUseEntry](settingsPath, postToolUseEvent)
	if err != nil {
		return nil, nil, err
	}

	kept := slices.DeleteFunc(entries, func(e PostToolUseEntry) bool { return hasBin(e, binPath) })

	after, err = renderHooks(top, hooks, postToolUseEvent, kept)
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
	case hookBinaryName, hookBinaryName + ".exe":
		return true
	default:
		return false
	}
}

// Install wires the PostToolUse hook into opts.SettingsPath. If dryRun,
// the settings file is left untouched and a diff is printed to out
// instead of being written.
func Install(opts Options, dryRun bool, out io.Writer) error {
	before, after, err := Wire(opts.SettingsPath, opts.BinPath)
	if err != nil {
		return err
	}
	claudeChanged := !bytes.Equal(before, after)
	var cursorBefore, cursorAfter []byte
	if opts.CursorHooksPath != "" {
		cursorBefore, cursorAfter, err = WireCursor(opts.CursorHooksPath, opts.BinPath)
		if err != nil {
			return err
		}
	}
	if err := applyChange(opts, before, after, dryRun, out, "Wired PostToolUse hook into"); err != nil {
		return err
	}
	if opts.CursorHooksPath != "" {
		cursorOpts := opts
		cursorOpts.SettingsPath = opts.CursorHooksPath
		if err := applyChange(cursorOpts, cursorBefore, cursorAfter, dryRun, out, "Wired afterFileEdit hook into"); err != nil {
			if !dryRun && claudeChanged {
				rollbackBefore, rollbackAfter, rollbackErr := Unwire(opts.SettingsPath, opts.BinPath)
				if rollbackErr == nil {
					rollbackErr = applyChange(opts, rollbackBefore, rollbackAfter, false, io.Discard, "Rolled back PostToolUse hook in")
				}
				if rollbackErr != nil {
					return fmt.Errorf("cursor hook write failed: %w; Claude rollback failed: %w", err, rollbackErr)
				}
			}
			return err
		}
	}
	return nil
}

// Uninstall removes the PostToolUse hook from opts.SettingsPath. If
// dryRun, the settings file is left untouched and a diff is printed to
// out instead of being written.
func Uninstall(opts Options, dryRun bool, out io.Writer) error {
	before, after, err := Unwire(opts.SettingsPath, opts.BinPath)
	if err != nil {
		return err
	}
	var cursorBefore, cursorAfter []byte
	if opts.CursorHooksPath != "" {
		cursorBefore, cursorAfter, err = UnwireCursor(opts.CursorHooksPath, opts.BinPath)
		if err != nil {
			return err
		}
	}
	if err := applyChange(opts, before, after, dryRun, out, "Removed PostToolUse hook from"); err != nil {
		return err
	}
	if opts.CursorHooksPath != "" {
		cursorOpts := opts
		cursorOpts.SettingsPath = opts.CursorHooksPath
		if err := applyChange(cursorOpts, cursorBefore, cursorAfter, dryRun, out, "Removed afterFileEdit hook from"); err != nil {
			return err
		}
	}
	return nil
}

// applyChange previews or writes a hooks file mutation. verb is the
// past-tense description printed on a real write, e.g. "Wired ... into".
// Before a real write, the settings file's current on-disk content (if any)
// is copied to a sibling ".bak" file, overwriting any previous backup —
// one rolling backup of the last-known-good state, not a write history.
func applyChange(opts Options, before, after []byte, dryRun bool, out io.Writer, verb string) error {
	if dryRun {
		_, _ = fmt.Fprintf(out, "==> --dry-run: %s diff (not written):\n", opts.SettingsPath)
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

// writeAtomic writes data to a unique temp file in path's directory, then
// renames it into place so concurrent writers cannot share a temp pathname
// and a process killed mid-write cannot leave path truncated.
func writeAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	removeTemp := true
	defer func() {
		if removeTemp {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	removeTemp = false
	return nil
}

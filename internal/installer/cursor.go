package installer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

const cursorEvent = "afterFileEdit"

type cursorHookCommand struct {
	Command string `json:"command"`
	Timeout int    `json:"timeout"`
}

func parseCursorHooks(path string) (before []byte, top, hooks *orderedMap, entries []json.RawMessage, exists bool, err error) {
	before, err = os.ReadFile(path) //nolint:gosec // caller-controlled hooks location
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, nil, nil, nil, false, err
		}
		before = []byte("{}")
	} else {
		exists = true
	}

	top = newOrderedMap()
	if err := json.Unmarshal(before, top); err != nil {
		return nil, nil, nil, nil, false, fmt.Errorf("parse %s: %w", path, err)
	}
	if _, ok := top.Get("version"); !ok {
		top.Set("version", json.RawMessage("1"))
	}

	hooks = newOrderedMap()
	if raw, ok := top.Get("hooks"); ok {
		if err := json.Unmarshal(raw, hooks); err != nil {
			return nil, nil, nil, nil, false, fmt.Errorf("parse %s: hooks: %w", path, err)
		}
	}
	if raw, ok := hooks.Get(cursorEvent); ok {
		if err := json.Unmarshal(raw, &entries); err != nil {
			return nil, nil, nil, nil, false, fmt.Errorf("parse %s: hooks.%s: %w", path, cursorEvent, err)
		}
	}
	return before, top, hooks, entries, exists, nil
}

func renderCursorHooks(top, hooks *orderedMap, entries []json.RawMessage) ([]byte, error) {
	if len(entries) > 0 {
		if entries == nil {
			entries = make([]json.RawMessage, 0)
		}
		eventRaw, err := json.Marshal(entries)
		if err != nil {
			return nil, err
		}
		hooks.Set(cursorEvent, eventRaw)
	} else if _, ok := hooks.Get(cursorEvent); ok {
		eventRaw, err := json.Marshal([]json.RawMessage{})
		if err != nil {
			return nil, err
		}
		hooks.Set(cursorEvent, eventRaw)
	}

	hooksRaw, err := json.Marshal(hooks)
	if err != nil {
		return nil, err
	}
	top.Set("hooks", hooksRaw)

	after, err := json.MarshalIndent(top, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(after, '\n'), nil
}

// WireCursor adds the format-dispatch afterFileEdit hook while preserving
// unrelated Cursor hooks and their source order.
func WireCursor(path, binPath string) (before, after []byte, err error) {
	before, top, hooks, entries, _, err := parseCursorHooks(path)
	if err != nil {
		return nil, nil, err
	}

	kept := slices.DeleteFunc(entries, func(entry json.RawMessage) bool {
		return cursorHookHasBin(entry, binPath)
	})
	hookRaw, err := json.Marshal(cursorHookCommand{Command: binPath, Timeout: hookTimeout})
	if err != nil {
		return nil, nil, err
	}
	kept = append(kept, hookRaw)

	after, err = renderCursorHooks(top, hooks, kept)
	if err != nil {
		return nil, nil, err
	}
	return before, after, nil
}

// UnwireCursor removes only format-dispatch afterFileEdit hooks.
func UnwireCursor(path, binPath string) (before, after []byte, err error) {
	before, top, hooks, entries, exists, err := parseCursorHooks(path)
	if err != nil {
		return nil, nil, err
	}
	if !exists {
		return before, before, nil
	}

	kept := slices.DeleteFunc(entries, func(entry json.RawMessage) bool {
		return cursorHookHasBin(entry, binPath)
	})
	after, err = renderCursorHooks(top, hooks, kept)
	if err != nil {
		return nil, nil, err
	}
	return before, after, nil
}

func cursorHookHasBin(raw json.RawMessage, binPath string) bool {
	var hook cursorHookCommand
	if err := json.Unmarshal(raw, &hook); err != nil {
		return false
	}
	return hook.Command == binPath || isHookBinaryCommand(hook.Command) ||
		filepath.Base(hook.Command) == filepath.Base(binPath)
}

package installer

import (
	"encoding/json"
	"slices"
)

const cursorEvent = "afterFileEdit"

type cursorHookCommand struct {
	Command string `json:"command"`
	Timeout int    `json:"timeout"`
}

// WireCursor adds the format-dispatch afterFileEdit hook while preserving
// unrelated Cursor hooks.
func WireCursor(path, binPath string) (before, after []byte, err error) {
	before, top, hooks, entries, exists, err := parseHookDoc[json.RawMessage](path, cursorEvent)
	if err != nil {
		return nil, nil, err
	}
	if !exists {
		top["version"] = json.RawMessage("1")
	}

	kept := slices.DeleteFunc(entries, func(entry json.RawMessage) bool {
		return cursorHookHasBin(entry, binPath)
	})
	hookRaw, err := json.Marshal(cursorHookCommand{Command: binPath, Timeout: hookTimeout})
	if err != nil {
		return nil, nil, err
	}
	kept = append(kept, hookRaw)

	after, err = renderHooks(top, hooks, cursorEvent, kept)
	if err != nil {
		return nil, nil, err
	}
	return before, after, nil
}

// WiredCursor is Wired for Cursor's afterFileEdit document.
func WiredCursor(path, binPath string) (bool, error) {
	_, _, _, entries, exists, err := parseHookDoc[json.RawMessage](path, cursorEvent)
	if err != nil || !exists {
		return false, err
	}
	return slices.ContainsFunc(entries, func(entry json.RawMessage) bool {
		return cursorHookHasBin(entry, binPath)
	}), nil
}

// UnwireCursor removes only format-dispatch afterFileEdit hooks.
func UnwireCursor(path, binPath string) (before, after []byte, err error) {
	before, top, hooks, entries, exists, err := parseHookDoc[json.RawMessage](path, cursorEvent)
	if err != nil {
		return nil, nil, err
	}
	if !exists {
		return before, before, nil
	}

	kept := slices.DeleteFunc(entries, func(entry json.RawMessage) bool {
		return cursorHookHasBin(entry, binPath)
	})
	after, err = renderHooks(top, hooks, cursorEvent, kept)
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
	return hook.Command == binPath || isHookBinaryCommand(hook.Command)
}

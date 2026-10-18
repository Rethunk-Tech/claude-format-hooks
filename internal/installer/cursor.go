package installer

import (
	"encoding/json"
	"slices"
)

const (
	cursorEvent    = "afterFileEdit"
	cursorTabEvent = "afterTabFileEdit"
)

var cursorEvents = []string{cursorEvent, cursorTabEvent}

type cursorHookCommand struct {
	Command string `json:"command"`
	Timeout int    `json:"timeout"`
}

// WireCursor adds the format-dispatch afterFileEdit and afterTabFileEdit
// hooks while preserving unrelated Cursor hooks.
func WireCursor(path, binPath string) (before, after []byte, err error) {
	before, top, exists, err := readHooksDoc(path)
	if err != nil {
		return nil, nil, err
	}
	hooks, err := hooksObject(top, path)
	if err != nil {
		return nil, nil, err
	}
	if !exists {
		top["version"] = json.RawMessage("1")
	}

	hookRaw, err := json.Marshal(cursorHookCommand{Command: binPath, Timeout: hookTimeout})
	if err != nil {
		return nil, nil, err
	}
	for _, event := range cursorEvents {
		entries, err := eventEntries[json.RawMessage](hooks, path, event)
		if err != nil {
			return nil, nil, err
		}
		kept := slices.DeleteFunc(entries, func(entry json.RawMessage) bool {
			return cursorHookHasBin(entry, binPath)
		})
		kept = append(kept, hookRaw)
		after, err = renderHooks(top, hooks, event, kept)
		if err != nil {
			return nil, nil, err
		}
	}
	return before, after, nil
}

// WiredCursor is Wired for Cursor's afterFileEdit and afterTabFileEdit
// document. Both events must point at binPath; an older install that only
// wired Agent edits is not complete.
func WiredCursor(path, binPath string) (bool, error) {
	for _, event := range cursorEvents {
		entries, exists, err := hookDocEntries[json.RawMessage](path, event)
		if err != nil || !exists {
			return false, err
		}
		if !slices.ContainsFunc(entries, func(entry json.RawMessage) bool {
			return cursorHookHasBin(entry, binPath)
		}) {
			return false, nil
		}
	}
	return true, nil
}

// UnwireCursor removes only format-dispatch afterFileEdit and
// afterTabFileEdit hooks.
func UnwireCursor(path, binPath string) (before, after []byte, err error) {
	before, top, exists, err := readHooksDoc(path)
	if err != nil {
		return nil, nil, err
	}
	if !exists {
		return before, before, nil
	}
	hooks, err := hooksObject(top, path)
	if err != nil {
		return nil, nil, err
	}

	for _, event := range cursorEvents {
		entries, err := eventEntries[json.RawMessage](hooks, path, event)
		if err != nil {
			return nil, nil, err
		}
		kept := slices.DeleteFunc(entries, func(entry json.RawMessage) bool {
			return cursorHookHasBin(entry, binPath)
		})
		after, err = renderHooks(top, hooks, event, kept)
		if err != nil {
			return nil, nil, err
		}
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

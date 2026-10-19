package main

import (
	"encoding/json"
	"fmt"
	"io"
)

// modelContext is the PostToolUse hook output Claude Code reads back into
// the conversation. The hook writes to stderr for the operator, but stderr
// on a zero exit reaches nobody but the transcript -- so a formatter
// diagnostic about a file the model just wrote never reached the model
// that wrote it. This is the channel that does, without exit 2's cost:
// a hook that fails the tool call it was reacting to.
type modelContext struct {
	HookEventName     string `json:"hookEventName"`
	AdditionalContext string `json:"additionalContext"`
}

type hookOutput struct {
	HookSpecificOutput modelContext `json:"hookSpecificOutput"`
}

// emitModelContext writes one JSON line carrying text back to the model.
//
// It is a no-op unless the payload came from Claude: Cursor's afterFileEdit
// and afterTabFileEdit envelopes do not define this channel, and writing it
// there would put a stray JSON line into stdout for a contract that never
// asked for one.
//
// Silent on a marshal failure. Every caller is already on a failure path
// reporting to stderr, and a hook that cannot format a file must not then
// fail the tool call over its own error reporting.
// modelFacingFailure is the PostToolUse additionalContext for a formatter
// that could not rewrite abs. It names the tool and the file and nothing
// else: combined formatter output can echo source, config, or env, and that
// text must stay on operator stderr rather than enter the model transcript.
func modelFacingFailure(formatter, abs string) string {
	return fmt.Sprintf("%s could not format %s", formatter, abs)
}

func emitModelContext(w io.Writer, claude bool, text string) {
	if !claude || text == "" {
		return
	}
	out, err := json.Marshal(hookOutput{HookSpecificOutput: modelContext{
		HookEventName:     "PostToolUse",
		AdditionalContext: text,
	}})
	if err != nil {
		return
	}
	_, _ = fmt.Fprintln(w, string(out))
}

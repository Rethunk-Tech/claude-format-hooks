// Package hookio parses the Claude Code PostToolUse JSON envelope and the
// Cursor afterFileEdit envelope, which carries its path in top-level
// file_path. The payload is tiny (a handful of string fields, well under
// 1KB) — standard library encoding/json is already far faster than the
// ~1-3ms process startup cost it's measured against, so there is no case for
// a third-party JSON library here.
package hookio

import "encoding/json"

// Payload is the subset of Claude PostToolUse and Cursor afterFileEdit JSON
// this package reads.
type Payload struct {
	CursorFilePath string `json:"file_path"`
	ToolInput      struct {
		FilePath     string `json:"file_path"`
		NotebookPath string `json:"notebook_path"`
	} `json:"tool_input"`
	ToolResponse struct {
		FilePath string `json:"filePath"`
	} `json:"tool_response"`
	// tool_result sits beside tool_response in the PostToolUse envelope;
	// CHANGELOG.md records the behaviour and the precedence (tool_response
	// wins when both are set). The camelCase spelling is deliberate: the
	// snake_case variant is explicitly ignored, and a test pins that.
	ToolResult struct {
		FilePath string `json:"filePath"`
	} `json:"tool_result"`
}

// FilePath returns the file that was written or edited, matching the same
// field-fallback order: tool_response.filePath, tool_result.filePath,
// tool_input.file_path, tool_input.notebook_path, then Cursor afterFileEdit's
// top-level file_path.
func (p Payload) FilePath() string {
	path, _ := p.resolve()
	return path
}

// IsClaude reports whether the path came from a field only the Claude
// PostToolUse envelope defines, rather than Cursor's top-level file_path.
// Only that contract defines a JSON channel on stdout back to the model, so
// a caller must ask before writing one: Cursor's afterFileEdit does not
// read it, and writing it there would put a stray JSON line into whatever
// Cursor does with the hook's stdout.
//
// It shares resolve with FilePath rather than repeating the switch, so the
// two can never disagree about which envelope a payload came from.
func (p Payload) IsClaude() bool {
	_, claude := p.resolve()
	return claude
}

// resolve answers both questions at once: which path, and whether the field
// it came from is Claude's. An empty payload reports Cursor, which is the
// safe direction -- callers return early on an empty path, and a payload
// this package cannot recognize must not be assumed to speak a protocol it
// might not.
func (p Payload) resolve() (path string, claude bool) {
	switch {
	case p.ToolResponse.FilePath != "":
		return p.ToolResponse.FilePath, true
	case p.ToolResult.FilePath != "":
		return p.ToolResult.FilePath, true
	case p.ToolInput.FilePath != "":
		return p.ToolInput.FilePath, true
	case p.ToolInput.NotebookPath != "":
		return p.ToolInput.NotebookPath, true
	default:
		return p.CursorFilePath, false
	}
}

// Parse decodes raw stdin bytes into a Payload. An empty or malformed
// payload decodes to a Payload with an empty FilePath(), which callers
// treat as "nothing to do" rather than an error — a hook must never fail
// the tool call it's reacting to.
func Parse(raw []byte) Payload {
	var p Payload
	_ = json.Unmarshal(raw, &p)
	return p
}

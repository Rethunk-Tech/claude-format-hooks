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
	// tool_result is not in the documented PostToolUse envelope, which uses
	// tool_response. It was added as a fix against an observed payload, so
	// it stays until a capture shows it is never sent; the camelCase spelling
	// is deliberate and pinned by a test.
	ToolResult struct {
		FilePath string `json:"filePath"`
	} `json:"tool_result"`
}

// FilePath returns the file that was written or edited, matching the same
// field-fallback order: tool_response.filePath, tool_result.filePath,
// tool_input.file_path, tool_input.notebook_path, then Cursor afterFileEdit's
// top-level file_path.
func (p Payload) FilePath() string {
	switch {
	case p.ToolResponse.FilePath != "":
		return p.ToolResponse.FilePath
	case p.ToolResult.FilePath != "":
		return p.ToolResult.FilePath
	case p.ToolInput.FilePath != "":
		return p.ToolInput.FilePath
	case p.ToolInput.NotebookPath != "":
		return p.ToolInput.NotebookPath
	default:
		return p.CursorFilePath
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

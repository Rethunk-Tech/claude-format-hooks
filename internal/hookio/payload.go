// Package hookio parses the PostToolUse JSON envelope Claude Code sends on
// stdin. The payload is tiny (a handful of string fields, well under 1KB) —
// standard library encoding/json is already far faster than the ~1-3ms
// process startup cost it's measured against, so there is no case for a
// third-party JSON library here.
package hookio

import "encoding/json"

// Payload is the subset of the PostToolUse JSON envelope this package reads.
type Payload struct {
	ToolInput struct {
		FilePath     string `json:"file_path"`
		NotebookPath string `json:"notebook_path"`
	} `json:"tool_input"`
	ToolResponse struct {
		FilePath string `json:"filePath"`
	} `json:"tool_response"`
}

// FilePath returns the file that was written or edited, matching the same
// field-fallback order the original hand-written hooks used: the tool
// response's resolved path first, then tool_input.file_path, then
// tool_input.notebook_path for NotebookEdit calls.
func (p Payload) FilePath() string {
	switch {
	case p.ToolResponse.FilePath != "":
		return p.ToolResponse.FilePath
	case p.ToolInput.FilePath != "":
		return p.ToolInput.FilePath
	default:
		return p.ToolInput.NotebookPath
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

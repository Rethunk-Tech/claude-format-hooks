package hookio

import (
	"testing"

	"github.com/go-quicktest/qt"
)

func TestFilePathFallbackOrder(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "tool_response.filePath wins when present",
			raw:  `{"tool_response":{"filePath":"/resolved.go"},"tool_input":{"file_path":"/input.go","notebook_path":"/nb.ipynb"}}`,
			want: "/resolved.go",
		},
		{
			name: "tool_result.filePath resolves when response is absent",
			raw:  `{"tool_result":{"filePath":"/result.go"}}`,
			want: "/result.go",
		},
		{
			name: "tool_response.filePath wins over tool_result.filePath",
			raw:  `{"tool_response":{"filePath":"/resolved.go"},"tool_result":{"filePath":"/result.go"}}`,
			want: "/resolved.go",
		},
		{
			name: "tool_result.filePath wins over tool_input.file_path",
			raw:  `{"tool_result":{"filePath":"/result.go"},"tool_input":{"file_path":"/input.go","notebook_path":"/nb.ipynb"}}`,
			want: "/result.go",
		},
		{
			name: "ignores snake_case tool_result.file_path",
			raw:  `{"tool_result":{"file_path":"/snake.go"}}`,
			want: "",
		},
		{
			name: "falls back to top-level file_path from Cursor",
			raw:  `{"file_path":"/cursor.go","edits":[{"old_string":"x","new_string":"y"}]}`,
			want: "/cursor.go",
		},
		{
			name: "Claude fields win over top-level file_path",
			raw:  `{"file_path":"/cursor.go","tool_input":{"file_path":"/input.go"}}`,
			want: "/input.go",
		},
		{
			name: "ignores top-level camelCase filePath",
			raw:  `{"filePath":"/camel.go"}`,
			want: "",
		},
		{
			name: "falls back to tool_input.file_path",
			raw:  `{"tool_input":{"file_path":"/input.go","notebook_path":"/nb.ipynb"}}`,
			want: "/input.go",
		},
		{
			name: "falls back to tool_input.notebook_path for NotebookEdit",
			raw:  `{"tool_input":{"notebook_path":"/nb.ipynb"}}`,
			want: "/nb.ipynb",
		},
		{
			name: "empty payload yields empty path",
			raw:  `{}`,
			want: "",
		},
		{
			name: "malformed JSON yields empty path, no panic",
			raw:  `not json`,
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Parse([]byte(tc.raw)).FilePath()
			qt.Check(t, qt.Equals(got, tc.want))
		})
	}
}

// Only the Claude PostToolUse contract defines a JSON channel on stdout
// back to the model. Misreading a Cursor payload as Claude's would emit a
// stray JSON line into a stdout that never asked for one, so the
// discriminator is pinned here rather than left to the caller.
func TestIsClaudeMatchesTheEnvelopeTheFieldCameFrom(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want bool
	}{
		{"tool_response", `{"tool_response":{"filePath":"/a.go"}}`, true},
		{"tool_result", `{"tool_result":{"filePath":"/a.go"}}`, true},
		{"tool_input.file_path", `{"tool_input":{"file_path":"/a.go"}}`, true},
		{"tool_input.notebook_path", `{"tool_input":{"notebook_path":"/a.ipynb"}}`, true},
		{"cursor top-level file_path", `{"file_path":"/a.go"}`, false},
		{"empty payload", `{}`, false},
		{"malformed payload", `not json`, false},
		// A Claude field present alongside Cursor's wins, matching
		// FilePath's own precedence.
		{"both shapes", `{"file_path":"/cursor.go","tool_input":{"file_path":"/claude.go"}}`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := Parse([]byte(tc.raw))
			qt.Check(t, qt.Equals(p.IsClaude(), tc.want))
		})
	}
}

// IsClaude and FilePath share one resolver, so a payload whose path comes
// from Cursor's field must never report Claude, and vice versa.
func TestIsClaudeAgreesWithFilePath(t *testing.T) {
	claude := Parse([]byte(`{"tool_input":{"file_path":"/claude.go"}}`))
	qt.Check(t, qt.Equals(claude.FilePath(), "/claude.go"))
	qt.Check(t, qt.IsTrue(claude.IsClaude()))

	cursor := Parse([]byte(`{"file_path":"/cursor.go"}`))
	qt.Check(t, qt.Equals(cursor.FilePath(), "/cursor.go"))
	qt.Check(t, qt.IsFalse(cursor.IsClaude()))
}

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

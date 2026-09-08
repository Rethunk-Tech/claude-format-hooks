package formatters

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/go-quicktest/qt"

	"github.com/Rethunk-Tech/claude-format-hooks/internal/config"
)

func TestGraphQLRouterName(t *testing.T) {
	qt.Check(t, qt.Equals(NewGraphQLRouter(config.Default()).Name(), "prettier"))
}

func TestGraphQLRouterPicksAFormatter(t *testing.T) {
	biome := graphqlTestTool{name: "biome", marker: "biome"}
	prettier := graphqlTestTool{name: "prettier", marker: "prettier"}

	tests := []struct {
		name        string
		tools       []graphqlTestTool
		biomeConfig bool
		ext         string
		disabled    []string
		want        string
	}{
		{
			name: "no biome config leaves prettier in charge",
			ext:  ".graphql",
			want: "prettier",
		},
		{
			name:        "a biome config selects biome",
			tools:       []graphqlTestTool{biome},
			biomeConfig: true,
			ext:         ".gql",
			want:        "biome",
		},
		{
			name:        "no biome launcher falls back to prettier",
			tools:       []graphqlTestTool{prettier},
			biomeConfig: true,
			ext:         ".graphql",
			want:        "prettier",
		},
		{
			name:        "bunx stands in for a missing biome binary",
			tools:       []graphqlTestTool{{name: "bunx", marker: "biome"}},
			biomeConfig: true,
			ext:         ".graphql",
			want:        "biome",
		},
		{
			name:        "disabling biome leaves prettier in charge",
			biomeConfig: true,
			ext:         ".graphql",
			disabled:    []string{"BIOME"},
			want:        "prettier",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			isolateDiskCache(t)
			dir := t.TempDir()
			marker := filepath.Join(dir, "marker")
			t.Setenv("GRAPHQL_MARKER", marker)
			writeGraphQLTools(t, tc.tools...)
			if tc.biomeConfig {
				qt.Assert(t, qt.IsNil(os.WriteFile(filepath.Join(dir, "biome.json"), []byte("{}\n"), 0o600)))
			}

			abs := filepath.Join(dir, "schema"+tc.ext)
			qt.Assert(t, qt.IsNil(os.WriteFile(abs, []byte("type Query { hello: String }"), 0o600)))

			cfg := config.Default()
			cfg.DisabledFormatters = tc.disabled
			res := NewGraphQLRouter(cfg).Format(t.Context(), dir, abs)
			qt.Assert(t, qt.IsNil(res.Err))
			qt.Check(t, qt.IsFalse(res.Skipped))
			qt.Check(t, qt.Equals(string(readSource(t, marker)), tc.want))
		})
	}
}

type graphqlTestTool struct {
	name   string
	marker string
}

func writeGraphQLTools(t *testing.T, tools ...graphqlTestTool) {
	t.Helper()
	if len(tools) == 0 {
		tools = []graphqlTestTool{
			{name: "biome", marker: "biome"},
			{name: "prettier", marker: "prettier"},
		}
	}

	dir := t.TempDir()
	for _, tool := range tools {
		name := tool.name
		script := "#!/bin/sh\nprintf '" + tool.marker + "' > \"$GRAPHQL_MARKER\"\n"
		if runtime.GOOS == "windows" {
			name += ".cmd"
			script = "@echo off\r\n<nul set /p \"=" + tool.marker + "\" > \"%GRAPHQL_MARKER%\"\r\n"
		}
		path := filepath.Join(dir, name)
		qt.Assert(t, qt.IsNil(os.WriteFile(path, []byte(script), 0o755)))
	}
	t.Setenv("PATH", dir)
}

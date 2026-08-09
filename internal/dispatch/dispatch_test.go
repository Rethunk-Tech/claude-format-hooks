package dispatch

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-quicktest/qt"

	"github.com/Rethunk-Tech/claude-format-hooks/internal/config"
)

func TestSupported(t *testing.T) {
	r := NewRegistry(config.Default())

	supported := []string{
		".json", ".sh", ".bash", ".go",
		".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".mts", ".cts", ".css", ".jsonc",
		".md", ".mdx", ".markdown", ".toml", ".yaml", ".yml", ".html", ".scss", ".less",
		".graphql", ".gql", ".sql", ".py", ".pyi", ".rs", ".tf", ".tfvars",
	}
	for _, ext := range supported {
		qt.Check(t, qt.IsTrue(r.Supported(ext)), qt.Commentf("ext=%q", ext))
	}

	qt.Check(t, qt.IsTrue(r.Supported(".JSON")), qt.Commentf("case-insensitive"))
	qt.Check(t, qt.IsFalse(r.Supported(".rb")), qt.Commentf("no formatter registered"))
	qt.Check(t, qt.IsFalse(r.Supported("")))
}

func TestSupportedRespectsDisabled(t *testing.T) {
	cfg := config.Default()
	cfg.Disabled = []string{".sql"}
	r := NewRegistry(cfg)

	qt.Check(t, qt.IsFalse(r.Supported(".sql")))
	qt.Check(t, qt.IsFalse(r.Supported(".SQL")), qt.Commentf("disabled match is case-insensitive"))
	qt.Check(t, qt.IsTrue(r.Supported(".json")), qt.Commentf("unrelated ext stays enabled"))
}

func TestKnownExtension(t *testing.T) {
	qt.Check(t, qt.IsTrue(KnownExtension(".json")), qt.Commentf("registered by every Config, including empty"))
	qt.Check(t, qt.IsTrue(KnownExtension(".JSON")), qt.Commentf("case-insensitive"))
	qt.Check(t, qt.IsFalse(KnownExtension(".rb")), qt.Commentf("no formatter registered"))
	qt.Check(t, qt.IsFalse(KnownExtension("")))
}

func TestKnownExtensionIgnoresDisabled(t *testing.T) {
	// Disabled only removes entries from a Registry's byExt map at
	// construction time — it can never widen or narrow the static
	// superset KnownExtension answers from.
	qt.Check(t, qt.IsTrue(KnownExtension(".sql")))
}

func TestName(t *testing.T) {
	r := NewRegistry(config.Default())

	qt.Check(t, qt.Equals(r.Name(".json"), "json"))
	qt.Check(t, qt.Equals(r.Name(".sh"), "shfmt"))
	qt.Check(t, qt.Equals(r.Name(".unknown"), ""))
}

func TestDispatchRoutesToFormatter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "t.json")
	qt.Assert(t, qt.IsNil(os.WriteFile(path, []byte(`{"b":1,"a":2}`), 0o644)))

	r := NewRegistry(config.Default())
	result := r.Dispatch(t.Context(), dir, path)
	qt.Assert(t, qt.IsNil(result.Err))

	out, err := os.ReadFile(path)
	qt.Assert(t, qt.IsNil(err))
	qt.Assert(t, qt.Equals(string(out), "{\n  \"b\": 1,\n  \"a\": 2\n}\n"))
}

func TestInVendoredDir(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"src/app.ts", false},
		{"node_modules/pkg/index.js", true},
		{"src/node_modules/pkg/index.js", true},
		{".next/cache/foo.json", true},
		{".yarn/releases/yarn.js", true},
		{".git/hooks/pre-commit", true},
		{".agents/skills/foo.md", true},
		{"dist/bundle.js", true},
		{"build/out.css", true},
		{"coverage/lcov.info", true},
		{"test-results/report.json", true},
		{"vendor/github.com/foo/bar.go", true},
		{".venv/lib/python.py", true},
		// A directory name that merely contains a vendored segment as a
		// substring, rather than matching a full path segment, must not
		// be treated as vendored.
		{"node_modules_extra/foo.js", false},
		{"src/vendored/foo.ts", false},
	}
	for _, tc := range cases {
		qt.Check(t, qt.Equals(InVendoredDir(tc.path), tc.want), qt.Commentf("path=%q", tc.path))
	}
}

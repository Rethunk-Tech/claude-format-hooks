package dispatch

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-quicktest/qt"

	"github.com/Rethunk-Tech/claude-format-hooks/internal/config"
)

var supportedExtensions = []string{
	".json", ".sh", ".bash", ".go",
	".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".mts", ".cts", ".css", ".jsonc",
	".md", ".mdx", ".markdown", ".toml", ".yaml", ".yml", ".html", ".scss", ".less",
	".graphql", ".gql", ".sql", ".py", ".pyi", ".ipynb", ".rs", ".tf", ".tfvars",
	".tftest.hcl", ".tfmock.hcl", ".tfquery.hcl", ".proto",
}

func TestSupported(t *testing.T) {
	r := NewRegistry(config.Default())

	for _, ext := range supportedExtensions {
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

func TestSupportedRespectsDisabledFormatters(t *testing.T) {
	cfg := config.Default()
	cfg.DisabledFormatters = []string{"BIOME"}
	r := NewRegistry(cfg)

	for _, ext := range []string{".ts", ".tsx", ".js", ".jsx", ".jsonc"} {
		qt.Check(t, qt.IsFalse(r.Supported(ext)), qt.Commentf("biome ext=%q", ext))
	}
	for _, ext := range []string{".graphql", ".gql"} {
		qt.Check(t, qt.IsTrue(r.Supported(ext)), qt.Commentf("graphql fallback ext=%q", ext))
	}
	qt.Check(t, qt.IsTrue(r.Supported(".json")), qt.Commentf("json router falls back to native"))
	qt.Check(t, qt.IsTrue(KnownExtension(".ts")), qt.Commentf("known extensions stay unfiltered"))

	cfg.DisabledFormatters = []string{"JSON"}
	r = NewRegistry(cfg)
	qt.Check(t, qt.IsFalse(r.Supported(".json")))
	qt.Check(t, qt.IsTrue(r.Supported(".ts")), qt.Commentf("unrelated formatter stays enabled"))

	cfg.DisabledFormatters = []string{"PRETTIER"}
	r = NewRegistry(cfg)
	for _, ext := range []string{".graphql", ".gql"} {
		qt.Check(t, qt.IsFalse(r.Supported(ext)), qt.Commentf("prettier ext=%q", ext))
	}
}

func TestKnownExtension(t *testing.T) {
	qt.Check(t, qt.IsTrue(KnownExtension(".json")), qt.Commentf("registered by every Config, including empty"))
	qt.Check(t, qt.IsTrue(KnownExtension(".ipynb")), qt.Commentf("notebook formatter is registered by every Config"))
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

func TestTerraformMultiDotExtensionsRespectExactDisables(t *testing.T) {
	cfg := config.Default()
	cfg.Disabled = []string{".hcl"}
	r := NewRegistry(cfg)

	for _, ext := range []string{".tftest.hcl", ".tfmock.hcl", ".tfquery.hcl"} {
		qt.Check(t, qt.IsTrue(r.Supported(ext)), qt.Commentf("ext=%q", ext))
	}
	qt.Check(t, qt.IsFalse(r.Supported(".hcl")))

	cfg.Disabled = []string{".tftest.hcl"}
	r = NewRegistry(cfg)
	qt.Check(t, qt.IsFalse(r.Supported(".tftest.hcl")))
	qt.Check(t, qt.IsTrue(r.Supported(".tfmock.hcl")))
}

func TestName(t *testing.T) {
	r := NewRegistry(config.Default())

	want := map[string]string{
		".json":        "json",
		".sh":          "shfmt",
		".bash":        "shfmt",
		".go":          "gofmt",
		".ts":          "biome",
		".tsx":         "biome",
		".js":          "biome",
		".jsx":         "biome",
		".mjs":         "biome",
		".cjs":         "biome",
		".mts":         "biome",
		".cts":         "biome",
		".css":         "biome",
		".jsonc":       "biome",
		".md":          "markdownlint-cli2",
		".mdx":         "markdownlint-cli2",
		".markdown":    "markdownlint-cli2",
		".toml":        "taplo",
		".yaml":        "prettier",
		".yml":         "prettier",
		".html":        "prettier",
		".scss":        "prettier",
		".less":        "prettier",
		".graphql":     "prettier",
		".gql":         "prettier",
		".sql":         "sqlfluff",
		".py":          "ruff/black",
		".pyi":         "ruff/black",
		".ipynb":       "ruff/black-notebook",
		".rs":          "rustfmt",
		".tf":          "terraform",
		".tfvars":      "terraform",
		".tftest.hcl":  "terraform",
		".tfmock.hcl":  "terraform",
		".tfquery.hcl": "terraform",
		".proto":       "buf",
	}
	for _, ext := range supportedExtensions {
		qt.Check(t, qt.Equals(r.Name(ext), want[ext]), qt.Commentf("ext=%q", ext))
	}

	for _, tc := range []struct {
		ext  string
		want string
	}{
		{".unknown", ""},
		{".JSON", "json"},
	} {
		qt.Check(t, qt.Equals(r.Name(tc.ext), tc.want), qt.Commentf("ext=%q", tc.ext))
	}
}

func TestDispatchRoutesToFormatter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "t.json")
	qt.Assert(t, qt.IsNil(os.WriteFile(path, []byte(`{"b":1,"a":2}`), 0o644)))

	r := NewRegistry(config.Default())
	result := r.Dispatch(t.Context(), dir, path, ".json")
	qt.Assert(t, qt.IsNil(result.Err))

	out, err := os.ReadFile(path)
	qt.Assert(t, qt.IsNil(err))
	qt.Assert(t, qt.Equals(string(out), "{\n  \"b\": 1,\n  \"a\": 2\n}\n"))
}

func TestDispatchUsesCallerResolvedExtensionForExtensionlessPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "script")
	qt.Assert(t, qt.IsNil(os.WriteFile(path, []byte("#!/bin/sh\necho hi\n"), 0o755)))

	r := NewRegistry(config.Default())
	result := r.Dispatch(t.Context(), dir, path, ".sh")

	qt.Assert(t, qt.IsNil(result.Err))
	qt.Check(t, qt.Equals(result.Diagnostic, ""))
}

func TestDispatchSkipsMissingFormatter(t *testing.T) {
	r := NewRegistry(config.Default())

	result := r.Dispatch(t.Context(), "", "", ".unknown")

	qt.Check(t, qt.IsTrue(result.Skipped))
	qt.Check(t, qt.IsNil(result.Err))
}

func TestInSkippedDir(t *testing.T) {
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
		{"target/debug/build/foo/out/generated.rs", true},
		{".orchestrate/contracts-wave1.md", true},
		{".playwright-mcp/trace.zip", true},
		{"coverage/lcov.info", true},
		{"test-results/report.json", true},
		{"vendor/github.com/foo/bar.go", true},
		{".venv/lib/python.py", true},
		{".terraform/providers/registry.terraform.io/hashicorp/aws/main.tf", true},
		{"src/__pycache__/module.pyc", true},
		{".ruff_cache/0.8.0/README.md", true},
		{"src/.mypy_cache/3.12/module.meta.json", true},
		{".pytest_cache/v/cache/nodeids", true},
		{"tests/.tox/py312/bin/python", true},
		// A directory name that merely contains a skipped segment as a
		// substring, rather than matching a full path segment, must not
		// be treated as skipped.
		{"node_modules_extra/foo.js", false},
		{"src/vendored/foo.ts", false},
		{"target_extra/generated.go", false},
	}
	for _, tc := range cases {
		qt.Check(t, qt.Equals(InSkippedDir(tc.path), tc.want), qt.Commentf("path=%q", tc.path))
	}
}

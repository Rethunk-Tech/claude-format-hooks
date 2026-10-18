package dispatch

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/go-quicktest/qt"

	"github.com/Rethunk-Tech/claude-format-hooks/internal/config"
)

var supportedExtensions = []string{
	".json", ".json.example", ".sh", ".bash", ".zsh", ".ksh", ".dash",
	".sh.example", ".bash.example", ".zsh.example", ".ksh.example", ".dash.example",
	".go", ".go.example", ".mod", ".mod.example", ".work", ".work.example",
	".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".mts", ".cts", ".css", ".jsonc", ".jsonc.example",
	".ts.example", ".tsx.example", ".js.example", ".jsx.example", ".mjs.example",
	".cjs.example", ".mts.example", ".cts.example", ".css.example",
	".md", ".mdx", ".markdown", ".md.example", ".mdx.example", ".markdown.example", ".mdc.example",
	".toml", ".toml.example", ".yaml", ".yml", ".yaml.example", ".yml.example",
	".html", ".html.example", ".scss", ".scss.example", ".less", ".less.example",
	".graphql", ".gql", ".graphql.example", ".gql.example",
	".sql", ".sql.example", ".py", ".py.example", ".pyi", ".pyi.example",
	".ipynb", ".ipynb.example", ".rs", ".rs.example", ".tf", ".tf.example", ".tfvars", ".tfvars.example",
	".tftest.hcl", ".tfmock.hcl", ".tfquery.hcl",
	".tftest.hcl.example", ".tfmock.hcl.example", ".tfquery.hcl.example",
	".proto", ".proto.example",
}

func TestSupported(t *testing.T) {
	r := NewRegistry(config.Default())

	for _, ext := range supportedExtensions {
		qt.Check(t, qt.IsTrue(r.Supported(ext)), qt.Commentf("ext=%q", ext))
	}

	qt.Check(t, qt.IsTrue(r.Supported(".JSON")), qt.Commentf("case-insensitive"))
	qt.Check(t, qt.IsFalse(r.Supported(".xyz")), qt.Commentf("no formatter registered"))
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

	for _, ext := range []string{".ts", ".tsx", ".js", ".jsx", ".jsonc", ".jsonc.example", ".ts.example"} {
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
	qt.Check(t, qt.IsFalse(KnownExtension(".xyz")), qt.Commentf("no formatter registered"))
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
		".json":                "json",
		".json.example":        "json",
		".sh":                  "shfmt",
		".bash":                "shfmt",
		".zsh":                 "shfmt",
		".ksh":                 "shfmt",
		".dash":                "shfmt",
		".sh.example":          "shfmt",
		".bash.example":        "shfmt",
		".zsh.example":         "shfmt",
		".ksh.example":         "shfmt",
		".dash.example":        "shfmt",
		".go":                  "gofmt",
		".go.example":          "gofmt",
		".mod":                 "go mod edit",
		".mod.example":         "go mod edit",
		".work":                "go work edit",
		".work.example":        "go work edit",
		".ts":                  "biome",
		".tsx":                 "biome",
		".js":                  "biome",
		".jsx":                 "biome",
		".mjs":                 "biome",
		".cjs":                 "biome",
		".mts":                 "biome",
		".cts":                 "biome",
		".css":                 "biome",
		".jsonc":               "biome",
		".jsonc.example":       "biome",
		".ts.example":          "biome",
		".tsx.example":         "biome",
		".js.example":          "biome",
		".jsx.example":         "biome",
		".mjs.example":         "biome",
		".cjs.example":         "biome",
		".mts.example":         "biome",
		".cts.example":         "biome",
		".css.example":         "biome",
		".md":                  "markdownlint-cli2",
		".mdx":                 "markdownlint-cli2",
		".markdown":            "markdownlint-cli2",
		".md.example":          "markdownlint-cli2",
		".mdx.example":         "markdownlint-cli2",
		".markdown.example":    "markdownlint-cli2",
		".mdc.example":         "markdownlint-cli2",
		".toml":                "taplo",
		".toml.example":        "taplo",
		".yaml":                "prettier",
		".yml":                 "prettier",
		".yaml.example":        "prettier",
		".yml.example":         "prettier",
		".html":                "prettier",
		".html.example":        "prettier",
		".scss":                "prettier",
		".scss.example":        "prettier",
		".less":                "prettier",
		".less.example":        "prettier",
		".graphql":             "prettier",
		".gql":                 "prettier",
		".graphql.example":     "prettier",
		".gql.example":         "prettier",
		".sql":                 "sqlfluff",
		".sql.example":         "sqlfluff",
		".py":                  "ruff/black",
		".py.example":          "ruff/black",
		".pyi":                 "ruff/black",
		".pyi.example":         "ruff/black",
		".ipynb":               "ruff/black-notebook",
		".ipynb.example":       "ruff/black-notebook",
		".rs":                  "rustfmt",
		".rs.example":          "rustfmt",
		".tf":                  "terraform",
		".tf.example":          "terraform",
		".tfvars":              "terraform",
		".tfvars.example":      "terraform",
		".tftest.hcl":          "terraform",
		".tfmock.hcl":          "terraform",
		".tfquery.hcl":         "terraform",
		".tftest.hcl.example":  "terraform",
		".tfmock.hcl.example":  "terraform",
		".tfquery.hcl.example": "terraform",
		".proto":               "buf",
		".proto.example":       "buf",
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
	qt.Assert(t, qt.IsNil(os.WriteFile(path, []byte(`{"b":1,"a":2}`), 0o600)))

	r := NewRegistry(config.Default())
	result := r.Dispatch(t.Context(), dir, path, ".json")
	qt.Assert(t, qt.IsNil(result.Err))

	out, err := os.ReadFile(path) //nolint:gosec // test path is created under t.TempDir
	qt.Assert(t, qt.IsNil(err))
	qt.Assert(t, qt.Equals(string(out), "{\n  \"b\": 1,\n  \"a\": 2\n}\n"))
}

func TestDispatchFormatsExampleSuffixes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json.example")
	qt.Assert(t, qt.IsNil(os.WriteFile(path, []byte(`{"b":1,"a":2}`), 0o600)))

	r := NewRegistry(config.Default())
	ext := ResolveExtension(path)
	qt.Assert(t, qt.Equals(ext, ".json.example"))
	result := r.Dispatch(t.Context(), dir, path, ext)
	qt.Assert(t, qt.IsNil(result.Err))

	out, err := os.ReadFile(path) //nolint:gosec // test path is created under t.TempDir
	qt.Assert(t, qt.IsNil(err))
	qt.Assert(t, qt.Equals(string(out), "{\n  \"b\": 1,\n  \"a\": 2\n}\n"))

	for _, tc := range []struct {
		name string
		want string
	}{
		{"config.yaml.example", "prettier"},
		{"config.yml.example", "prettier"},
		{"config.toml.example", "taplo"},
		{"config.jsonc.example", "biome"},
		{"config.sql.example", "sqlfluff"},
		{"README.md.example", "markdownlint-cli2"},
		{"app.py.example", "ruff/black"},
		{"app.ts.example", "biome"},
		{"page.html.example", "prettier"},
		{"main.go.example", "gofmt"},
		{"app.pyi.example", "ruff/black"},
	} {
		ext := ResolveExtension(tc.name)
		qt.Check(t, qt.Equals(r.Name(ext), tc.want), qt.Commentf("path=%q", tc.name))
		qt.Check(t, qt.IsTrue(r.Supported(ext)), qt.Commentf("path=%q", tc.name))
	}
	notes := filepath.Join(dir, "notes.example")
	qt.Assert(t, qt.IsNil(os.WriteFile(notes, []byte("keep\n"), 0o600)))
	notesExt := ResolveExtension(notes)
	qt.Assert(t, qt.Equals(notesExt, ".example"))
	qt.Check(t, qt.IsFalse(r.Supported(notesExt)))
	notesResult := r.Dispatch(t.Context(), dir, notes, notesExt)
	qt.Check(t, qt.IsTrue(notesResult.Skipped))
	notesOut, err := os.ReadFile(notes) //nolint:gosec // test path is created under t.TempDir
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.Equals(string(notesOut), "keep\n"))

	if runtime.GOOS == "windows" {
		return
	}
	t.Setenv("CLAUDE_FORMAT_HOOKS_CACHE", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	bin := t.TempDir()
	script := "#!/bin/sh\nprintf 'a = 1\\n'\n"
	qt.Assert(t, qt.IsNil(os.WriteFile(filepath.Join(bin, "taplo"), []byte(script), 0o755))) //nolint:gosec // test formatter fixture must be executable
	biomeScript := "#!/bin/sh\nprintf '{\\n\\t\"a\": 1 // c\\n}\\n'\n"
	qt.Assert(t, qt.IsNil(os.WriteFile(filepath.Join(bin, "biome"), []byte(biomeScript), 0o755))) //nolint:gosec // test formatter fixture must be executable
	sqlScript := "#!/bin/sh\nif [ \"$1\" = render ]; then exit 0; fi\nprintf 'SELECT 1\\n'\n"
	qt.Assert(t, qt.IsNil(os.WriteFile(filepath.Join(bin, "sqlfluff"), []byte(sqlScript), 0o755))) //nolint:gosec // test formatter fixture must be executable
	mdScript := "#!/bin/sh\nprev=\nfor arg do\n  if [ \"$prev\" = -- ]; then target=$arg; fi\n  prev=$arg\ndone\nprintf '# Title\\n' > \"$target\"\n"
	qt.Assert(t, qt.IsNil(os.WriteFile(filepath.Join(bin, "markdownlint-cli2"), []byte(mdScript), 0o755))) //nolint:gosec // test formatter fixture must be executable
	ruffScript := "#!/bin/sh\nprintf 'def f():\\n    return 1\\n'\n"
	qt.Assert(t, qt.IsNil(os.WriteFile(filepath.Join(bin, "ruff"), []byte(ruffScript), 0o755))) //nolint:gosec // test formatter fixture must be executable
	t.Setenv("PATH", bin)

	tomlPath := filepath.Join(dir, "config.toml.example")
	qt.Assert(t, qt.IsNil(os.WriteFile(tomlPath, []byte("a=1\n"), 0o600)))
	tomlExt := ResolveExtension(tomlPath)
	qt.Assert(t, qt.Equals(tomlExt, ".toml.example"))
	tomlResult := r.Dispatch(t.Context(), dir, tomlPath, tomlExt)
	qt.Assert(t, qt.IsNil(tomlResult.Err))
	qt.Assert(t, qt.IsFalse(tomlResult.Skipped))
	tomlOut, err := os.ReadFile(tomlPath) //nolint:gosec // test path is created under t.TempDir
	qt.Assert(t, qt.IsNil(err))
	qt.Assert(t, qt.Equals(string(tomlOut), "a = 1\n"))

	jsoncPath := filepath.Join(dir, "config.jsonc.example")
	qt.Assert(t, qt.IsNil(os.WriteFile(jsoncPath, []byte("{ \"a\":1, // c\n}\n"), 0o600)))
	jsoncExt := ResolveExtension(jsoncPath)
	qt.Assert(t, qt.Equals(jsoncExt, ".jsonc.example"))
	qt.Assert(t, qt.Equals(r.Name(jsoncExt), "biome"))
	jsoncResult := r.Dispatch(t.Context(), dir, jsoncPath, jsoncExt)
	qt.Assert(t, qt.IsNil(jsoncResult.Err))
	qt.Assert(t, qt.IsFalse(jsoncResult.Skipped))
	jsoncOut, err := os.ReadFile(jsoncPath) //nolint:gosec // test path is created under t.TempDir
	qt.Assert(t, qt.IsNil(err))
	qt.Assert(t, qt.Equals(string(jsoncOut), "{\n\t\"a\": 1 // c\n}\n"))

	sqlPath := filepath.Join(dir, "query.sql.example")
	qt.Assert(t, qt.IsNil(os.WriteFile(sqlPath, []byte("SELECT  1\n"), 0o600)))
	sqlExt := ResolveExtension(sqlPath)
	qt.Assert(t, qt.Equals(sqlExt, ".sql.example"))
	qt.Assert(t, qt.Equals(r.Name(sqlExt), "sqlfluff"))
	sqlResult := r.Dispatch(t.Context(), dir, sqlPath, sqlExt)
	qt.Assert(t, qt.IsNil(sqlResult.Err))
	qt.Assert(t, qt.IsFalse(sqlResult.Skipped))
	sqlOut, err := os.ReadFile(sqlPath) //nolint:gosec // test path is created under t.TempDir
	qt.Assert(t, qt.IsNil(err))
	qt.Assert(t, qt.Equals(string(sqlOut), "SELECT 1\n"))

	mdPath := filepath.Join(dir, "README.md.example")
	qt.Assert(t, qt.IsNil(os.WriteFile(mdPath, []byte("#Title\n"), 0o600)))
	mdExt := ResolveExtension(mdPath)
	qt.Assert(t, qt.Equals(mdExt, ".md.example"))
	qt.Assert(t, qt.Equals(r.Name(mdExt), "markdownlint-cli2"))
	mdResult := r.Dispatch(t.Context(), dir, mdPath, mdExt)
	qt.Assert(t, qt.IsNil(mdResult.Err))
	qt.Assert(t, qt.IsFalse(mdResult.Skipped))
	mdOut, err := os.ReadFile(mdPath) //nolint:gosec // test path is created under t.TempDir
	qt.Assert(t, qt.IsNil(err))
	qt.Assert(t, qt.Equals(string(mdOut), "# Title\n"))

	pyPath := filepath.Join(dir, "app.py.example")
	qt.Assert(t, qt.IsNil(os.WriteFile(pyPath, []byte("def f( ):\n  return 1\n"), 0o600)))
	pyExt := ResolveExtension(pyPath)
	qt.Assert(t, qt.Equals(pyExt, ".py.example"))
	qt.Assert(t, qt.Equals(r.Name(pyExt), "ruff/black"))
	pyResult := r.Dispatch(t.Context(), dir, pyPath, pyExt)
	qt.Assert(t, qt.IsNil(pyResult.Err))
	qt.Assert(t, qt.IsFalse(pyResult.Skipped))
	pyOut, err := os.ReadFile(pyPath) //nolint:gosec // test path is created under t.TempDir
	qt.Assert(t, qt.IsNil(err))
	qt.Assert(t, qt.Equals(string(pyOut), "def f():\n    return 1\n"))
}

func TestDispatchUsesCallerResolvedExtensionForExtensionlessPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "script")
	qt.Assert(t, qt.IsNil(os.WriteFile(path, []byte("#!/bin/sh\necho hi\n"), 0o755))) //nolint:gosec // test shell fixture must be executable

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
		{"out/index.html", true},
		{".turbo/daemon/log.json", true},
		{".swc/plugins/cache.json", true},
		{".cache/tool/state.json", true},
		{".gradle/caches/modules-2/foo.json", true},
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
		{"outbound/handler.ts", false},
	}
	for _, tc := range cases {
		qt.Check(t, qt.Equals(InSkippedDir(tc.path, nil), tc.want), qt.Commentf("path=%q", tc.path))
	}
}

// The registry is the only place an extension becomes supported, so a
// formatter added without its extensions wired here is dead code. These
// are the extensions added alongside the systemFormatter languages, the
// prettier plugin types, and Cursor's rule files.
func TestRegistryCoversAddedLanguages(t *testing.T) {
	r := NewRegistry(config.Config{})
	for ext, want := range map[string]string{
		".c":              "clang-format",
		".hpp":            "clang-format",
		".mm":             "clang-format",
		".java":           "google-java-format",
		".kt":             "ktlint",
		".swift":          "swift-format",
		".rb":             "rubocop",
		".gemspec":        "rubocop",
		".php":            "php-cs-fixer",
		".php.example":    "php-cs-fixer",
		".nix":            "nixfmt",
		".nix.example":    "nixfmt",
		".lua":            "stylua",
		".lua.example":    "stylua",
		".mod":            "go mod edit",
		".mod.example":    "go mod edit",
		".work":           "go work edit",
		".work.example":   "go work edit",
		".vue":            "prettier",
		".vue.example":    "prettier",
		".svelte":         "prettier",
		".svelte.example": "prettier",
		".astro":          "prettier",
		".astro.example":  "prettier",
		".mdc":            "markdownlint-cli2",
		".java.example":   "google-java-format",
		".kt.example":     "ktlint",
		".swift.example":  "swift-format",
		".rb.example":     "rubocop",
		".c.example":      "clang-format",
	} {
		qt.Check(t, qt.IsTrue(r.Supported(ext)), qt.Commentf("%s", ext))
		qt.Check(t, qt.Equals(r.Name(ext), want), qt.Commentf("%s", ext))
	}
}

// AllKnownExtensions is what --doctor subtracts the enabled set from, so it
// must report the full superset regardless of the config in play.
func TestAllKnownExtensionsIsConfigIndependent(t *testing.T) {
	all := AllKnownExtensions()
	qt.Check(t, qt.IsTrue(slices.Contains(all, ".rs")))
	qt.Check(t, qt.IsTrue(slices.IsSorted(all)))
	qt.Check(t, qt.Not(qt.HasLen(all, 0)))
}

// The built-in skip list has needed three separate fixes for generated
// directories nobody had hit yet. Config-supplied segments are what stops
// the next one needing a release, and they compose with the built-ins
// rather than replacing them.
func TestInSkippedDirHonorsConfiguredSegments(t *testing.T) {
	extra := []string{"generated", "  Bazel-Out  "}

	qt.Check(t, qt.IsTrue(InSkippedDir("src/generated/api.ts", extra)))
	qt.Check(t, qt.IsTrue(InSkippedDir("bazel-out/x.go", extra)), qt.Commentf("trimmed and case-insensitive, like the other config lists"))
	qt.Check(t, qt.IsTrue(InSkippedDir("node_modules/x.js", extra)), qt.Commentf("built-ins still apply"))
	qt.Check(t, qt.IsFalse(InSkippedDir("src/app.ts", extra)))
	// A segment must match whole, not as a substring of a real directory.
	qt.Check(t, qt.IsFalse(InSkippedDir("src/generated-docs/a.md", extra)))
}

// The directory skip list never sees a generated file that sits in a
// source directory under a source extension. Formatting one is not merely
// wasted work: a lockfile is churn the package manager reverts, and a
// minified bundle expanded into readable source is a corrupted artifact.
func TestInSkippedFile(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"app.min.js", true},
		{"app.min.mjs", true},
		{"styles.min.css", true},
		{"app.min.ts", true},
		{"app.min.tsx", true},
		{"app.min.mts", true},
		{"app.min.cts", true},
		{"package-lock.json", true},
		{"Package-Lock.json", true},
		{"pnpm-lock.yaml", true},
		{"npm-shrinkwrap.json", true},
		// Real source that merely resembles the patterns must survive.
		{"app.js", false},
		{"minify.js", false},
		{"lock.json", false},
		{"unlock.ts", false},
		{"main.go", false},
	}
	for _, tc := range cases {
		qt.Check(t, qt.Equals(InSkippedFile(tc.name, nil), tc.want), qt.Commentf("name=%q", tc.name))
	}
}

// Config-supplied globs compose with the built-ins rather than replacing
// them, matched the way the other config lists are.
func TestInSkippedFileHonorsConfiguredGlobs(t *testing.T) {
	extra := []string{"*.generated.ts", "  SCHEMA.JSON  "}

	qt.Check(t, qt.IsTrue(InSkippedFile("api.generated.ts", extra)))
	qt.Check(t, qt.IsTrue(InSkippedFile("schema.json", extra)), qt.Commentf("trimmed and case-insensitive"))
	qt.Check(t, qt.IsTrue(InSkippedFile("app.min.js", extra)), qt.Commentf("built-ins still apply"))
	qt.Check(t, qt.IsFalse(InSkippedFile("api.ts", extra)))
	// A malformed glob must not match everything.
	qt.Check(t, qt.IsFalse(InSkippedFile("api.ts", []string{"[bad"})))
}

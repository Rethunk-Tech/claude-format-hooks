package formatters

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-quicktest/qt"

	"github.com/Rethunk-Tech/claude-format-hooks/internal/config"
)

func TestJSONFormatterName(t *testing.T) {
	qt.Check(t, qt.Equals(NewJSON(config.Default()).Name(), "json"))
}

func TestJSONFormatterIdempotent(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"already formatted with trailing newline", "{\n  \"a\": 1\n}\n"},
		{"no trailing newline", `{"a":1}`},
		{"trailing blank lines", "{\n  \"a\": 1\n}\n\n\n"},
		{"messy source", "{\n\"zebra\":1,\n   \"apple\":2\n}"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir, path := sourceFile(t, "t.json", tc.src)

			out := assertIdempotent(t, NewJSON(config.Default()), dir, path)
			qt.Check(t, qt.IsTrue(len(out) > 0 && out[len(out)-1] == '\n'),
				qt.Commentf("output must end with exactly one newline, got %q", out))
			qt.Check(t, qt.IsFalse(len(out) >= 2 && out[len(out)-2] == '\n'),
				qt.Commentf("output has a trailing blank line, got %q", out))
		})
	}
}

func TestJSONFormatterReadErrorIsNotSkipped(t *testing.T) {
	// A directory always fails os.ReadFile without being IsNotExist,
	// distinguishing this from the "not valid JSON" diagnostic path.
	assertDirectoryIsAReadError(t, NewJSON(config.Default()))
}

func TestJSONFormatterInvalidJSONIsDiagnostic(t *testing.T) {
	dir, path := sourceFile(t, "t.json", `{"a": `)

	res := NewJSON(config.Default()).Format(t.Context(), dir, path)

	qt.Check(t, qt.IsFalse(res.Skipped), qt.Commentf("syntax decline must not look like a missing tool"))
	qt.Check(t, qt.Not(qt.Equals(res.Diagnostic, "")))
	qt.Check(t, qt.Equals(string(readSource(t, path)), `{"a": `))
}

func TestJSONFormatterClampsHugeIndentSize(t *testing.T) {
	src := `{"a":1}`
	cfgCap := config.Default()
	cfgCap.JSON.IndentSize = maxIndentWidth
	cfgHuge := config.Default()
	cfgHuge.JSON.IndentSize = maxIndentWidth * 10

	_, pathCap := sourceFile(t, "t.json", src)
	_, pathHuge := sourceFile(t, "t.json", src)
	qt.Assert(t, qt.IsNil(NewJSON(cfgCap).Format(t.Context(), filepath.Dir(pathCap), pathCap).Err))
	qt.Assert(t, qt.IsNil(NewJSON(cfgHuge).Format(t.Context(), filepath.Dir(pathHuge), pathHuge).Err))
	qt.Check(t, qt.Equals(string(readSource(t, pathHuge)), string(readSource(t, pathCap))))
}

func TestJSONFormatterPreservesKeyOrder(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "t.json")
	src := "{\n\"zebra\": 1,\n\"apple\": 2\n}\n"
	qt.Assert(t, qt.IsNil(os.WriteFile(path, []byte(src), 0o600)))

	NewJSON(config.Default()).Format(t.Context(), dir, path)

	out := readSource(t, path)
	want := "{\n  \"zebra\": 1,\n  \"apple\": 2\n}\n"
	qt.Check(t, qt.Equals(string(out), want), qt.Commentf("key order not preserved"))
}

func TestJSONRouterName(t *testing.T) {
	qt.Check(t, qt.Equals(NewJSONRouter(config.Default()).Name(), "json"))
}

func TestJSONRouterTools(t *testing.T) {
	qt.Check(t, qt.DeepEquals(NewJSONRouter(config.Default()).Tools(), []string{"biome", "bunx"}))
}

// Without a biome config the native formatter owns .json, so the file is
// reformatted in-process with no external tool involved.
func TestJSONRouterUsesNativeWithoutBiomeConfig(t *testing.T) {
	isolateDiskCache(t)
	dir, path := sourceFile(t, "t.json", `{"a":1}`)

	res := NewJSONRouter(config.Default()).Format(t.Context(), dir, path)
	qt.Assert(t, qt.IsFalse(res.Skipped))

	got := readSource(t, path)
	qt.Check(t, qt.Equals(string(got), "{\n  \"a\": 1\n}\n"))
}

// A biome config routes .json to biome, but biome needs bunx. Without it the
// native formatter still runs: leaving the file untouched would be worse than
// formatting it without biome's `expand` setting.
func TestJSONRouterFallsBackToNativeWithoutBunx(t *testing.T) {
	isolateDiskCache(t)
	clearPath(t)
	dir, path := biomeProject(t, "t.json", `{"a":1}`)

	res := NewJSONRouter(config.Default()).Format(t.Context(), dir, path)
	qt.Assert(t, qt.IsFalse(res.Skipped))

	got := readSource(t, path)
	qt.Check(t, qt.Equals(string(got), "{\n  \"a\": 1\n}\n"))
}

func TestJSONRouterUsesPathBiomeWithoutBunx(t *testing.T) {
	isolateDiskCache(t)
	dir := t.TempDir()
	writeFakeTool(t, "biome", "exit 0")
	qt.Assert(t, qt.IsNil(os.WriteFile(filepath.Join(dir, "biome.json"), []byte("{}\n"), 0o600)))
	path := filepath.Join(dir, "t.json")
	src := `{"a":1}`
	qt.Assert(t, qt.IsNil(os.WriteFile(path, []byte(src), 0o600)))

	res := NewJSONRouter(config.Default()).Format(t.Context(), dir, path)
	qt.Assert(t, qt.IsFalse(res.Skipped))

	got := readSource(t, path)
	qt.Check(t, qt.Equals(string(got), src), qt.Commentf("PATH biome should handle the file without bunx"))
}

func TestJSONRouterFormatsExampleCopyWithoutBiome(t *testing.T) {
	isolateDiskCache(t)
	dir := t.TempDir()
	writeFakeTool(t, "biome", "exit 0")
	qt.Assert(t, qt.IsNil(os.WriteFile(filepath.Join(dir, "biome.json"), []byte("{}\n"), 0o600)))
	path := filepath.Join(dir, "config.json.example")
	qt.Assert(t, qt.IsNil(os.WriteFile(path, []byte(`{"a":1}`), 0o600)))

	res := NewJSONRouter(config.Default()).Format(t.Context(), dir, path)
	qt.Assert(t, qt.IsFalse(res.Skipped))
	qt.Check(t, qt.Equals(string(readSource(t, path)), "{\n  \"a\": 1\n}\n"))
}

func TestJSONRouterSkipsDisabledBiome(t *testing.T) {
	isolateDiskCache(t)
	dir, path := biomeProject(t, "t.json", `{"a":1}`)
	writeFakeTool(t, "biome", "exit 0")

	cfg := config.Default()
	cfg.DisabledFormatters = []string{"BIOME"}
	res := NewJSONRouter(cfg).Format(t.Context(), dir, path)
	qt.Assert(t, qt.IsFalse(res.Skipped))

	got := readSource(t, path)
	qt.Check(t, qt.Equals(string(got), "{\n  \"a\": 1\n}\n"),
		qt.Commentf("disabled biome should fall back to native JSON formatting"))
}

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

			assertSingleTrailingNewline(t, assertIdempotent(t, NewJSON(config.Default()), dir, path))
		})
	}
}

func TestJSONFormatterReadErrorIsNotSkipped(t *testing.T) {
	// A directory always fails os.ReadFile without being IsNotExist,
	// distinguishing this from the "not valid JSON" Skipped path.
	assertDirectoryIsAReadError(t, NewJSON(config.Default()))
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
	dir := t.TempDir()
	qt.Assert(t, qt.IsNil(os.WriteFile(filepath.Join(dir, "biome.json"), []byte("{}\n"), 0o600)))
	path := filepath.Join(dir, "t.json")
	qt.Assert(t, qt.IsNil(os.WriteFile(path, []byte(`{"a":1}`), 0o600)))

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

func TestJSONRouterSkipsDisabledBiome(t *testing.T) {
	isolateDiskCache(t)
	dir := t.TempDir()
	writeFakeTool(t, "biome", "exit 0")
	qt.Assert(t, qt.IsNil(os.WriteFile(filepath.Join(dir, "biome.json"), []byte("{}\n"), 0o600)))
	path := filepath.Join(dir, "t.json")
	qt.Assert(t, qt.IsNil(os.WriteFile(path, []byte(`{"a":1}`), 0o600)))

	cfg := config.Default()
	cfg.DisabledFormatters = []string{"BIOME"}
	res := NewJSONRouter(cfg).Format(t.Context(), dir, path)
	qt.Assert(t, qt.IsFalse(res.Skipped))

	got := readSource(t, path)
	qt.Check(t, qt.Equals(string(got), "{\n  \"a\": 1\n}\n"),
		qt.Commentf("disabled biome should fall back to native JSON formatting"))
}

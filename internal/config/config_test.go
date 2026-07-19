package config

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/go-quicktest/qt"

	"github.com/Rethunk-Tech/claude-format-hooks/internal/diskcache"
)

// isolateDiskCache points internal/diskcache at a fresh, empty temp dir
// for the duration of the test, so resolveIndent's EditorConfig cache
// neither leaks between tests nor writes into the developer's real OS
// cache directory when running `go test` locally.
func isolateDiskCache(t *testing.T) {
	t.Helper()
	t.Setenv("CLAUDE_FORMAT_HOOKS_CACHE", t.TempDir())
}

func TestLoad(t *testing.T) {
	t.Run("missing file returns defaults, no error", func(t *testing.T) {
		cfg, err := Load(filepath.Join(t.TempDir(), "does-not-exist.json"))
		qt.Assert(t, qt.IsNil(err))
		qt.Check(t, qt.DeepEquals(cfg, Default()))
	})

	t.Run("valid file overrides only the fields it sets", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "cfg.json")
		qt.Assert(t, qt.IsNil(os.WriteFile(path, []byte(`{"json":{"indentSize":4,"useTabs":true},"disabled":[".sql"]}`), 0o600)))

		cfg, err := Load(path)
		qt.Assert(t, qt.IsNil(err))
		qt.Check(t, qt.Equals(cfg.JSON.IndentSize, 4))
		qt.Check(t, qt.IsTrue(cfg.JSON.UseTabs))
		qt.Check(t, qt.DeepEquals(cfg.Shell, Default().Shell), qt.Commentf("unset section keeps built-in default"))
		qt.Check(t, qt.DeepEquals(cfg.Disabled, []string{".sql"}))
	})

	t.Run("malformed file returns defaults and an error", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "cfg.json")
		qt.Assert(t, qt.IsNil(os.WriteFile(path, []byte(`{not valid json`), 0o600)))

		cfg, err := Load(path)
		qt.Check(t, qt.IsNotNil(err))
		qt.Check(t, qt.DeepEquals(cfg, Default()))
	})
}

func TestIsDisabled(t *testing.T) {
	cfg := Config{Disabled: []string{".sql", ".TOML"}}

	qt.Check(t, qt.IsTrue(cfg.IsDisabled(".sql")))
	qt.Check(t, qt.IsTrue(cfg.IsDisabled(".SQL")), qt.Commentf("case-insensitive"))
	qt.Check(t, qt.IsTrue(cfg.IsDisabled(".toml")), qt.Commentf("case-insensitive against a mixed-case entry"))
	qt.Check(t, qt.IsFalse(cfg.IsDisabled(".json")))
}

func TestResolveIndentEditorConfigLayering(t *testing.T) {
	writeEditorConfig := func(t *testing.T, dir, body string) {
		t.Helper()
		qt.Assert(t, qt.IsNil(os.WriteFile(filepath.Join(dir, ".editorconfig"), []byte(body), 0o600)))
	}

	t.Run("no covering section falls back to cfg default", func(t *testing.T) {
		isolateDiskCache(t)
		dir := t.TempDir()
		writeEditorConfig(t, dir, "root = true\n\n[*.txt]\nindent_style = tab\n")

		spec := ResolveJSONIndent(Default(), filepath.Join(dir, "t.json"))
		qt.Check(t, qt.Equals(spec, IndentSpec{Size: 2, UseTabs: false}))
	})

	t.Run("editorconfig indent_style/indent_size win over cfg default", func(t *testing.T) {
		isolateDiskCache(t)
		dir := t.TempDir()
		writeEditorConfig(t, dir, "root = true\n\n[*.sh]\nindent_style = tab\n")

		spec := ResolveShellIndent(Default(), filepath.Join(dir, "t.sh"))
		qt.Check(t, qt.IsTrue(spec.UseTabs))
	})

	t.Run("editorconfig indent_size overrides without switching to tabs", func(t *testing.T) {
		isolateDiskCache(t)
		dir := t.TempDir()
		writeEditorConfig(t, dir, "root = true\n\n[*.json]\nindent_style = space\nindent_size = 4\n")

		spec := ResolveJSONIndent(Default(), filepath.Join(dir, "t.json"))
		qt.Check(t, qt.Equals(spec, IndentSpec{Size: 4, UseTabs: false}))
	})

	t.Run("missing editorconfig falls back to cfg default", func(t *testing.T) {
		isolateDiskCache(t)
		dir := t.TempDir()
		spec := ResolveJSONIndent(Default(), filepath.Join(dir, "t.json"))
		qt.Check(t, qt.Equals(spec, IndentSpec{Size: 2, UseTabs: false}))
	})
}

func TestResolveIndentCachesAFreshResultAgainstALaterEditorConfigChange(t *testing.T) {
	isolateDiskCache(t)
	dir := t.TempDir()
	abs := filepath.Join(dir, "t.json")

	first := ResolveJSONIndent(Default(), abs)
	qt.Check(t, qt.Equals(first, IndentSpec{Size: 2, UseTabs: false}), qt.Commentf("no .editorconfig exists yet"))

	qt.Assert(t, qt.IsNil(os.WriteFile(filepath.Join(dir, ".editorconfig"), []byte("root = true\n\n[*.json]\nindent_style = tab\n"), 0o600)))
	second := ResolveJSONIndent(Default(), abs)
	qt.Check(t, qt.Equals(second, first), qt.Commentf("a fresh cached result must still mask an .editorconfig that appears moments later"))
}

func TestResolveIndentRechecksAfterTTLExpires(t *testing.T) {
	isolateDiskCache(t)
	dir := t.TempDir()
	abs := filepath.Join(dir, "t.json")
	fallback := IndentSpec{Size: Default().JSON.IndentSize, UseTabs: Default().JSON.UseTabs}

	cacheDir, ok := diskcache.Dir()
	qt.Assert(t, qt.IsTrue(ok))
	key := diskcache.Key("editorconfig", abs, strconv.Itoa(fallback.Size), strconv.FormatBool(fallback.UseTabs))
	stalePath := filepath.Join(cacheDir, key)
	stale := time.Now().Add(-2 * editorconfigCacheTTL).Unix()
	qt.Assert(t, qt.IsNil(os.MkdirAll(cacheDir, 0o700)))
	qt.Assert(t, qt.IsNil(os.WriteFile(stalePath, []byte(strconv.FormatInt(stale, 10)+"\n"+encodeIndentSpec(fallback)), 0o600))) //nolint:gosec // test fixture

	qt.Assert(t, qt.IsNil(os.WriteFile(filepath.Join(dir, ".editorconfig"), []byte("root = true\n\n[*.json]\nindent_style = tab\n"), 0o600)))
	spec := ResolveJSONIndent(Default(), abs)
	qt.Check(t, qt.IsTrue(spec.UseTabs), qt.Commentf("a stale cached result must not mask a now-present .editorconfig"))
}

func TestResolveIndentDegradesGracefullyWithNoCacheDirAvailable(t *testing.T) {
	t.Setenv("CLAUDE_FORMAT_HOOKS_CACHE", "")
	t.Setenv("LocalAppData", "")
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("HOME", "")
	_, ok := diskcache.Dir()
	qt.Assert(t, qt.IsFalse(ok), qt.Commentf("test environment must be able to force this for the assertion below to be meaningful"))

	dir := t.TempDir()
	spec := ResolveJSONIndent(Default(), filepath.Join(dir, "t.json"))
	qt.Check(t, qt.Equals(spec, IndentSpec{Size: 2, UseTabs: false}))
}

func TestEncodeDecodeIndentSpecRoundTrip(t *testing.T) {
	cases := []IndentSpec{{Size: 2, UseTabs: false}, {Size: 4, UseTabs: true}, {Size: 8, UseTabs: false}}
	for _, spec := range cases {
		got, ok := decodeIndentSpec(encodeIndentSpec(spec))
		qt.Check(t, qt.IsTrue(ok))
		qt.Check(t, qt.Equals(got, spec))
	}
}

func TestDecodeIndentSpecRejectsMalformedInput(t *testing.T) {
	for _, s := range []string{"", "no-comma", "abc,0"} {
		_, ok := decodeIndentSpec(s)
		qt.Check(t, qt.IsFalse(ok), qt.Commentf("input %q should not decode", s))
	}
}

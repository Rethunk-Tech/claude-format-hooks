package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-quicktest/qt"
)

func TestLoad(t *testing.T) {
	t.Run("missing file returns defaults, no error", func(t *testing.T) {
		cfg, err := Load(filepath.Join(t.TempDir(), "does-not-exist.json"))
		qt.Assert(t, qt.IsNil(err))
		qt.Check(t, qt.DeepEquals(cfg, Default()))
	})

	t.Run("valid file overrides only the fields it sets", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "cfg.json")
		qt.Assert(t, qt.IsNil(os.WriteFile(path, []byte(`{"json":{"indentSize":4,"useTabs":true},"disabled":[".sql"]}`), 0o644)))

		cfg, err := Load(path)
		qt.Assert(t, qt.IsNil(err))
		qt.Check(t, qt.Equals(cfg.JSON.IndentSize, 4))
		qt.Check(t, qt.IsTrue(cfg.JSON.UseTabs))
		qt.Check(t, qt.DeepEquals(cfg.Shell, Default().Shell), qt.Commentf("unset section keeps built-in default"))
		qt.Check(t, qt.DeepEquals(cfg.Disabled, []string{".sql"}))
	})

	t.Run("malformed file returns defaults and an error", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "cfg.json")
		qt.Assert(t, qt.IsNil(os.WriteFile(path, []byte(`{not valid json`), 0o644)))

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
		qt.Assert(t, qt.IsNil(os.WriteFile(filepath.Join(dir, ".editorconfig"), []byte(body), 0o644)))
	}

	t.Run("no covering section falls back to cfg default", func(t *testing.T) {
		dir := t.TempDir()
		writeEditorConfig(t, dir, "root = true\n\n[*.txt]\nindent_style = tab\n")

		spec := ResolveJSONIndent(Default(), filepath.Join(dir, "t.json"))
		qt.Check(t, qt.Equals(spec, IndentSpec{Size: 2, UseTabs: false}))
	})

	t.Run("editorconfig indent_style/indent_size win over cfg default", func(t *testing.T) {
		dir := t.TempDir()
		writeEditorConfig(t, dir, "root = true\n\n[*.sh]\nindent_style = tab\n")

		spec := ResolveShellIndent(Default(), filepath.Join(dir, "t.sh"))
		qt.Check(t, qt.IsTrue(spec.UseTabs))
	})

	t.Run("editorconfig indent_size overrides without switching to tabs", func(t *testing.T) {
		dir := t.TempDir()
		writeEditorConfig(t, dir, "root = true\n\n[*.json]\nindent_style = space\nindent_size = 4\n")

		spec := ResolveJSONIndent(Default(), filepath.Join(dir, "t.json"))
		qt.Check(t, qt.Equals(spec, IndentSpec{Size: 4, UseTabs: false}))
	})

	t.Run("missing editorconfig falls back to cfg default", func(t *testing.T) {
		dir := t.TempDir()
		spec := ResolveJSONIndent(Default(), filepath.Join(dir, "t.json"))
		qt.Check(t, qt.Equals(spec, IndentSpec{Size: 2, UseTabs: false}))
	})
}

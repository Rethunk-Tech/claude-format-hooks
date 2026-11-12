package dispatch

import (
	"path/filepath"
	"testing"

	"github.com/go-quicktest/qt"
)

func TestResolveExtensionFallsBackToPathExtension(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"config.json", ".json"},
		{"config.JSON", ".json"},
		{"script.zsh", ".zsh"},
		{"notes.example", ".example"},
		{"config.jsonc.sample", ".sample"},
		{"README", ""},
		{"", ""},
		{filepath.Join("nested.dir", "notes.UNKNOWN"), ".UNKNOWN"},
	}
	for _, tc := range cases {
		qt.Check(t, qt.Equals(ResolveExtension(tc.path), tc.want), qt.Commentf("path=%q", tc.path))
	}
}

func TestResolveExtensionPrefersLongestRegisteredSuffix(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"fixture.tftest.hcl", ".tftest.hcl"},
		{"fixture.tfmock.hcl", ".tfmock.hcl"},
		{"fixture.TFQUERY.HCL", ".tfquery.hcl"},
		{"fixture.tf.json", ".json"},
		{"config.yaml.example", ".yaml.example"},
		{"config.yml.example", ".yml.example"},
		{"config.json.example", ".json.example"},
		{"config.jsonc.example", ".jsonc.example"},
		{"config.JSONC.EXAMPLE", ".jsonc.example"},
		{"config.toml.example", ".toml.example"},
		{"config.sql.example", ".sql.example"},
		{"config.SQL.EXAMPLE", ".sql.example"},
		{"README.md.example", ".md.example"},
		{"README.MD.EXAMPLE", ".md.example"},
		{"app.py.example", ".py.example"},
		{"app.PY.EXAMPLE", ".py.example"},
		{"config.TOML.EXAMPLE", ".toml.example"},
		{"config.YAML.EXAMPLE", ".yaml.example"},
		{"app.ts.example", ".ts.example"},
		{"app.TS.EXAMPLE", ".ts.example"},
		{"page.html.example", ".html.example"},
		{"main.go.example", ".go.example"},
		{"script.sh.example", ".sh.example"},
		{"app.pyi.example", ".pyi.example"},
		{"go.mod.example", ".mod.example"},
		{"go.work.example", ".work.example"},
		{"nb.ipynb.example", ".ipynb.example"},
		{"lib.rs.example", ".rs.example"},
		{"main.tf.example", ".tf.example"},
		{"fixture.tftest.hcl.example", ".tftest.hcl.example"},
		{"api.proto.example", ".proto.example"},
		{"Main.java.example", ".java.example"},
		{"a.lua.example", ".lua.example"},
		{"page.vue.example", ".vue.example"},
	}
	for _, tc := range cases {
		qt.Check(t, qt.Equals(ResolveExtension(tc.path), tc.want), qt.Commentf("path=%q", tc.path))
	}
}

func FuzzResolveExtension(f *testing.F) {
	f.Add("foo.go")
	f.Add("foo.json")
	f.Add("foo.json.example")
	f.Add("go.mod")
	f.Add("go.work")
	f.Add(".bashrc")
	f.Add("Makefile")
	f.Add("foo.tar.gz")
	f.Add("file.min.js")
	f.Add("package-lock.json")
	f.Add("")
	f.Add("foo.")
	f.Add(".")
	f.Add("..")
	f.Add("a.b.c.d")
	f.Fuzz(func(t *testing.T, path string) {
		got := ResolveExtension(path)
		if got == "" {
			return
		}
		if got[0] != '.' {
			t.Fatalf("ResolveExtension(%q) = %q; want empty or '.' prefix", path, got)
		}
	})
}

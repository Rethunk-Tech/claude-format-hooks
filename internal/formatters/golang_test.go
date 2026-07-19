package formatters

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-quicktest/qt"
)

func TestGoFormatterName(t *testing.T) {
	qt.Check(t, qt.Equals(NewGo().Name(), "gofmt"))
}

func TestGoFormatterIdempotent(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"already gofmt'd", "package main\n\nfunc main() {}\n"},
		{"messy braces and spacing", "package main\n\nfunc main(){\n    println(\"hi\")\n}\n"},
		{"spaces instead of tabs", "package main\n\ntype T struct {\n    A int\n    B string\n}\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "t.go")
			qt.Assert(t, qt.IsNil(os.WriteFile(path, []byte(tc.src), 0o600)))

			f := NewGo()
			ctx := t.Context()

			f.Format(ctx, dir, path)
			first, err := os.ReadFile(path)
			qt.Assert(t, qt.IsNil(err))

			f.Format(ctx, dir, path)
			second, err := os.ReadFile(path)
			qt.Assert(t, qt.IsNil(err))

			qt.Check(t, qt.DeepEquals(first, second), qt.Commentf("not idempotent"))
		})
	}
}

func TestGoFormatterReformatsIndentation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "t.go")
	src := "package main\n\nfunc main(){\nprintln(\"hi\")\n}\n"
	qt.Assert(t, qt.IsNil(os.WriteFile(path, []byte(src), 0o600)))

	res := NewGo().Format(t.Context(), dir, path)
	qt.Check(t, qt.IsNil(res.Err))

	out, err := os.ReadFile(path)
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.StringContains(string(out), "\tprintln(\"hi\")"),
		qt.Commentf("expected tab-indented output, got %q", out))
}

func TestGoFormatterSkipsInvalidSyntax(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "t.go")
	src := "package main\nfunc {\n"
	qt.Assert(t, qt.IsNil(os.WriteFile(path, []byte(src), 0o600)))

	res := NewGo().Format(t.Context(), dir, path)
	qt.Check(t, qt.IsTrue(res.Skipped))

	out, err := os.ReadFile(path)
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.Equals(string(out), src), qt.Commentf("malformed source must not be modified"))
}

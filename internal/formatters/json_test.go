package formatters

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-quicktest/qt"

	"github.com/Rethunk-Tech/claude-format-hooks/internal/config"
)

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
			dir := t.TempDir()
			path := filepath.Join(dir, "t.json")
			qt.Assert(t, qt.IsNil(os.WriteFile(path, []byte(tc.src), 0o600)))

			f := NewJSON(config.Default())
			ctx := context.Background()

			f.Format(ctx, dir, path)
			first, err := os.ReadFile(path)
			qt.Assert(t, qt.IsNil(err))

			f.Format(ctx, dir, path)
			second, err := os.ReadFile(path)
			qt.Assert(t, qt.IsNil(err))

			qt.Check(t, qt.DeepEquals(first, second), qt.Commentf("not idempotent"))
			qt.Check(t, qt.IsTrue(len(first) > 0 && first[len(first)-1] == '\n'),
				qt.Commentf("output must end with exactly one newline, got %q", first))
			qt.Check(t, qt.IsFalse(len(first) >= 2 && first[len(first)-2] == '\n'),
				qt.Commentf("output has a trailing blank line, got %q", first))
		})
	}
}

func TestJSONFormatterPreservesKeyOrder(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "t.json")
	src := "{\n\"zebra\": 1,\n\"apple\": 2\n}\n"
	qt.Assert(t, qt.IsNil(os.WriteFile(path, []byte(src), 0o600)))

	NewJSON(config.Default()).Format(context.Background(), dir, path)

	out, err := os.ReadFile(path)
	qt.Assert(t, qt.IsNil(err))
	want := "{\n  \"zebra\": 1,\n  \"apple\": 2\n}\n"
	qt.Check(t, qt.Equals(string(out), want), qt.Commentf("key order not preserved"))
}

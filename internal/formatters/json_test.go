package formatters

import (
	"context"
	"os"
	"path/filepath"
	"testing"

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
			if err := os.WriteFile(path, []byte(tc.src), 0o600); err != nil {
				t.Fatal(err)
			}

			f := NewJSON(config.Default())
			ctx := context.Background()

			f.Format(ctx, dir, path)
			first, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}

			f.Format(ctx, dir, path)
			second, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}

			if string(first) != string(second) {
				t.Fatalf("not idempotent:\nfirst:  %q\nsecond: %q", first, second)
			}
			if len(first) == 0 || first[len(first)-1] != '\n' {
				t.Fatalf("output must end with exactly one newline, got %q", first)
			}
			if len(first) >= 2 && first[len(first)-2] == '\n' {
				t.Fatalf("output has a trailing blank line, got %q", first)
			}
		})
	}
}

func TestJSONFormatterPreservesKeyOrder(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "t.json")
	src := "{\n\"zebra\": 1,\n\"apple\": 2\n}\n"
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}

	NewJSON(config.Default()).Format(context.Background(), dir, path)

	out, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"zebra\": 1,\n  \"apple\": 2\n}\n"
	if string(out) != want {
		t.Fatalf("key order not preserved:\ngot:  %q\nwant: %q", out, want)
	}
}

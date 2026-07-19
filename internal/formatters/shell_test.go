package formatters

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Rethunk-Tech/claude-format-hooks/internal/config"
)

func TestShellFormatterIdempotent(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"already formatted", "#!/bin/sh\necho hello\n"},
		{"messy indentation", "#!/bin/sh\nif true; then\n      echo hi\nfi\n"},
		{"comments preserved", "#!/bin/sh\n# a comment\necho hi # trailing\n"},
		{"switch case block", "#!/bin/sh\ncase \"$1\" in\na) echo a ;;\nb) echo b ;;\nesac\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "t.sh")
			if err := os.WriteFile(path, []byte(tc.src), 0o600); err != nil {
				t.Fatal(err)
			}

			f := NewShell(config.Default())
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
		})
	}
}

func TestShellFormatterUsesTabsWhenConfigured(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "t.sh")
	src := "#!/bin/sh\nif true; then\necho hi\nfi\n"
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.Shell.UseTabs = true

	NewShell(cfg).Format(context.Background(), dir, path)

	out, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "#!/bin/sh\nif true; then\n\techo hi\nfi\n"
	if string(out) != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}

func TestShellFormatterSkipsInvalidSyntax(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "t.sh")
	src := "#!/bin/sh\nif true; then\necho hi\n" // missing `fi`
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}

	result := NewShell(config.Default()).Format(context.Background(), dir, path)
	if !result.Skipped {
		t.Fatalf("Format on invalid shell syntax: got Skipped=%v, want true", result.Skipped)
	}
	if result.Err != nil {
		t.Fatalf("Format on invalid shell syntax: got Err=%v, want nil", result.Err)
	}
}

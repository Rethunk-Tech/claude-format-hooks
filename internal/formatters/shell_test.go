package formatters

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-quicktest/qt"

	"github.com/Rethunk-Tech/claude-format-hooks/internal/config"
)

func TestShellFormatterName(t *testing.T) {
	qt.Check(t, qt.Equals(NewShell(config.Default()).Name(), "shfmt"))
}

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
			qt.Assert(t, qt.IsNil(os.WriteFile(path, []byte(tc.src), 0o600)))

			f := NewShell(config.Default())
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

func TestShellFormatterReadErrorIsNotSkipped(t *testing.T) {
	dir := t.TempDir()
	res := NewShell(config.Default()).Format(t.Context(), dir, dir)
	qt.Check(t, qt.IsNotNil(res.Err))
	qt.Check(t, qt.IsFalse(res.Skipped))
}

func TestShellFormatterUsesTabsWhenConfigured(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "t.sh")
	src := "#!/bin/sh\nif true; then\necho hi\nfi\n"
	qt.Assert(t, qt.IsNil(os.WriteFile(path, []byte(src), 0o600)))

	cfg := config.Default()
	cfg.Shell.UseTabs = true

	NewShell(cfg).Format(t.Context(), dir, path)

	out, err := os.ReadFile(path)
	qt.Assert(t, qt.IsNil(err))
	want := "#!/bin/sh\nif true; then\n\techo hi\nfi\n"
	qt.Check(t, qt.Equals(string(out), want))
}

func TestShellFormatterClampsNonPositiveIndentSize(t *testing.T) {
	src := "#!/bin/sh\nif true; then\necho hi\nfi\n"
	want := "#!/bin/sh\nif true; then\n echo hi\nfi\n"

	for _, size := range []int{0, -1, -4} {
		t.Run(fmt.Sprintf("indentSize=%d", size), func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "t.sh")
			qt.Assert(t, qt.IsNil(os.WriteFile(path, []byte(src), 0o600)))

			cfg := config.Default()
			cfg.Shell.IndentSize = size

			result := NewShell(cfg).Format(t.Context(), dir, path)
			qt.Assert(t, qt.IsNil(result.Err))

			out, err := os.ReadFile(path)
			qt.Assert(t, qt.IsNil(err))
			qt.Check(t, qt.Equals(string(out), want), qt.Commentf("a non-positive indentSize should clamp to 1 space, not wrap to a huge uint"))
		})
	}
}

func TestShellFormatterSkipsInvalidSyntax(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "t.sh")
	src := "#!/bin/sh\nif true; then\necho hi\n" // missing `fi`
	qt.Assert(t, qt.IsNil(os.WriteFile(path, []byte(src), 0o600)))

	result := NewShell(config.Default()).Format(t.Context(), dir, path)
	qt.Check(t, qt.IsTrue(result.Skipped))
	qt.Check(t, qt.IsNil(result.Err))
}

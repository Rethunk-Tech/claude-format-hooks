package formatters

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// writeFakeTool puts an executable named name on a fresh PATH containing
// only tmpDir, so exec.LookPath finds it without depending on any real
// external formatter being installed. body is the script's shell body.
func writeFakeTool(t *testing.T, name, body string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake shell-script tools are POSIX-shell only")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	script := "#!/bin/sh\n" + body + "\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil { //nolint:gosec // test fixture, not the file under format
		t.Fatalf("write fake %s: %v", name, err)
	}
	t.Setenv("PATH", dir)
}

func clearPath(t *testing.T) {
	t.Helper()
	t.Setenv("PATH", t.TempDir())
}

func TestExternalFormatterNames(t *testing.T) {
	cases := []struct {
		f    Formatter
		want string
	}{
		{NewBiome(), "biome"},
		{NewMarkdown(), "markdownlint-cli2"},
		{NewTOML(), "taplo"},
		{NewPrettier(), "prettier"},
		{NewSQLFluff(), "sqlfluff"},
	}
	for _, tc := range cases {
		if got := tc.f.Name(); got != tc.want {
			t.Errorf("Name() = %q, want %q", got, tc.want)
		}
	}
}

func TestBunxFormattersSkipWhenBunxMissing(t *testing.T) {
	clearPath(t)
	abs := filepath.Join(t.TempDir(), "f.txt")
	for _, f := range []Formatter{NewBiome(), NewMarkdown(), NewTOML(), NewPrettier()} {
		res := f.Format(context.Background(), t.TempDir(), abs)
		if !res.Skipped {
			t.Errorf("%s: Skipped = false, want true when bunx is not on PATH", f.Name())
		}
	}
}

func TestSQLFluffSkipsWhenMissing(t *testing.T) {
	clearPath(t)
	abs := filepath.Join(t.TempDir(), "f.sql")
	res := NewSQLFluff().Format(context.Background(), t.TempDir(), abs)
	if !res.Skipped {
		t.Errorf("Skipped = false, want true when sqlfluff is not on PATH")
	}
}

func TestBiomeFormatSuccess(t *testing.T) {
	writeFakeTool(t, "bunx", "exit 0")
	dir := t.TempDir()
	abs := filepath.Join(dir, "f.ts")
	res := NewBiome().Format(context.Background(), dir, abs)
	if res.Err != nil || res.Diagnostic != "" || res.Skipped {
		t.Errorf("Format() = %+v, want a clean success", res)
	}
}

func TestBiomeFormatFailureTruncatesDiagnostic(t *testing.T) {
	writeFakeTool(t, "bunx", `i=1; while [ $i -le 20 ]; do echo "line $i"; i=$((i+1)); done; exit 1`)
	dir := t.TempDir()
	abs := filepath.Join(dir, "f.ts")
	res := NewBiome().Format(context.Background(), dir, abs)
	if res.Diagnostic == "" {
		t.Fatal("Diagnostic empty, want the truncated failure output")
	}
	if got := len(splitLines(res.Diagnostic)); got > 10 {
		t.Errorf("diagnostic has %d lines, want <=10", got)
	}
}

func TestSQLFluffFormatSuccessAndFailure(t *testing.T) {
	dir := t.TempDir()
	abs := filepath.Join(dir, "f.sql")

	t.Run("success", func(t *testing.T) {
		writeFakeTool(t, "sqlfluff", "exit 0")
		res := NewSQLFluff().Format(context.Background(), dir, abs)
		if res.Err != nil || res.Diagnostic != "" {
			t.Errorf("Format() = %+v, want a clean success", res)
		}
	})

	t.Run("failure with no output falls back to the process error", func(t *testing.T) {
		writeFakeTool(t, "sqlfluff", "exit 1")
		res := NewSQLFluff().Format(context.Background(), dir, abs)
		if res.Diagnostic == "" {
			t.Fatal("Diagnostic empty, want the process error as fallback")
		}
	})
}

func TestFindUpward(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(root, "a", "biome.json")
	if err := os.WriteFile(marker, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	if got := findUpward(sub, root, "biome.json", "biome.jsonc"); got != filepath.Join(root, "a") {
		t.Errorf("findUpward() = %q, want %q", got, filepath.Join(root, "a"))
	}
	if got := findUpward(root, root, "nope.json"); got != "" {
		t.Errorf("findUpward() = %q, want \"\" when nothing matches up to root", got)
	}
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i, c := range s {
		if c == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	lines = append(lines, s[start:])
	return lines
}

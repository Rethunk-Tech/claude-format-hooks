package formatters

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/go-quicktest/qt"
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
	qt.Assert(t, qt.IsNil(os.WriteFile(path, []byte(script), 0o755))) //nolint:gosec // test fixture, not the file under format
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
		qt.Check(t, qt.Equals(tc.f.Name(), tc.want))
	}
}

func TestBunxFormattersSkipWhenBunxMissing(t *testing.T) {
	clearPath(t)
	abs := filepath.Join(t.TempDir(), "f.txt")
	for _, f := range []Formatter{NewBiome(), NewMarkdown(), NewTOML(), NewPrettier()} {
		res := f.Format(context.Background(), t.TempDir(), abs)
		qt.Check(t, qt.IsTrue(res.Skipped), qt.Commentf("%s should skip when bunx is not on PATH", f.Name()))
	}
}

func TestSQLFluffSkipsWhenMissing(t *testing.T) {
	clearPath(t)
	abs := filepath.Join(t.TempDir(), "f.sql")
	res := NewSQLFluff().Format(context.Background(), t.TempDir(), abs)
	qt.Check(t, qt.IsTrue(res.Skipped))
}

func TestBiomeFormatSuccess(t *testing.T) {
	writeFakeTool(t, "bunx", "exit 0")
	dir := t.TempDir()
	abs := filepath.Join(dir, "f.ts")
	res := NewBiome().Format(context.Background(), dir, abs)
	qt.Check(t, qt.IsNil(res.Err))
	qt.Check(t, qt.Equals(res.Diagnostic, ""))
	qt.Check(t, qt.IsFalse(res.Skipped))
}

func TestBiomeFormatFailureTruncatesDiagnostic(t *testing.T) {
	writeFakeTool(t, "bunx", `i=1; while [ $i -le 20 ]; do echo "line $i"; i=$((i+1)); done; exit 1`)
	dir := t.TempDir()
	abs := filepath.Join(dir, "f.ts")
	res := NewBiome().Format(context.Background(), dir, abs)
	qt.Assert(t, qt.Not(qt.Equals(res.Diagnostic, "")))
	qt.Check(t, qt.IsTrue(strings.Count(res.Diagnostic, "\n")+1 <= 10),
		qt.Commentf("diagnostic has more than 10 lines: %q", res.Diagnostic))
}

func TestSQLFluffFormatSuccessAndFailure(t *testing.T) {
	dir := t.TempDir()
	abs := filepath.Join(dir, "f.sql")

	t.Run("success", func(t *testing.T) {
		writeFakeTool(t, "sqlfluff", "exit 0")
		res := NewSQLFluff().Format(context.Background(), dir, abs)
		qt.Check(t, qt.IsNil(res.Err))
		qt.Check(t, qt.Equals(res.Diagnostic, ""))
	})

	t.Run("failure with no output falls back to the process error", func(t *testing.T) {
		writeFakeTool(t, "sqlfluff", "exit 1")
		res := NewSQLFluff().Format(context.Background(), dir, abs)
		qt.Check(t, qt.Not(qt.Equals(res.Diagnostic, "")))
	})
}

func TestFindUpward(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "a", "b")
	qt.Assert(t, qt.IsNil(os.MkdirAll(sub, 0o700)))
	marker := filepath.Join(root, "a", "biome.json")
	qt.Assert(t, qt.IsNil(os.WriteFile(marker, []byte("{}"), 0o600)))

	qt.Check(t, qt.Equals(findUpward(sub, root, "biome.json", "biome.jsonc"), filepath.Join(root, "a")))
	qt.Check(t, qt.Equals(findUpward(root, root, "nope.json"), ""))
}

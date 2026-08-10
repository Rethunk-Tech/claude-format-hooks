package formatters

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-quicktest/qt"

	"github.com/Rethunk-Tech/claude-format-hooks/internal/config"
	"github.com/Rethunk-Tech/claude-format-hooks/internal/diskcache"
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

// isolateDiskCache points every internal/diskcache-backed lookup (missing
// binaries, biome's resolved config directory, EditorConfig resolution)
// at a fresh, empty temp dir for the duration of the test. Without this,
// every test in this file would share the real OS cache directory: a
// miss cached by one test (e.g. clearPath's empty PATH) would leak into
// another test that expects the same binary name to be found moments
// later, and — worse — would write real files under the developer's own
// cache directory when running `go test` locally.
func isolateDiskCache(t *testing.T) {
	t.Helper()
	t.Setenv("CLAUDE_FORMAT_HOOKS_CACHE", t.TempDir())
}

func TestExternalFormatterNames(t *testing.T) {
	cases := []struct {
		f    Formatter
		want string
	}{
		{NewJSONRouter(config.Default()), "json"},
		{NewShell(config.Default()), "shfmt"},
		{NewGo(), "gofmt"},
		{NewProto(), "buf"},
		{NewBiome(), "biome"},
		{NewMarkdown(), "markdownlint-cli2"},
		{NewTOML(), "taplo"},
		{NewPrettier(), "prettier"},
		{NewSQLFluff(), "sqlfluff"},
		{NewPython(), "ruff/black"},
		{NewNotebook(), "ruff/black-notebook"},
		{NewRust(), "rustfmt"},
		{NewTerraform(), "terraform"},
	}
	for _, tc := range cases {
		qt.Check(t, qt.Equals(tc.f.Name(), tc.want))
	}
}

func TestBunxFormattersSkipWhenBunxMissing(t *testing.T) {
	isolateDiskCache(t)
	clearPath(t)
	abs := filepath.Join(t.TempDir(), "f.txt")
	for _, f := range []Formatter{NewBiome(), NewMarkdown(), NewTOML(), NewPrettier()} {
		res := f.Format(t.Context(), t.TempDir(), abs)
		qt.Check(t, qt.IsTrue(res.Skipped), qt.Commentf("%s should skip when bunx is not on PATH", f.Name()))
	}
}

func TestPathFormattersRunWithoutBunx(t *testing.T) {
	isolateDiskCache(t)
	dir := t.TempDir()
	cases := []struct {
		name string
		new  func() Formatter
		file string
	}{
		{"biome", NewBiome, "f.ts"},
		{"markdownlint-cli2", NewMarkdown, "f.md"},
		{"taplo", NewTOML, "f.toml"},
		{"prettier", NewPrettier, "f.yaml"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			writeFakeTool(t, tc.name, "exit 0")
			abs := filepath.Join(dir, tc.file)
			res := tc.new().Format(t.Context(), dir, abs)
			qt.Check(t, qt.IsFalse(res.Skipped))
			qt.Check(t, qt.IsNil(res.Err))
			qt.Check(t, qt.Equals(res.Diagnostic, ""))
		})
	}
}

func TestBunxFormatterSuccessAndFailure(t *testing.T) {
	isolateDiskCache(t)
	dir := t.TempDir()
	abs := filepath.Join(dir, "f.md")

	t.Run("success", func(t *testing.T) {
		writeFakeTool(t, "bunx", "exit 0")
		res := NewMarkdown().Format(t.Context(), dir, abs)
		qt.Check(t, qt.IsNil(res.Err))
		qt.Check(t, qt.Equals(res.Diagnostic, ""))
	})

	t.Run("failure with no output falls back to the process error", func(t *testing.T) {
		writeFakeTool(t, "bunx", "exit 1")
		res := NewPrettier().Format(t.Context(), dir, abs)
		qt.Check(t, qt.Not(qt.Equals(res.Diagnostic, "")))
	})
}

func TestTOMLFormatterSuccess(t *testing.T) {
	isolateDiskCache(t)
	writeFakeTool(t, "bunx", "exit 0")
	dir := t.TempDir()
	abs := filepath.Join(dir, "f.toml")
	res := NewTOML().Format(t.Context(), dir, abs)
	qt.Check(t, qt.IsNil(res.Err))
	qt.Check(t, qt.Equals(res.Diagnostic, ""))
}

func TestSQLFluffSkipsWhenMissing(t *testing.T) {
	isolateDiskCache(t)
	clearPath(t)
	abs := filepath.Join(t.TempDir(), "f.sql")
	res := NewSQLFluff().Format(t.Context(), t.TempDir(), abs)
	qt.Check(t, qt.IsTrue(res.Skipped))
}

func TestBiomeFormatSuccess(t *testing.T) {
	isolateDiskCache(t)
	writeFakeTool(t, "bunx", "exit 0")
	dir := t.TempDir()
	abs := filepath.Join(dir, "f.ts")
	res := NewBiome().Format(t.Context(), dir, abs)
	qt.Check(t, qt.IsNil(res.Err))
	qt.Check(t, qt.Equals(res.Diagnostic, ""))
	qt.Check(t, qt.IsFalse(res.Skipped))
}

func TestBiomeFormatFailureTruncatesDiagnostic(t *testing.T) {
	isolateDiskCache(t)
	writeFakeTool(t, "bunx", `i=1; while [ $i -le 20 ]; do echo "line $i"; i=$((i+1)); done; exit 1`)
	dir := t.TempDir()
	abs := filepath.Join(dir, "f.ts")
	res := NewBiome().Format(t.Context(), dir, abs)
	qt.Assert(t, qt.Not(qt.Equals(res.Diagnostic, "")))
	qt.Check(t, qt.IsTrue(strings.Count(res.Diagnostic, "\n")+1 <= 10),
		qt.Commentf("diagnostic has more than 10 lines: %q", res.Diagnostic))
}

func TestSQLFluffFormatSuccessAndFailure(t *testing.T) {
	isolateDiskCache(t)
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	abs := filepath.Join(dir, "f.sql")

	t.Run("success", func(t *testing.T) {
		writeFakeTool(t, "sqlfluff", "exit 0")
		res := NewSQLFluff().Format(t.Context(), dir, abs)
		qt.Check(t, qt.IsNil(res.Err))
		qt.Check(t, qt.Equals(res.Diagnostic, ""))
	})

	t.Run("failure with no output falls back to the process error", func(t *testing.T) {
		// A silent non-zero exit is not the "violations remain" case
		// sqlfluff.go suppresses -- sqlfluff always prints something when it
		// really runs, so this means it died and must stay visible.
		writeFakeTool(t, "sqlfluff", "exit 1")
		res := NewSQLFluff().Format(t.Context(), dir, abs)
		qt.Check(t, qt.Not(qt.Equals(res.Diagnostic, "")))
	})

	t.Run("unfixable violations are not a failure", func(t *testing.T) {
		writeFakeTool(t, "sqlfluff", "echo '  [1 unfixable linting violations found]'; exit 1")
		res := NewSQLFluff().Format(t.Context(), dir, abs)
		qt.Check(t, qt.Equals(res.Diagnostic, ""), qt.Commentf("the file was still rewritten; nothing here can act on the remainder"))
	})

	t.Run("unparsable input is a failure", func(t *testing.T) {
		writeFakeTool(t, "sqlfluff", "echo '  [1 templating/parsing errors found]'; exit 1")
		res := NewSQLFluff().Format(t.Context(), dir, abs)
		qt.Check(t, qt.Not(qt.Equals(res.Diagnostic, "")))
	})
}

func TestPythonFormatterSkipsWhenBothMissing(t *testing.T) {
	isolateDiskCache(t)
	clearPath(t)
	abs := filepath.Join(t.TempDir(), "f.py")
	res := NewPython().Format(t.Context(), t.TempDir(), abs)
	qt.Check(t, qt.IsTrue(res.Skipped))
}

func TestPythonFormatterPrefersRuffOverBlack(t *testing.T) {
	isolateDiskCache(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "ruff")
	qt.Assert(t, qt.IsNil(os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755))) //nolint:gosec // test fixture
	blackPath := filepath.Join(dir, "black")
	qt.Assert(t, qt.IsNil(os.WriteFile(blackPath, []byte("#!/bin/sh\nexit 1\n"), 0o755))) //nolint:gosec // test fixture
	t.Setenv("PATH", dir)

	fileDir := t.TempDir()
	for _, name := range []string{"f.py", "f.pyi"} {
		abs := filepath.Join(fileDir, name)
		res := NewPython().Format(t.Context(), fileDir, abs)
		qt.Check(t, qt.IsNil(res.Err), qt.Commentf("path=%q", abs))
		qt.Check(t, qt.Equals(res.Diagnostic, ""), qt.Commentf("black would have failed; ruff must have run instead for %q", abs))
	}
}

func TestPythonFormatterFallsBackToBlack(t *testing.T) {
	isolateDiskCache(t)
	writeFakeTool(t, "black", "exit 0")
	dir := t.TempDir()
	for _, name := range []string{"f.py", "f.pyi"} {
		abs := filepath.Join(dir, name)
		res := NewPython().Format(t.Context(), dir, abs)
		qt.Check(t, qt.IsNil(res.Err), qt.Commentf("path=%q", abs))
		qt.Check(t, qt.Equals(res.Diagnostic, ""), qt.Commentf("path=%q", abs))
	}
}

func TestPythonFormatterFailure(t *testing.T) {
	isolateDiskCache(t)
	writeFakeTool(t, "ruff", "exit 1")
	dir := t.TempDir()
	for _, name := range []string{"f.py", "f.pyi"} {
		abs := filepath.Join(dir, name)
		res := NewPython().Format(t.Context(), dir, abs)
		qt.Check(t, qt.Not(qt.Equals(res.Diagnostic, "")), qt.Commentf("path=%q", abs))
	}
}

func TestRustFormatterSkipsWhenMissing(t *testing.T) {
	isolateDiskCache(t)
	clearPath(t)
	abs := filepath.Join(t.TempDir(), "f.rs")
	res := NewRust().Format(t.Context(), t.TempDir(), abs)
	qt.Check(t, qt.IsTrue(res.Skipped))
}

func TestRustFormatterSuccessAndFailure(t *testing.T) {
	isolateDiskCache(t)
	dir := t.TempDir()
	abs := filepath.Join(dir, "f.rs")

	t.Run("success", func(t *testing.T) {
		writeFakeTool(t, "rustfmt", "exit 0")
		res := NewRust().Format(t.Context(), dir, abs)
		qt.Check(t, qt.IsNil(res.Err))
		qt.Check(t, qt.Equals(res.Diagnostic, ""))
	})

	t.Run("failure with no output falls back to the process error", func(t *testing.T) {
		writeFakeTool(t, "rustfmt", "exit 1")
		res := NewRust().Format(t.Context(), dir, abs)
		qt.Check(t, qt.Not(qt.Equals(res.Diagnostic, "")))
	})
}

func TestTerraformFormatterSkipsWhenMissing(t *testing.T) {
	isolateDiskCache(t)
	clearPath(t)
	abs := filepath.Join(t.TempDir(), "f.tf")
	res := NewTerraform().Format(t.Context(), t.TempDir(), abs)
	qt.Check(t, qt.IsTrue(res.Skipped))
}

func TestTerraformFormatterSuccessAndFailure(t *testing.T) {
	isolateDiskCache(t)
	dir := t.TempDir()
	paths := []string{
		filepath.Join(dir, "f.tf"),
		filepath.Join(dir, "f.tfvars"),
	}

	t.Run("success", func(t *testing.T) {
		writeFakeTool(t, "terraform", "exit 0")
		for _, abs := range paths {
			res := NewTerraform().Format(t.Context(), dir, abs)
			qt.Check(t, qt.IsNil(res.Err), qt.Commentf("path=%q", abs))
			qt.Check(t, qt.Equals(res.Diagnostic, ""), qt.Commentf("path=%q", abs))
		}
	})

	t.Run("failure with no output falls back to the process error", func(t *testing.T) {
		writeFakeTool(t, "terraform", "exit 1")
		for _, abs := range paths {
			res := NewTerraform().Format(t.Context(), dir, abs)
			qt.Check(t, qt.Not(qt.Equals(res.Diagnostic, "")), qt.Commentf("path=%q", abs))
		}
	})
}

func TestCachedFindUpwardMatchesFindUpward(t *testing.T) {
	isolateDiskCache(t)
	root := t.TempDir()
	sub := filepath.Join(root, "a", "b")
	qt.Assert(t, qt.IsNil(os.MkdirAll(sub, 0o700)))
	marker := filepath.Join(root, "a", "biome.json")
	qt.Assert(t, qt.IsNil(os.WriteFile(marker, []byte("{}"), 0o600)))

	qt.Check(t, qt.Equals(cachedFindUpward(sub, root, "biome.json", "biome.jsonc"), filepath.Join(root, "a")))
}

func TestCachedFindUpwardCachesAFreshMissAgainstANewlyCreatedConfig(t *testing.T) {
	isolateDiskCache(t)
	root := t.TempDir()

	qt.Check(t, qt.Equals(cachedFindUpward(root, root, "biome.json", "biome.jsonc"), ""),
		qt.Commentf("no config exists yet"))

	qt.Assert(t, qt.IsNil(os.WriteFile(filepath.Join(root, "biome.json"), []byte("{}"), 0o600)))
	qt.Check(t, qt.Equals(cachedFindUpward(root, root, "biome.json", "biome.jsonc"), ""),
		qt.Commentf("a fresh cached miss must still mask a config file that appears moments later"))
}

func TestCachedFindUpwardRechecksAfterTTLExpires(t *testing.T) {
	isolateDiskCache(t)
	root := t.TempDir()

	cacheDir, ok := diskcache.Dir()
	qt.Assert(t, qt.IsTrue(ok))
	key := diskcache.Key("findupward", root, root, "biome.json", "biome.jsonc")
	diskcache.Set(cacheDir, key, "")
	stalePath := filepath.Join(cacheDir, key)
	stale := time.Now().Add(-2 * findUpwardCacheTTL).Unix()
	qt.Assert(t, qt.IsNil(os.WriteFile(stalePath, []byte(strconv.FormatInt(stale, 10)+"\n"), 0o600))) //nolint:gosec // test fixture

	qt.Assert(t, qt.IsNil(os.WriteFile(filepath.Join(root, "biome.json"), []byte("{}"), 0o600)))
	qt.Check(t, qt.Equals(cachedFindUpward(root, root, "biome.json", "biome.jsonc"), root),
		qt.Commentf("a stale cached miss must not mask a now-present config file"))
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

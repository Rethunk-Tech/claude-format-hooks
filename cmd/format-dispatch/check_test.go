package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-quicktest/qt"
)

// unformattedJSON needs no external tool: JSON is formatted natively
// in-process, so these tests exercise --check without depending on bunx,
// sqlfluff, or anything else being installed on the runner.
const (
	unformattedJSON = "{\"a\":1,   \"b\":2}\n"
	formattedJSON   = "{\n  \"a\": 1,\n  \"b\": 2\n}\n"
)

// writeCheckFile is writeFile's dir+name form, returning the path so tests
// can assert on it. run_test.go's writeFile takes a full path.
func writeCheckFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	writeFile(t, path, content)
	return path
}

func TestCheckExitsZeroWhenEverythingIsFormatted(t *testing.T) {
	dir := t.TempDir()
	writeCheckFile(t, dir, "ok.json", formattedJSON)

	var out, errOut bytes.Buffer
	qt.Check(t, qt.Equals(runCheck([]string{dir}, &out, &errOut), 0))
}

func TestCheckExitsOneAndNamesTheFile(t *testing.T) {
	dir := t.TempDir()
	path := writeCheckFile(t, dir, "bad.json", unformattedJSON)

	var out, errOut bytes.Buffer
	qt.Assert(t, qt.Equals(runCheck([]string{dir}, &out, &errOut), 1))
	qt.Check(t, qt.StringContains(out.String(), path),
		qt.Commentf("the offending path must be named, or CI output is useless"))
}

func TestCheckNeverModifiesTheFilesItInspects(t *testing.T) {
	// The whole contract: a check that reformats what it inspects is not a
	// check. This is also why the copy is made beside the original rather
	// than the original being formatted and restored.
	dir := t.TempDir()
	path := writeCheckFile(t, dir, "bad.json", unformattedJSON)

	var out, errOut bytes.Buffer
	qt.Assert(t, qt.Equals(runCheck([]string{dir}, &out, &errOut), 1))

	after, err := os.ReadFile(path) //nolint:gosec // path is the temp file this test wrote
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.Equals(string(after), unformattedJSON))
}

func TestCheckLeavesNoScratchFilesBehind(t *testing.T) {
	// The scratch copy lands in the file's own directory (so config
	// discovery still works), which makes cleanup load-bearing -- a leaked
	// .fmtcheck-* file would show up in the operator's git status.
	dir := t.TempDir()
	writeCheckFile(t, dir, "bad.json", unformattedJSON)

	var out, errOut bytes.Buffer
	qt.Assert(t, qt.Equals(runCheck([]string{dir}, &out, &errOut), 1))

	entries, err := os.ReadDir(dir)
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.Equals(len(entries), 1))
}

func TestCheckAcceptsIndividualFiles(t *testing.T) {
	dir := t.TempDir()
	good := writeCheckFile(t, dir, "ok.json", formattedJSON)
	bad := writeCheckFile(t, dir, "bad.json", unformattedJSON)

	var out, errOut bytes.Buffer
	qt.Check(t, qt.Equals(runCheck([]string{good}, &out, &errOut), 0))

	out.Reset()
	qt.Check(t, qt.Equals(runCheck([]string{bad}, &out, &errOut), 1))
}

func TestCheckHonorsProjectConfigDisablesFormatter(t *testing.T) {
	projectRoot := t.TempDir()
	path := writeCheckFile(t, projectRoot, "bad.json", unformattedJSON)
	writeCheckFile(t, projectRoot, projectConfigFile, `{"disabled": [".json"]}`)
	configPath := filepath.Join(t.TempDir(), "claude-format-hooks.json")
	writeFile(t, configPath, `{}`)

	t.Setenv("CLAUDE_PROJECT_DIR", projectRoot)
	t.Setenv("CLAUDE_FORMAT_HOOKS_CONFIG", configPath)

	var out, errOut bytes.Buffer
	qt.Check(t, qt.Equals(runCheck([]string{projectRoot}, &out, &errOut), 0))
	qt.Check(t, qt.Equals(readFile(t, path), unformattedJSON),
		qt.Commentf("project-disabled extension must not be reported for formatting"))
}

func TestCheckProjectConfigMalformedWarnsAndContinues(t *testing.T) {
	projectRoot := t.TempDir()
	path := writeCheckFile(t, projectRoot, "bad.json", unformattedJSON)
	writeCheckFile(t, projectRoot, projectConfigFile, "not valid json")
	configPath := filepath.Join(t.TempDir(), "claude-format-hooks.json")
	writeFile(t, configPath, `{}`)

	t.Setenv("CLAUDE_PROJECT_DIR", projectRoot)
	t.Setenv("CLAUDE_FORMAT_HOOKS_CONFIG", configPath)

	var out, errOut bytes.Buffer
	qt.Check(t, qt.Equals(runCheck([]string{projectRoot}, &out, &errOut), 1))
	qt.Check(t, qt.StringContains(errOut.String(), "project config:"))
	qt.Check(t, qt.StringContains(out.String(), path))
	qt.Check(t, qt.Equals(readFile(t, path), unformattedJSON))
}

func TestCheckDispatchesNestedBiomeFromProjectRoot(t *testing.T) {
	if filepath.Separator == '\\' {
		t.Skip("fake formatter script is POSIX-shell only")
	}
	projectRoot := t.TempDir()
	path := writeCheckFile(t, filepath.Join(projectRoot, "nested"), "bad.ts", "const value={answer:42}\n")
	writeCheckFile(t, projectRoot, "biome.json", `{}`)
	configPath := filepath.Join(t.TempDir(), "claude-format-hooks.json")
	writeFile(t, configPath, `{}`)

	toolDir := t.TempDir()
	marker := filepath.Join(t.TempDir(), "formatter-cwd")
	script := "#!/bin/sh\nprintf '%s' \"$PWD\" > \"$FMTCHECK_MARKER\"\nfor arg in \"$@\"; do\n  case \"$arg\" in\n    *.ts) printf 'const value = { answer: 42 };\\n' > \"$arg\"; exit 0 ;;\n  esac\ndone\nexit 1\n"
	for _, name := range []string{"biome", "bunx"} {
		tool := filepath.Join(toolDir, name)
		qt.Assert(t, qt.IsNil(os.WriteFile(tool, []byte(script), 0o755))) //nolint:gosec // test fixture
	}

	t.Setenv("CLAUDE_PROJECT_DIR", projectRoot)
	t.Setenv("CLAUDE_FORMAT_HOOKS_CONFIG", configPath)
	t.Setenv("FMTCHECK_MARKER", marker)
	t.Setenv("PATH", toolDir)

	var out, errOut bytes.Buffer
	qt.Check(t, qt.Equals(runCheck([]string{projectRoot}, &out, &errOut), 1))
	qt.Check(t, qt.Equals(readFile(t, marker), projectRoot),
		qt.Commentf("nested checks must dispatch from the project root"))
	qt.Check(t, qt.StringContains(out.String(), path))

	hungPath := writeCheckFile(t, filepath.Join(projectRoot, "nested"), "hung.ts", "const value={answer:42}\n")
	writeFile(t, path, "const value = { answer: 42 };\n")
	timeoutScript := "#!/bin/sh\nprintf 'timeout:%s' \"$PWD\" > \"$FMTCHECK_MARKER\"\nexec /bin/sleep 30\n"
	for _, name := range []string{"biome", "bunx"} {
		tool := filepath.Join(toolDir, name)
		qt.Assert(t, qt.IsNil(os.WriteFile(tool, []byte(timeoutScript), 0o755))) //nolint:gosec // test fixture
	}
	var timeoutOut, timeoutErrOut bytes.Buffer
	started := time.Now()
	code := runCheck([]string{hungPath}, &timeoutOut, &timeoutErrOut)
	elapsed := time.Since(started)

	qt.Check(t, qt.Equals(code, 2))
	qt.Check(t, qt.IsTrue(elapsed < 10*time.Second),
		qt.Commentf("a single hung formatter must use the per-file timeout, not checkTimeout"))
	qt.Check(t, qt.Equals(readFile(t, marker), "timeout:"+projectRoot))
	qt.Check(t, qt.StringContains(timeoutErrOut.String(), hungPath))
	qt.Check(t, qt.StringContains(timeoutErrOut.String(), errFormatterTimeout.Error()))
}

func TestCollectCheckTargetsSkipsScratchFiles(t *testing.T) {
	dir := t.TempDir()
	keep := writeCheckFile(t, dir, "keep.json", formattedJSON)
	writeCheckFile(t, dir, ".fmtcheck-leftover-keep.json", formattedJSON)

	targets, err := collectCheckTargets([]string{dir})
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.DeepEquals(targets, []string{keep}))
}

func TestCheckIgnoresUnsupportedExtensions(t *testing.T) {
	dir := t.TempDir()
	writeCheckFile(t, dir, "notes.xyz", "whatever   \n")

	var out, errOut bytes.Buffer
	qt.Check(t, qt.Equals(runCheck([]string{dir}, &out, &errOut), 0))
}

func TestCheckSkipsVendoredDirectories(t *testing.T) {
	// Formatting is never applied inside node_modules, so failing a build
	// over a dependency's formatting would be unfixable by design.
	dir := t.TempDir()
	vendored := filepath.Join(dir, "node_modules")
	qt.Assert(t, qt.IsNil(os.MkdirAll(vendored, 0o750)))
	writeCheckFile(t, vendored, "dep.json", unformattedJSON)

	var out, errOut bytes.Buffer
	qt.Check(t, qt.Equals(runCheck([]string{dir}, &out, &errOut), 0))
}

func TestCheckRejectsNoPaths(t *testing.T) {
	var out, errOut bytes.Buffer
	// 2, not 1: "you invoked me wrong" must be distinguishable from
	// "your files need formatting", or CI cannot tell a broken pipeline
	// from a legitimate failure.
	qt.Check(t, qt.Equals(runCheck(nil, &out, &errOut), 2))
}

func TestCheckReportsAMissingPath(t *testing.T) {
	var out, errOut bytes.Buffer
	missing := filepath.Join(t.TempDir(), "nope")
	qt.Check(t, qt.Equals(runCheck([]string{missing}, &out, &errOut), 2))
}

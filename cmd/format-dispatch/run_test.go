package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/go-quicktest/qt"
)

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stderr
	r, w, err := os.Pipe()
	qt.Assert(t, qt.IsNil(err))
	os.Stderr = w
	defer func() { os.Stderr = orig }()

	fn()

	qt.Assert(t, qt.IsNil(w.Close()))
	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	qt.Assert(t, qt.IsNil(err))
	return buf.String()
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	qt.Assert(t, qt.IsNil(err))
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	fn()

	qt.Assert(t, qt.IsNil(w.Close()))
	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	qt.Assert(t, qt.IsNil(err))
	return buf.String()
}

func TestDispatchArgsHelp(t *testing.T) {
	for _, flag := range []string{"--help", "-h"} {
		t.Run(flag, func(t *testing.T) {
			var code int
			stdout := captureStdout(t, func() { code = dispatchArgs([]string{flag}) })
			qt.Check(t, qt.Equals(code, 0))
			qt.Check(t, qt.StringContains(stdout, "Usage:"))
		})
	}
}

func TestDispatchArgsVersion(t *testing.T) {
	var code int
	stdout := captureStdout(t, func() { code = dispatchArgs([]string{"--version"}) })
	qt.Check(t, qt.Equals(code, 0))
	qt.Check(t, qt.StringContains(stdout, "format-dispatch"))
}

func TestDispatchArgsUnknownFlagPrintsUsageAndFails(t *testing.T) {
	var code int
	stderr := captureStderr(t, func() { code = dispatchArgs([]string{"--bogus"}) })
	qt.Check(t, qt.Equals(code, 1))
	qt.Check(t, qt.StringContains(stderr, "Usage:"))
}

func TestVersionStringReportsBuildInfo(t *testing.T) {
	got := versionString()
	qt.Check(t, qt.StringContains(got, "format-dispatch"))
}

func TestDispatchArgsRoutesInstallAndUninstall(t *testing.T) {
	t.Run("--install", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("CLAUDE_HOOKS_BIN_DIR", filepath.Join(dir, "bin"))
		settingsPath := filepath.Join(dir, "settings.json")
		t.Setenv("CLAUDE_SETTINGS_FILE", settingsPath)

		qt.Check(t, qt.Equals(dispatchArgs([]string{"--install", "--dry-run"}), 0))
		_, err := os.Stat(settingsPath)
		qt.Check(t, qt.IsTrue(os.IsNotExist(err)), qt.Commentf("--dry-run must not write settings.json"))
	})

	t.Run("--uninstall", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("CLAUDE_HOOKS_BIN_DIR", filepath.Join(dir, "bin"))
		settingsPath := filepath.Join(dir, "settings.json")
		t.Setenv("CLAUDE_SETTINGS_FILE", settingsPath)

		qt.Check(t, qt.Equals(dispatchArgs([]string{"--install"}), 0))
		qt.Check(t, qt.Equals(dispatchArgs([]string{"--uninstall"}), 0))
		qt.Check(t, qt.IsFalse(strings.Contains(readFile(t, settingsPath), filepath.Join(dir, "bin", "format-dispatch"))))
	})
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	qt.Assert(t, qt.IsNil(os.MkdirAll(filepath.Dir(path), 0o700)))
	qt.Assert(t, qt.IsNil(os.WriteFile(path, []byte(content), 0o600))) //nolint:gosec // test fixture
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path) //nolint:gosec // test fixture
	qt.Assert(t, qt.IsNil(err))
	return string(b)
}

// payload builds a PostToolUse JSON payload naming filePath. Windows
// paths contain backslashes, which must be JSON-escaped — json.Marshal
// on the raw string (rather than naive concatenation) gets this right
// regardless of platform.
func payload(filePath string) string {
	encoded, err := json.Marshal(filePath)
	if err != nil {
		panic(err) // Marshal on a string value never errors
	}
	return `{"tool_input":{"file_path":` + string(encoded) + `}}`
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("boom") }

func TestRunInstallWiresAndUninstallsSettings(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_HOOKS_BIN_DIR", filepath.Join(dir, "bin"))
	settingsPath := filepath.Join(dir, "settings.json")
	t.Setenv("CLAUDE_SETTINGS_FILE", settingsPath)

	qt.Check(t, qt.Equals(runInstall(nil, false), 0))
	qt.Check(t, qt.StringContains(readFile(t, settingsPath), "format-dispatch"))

	qt.Check(t, qt.Equals(runInstall(nil, true), 0))
	qt.Check(t, qt.IsFalse(strings.Contains(readFile(t, settingsPath), filepath.Join(dir, "bin", "format-dispatch"))))
}

func TestRunInstallDryRunDoesNotWrite(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_HOOKS_BIN_DIR", filepath.Join(dir, "bin"))
	settingsPath := filepath.Join(dir, "settings.json")
	t.Setenv("CLAUDE_SETTINGS_FILE", settingsPath)

	qt.Check(t, qt.Equals(runInstall([]string{"--dry-run"}, false), 0))
	_, err := os.Stat(settingsPath)
	qt.Check(t, qt.IsTrue(os.IsNotExist(err)), qt.Commentf("--dry-run must not write settings.json"))
}

func TestRunInstallRejectsUnrecognizedArgs(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantSubstr string
	}{
		{"typo of --dry-run", []string{"--dryrun"}, "unrecognized argument"},
		{"unrelated flag", []string{"--bogus"}, "unrecognized argument"},
		{"extra arguments", []string{"--dry-run", "extra"}, "unexpected arguments"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("CLAUDE_HOOKS_BIN_DIR", filepath.Join(dir, "bin"))
			settingsPath := filepath.Join(dir, "settings.json")
			t.Setenv("CLAUDE_SETTINGS_FILE", settingsPath)

			var code int
			stderr := captureStderr(t, func() { code = runInstall(tc.args, false) })
			qt.Check(t, qt.Equals(code, 1))
			qt.Check(t, qt.StringContains(stderr, tc.wantSubstr))
			_, err := os.Stat(settingsPath)
			qt.Check(t, qt.IsTrue(os.IsNotExist(err)), qt.Commentf("a rejected arg must not write settings.json"))
		})
	}
}

func TestRunInstallReportsDefaultOptionsError(t *testing.T) {
	t.Setenv("HOME", "")
	// os.UserHomeDir() reads USERPROFILE on Windows, not HOME — clearing
	// only HOME leaves the real runner profile dir in place there.
	t.Setenv("USERPROFILE", "")
	t.Setenv("CLAUDE_HOOKS_BIN_DIR", "")
	t.Setenv("CLAUDE_SETTINGS_FILE", "")

	qt.Check(t, qt.Equals(runInstall(nil, false), 1))
}

func TestRunReadStdinError(t *testing.T) {
	qt.Check(t, qt.Equals(run(errReader{}), 1))
}

func TestRunEmptyPayloadIsNoop(t *testing.T) {
	qt.Check(t, qt.Equals(run(strings.NewReader(`{}`)), 0))
}

func TestRunUnsupportedExtensionIsNoop(t *testing.T) {
	abs := filepath.Join(t.TempDir(), "f.xyz")
	writeFile(t, abs, "irrelevant")

	qt.Check(t, qt.Equals(run(strings.NewReader(payload(abs))), 0))
	qt.Check(t, qt.Equals(readFile(t, abs), "irrelevant"))
}

func TestRunOutsideProjectRootIsSkipped(t *testing.T) {
	projectRoot := t.TempDir()
	outside := t.TempDir()
	abs := filepath.Join(outside, "f.json")
	writeFile(t, abs, `{"b":1,"a":2}`)

	t.Setenv("CLAUDE_PROJECT_DIR", projectRoot)
	qt.Check(t, qt.Equals(run(strings.NewReader(payload(abs))), 0))
	qt.Check(t, qt.Equals(readFile(t, abs), `{"b":1,"a":2}`))
}

func TestRunVendoredDirIsSkipped(t *testing.T) {
	projectRoot := t.TempDir()
	abs := filepath.Join(projectRoot, "node_modules", "pkg", "f.json")
	writeFile(t, abs, `{"b":1,"a":2}`)

	t.Setenv("CLAUDE_PROJECT_DIR", projectRoot)
	qt.Check(t, qt.Equals(run(strings.NewReader(payload(abs))), 0))
	qt.Check(t, qt.Equals(readFile(t, abs), `{"b":1,"a":2}`))
}

func TestRunDispatchesToJSONFormatter(t *testing.T) {
	projectRoot := t.TempDir()
	abs := filepath.Join(projectRoot, "f.json")
	writeFile(t, abs, `{"b":1,"a":2}`)

	t.Setenv("CLAUDE_PROJECT_DIR", projectRoot)
	qt.Check(t, qt.Equals(run(strings.NewReader(payload(abs))), 0))
	qt.Check(t, qt.Equals(readFile(t, abs), "{\n  \"b\": 1,\n  \"a\": 2\n}\n"))
}

func TestRunProjectConfigDisablesFormatter(t *testing.T) {
	projectRoot := t.TempDir()
	abs := filepath.Join(projectRoot, "f.json")
	src := `{"b":1,"a":2}`
	writeFile(t, abs, src)
	writeFile(t, filepath.Join(projectRoot, projectConfigFile), `{"disabled": [".json"]}`)

	t.Setenv("CLAUDE_PROJECT_DIR", projectRoot)
	qt.Check(t, qt.Equals(run(strings.NewReader(payload(abs))), 0))
	qt.Check(t, qt.Equals(readFile(t, abs), src), qt.Commentf("project-disabled extension must not be formatted"))
}

func TestRunProjectConfigMalformedFallsBackAndWarns(t *testing.T) {
	projectRoot := t.TempDir()
	abs := filepath.Join(projectRoot, "f.json")
	writeFile(t, abs, `{"b":1,"a":2}`)
	writeFile(t, filepath.Join(projectRoot, projectConfigFile), `not valid json`)

	t.Setenv("CLAUDE_PROJECT_DIR", projectRoot)
	var code int
	stderr := captureStderr(t, func() { code = run(strings.NewReader(payload(abs))) })
	qt.Check(t, qt.Equals(code, 0))
	qt.Check(t, qt.StringContains(stderr, "project config"))
	qt.Check(t, qt.Equals(readFile(t, abs), "{\n  \"b\": 1,\n  \"a\": 2\n}\n"),
		qt.Commentf("a malformed project config must not block formatting"))
}

func TestLogInvocationNoopWhenEnvUnset(t *testing.T) {
	t.Setenv("CLAUDE_FORMAT_HOOKS_LOG", "")
	logPath := filepath.Join(t.TempDir(), "format-dispatch.log")

	logInvocation("/some/file.json", "json", "ok", time.Millisecond)

	_, err := os.Stat(logPath)
	qt.Check(t, qt.IsTrue(os.IsNotExist(err)), qt.Commentf("logInvocation must not write anywhere when unset"))
}

func TestLogInvocationSwallowsWriteFailure(t *testing.T) {
	// A directory can't be opened for writing as a regular file — this
	// must not panic or otherwise surface.
	t.Setenv("CLAUDE_FORMAT_HOOKS_LOG", t.TempDir())
	logInvocation("/some/file.json", "json", "ok", time.Millisecond)
}

func TestRunLogsInvocationWhenEnvSet(t *testing.T) {
	projectRoot := t.TempDir()
	abs := filepath.Join(projectRoot, "f.json")
	writeFile(t, abs, `{"b":1,"a":2}`)
	logPath := filepath.Join(t.TempDir(), "format-dispatch.log")

	t.Setenv("CLAUDE_PROJECT_DIR", projectRoot)
	t.Setenv("CLAUDE_FORMAT_HOOKS_LOG", logPath)
	qt.Check(t, qt.Equals(run(strings.NewReader(payload(abs))), 0))

	log := readFile(t, logPath)
	// abs is %q-quoted in the log line, so on Windows its backslashes are
	// doubled — compare against the same %q form logInvocation actually
	// writes, not the raw path.
	qt.Check(t, qt.StringContains(log, fmt.Sprintf("path=%q", abs)))
	qt.Check(t, qt.StringContains(log, `formatter="json"`))
	qt.Check(t, qt.StringContains(log, `outcome="ok"`))
}

func TestRunLogsSkipReason(t *testing.T) {
	abs := filepath.Join(t.TempDir(), "f.xyz")
	writeFile(t, abs, "irrelevant")
	logPath := filepath.Join(t.TempDir(), "format-dispatch.log")

	t.Setenv("CLAUDE_FORMAT_HOOKS_LOG", logPath)
	qt.Check(t, qt.Equals(run(strings.NewReader(payload(abs))), 0))

	log := readFile(t, logPath)
	qt.Check(t, qt.StringContains(log, `outcome="skip: unsupported extension"`))
}

func TestRunPrintsDiagnosticOnFormatterFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake shell-script tool is POSIX-shell only")
	}
	toolDir := t.TempDir()
	script := filepath.Join(toolDir, "bunx")
	body := "#!/bin/sh\ni=1\nwhile [ $i -le 20 ]; do echo \"line $i\"; i=$((i+1)); done\nexit 1\n"
	qt.Assert(t, qt.IsNil(os.WriteFile(script, []byte(body), 0o755))) //nolint:gosec // test fixture
	t.Setenv("PATH", toolDir)

	projectRoot := t.TempDir()
	abs := filepath.Join(projectRoot, "f.md")
	writeFile(t, abs, "# heading")
	t.Setenv("CLAUDE_PROJECT_DIR", projectRoot)

	var got int
	stderr := captureStderr(t, func() {
		got = run(strings.NewReader(payload(abs)))
	})
	qt.Check(t, qt.Equals(got, 0))
	qt.Check(t, qt.StringContains(stderr, "fixer failed"))
	qt.Check(t, qt.StringContains(stderr, "line 1"))
	qt.Check(t, qt.IsFalse(strings.Contains(stderr, "line 11")), qt.Commentf("want output truncated to 10 lines"))
}

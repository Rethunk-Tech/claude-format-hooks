package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

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

func payload(filePath string) string {
	return `{"tool_input":{"file_path":"` + filePath + `"}}`
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

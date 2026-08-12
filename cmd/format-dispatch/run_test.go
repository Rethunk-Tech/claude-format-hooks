package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Rethunk-Tech/claude-format-hooks/internal/installer"
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

func TestVersionStringFrom(t *testing.T) {
	cases := []struct {
		name     string
		settings []debug.BuildSetting
		want     string
	}{
		{"no VCS metadata omits the parenthetical", nil, "format-dispatch v1.2.3"},
		{
			"revision longer than 12 chars is truncated",
			[]debug.BuildSetting{{Key: "vcs.revision", Value: "abcdef0123456789"}},
			"format-dispatch v1.2.3 (abcdef012345)",
		},
		{
			"vcs.modified=true appends -dirty",
			[]debug.BuildSetting{{Key: "vcs.revision", Value: "abc123"}, {Key: "vcs.modified", Value: "true"}},
			"format-dispatch v1.2.3 (abc123-dirty)",
		},
		{
			"vcs.modified=false appends nothing",
			[]debug.BuildSetting{{Key: "vcs.revision", Value: "abc123"}, {Key: "vcs.modified", Value: "false"}},
			"format-dispatch v1.2.3 (abc123)",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			info := &debug.BuildInfo{Main: debug.Module{Version: "v1.2.3"}, Settings: tc.settings}
			qt.Check(t, qt.Equals(versionStringFrom(info), tc.want))
		})
	}
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

func TestRunUpgradeRejectsUnrecognizedArgs(t *testing.T) {
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
			binDir := filepath.Join(dir, "bin")
			t.Setenv("CLAUDE_HOOKS_BIN_DIR", binDir)
			t.Setenv("CLAUDE_SETTINGS_FILE", filepath.Join(dir, "settings.json"))

			var code int
			stderr := captureStderr(t, func() { code = runUpgrade(tc.args) })
			qt.Check(t, qt.Equals(code, 1))
			qt.Check(t, qt.StringContains(stderr, tc.wantSubstr))
			qt.Check(t, qt.StringContains(stderr, "format-dispatch --upgrade:"))
			entries, err := os.ReadDir(binDir)
			qt.Check(t, qt.IsTrue(os.IsNotExist(err) || len(entries) == 0),
				qt.Commentf("rejected args must not write a binary"))
		})
	}
}

func TestDispatchArgsRoutesUpgrade(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_HOOKS_BIN_DIR", filepath.Join(dir, "bin"))
	t.Setenv("CLAUDE_SETTINGS_FILE", filepath.Join(dir, "settings.json"))

	var code int
	stderr := captureStderr(t, func() { code = dispatchArgs([]string{"--upgrade", "--bogus"}) })
	qt.Check(t, qt.Equals(code, 1))
	qt.Check(t, qt.StringContains(stderr, "format-dispatch --upgrade:"))
}

func TestDispatchArgsUpgradeHappyPathViaReleaseAPI(t *testing.T) {
	binary := []byte("new release binary\n")
	digest := sha256.Sum256(binary)

	dir := t.TempDir()
	binDir := filepath.Join(dir, "bin")
	target := installer.HookBinaryPath(binDir)
	oldBinary := []byte("old release binary\n")
	qt.Assert(t, qt.IsNil(os.MkdirAll(binDir, 0o700)))
	qt.Assert(t, qt.IsNil(os.WriteFile(target, oldBinary, 0o751))) //nolint:gosec // test fixture

	assetName := fmt.Sprintf("format-dispatch-%s-%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		assetName += ".exe"
	}
	checksumFile := []byte(fmt.Sprintf("%x  %s\n", digest, assetName))

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/Rethunk-Tech/claude-format-hooks/releases/latest":
			_, _ = fmt.Fprintf(w, `{"tag_name":"v9.9.9","assets":[{"name":%q,"browser_download_url":%q},{"name":%q,"browser_download_url":%q}]}`,
				assetName, server.URL+"/binary", assetName+".sha256", server.URL+"/checksum")
		case "/binary":
			_, _ = w.Write(binary)
		case "/checksum":
			_, _ = w.Write(checksumFile)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	t.Setenv("CLAUDE_FORMAT_HOOKS_RELEASE_API", server.URL)
	t.Setenv("CLAUDE_HOOKS_BIN_DIR", binDir)
	t.Setenv("CLAUDE_SETTINGS_FILE", filepath.Join(dir, "settings.json"))

	var code int
	stdout := captureStdout(t, func() {
		code = dispatchArgs([]string{"--upgrade"})
	})
	qt.Check(t, qt.Equals(code, 0))
	qt.Check(t, qt.StringContains(stdout, "upgraded"))
	qt.Check(t, qt.StringContains(stdout, target))
	qt.Check(t, qt.Equals(readFile(t, target), string(binary)))
}

func TestDispatchArgsUpgradeDryRunViaReleaseAPI(t *testing.T) {
	dir := t.TempDir()
	binDir := filepath.Join(dir, "bin")
	target := installer.HookBinaryPath(binDir)
	oldBinary := []byte("old release binary\n")
	qt.Assert(t, qt.IsNil(os.MkdirAll(binDir, 0o700)))
	qt.Assert(t, qt.IsNil(os.WriteFile(target, oldBinary, 0o751))) //nolint:gosec // test fixture

	var binaryRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/binary" {
			binaryRequests.Add(1)
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	t.Setenv("CLAUDE_FORMAT_HOOKS_RELEASE_API", server.URL)
	t.Setenv("CLAUDE_HOOKS_BIN_DIR", binDir)
	t.Setenv("CLAUDE_SETTINGS_FILE", filepath.Join(dir, "settings.json"))

	var code int
	stdout := captureStdout(t, func() {
		code = dispatchArgs([]string{"--upgrade", "--dry-run"})
	})
	qt.Check(t, qt.Equals(code, 0))
	qt.Check(t, qt.StringContains(stdout, "--dry-run"))
	qt.Check(t, qt.StringContains(stdout, "would download"))
	qt.Check(t, qt.StringContains(stdout, target))
	qt.Check(t, qt.StringContains(stdout, "not written"))
	qt.Check(t, qt.Equals(readFile(t, target), string(oldBinary)))
	qt.Check(t, qt.Equals(binaryRequests.Load(), int32(0)))
}

func TestRunInstallReportsActionError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_HOOKS_BIN_DIR", filepath.Join(dir, "bin"))
	// A directory settings path is never IsNotExist but always fails the
	// installer's read -- forcing Install/Uninstall to return an error
	// runInstall must propagate, not swallow.
	t.Setenv("CLAUDE_SETTINGS_FILE", dir)

	var code int
	stderr := captureStderr(t, func() { code = runInstall(nil, false) })
	qt.Check(t, qt.Equals(code, 1))
	qt.Check(t, qt.StringContains(stderr, "format-dispatch --install:"))
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

func TestRunUpgradeReportsDefaultOptionsError(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	t.Setenv("CLAUDE_HOOKS_BIN_DIR", "")
	t.Setenv("CLAUDE_SETTINGS_FILE", "")

	qt.Check(t, qt.Equals(runUpgrade(nil), 1))
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

func TestRunDispatchesExtensionlessShellShebang(t *testing.T) {
	projectRoot := t.TempDir()
	abs := filepath.Join(projectRoot, "script")
	src := "#!/usr/bin/env bash\necho    hello\n"
	writeFile(t, abs, src)

	t.Setenv("CLAUDE_PROJECT_DIR", projectRoot)
	qt.Check(t, qt.Equals(run(strings.NewReader(payload(abs))), 0))
	got := readFile(t, abs)
	qt.Check(t, qt.StringContains(got, "#!/usr/bin/env bash"))
	qt.Check(t, qt.IsFalse(got == src), qt.Commentf("shell shebang file should be formatted"))
}

func TestRunExtensionlessNonShellFilesAreNoop(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"non-shell shebang", "#!/usr/bin/env python\nprint('hello')\n"},
		{"no shebang", "plain text\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			projectRoot := t.TempDir()
			abs := filepath.Join(projectRoot, "script")
			writeFile(t, abs, tc.src)

			t.Setenv("CLAUDE_PROJECT_DIR", projectRoot)
			qt.Check(t, qt.Equals(run(strings.NewReader(payload(abs))), 0))
			qt.Check(t, qt.Equals(readFile(t, abs), tc.src))
		})
	}
}

func TestRunRealExtensionsSkipWithoutShebangPeek(t *testing.T) {
	for _, ext := range []string{".ts", ".md"} {
		t.Run(ext, func(t *testing.T) {
			projectRoot := t.TempDir()
			abs := filepath.Join(projectRoot, "missing"+ext)
			logPath := filepath.Join(t.TempDir(), "format-dispatch.log")

			t.Setenv("CLAUDE_PROJECT_DIR", projectRoot)
			t.Setenv("CLAUDE_FORMAT_HOOKS_LOG", logPath)
			qt.Check(t, qt.Equals(run(strings.NewReader(payload(abs))), 0))
			qt.Check(t, qt.StringContains(readFile(t, logPath), "skip: stat failed or is a directory"))
		})
	}
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

func TestRunDispatchesJSONFromToolResult(t *testing.T) {
	projectRoot := t.TempDir()
	abs := filepath.Join(projectRoot, "f.json")
	writeFile(t, abs, `{"b":1,"a":2}`)
	encoded, err := json.Marshal(abs)
	qt.Assert(t, qt.IsNil(err))

	t.Setenv("CLAUDE_PROJECT_DIR", projectRoot)
	raw := `{"tool_result":{"filePath":` + string(encoded) + `}}`
	qt.Check(t, qt.Equals(run(strings.NewReader(raw)), 0))
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

func TestRunProjectConfigDisablesFormatterByName(t *testing.T) {
	projectRoot := t.TempDir()
	abs := filepath.Join(projectRoot, "f.ts")
	src := "const value={answer:42}\n"
	writeFile(t, abs, src)
	writeFile(t, filepath.Join(projectRoot, projectConfigFile), `{"disabledFormatters":["BIOME"]}`)

	t.Setenv("CLAUDE_PROJECT_DIR", projectRoot)
	qt.Check(t, qt.Equals(run(strings.NewReader(payload(abs))), 0))
	qt.Check(t, qt.Equals(readFile(t, abs), src), qt.Commentf("project-disabled formatter must not be formatted"))
}

func TestRunProjectConfigDisablesBiomeForJSONRouter(t *testing.T) {
	projectRoot := t.TempDir()
	abs := filepath.Join(projectRoot, "f.json")
	src := `{"b":1,"a":2}`
	writeFile(t, abs, src)
	writeFile(t, filepath.Join(projectRoot, "biome.json"), "{}\n")
	writeFile(t, filepath.Join(projectRoot, projectConfigFile), `{"disabledFormatters":["biome"]}`)

	t.Setenv("CLAUDE_PROJECT_DIR", projectRoot)
	qt.Check(t, qt.Equals(run(strings.NewReader(payload(abs))), 0))
	qt.Check(t, qt.Equals(readFile(t, abs), "{\n  \"b\": 1,\n  \"a\": 2\n}\n"),
		qt.Commentf("project-disabled biome must leave the native JSON router enabled"))
}

func TestRunProjectConfigDisablesBiomeForGraphQLRouter(t *testing.T) {
	toolDir := t.TempDir()
	marker := filepath.Join(t.TempDir(), "formatter")
	scripts := map[string]string{
		"biome": "#!/bin/sh\nprintf 'biome' > \"$FORMATTER_MARKER\"\nexit 0\n",
		"bunx":  "#!/bin/sh\nprintf 'prettier' > \"$FORMATTER_MARKER\"\nexit 0\n",
	}
	if filepath.Separator == '\\' {
		scripts = map[string]string{
			"biome.cmd": "@echo off\r\n@<nul set /p \"=biome\" > \"%FORMATTER_MARKER%\"\r\n",
			"bunx.cmd":  "@echo off\r\n@<nul set /p \"=prettier\" > \"%FORMATTER_MARKER%\"\r\n",
		}
	}
	for name, script := range scripts {
		qt.Assert(t, qt.IsNil(os.WriteFile(filepath.Join(toolDir, name), []byte(script), 0o755))) //nolint:gosec // test fixture
	}

	projectRoot := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "claude-format-hooks.json")
	writeFile(t, configPath, `{}`)
	t.Setenv("CLAUDE_PROJECT_DIR", projectRoot)
	t.Setenv("CLAUDE_FORMAT_HOOKS_CONFIG", configPath)
	t.Setenv("CLAUDE_FORMAT_HOOKS_CACHE", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("FORMATTER_MARKER", marker)
	t.Setenv("PATH", toolDir)
	writeFile(t, filepath.Join(projectRoot, "biome.json"), "{}\n")
	writeFile(t, filepath.Join(projectRoot, projectConfigFile), `{"disabledFormatters":["biome"]}`)

	for _, ext := range []string{".graphql", ".gql"} {
		t.Run(ext, func(t *testing.T) {
			abs := filepath.Join(projectRoot, "f"+ext)
			writeFile(t, abs, "query { user { id } }\n")
			qt.Check(t, qt.Equals(run(strings.NewReader(payload(abs))), 0))
			qt.Check(t, qt.Equals(readFile(t, marker), "prettier"),
				qt.Commentf("project-disabled biome must route %s through prettier", ext))
		})
	}
}

func TestRunProjectConfigDisablesBiomeForJSONC(t *testing.T) {
	projectRoot := t.TempDir()
	abs := filepath.Join(projectRoot, "f.jsonc")
	src := `{"a":1}`
	writeFile(t, abs, src)
	writeFile(t, filepath.Join(projectRoot, projectConfigFile), `{"disabledFormatters":["biome"]}`)

	t.Setenv("CLAUDE_PROJECT_DIR", projectRoot)
	qt.Check(t, qt.Equals(run(strings.NewReader(payload(abs))), 0))
	qt.Check(t, qt.Equals(readFile(t, abs), src), qt.Commentf("project-disabled biome must not format JSONC"))
}

func TestRunUserConfigDisablesFormatter(t *testing.T) {
	projectRoot := t.TempDir()
	abs := filepath.Join(projectRoot, "f.json")
	src := `{"b":1,"a":2}`
	writeFile(t, abs, src)
	configPath := filepath.Join(t.TempDir(), "claude-format-hooks.json")
	writeFile(t, configPath, `{"disabled": [".json"]}`)

	t.Setenv("CLAUDE_PROJECT_DIR", projectRoot)
	t.Setenv("CLAUDE_FORMAT_HOOKS_CONFIG", configPath)
	logPath := filepath.Join(t.TempDir(), "format-dispatch.log")
	t.Setenv("CLAUDE_FORMAT_HOOKS_LOG", logPath)

	qt.Check(t, qt.Equals(run(strings.NewReader(payload(abs))), 0))
	qt.Check(t, qt.Equals(readFile(t, abs), src), qt.Commentf("user-disabled extension must not be formatted"))
	qt.Check(t, qt.StringContains(readFile(t, logPath), `outcome="skip: disabled by config"`))
}

func TestRunUserConfigDisablesFormatterByName(t *testing.T) {
	projectRoot := t.TempDir()
	abs := filepath.Join(projectRoot, "f.ts")
	src := "const value={answer:42}\n"
	writeFile(t, abs, src)
	configPath := filepath.Join(t.TempDir(), "claude-format-hooks.json")
	writeFile(t, configPath, `{"disabledFormatters":["biome"]}`)

	t.Setenv("CLAUDE_PROJECT_DIR", projectRoot)
	t.Setenv("CLAUDE_FORMAT_HOOKS_CONFIG", configPath)
	qt.Check(t, qt.Equals(run(strings.NewReader(payload(abs))), 0))
	qt.Check(t, qt.Equals(readFile(t, abs), src), qt.Commentf("user-disabled formatter must not be formatted"))
}

func TestRunUserConfigDisablesJSONFormatterByName(t *testing.T) {
	projectRoot := t.TempDir()
	abs := filepath.Join(projectRoot, "f.json")
	src := `{"b":1,"a":2}`
	writeFile(t, abs, src)
	configPath := filepath.Join(t.TempDir(), "claude-format-hooks.json")
	writeFile(t, configPath, `{"disabledFormatters":["json"]}`)

	t.Setenv("CLAUDE_PROJECT_DIR", projectRoot)
	t.Setenv("CLAUDE_FORMAT_HOOKS_CONFIG", configPath)
	qt.Check(t, qt.Equals(run(strings.NewReader(payload(abs))), 0))
	qt.Check(t, qt.Equals(readFile(t, abs), src), qt.Commentf("user-disabled json formatter must not be formatted"))
}

func TestRunUserConfigMalformedFallsBackAndWarns(t *testing.T) {
	projectRoot := t.TempDir()
	abs := filepath.Join(projectRoot, "f.json")
	writeFile(t, abs, `{"b":1,"a":2}`)
	configPath := filepath.Join(t.TempDir(), "claude-format-hooks.json")
	writeFile(t, configPath, `not valid json`)

	t.Setenv("CLAUDE_PROJECT_DIR", projectRoot)
	t.Setenv("CLAUDE_FORMAT_HOOKS_CONFIG", configPath)
	var code int
	stderr := captureStderr(t, func() { code = run(strings.NewReader(payload(abs))) })
	qt.Check(t, qt.Equals(code, 0))
	qt.Check(t, qt.StringContains(stderr, "config:"))
	qt.Check(t, qt.Equals(readFile(t, abs), "{\n  \"b\": 1,\n  \"a\": 2\n}\n"),
		qt.Commentf("a malformed user config must not block formatting"))
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

package installer

import (
	"encoding/json"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/go-quicktest/qt"
)

const binPath = "/home/user/.claude/hooks/format-dispatch"

func TestHookBinaryBaseName(t *testing.T) {
	tests := []struct {
		name string
		goos string
		want string
	}{
		{name: "windows", goos: "windows", want: "format-dispatch.exe"},
		{name: "linux", goos: "linux", want: "format-dispatch"},
		{name: "darwin", goos: "darwin", want: "format-dispatch"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qt.Check(t, qt.Equals(hookBinaryBaseName(tt.goos), tt.want))
		})
	}

	want := "format-dispatch"
	if runtime.GOOS == "windows" {
		want = "format-dispatch.exe"
	}
	qt.Check(t, qt.Equals(HookBinaryBaseName(), want))
}

func TestDefaultOptionsUsesEnvOverrides(t *testing.T) {
	// Built via filepath.Join, not a hardcoded POSIX literal, since
	// DefaultOptions itself joins with the OS-native separator.
	binDir := filepath.Join(t.TempDir(), "custom", "bin")
	settingsFile := filepath.Join(t.TempDir(), "custom", "settings.json")
	cursorHooksFile := filepath.Join(t.TempDir(), "custom", "hooks.json")
	t.Setenv("CLAUDE_HOOKS_BIN_DIR", binDir)
	t.Setenv("CLAUDE_SETTINGS_FILE", settingsFile)
	t.Setenv("CURSOR_HOOKS_FILE", cursorHooksFile)

	opts, err := DefaultOptions()
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.Equals(opts.BinPath, HookBinaryPath(binDir)))
	qt.Check(t, qt.Equals(opts.SettingsPath, settingsFile))
	qt.Check(t, qt.Equals(opts.CursorHooksPath, cursorHooksFile))
}

func TestDefaultOptionsReportsUserHomeDirError(t *testing.T) {
	t.Setenv("HOME", "")
	// os.UserHomeDir() reads USERPROFILE on Windows, not HOME.
	t.Setenv("USERPROFILE", "")

	_, err := DefaultOptions()
	qt.Check(t, qt.IsNotNil(err))
}

func TestDefaultOptionsFallsBackUnderHome(t *testing.T) {
	t.Setenv("CLAUDE_HOOKS_BIN_DIR", "")
	t.Setenv("CLAUDE_SETTINGS_FILE", "")
	t.Setenv("CURSOR_HOOKS_FILE", "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	// os.UserHomeDir() reads USERPROFILE on Windows, not HOME.
	t.Setenv("USERPROFILE", home)

	opts, err := DefaultOptions()
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.Equals(opts.BinPath, HookBinaryPath(filepath.Join(home, ".claude", "hooks"))))
	qt.Check(t, qt.Equals(opts.SettingsPath, filepath.Join(home, ".claude", "settings.json")))
	qt.Check(t, qt.Equals(opts.CursorHooksPath, filepath.Join(home, ".cursor", "hooks.json")))
}

func settingsPostToolUse(t *testing.T, raw []byte) []PostToolUseEntry {
	t.Helper()
	var top map[string]json.RawMessage
	qt.Assert(t, qt.IsNil(json.Unmarshal(raw, &top)))
	var hooks map[string]json.RawMessage
	qt.Assert(t, qt.IsNil(json.Unmarshal(top["hooks"], &hooks)))
	var entries []PostToolUseEntry
	qt.Assert(t, qt.IsNil(json.Unmarshal(hooks["PostToolUse"], &entries)))
	return entries
}

func TestWireFreshInstall(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	qt.Assert(t, qt.IsNil(os.WriteFile(settingsPath, []byte(`{"theme":"dark"}`), 0o600)))

	_, after, err := Wire(settingsPath, binPath)
	qt.Assert(t, qt.IsNil(err))

	var top map[string]json.RawMessage
	qt.Assert(t, qt.IsNil(json.Unmarshal(after, &top)))
	var theme string
	qt.Assert(t, qt.IsNil(json.Unmarshal(top["theme"], &theme)))
	qt.Check(t, qt.Equals(theme, "dark"))

	entries := settingsPostToolUse(t, after)
	qt.Assert(t, qt.HasLen(entries, 1))
	qt.Check(t, qt.Equals(entries[0].Matcher, "Write|Edit|MultiEdit|NotebookEdit"))
	qt.Assert(t, qt.HasLen(entries[0].Hooks, 1))
	qt.Check(t, qt.Equals(entries[0].Hooks[0].Command, binPath))
	qt.Check(t, qt.Equals(entries[0].Hooks[0].Timeout, hookTimeout))

	// The written timeout is what Claude Code kills the hook at, and it has
	// to leave room for the binary's own formatterTimeout to fire first --
	// otherwise a hung formatter is killed with no diagnostic explaining
	// why. formatterTimeout lives in package main and can't be imported
	// here, so this pins the settings.json side against a stale edit and
	// main.go's comment carries the other half.
	qt.Check(t, qt.IsTrue(hookTimeout > 4), qt.Commentf("must exceed cmd/format-dispatch's formatterTimeout"))
}

func TestWireReportsUnreadableSettingsFile(t *testing.T) {
	// A directory is never IsNotExist but always fails os.ReadFile,
	// distinguishing "missing" (silently treated as {}) from "exists but
	// something else went wrong" (must propagate, not be swallowed).
	dir := t.TempDir()

	_, _, err := Wire(dir, binPath)
	qt.Check(t, qt.IsNotNil(err))
}

func TestUnwireReportsUnreadableSettingsFile(t *testing.T) {
	dir := t.TempDir()

	_, _, err := Unwire(dir, binPath)
	qt.Check(t, qt.IsNotNil(err))
}

func TestWireMissingSettingsFile(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")

	_, after, err := Wire(settingsPath, binPath)
	qt.Assert(t, qt.IsNil(err))

	entries := settingsPostToolUse(t, after)
	qt.Assert(t, qt.HasLen(entries, 1))
	qt.Check(t, qt.Equals(entries[0].Matcher, "Write|Edit|MultiEdit|NotebookEdit"))
	qt.Check(t, qt.Equals(entries[0].Hooks[0].Command, binPath))
}

func TestWireIdempotentReinstall(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	qt.Assert(t, qt.IsNil(os.WriteFile(settingsPath, []byte(`{}`), 0o600)))

	_, first, err := Wire(settingsPath, binPath)
	qt.Assert(t, qt.IsNil(err))
	qt.Assert(t, qt.IsNil(os.WriteFile(settingsPath, first, 0o600)))

	_, second, err := Wire(settingsPath, binPath)
	qt.Assert(t, qt.IsNil(err))

	entries := settingsPostToolUse(t, second)
	qt.Assert(t, qt.HasLen(entries, 1))
	qt.Check(t, qt.Equals(entries[0].Hooks[0].Command, binPath))
}

func TestWireReplacesLegacyBareBasename(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	exeBinPath := filepath.Join(t.TempDir(), "format-dispatch.exe")
	existing := `{
		"hooks": {
			"PostToolUse": [
				{"matcher": "Write|Edit|MultiEdit|NotebookEdit", "hooks": [{"type": "command", "command": "` + binPath + `"}]}
			]
		}
	}`
	qt.Assert(t, qt.IsNil(os.WriteFile(settingsPath, []byte(existing), 0o600)))

	_, after, err := Wire(settingsPath, exeBinPath)
	qt.Assert(t, qt.IsNil(err))

	entries := settingsPostToolUse(t, after)
	qt.Assert(t, qt.HasLen(entries, 1))
	qt.Check(t, qt.Equals(entries[0].Hooks[0].Command, exeBinPath))
}

func TestUnwireExeRemovesLegacyBareBasename(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	exeBinPath := filepath.Join(t.TempDir(), "format-dispatch.exe")
	existing := `{
		"hooks": {
			"PostToolUse": [
				{"matcher": "Write|Edit|MultiEdit|NotebookEdit", "hooks": [{"type": "command", "command": "` + binPath + `"}]}
			]
		}
	}`
	qt.Assert(t, qt.IsNil(os.WriteFile(settingsPath, []byte(existing), 0o600)))

	_, after, err := Unwire(settingsPath, exeBinPath)
	qt.Assert(t, qt.IsNil(err))

	entries := settingsPostToolUse(t, after)
	qt.Check(t, qt.HasLen(entries, 0))
}

func TestWireReplacesOldBiomeOnlyHook(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	old := `{
		"hooks": {
			"PostToolUse": [
				{
					"matcher": "Write|Edit",
					"hooks": [{"type": "command", "command": "biome check --write \"$FILE\""}]
				}
			]
		}
	}`
	qt.Assert(t, qt.IsNil(os.WriteFile(settingsPath, []byte(old), 0o600)))

	_, after, err := Wire(settingsPath, binPath)
	qt.Assert(t, qt.IsNil(err))

	entries := settingsPostToolUse(t, after)
	qt.Assert(t, qt.HasLen(entries, 1))
	qt.Check(t, qt.Equals(entries[0].Matcher, "Write|Edit|MultiEdit|NotebookEdit"))
	qt.Check(t, qt.Equals(entries[0].Hooks[0].Command, binPath))
}

func TestWirePreservesTopLevelKeyOrder(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	// A deliberately non-alphabetical order, mirroring a real operator's
	// hand-curated settings.json.
	existing := `{"env":{},"permissions":{},"model":"opus","hooks":{},"statusLine":{}}`
	qt.Assert(t, qt.IsNil(os.WriteFile(settingsPath, []byte(existing), 0o600)))

	_, after, err := Wire(settingsPath, binPath)
	qt.Assert(t, qt.IsNil(err))

	var top map[string]json.RawMessage
	qt.Assert(t, qt.IsNil(json.Unmarshal(after, &top)))
	qt.Check(t, qt.HasLen(top, 5))

	got := keysInOrder(t, after)
	qt.Check(t, qt.DeepEquals(got, []string{"env", "permissions", "model", "hooks", "statusLine"}))
}

func keysInOrder(t *testing.T, raw []byte) []string {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	tok, err := dec.Token()
	qt.Assert(t, qt.IsNil(err))
	qt.Assert(t, qt.IsTrue(tok == json.Delim('{')))

	var keys []string
	for dec.More() {
		keyTok, err := dec.Token()
		qt.Assert(t, qt.IsNil(err))
		keys = append(keys, keyTok.(string))
		var skip json.RawMessage
		qt.Assert(t, qt.IsNil(dec.Decode(&skip)))
	}
	return keys
}

func TestWirePreservesUnrelatedHooks(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	existing := `{
		"hooks": {
			"PreToolUse": [
				{"matcher": "Bash", "hooks": [{"type": "command", "command": "guard.sh"}]}
			],
			"PostToolUse": [
				{"matcher": "Bash", "hooks": [{"type": "command", "command": "log.sh"}]}
			]
		}
	}`
	qt.Assert(t, qt.IsNil(os.WriteFile(settingsPath, []byte(existing), 0o600)))

	_, after, err := Wire(settingsPath, binPath)
	qt.Assert(t, qt.IsNil(err))

	var top map[string]json.RawMessage
	qt.Assert(t, qt.IsNil(json.Unmarshal(after, &top)))
	var hooks map[string]json.RawMessage
	qt.Assert(t, qt.IsNil(json.Unmarshal(top["hooks"], &hooks)))
	_, hasPreToolUse := hooks["PreToolUse"]
	qt.Check(t, qt.IsTrue(hasPreToolUse))

	entries := settingsPostToolUse(t, after)
	qt.Assert(t, qt.HasLen(entries, 2))
	qt.Check(t, qt.Equals(entries[0].Matcher, "Bash"))
	qt.Check(t, qt.Equals(entries[0].Hooks[0].Command, "log.sh"))
	qt.Check(t, qt.Equals(entries[1].Hooks[0].Command, binPath))
}

func TestUnwireRemovesOwnEntry(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	qt.Assert(t, qt.IsNil(os.WriteFile(settingsPath, []byte(`{"theme":"dark"}`), 0o600)))

	_, wired, err := Wire(settingsPath, binPath)
	qt.Assert(t, qt.IsNil(err))
	qt.Assert(t, qt.IsNil(os.WriteFile(settingsPath, wired, 0o600)))

	before, after, err := Unwire(settingsPath, binPath)
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.DeepEquals(before, wired))

	var top map[string]json.RawMessage
	qt.Assert(t, qt.IsNil(json.Unmarshal(after, &top)))
	var theme string
	qt.Assert(t, qt.IsNil(json.Unmarshal(top["theme"], &theme)))
	qt.Check(t, qt.Equals(theme, "dark"))

	entries := settingsPostToolUse(t, after)
	qt.Check(t, qt.HasLen(entries, 0))
}

func TestUnwirePreservesOtherEntries(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	existing := `{
		"hooks": {
			"PostToolUse": [
				{"matcher": "Bash", "hooks": [{"type": "command", "command": "log.sh"}]},
				{"matcher": "Write|Edit|MultiEdit|NotebookEdit", "hooks": [{"type": "command", "command": "` + binPath + `"}]}
			]
		}
	}`
	qt.Assert(t, qt.IsNil(os.WriteFile(settingsPath, []byte(existing), 0o600)))

	_, after, err := Unwire(settingsPath, binPath)
	qt.Assert(t, qt.IsNil(err))

	entries := settingsPostToolUse(t, after)
	qt.Assert(t, qt.HasLen(entries, 1))
	qt.Check(t, qt.Equals(entries[0].Hooks[0].Command, "log.sh"))
}

func TestUnwireNoOwnEntryIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	qt.Assert(t, qt.IsNil(os.WriteFile(settingsPath, []byte(`{"theme":"dark"}`), 0o600)))

	_, after, err := Unwire(settingsPath, binPath)
	qt.Assert(t, qt.IsNil(err))

	entries := settingsPostToolUse(t, after)
	qt.Check(t, qt.HasLen(entries, 0))
}

func TestUninstallDryRunDoesNotWrite(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	_, wired, err := Wire(settingsPath, binPath)
	qt.Assert(t, qt.IsNil(err))
	qt.Assert(t, qt.IsNil(os.WriteFile(settingsPath, wired, 0o600)))

	var out strings.Builder
	qt.Assert(t, qt.IsNil(Uninstall(Options{BinPath: binPath, SettingsPath: settingsPath}, true, &out)))

	raw, err := os.ReadFile(settingsPath)
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.DeepEquals(raw, wired))
	qt.Check(t, qt.StringContains(out.String(), "dry-run"))
}

func TestUninstallWritesSettings(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	_, wired, err := Wire(settingsPath, binPath)
	qt.Assert(t, qt.IsNil(err))
	qt.Assert(t, qt.IsNil(os.WriteFile(settingsPath, wired, 0o600)))

	var out strings.Builder
	qt.Assert(t, qt.IsNil(Uninstall(Options{BinPath: binPath, SettingsPath: settingsPath}, false, &out)))

	entries := settingsPostToolUse(t, readFile(t, settingsPath))
	qt.Check(t, qt.HasLen(entries, 0))
}

func TestInstallPropagatesWireError(t *testing.T) {
	dir := t.TempDir()
	var out strings.Builder
	err := Install(Options{BinPath: binPath, SettingsPath: dir}, false, &out)
	qt.Check(t, qt.IsNotNil(err))
}

func TestUninstallPropagatesUnwireError(t *testing.T) {
	dir := t.TempDir()
	var out strings.Builder
	err := Uninstall(Options{BinPath: binPath, SettingsPath: dir}, false, &out)
	qt.Check(t, qt.IsNotNil(err))
}

func TestInstallNoOpWhenAlreadyWired(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	_, wired, err := Wire(settingsPath, binPath)
	qt.Assert(t, qt.IsNil(err))
	qt.Assert(t, qt.IsNil(os.WriteFile(settingsPath, wired, 0o600)))

	var out strings.Builder
	qt.Assert(t, qt.IsNil(Install(Options{BinPath: binPath, SettingsPath: settingsPath}, false, &out)))
	qt.Check(t, qt.StringContains(out.String(), "nothing to do"))
}

func TestInstallBacksUpExistingSettings(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	original := []byte(`{"theme":"dark"}`)
	qt.Assert(t, qt.IsNil(os.WriteFile(settingsPath, original, 0o600)))

	var out strings.Builder
	qt.Assert(t, qt.IsNil(Install(Options{BinPath: binPath, SettingsPath: settingsPath}, false, &out)))

	backup, err := os.ReadFile(settingsPath + ".bak")
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.DeepEquals(backup, original))
	qt.Check(t, qt.StringContains(out.String(), ".bak"))
}

func TestInstallFreshInstallWritesNoBackup(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")

	var out strings.Builder
	qt.Assert(t, qt.IsNil(Install(Options{BinPath: binPath, SettingsPath: settingsPath}, false, &out)))

	_, err := os.Stat(settingsPath + ".bak")
	qt.Check(t, qt.IsTrue(os.IsNotExist(err)), qt.Commentf("no prior settings.json existed, so nothing should be backed up"))
}

func TestInstallSecondRunOverwritesRollingBackup(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	qt.Assert(t, qt.IsNil(os.WriteFile(settingsPath, []byte(`{"theme":"dark"}`), 0o600)))

	var out strings.Builder
	qt.Assert(t, qt.IsNil(Install(Options{BinPath: binPath, SettingsPath: settingsPath}, false, &out)))
	afterFirstInstall := readFile(t, settingsPath)

	qt.Assert(t, qt.IsNil(Uninstall(Options{BinPath: binPath, SettingsPath: settingsPath}, false, &out)))

	backup, err := os.ReadFile(settingsPath + ".bak")
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.DeepEquals(backup, afterFirstInstall), qt.Commentf("the rolling backup should hold the state just before the most recent write, not the very first one"))
}

func TestInstallDryRunDoesNotWrite(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	qt.Assert(t, qt.IsNil(os.WriteFile(settingsPath, []byte(`{}`), 0o600)))

	var out strings.Builder
	err := Install(Options{BinPath: binPath, SettingsPath: settingsPath}, true, &out)
	qt.Assert(t, qt.IsNil(err))

	raw, err := os.ReadFile(settingsPath)
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.Equals(string(raw), "{}"))
	qt.Check(t, qt.StringContains(out.String(), "dry-run"))
}

func TestInstallWritesSettings(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")

	var out strings.Builder
	err := Install(Options{BinPath: binPath, SettingsPath: settingsPath}, false, &out)
	qt.Assert(t, qt.IsNil(err))

	entries := settingsPostToolUse(t, readFile(t, settingsPath))
	qt.Assert(t, qt.HasLen(entries, 1))
	qt.Check(t, qt.Equals(entries[0].Hooks[0].Command, binPath))
}

func TestParseSettingsReportsMalformedTopLevelJSON(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	qt.Assert(t, qt.IsNil(os.WriteFile(settingsPath, []byte(`{"hooks":`), 0o600)))

	_, _, err := Wire(settingsPath, binPath)
	qt.Assert(t, qt.IsNotNil(err))
	qt.Check(t, qt.StringContains(err.Error(), "parse "+settingsPath+":"))
}

func TestParseSettingsReportsHooksObjectError(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	qt.Assert(t, qt.IsNil(os.WriteFile(settingsPath, []byte(`{"hooks":[]}`), 0o600)))

	_, _, err := Wire(settingsPath, binPath)
	qt.Assert(t, qt.IsNotNil(err))
	qt.Check(t, qt.StringContains(err.Error(), "parse "+settingsPath+": hooks:"))
}

func TestParseSettingsReportsPostToolUseArrayError(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	qt.Assert(t, qt.IsNil(os.WriteFile(settingsPath, []byte(`{"hooks":{"PostToolUse":{}}}`), 0o600)))

	_, _, err := Wire(settingsPath, binPath)
	qt.Assert(t, qt.IsNotNil(err))
	qt.Check(t, qt.StringContains(err.Error(), "parse "+settingsPath+": hooks.PostToolUse:"))
}

func TestWriteAtomicWritesContentAndLeavesNoTempFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.json")

	qt.Assert(t, qt.IsNil(writeAtomic(path, []byte(`{"a":1}`), 0o600)))

	qt.Check(t, qt.DeepEquals(readFile(t, path), []byte(`{"a":1}`)))
	_, err := os.Stat(path + ".tmp")
	qt.Check(t, qt.IsTrue(os.IsNotExist(err)), qt.Commentf("a successful write must not leave its temp file behind"))
}

func TestWriteAtomicOverwritesExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.json")
	qt.Assert(t, qt.IsNil(os.WriteFile(path, []byte(`{"old":true}`), 0o600)))

	qt.Assert(t, qt.IsNil(writeAtomic(path, []byte(`{"new":true}`), 0o600)))

	qt.Check(t, qt.DeepEquals(readFile(t, path), []byte(`{"new":true}`)))
}

func TestWriteAtomicCreateTempFailure(t *testing.T) {
	dir := t.TempDir()
	parent := filepath.Join(dir, "parent")
	qt.Assert(t, qt.IsNil(os.WriteFile(parent, []byte("not a directory"), 0o600)))

	err := writeAtomic(filepath.Join(parent, "f.json"), []byte(`{"a":1}`), 0o600)
	qt.Check(t, qt.IsNotNil(err))
}

func TestWriteAtomicRenameFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.json")
	qt.Assert(t, qt.IsNil(os.Mkdir(path, 0o755)))

	err := writeAtomic(path, []byte(`{"a":1}`), 0o600)
	qt.Check(t, qt.IsNotNil(err))
	entries, readErr := os.ReadDir(dir)
	qt.Assert(t, qt.IsNil(readErr))
	qt.Check(t, qt.HasLen(entries, 1), qt.Commentf("failed rename must remove the temporary file"))
}

func TestWriteAtomicCreateTempFailureInReadOnlyDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not enforce directory mode bits")
	}

	dir := t.TempDir()
	qt.Assert(t, qt.IsNil(os.Chmod(dir, 0o555)))
	t.Cleanup(func() {
		_ = os.Chmod(dir, 0o700)
	})
	probe, err := os.CreateTemp(dir, "permission-probe-*")
	if err == nil {
		_ = probe.Close()
		_ = os.Remove(probe.Name())
		t.Skip("directory remains writable after chmod")
	}

	err = writeAtomic(filepath.Join(dir, "f.json"), []byte(`{"a":1}`), 0o600)
	qt.Check(t, qt.IsNotNil(err))
}

func TestApplyChangeReportsBackupWriteFailure(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	backupPath := settingsPath + ".bak"
	qt.Assert(t, qt.IsNil(os.WriteFile(settingsPath, []byte(`{"before":true}`), 0o600)))
	qt.Assert(t, qt.IsNil(os.Mkdir(backupPath, 0o755)))

	var out strings.Builder
	err := applyChange(
		Options{SettingsPath: settingsPath},
		[]byte(`{"before":true}`),
		[]byte(`{"after":true}`),
		false,
		&out,
		"updated",
	)
	qt.Assert(t, qt.IsNotNil(err))
	qt.Check(t, qt.StringContains(err.Error(), "backup "+settingsPath+":"))
}

func TestApplyChangeReportsSettingsReadFailure(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	qt.Assert(t, qt.IsNil(os.Mkdir(settingsPath, 0o755)))

	var out strings.Builder
	err := applyChange(
		Options{SettingsPath: settingsPath},
		[]byte(`{"before":true}`),
		[]byte(`{"after":true}`),
		false,
		&out,
		"updated",
	)
	qt.Check(t, qt.IsNotNil(err))
}

func TestApplyChangeReportsMkdirFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not enforce directory mode bits")
	}

	dir := t.TempDir()
	blocked := filepath.Join(dir, "blocked")
	qt.Assert(t, qt.IsNil(os.Mkdir(blocked, 0o755)))
	qt.Assert(t, qt.IsNil(os.Chmod(blocked, 0o555)))
	t.Cleanup(func() {
		_ = os.Chmod(blocked, 0o700)
	})
	probe := filepath.Join(blocked, "permission-probe")
	if err := os.Mkdir(probe, 0o700); err == nil {
		_ = os.Remove(probe)
		t.Skip("directory remains writable after chmod")
	}

	settingsPath := filepath.Join(blocked, "nested", "settings.json")
	var out strings.Builder
	err := applyChange(
		Options{SettingsPath: settingsPath},
		[]byte(`{"before":true}`),
		[]byte(`{"after":true}`),
		false,
		&out,
		"updated",
	)
	qt.Check(t, qt.IsNotNil(err))
}

func TestApplyChangeReportsSettingsWriteFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not enforce directory mode bits")
	}

	dir := t.TempDir()
	qt.Assert(t, qt.IsNil(os.Chmod(dir, 0o555)))
	t.Cleanup(func() {
		_ = os.Chmod(dir, 0o700)
	})
	probe := filepath.Join(dir, "permission-probe")
	if err := os.WriteFile(probe, []byte("probe"), 0o600); err == nil {
		_ = os.Remove(probe)
		t.Skip("directory remains writable after chmod")
	}

	settingsPath := filepath.Join(dir, "settings.json")
	var out strings.Builder
	err := applyChange(
		Options{SettingsPath: settingsPath},
		[]byte(`{"before":true}`),
		[]byte(`{"after":true}`),
		false,
		&out,
		"updated",
	)
	qt.Check(t, qt.IsNotNil(err))
}

func TestWriteAtomicWriteFailure(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux prlimit is required to exercise a real write failure")
	}
	if os.Getenv("FORMAT_DISPATCH_WRITE_ATOMIC_WRITE_HELPER") == "1" {
		withFileSizeLimit(t, func() {
			err := writeAtomic(filepath.Join(t.TempDir(), "f.json"), []byte(`{"a":1}`), 0o600)

			qt.Check(t, qt.IsNotNil(err))
		})
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestWriteAtomicWriteFailure$")
	cmd.Env = append(os.Environ(), "FORMAT_DISPATCH_WRITE_ATOMIC_WRITE_HELPER=1")
	output, err := cmd.CombinedOutput()
	qt.Assert(t, qt.IsNil(err), qt.Commentf("write-failure helper output: %s", output))
}

func TestWriteAtomicChmodFailure(t *testing.T) {
	original := writeAtomicChmod
	writeAtomicChmod = func(*os.File, os.FileMode) error {
		return os.ErrPermission
	}
	t.Cleanup(func() {
		writeAtomicChmod = original
	})

	dir := t.TempDir()
	err := writeAtomic(filepath.Join(dir, "f.json"), []byte(`{"a":1}`), 0o600)

	qt.Check(t, qt.IsNotNil(err))
	entries, readErr := os.ReadDir(dir)
	qt.Assert(t, qt.IsNil(readErr))
	qt.Check(t, qt.HasLen(entries, 0), qt.Commentf("failed chmod must remove the temporary file"))
}

func TestWriteAtomicCloseFailure(t *testing.T) {
	original := writeAtomicClose
	writeAtomicClose = func(f *os.File) error {
		_ = f.Close()
		return os.ErrPermission
	}
	t.Cleanup(func() {
		writeAtomicClose = original
	})

	dir := t.TempDir()
	path := filepath.Join(dir, "f.json")
	err := writeAtomic(path, []byte(`{"a":1}`), 0o600)

	qt.Check(t, qt.IsNotNil(err))
	_, statErr := os.Stat(path)
	qt.Check(t, qt.IsTrue(os.IsNotExist(statErr)), qt.Commentf("failed close must not create the destination"))
	entries, readErr := os.ReadDir(dir)
	qt.Assert(t, qt.IsNil(readErr))
	qt.Check(t, qt.HasLen(entries, 0), qt.Commentf("failed close must remove the temporary file"))
}

func withFileSizeLimit(t *testing.T, fn func()) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("Linux prlimit is required to exercise a real write failure")
	}

	pid := strconv.Itoa(os.Getpid())
	output, err := exec.Command("prlimit", "--pid", pid, "--fsize").Output()
	if err != nil {
		t.Skipf("prlimit is unavailable: %v", err)
	}
	fields := strings.Fields(string(output))
	if len(fields) < 3 {
		t.Fatalf("unexpected prlimit output: %q", output)
	}
	soft, hard := fields[len(fields)-3], fields[len(fields)-2]

	sigxfsz := syscall.Signal(25)
	signal.Ignore(sigxfsz)
	defer signal.Reset(sigxfsz)
	if err := exec.Command("prlimit", "--pid", pid, "--fsize=0:"+hard).Run(); err != nil {
		t.Fatalf("set file-size limit: %v", err)
	}
	defer func() {
		if err := exec.Command("prlimit", "--pid", pid, "--fsize="+soft+":"+hard).Run(); err != nil {
			t.Errorf("restore file-size limit: %v", err)
		}
	}()

	fn()
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	qt.Assert(t, qt.IsNil(err))
	return raw
}

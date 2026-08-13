package installer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/go-quicktest/qt"
)

func TestWireCursorPreservesCapturedShape(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hooks.json")
	captured := `{"version":1,"hooks":{"sessionStart":[{"command":"session-start","timeout":5}]}}`
	qt.Assert(t, qt.IsNil(os.WriteFile(path, []byte(captured), 0o600)))

	_, after, err := WireCursor(path, binPath)
	qt.Assert(t, qt.IsNil(err))

	var top map[string]json.RawMessage
	qt.Assert(t, qt.IsNil(json.Unmarshal(after, &top)))
	var version int
	qt.Assert(t, qt.IsNil(json.Unmarshal(top["version"], &version)))
	qt.Check(t, qt.Equals(version, 1))

	var hooks map[string]json.RawMessage
	qt.Assert(t, qt.IsNil(json.Unmarshal(top["hooks"], &hooks)))
	var sessionStart []cursorHookCommand
	qt.Assert(t, qt.IsNil(json.Unmarshal(hooks["sessionStart"], &sessionStart)))
	qt.Assert(t, qt.HasLen(sessionStart, 1))
	qt.Check(t, qt.Equals(sessionStart[0].Command, "session-start"))

	var afterFileEdit []cursorHookCommand
	qt.Assert(t, qt.IsNil(json.Unmarshal(hooks[cursorEvent], &afterFileEdit)))
	qt.Assert(t, qt.HasLen(afterFileEdit, 1))
	qt.Check(t, qt.Equals(afterFileEdit[0].Command, binPath))
	qt.Check(t, qt.Equals(afterFileEdit[0].Timeout, hookTimeout))
}

func TestWireCursorMissingFileCreatesVersionOne(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hooks.json")

	_, after, err := WireCursor(path, binPath)
	qt.Assert(t, qt.IsNil(err))

	var top map[string]json.RawMessage
	qt.Assert(t, qt.IsNil(json.Unmarshal(after, &top)))
	qt.Check(t, qt.DeepEquals(keysInOrder(t, after), []string{"version", "hooks"}))
	var version int
	qt.Assert(t, qt.IsNil(json.Unmarshal(top["version"], &version)))
	qt.Check(t, qt.Equals(version, 1))
}

func TestVersionlessCursorFileStaysVersionless(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hooks.json")
	existing := `{"hooks":{"sessionStart":[{"command":"session-start","timeout":5}]}}`
	qt.Assert(t, qt.IsNil(os.WriteFile(path, []byte(existing), 0o600)))

	_, wired, err := WireCursor(path, binPath)
	qt.Assert(t, qt.IsNil(err))
	assertCursorHasNoVersion(t, wired)
	qt.Assert(t, qt.IsNil(os.WriteFile(path, wired, 0o600)))

	_, unwired, err := UnwireCursor(path, binPath)
	qt.Assert(t, qt.IsNil(err))
	assertCursorHasNoVersion(t, unwired)
}

func TestUnwireCursorRemovesOnlyFormatDispatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hooks.json")
	existing := `{
		"version": 1,
		"hooks": {
			"sessionStart": [{"command": "session-start", "timeout": 5}],
			"afterFileEdit": [
				{"command": "format-docs", "timeout": 5},
				{"command": "` + binPath + `", "timeout": 5},
				{"command": "format-dispatch.exe", "timeout": 5}
			]
		}
	}`
	qt.Assert(t, qt.IsNil(os.WriteFile(path, []byte(existing), 0o600)))

	_, after, err := UnwireCursor(path, binPath)
	qt.Assert(t, qt.IsNil(err))

	var top map[string]json.RawMessage
	qt.Assert(t, qt.IsNil(json.Unmarshal(after, &top)))
	var hooks map[string]json.RawMessage
	qt.Assert(t, qt.IsNil(json.Unmarshal(top["hooks"], &hooks)))
	var sessionStart []cursorHookCommand
	qt.Assert(t, qt.IsNil(json.Unmarshal(hooks["sessionStart"], &sessionStart)))
	qt.Check(t, qt.HasLen(sessionStart, 1))
	var afterFileEdit []cursorHookCommand
	qt.Assert(t, qt.IsNil(json.Unmarshal(hooks[cursorEvent], &afterFileEdit)))
	qt.Assert(t, qt.HasLen(afterFileEdit, 1))
	qt.Check(t, qt.Equals(afterFileEdit[0].Command, "format-docs"))
}

func TestUnwireCursorDoesNotMatchBinBasenameAlone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hooks.json")
	existing := `{"hooks":{"afterFileEdit":[{"command":"/tmp/custom-hook","timeout":5}]}}`
	qt.Assert(t, qt.IsNil(os.WriteFile(path, []byte(existing), 0o600)))

	_, after, err := UnwireCursor(path, "/opt/custom-hook")
	qt.Assert(t, qt.IsNil(err))
	var top map[string]json.RawMessage
	qt.Assert(t, qt.IsNil(json.Unmarshal(after, &top)))
	var hooks map[string]json.RawMessage
	qt.Assert(t, qt.IsNil(json.Unmarshal(top["hooks"], &hooks)))
	var entries []cursorHookCommand
	qt.Assert(t, qt.IsNil(json.Unmarshal(hooks[cursorEvent], &entries)))
	qt.Assert(t, qt.HasLen(entries, 1))
	qt.Check(t, qt.Equals(entries[0].Command, "/tmp/custom-hook"))
}

func TestInstallAndUninstallWithCursorHooks(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	cursorPath := filepath.Join(dir, "hooks.json")
	cursorBefore := `{"version":1,"hooks":{"sessionStart":[{"command":"session-start","timeout":5}]}}`
	qt.Assert(t, qt.IsNil(os.WriteFile(cursorPath, []byte(cursorBefore), 0o600)))
	opts := Options{BinPath: binPath, SettingsPath: settingsPath, CursorHooksPath: cursorPath}

	var out strings.Builder
	qt.Assert(t, qt.IsNil(Install(opts, false, &out)))
	assertCursorHasOnlyOurHook(t, cursorPath, true)
	entries := settingsPostToolUse(t, readFile(t, settingsPath))
	qt.Assert(t, qt.HasLen(entries, 1))
	qt.Check(t, qt.Equals(entries[0].Hooks[0].Command, binPath))

	qt.Assert(t, qt.IsNil(Uninstall(opts, false, &out)))
	assertCursorEventEmpty(t, cursorPath)
	entries = settingsPostToolUse(t, readFile(t, settingsPath))
	qt.Check(t, qt.HasLen(entries, 0))
}

func TestUninstallMissingCursorLeavesFileMissing(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	cursorPath := filepath.Join(dir, "hooks.json")
	_, wired, err := Wire(settingsPath, binPath)
	qt.Assert(t, qt.IsNil(err))
	qt.Assert(t, qt.IsNil(os.WriteFile(settingsPath, wired, 0o600)))

	var out strings.Builder
	opts := Options{BinPath: binPath, SettingsPath: settingsPath, CursorHooksPath: cursorPath}
	qt.Assert(t, qt.IsNil(Uninstall(opts, false, &out)))
	_, err = os.Stat(cursorPath)
	qt.Check(t, qt.IsTrue(os.IsNotExist(err)))
}

func TestInstallRollsBackClaudeWhenCursorWriteFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not enforce directory mode bits")
	}

	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	cursorDir := filepath.Join(dir, "cursor")
	cursorPath := filepath.Join(cursorDir, "hooks.json")
	qt.Assert(t, qt.IsNil(os.Mkdir(cursorDir, 0o700)))
	qt.Assert(t, qt.IsNil(os.WriteFile(cursorPath, []byte(`{"version":1,"hooks":{"sessionStart":[]}}`), 0o600)))
	qt.Assert(t, qt.IsNil(os.Chmod(cursorDir, 0o500)))
	t.Cleanup(func() {
		_ = os.Chmod(cursorDir, 0o700)
	})
	probe := filepath.Join(cursorDir, "permission-probe")
	if err := os.WriteFile(probe, []byte("probe"), 0o600); err == nil {
		_ = os.Remove(probe)
		t.Skip("directory remains writable after chmod")
	}

	var out strings.Builder
	err := Install(Options{
		BinPath:         binPath,
		SettingsPath:    settingsPath,
		CursorHooksPath: cursorPath,
	}, false, &out)
	qt.Assert(t, qt.IsNotNil(err))
	entries := settingsPostToolUse(t, readFile(t, settingsPath))
	qt.Check(t, qt.HasLen(entries, 0))
}

func TestUnwireCursorWithoutEventOmitsEventKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hooks.json")
	existing := `{"version":1,"hooks":{"sessionStart":[{"command":"session-start","timeout":5}]}}`
	qt.Assert(t, qt.IsNil(os.WriteFile(path, []byte(existing), 0o600)))

	_, after, err := UnwireCursor(path, binPath)
	qt.Assert(t, qt.IsNil(err))
	qt.Assert(t, qt.IsNil(os.WriteFile(path, after, 0o600)))
	assertCursorHasOnlyOurHook(t, path, false)
}

func TestInstallWithoutCursorPathDoesNotTouchCursor(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	cursorPath := filepath.Join(dir, "hooks.json")
	original := []byte(`{"version":1,"hooks":{"sessionStart":[{"command":"session-start","timeout":5}]}}`)
	qt.Assert(t, qt.IsNil(os.WriteFile(cursorPath, original, 0o600)))

	var out strings.Builder
	qt.Assert(t, qt.IsNil(Install(Options{BinPath: binPath, SettingsPath: settingsPath}, false, &out)))
	qt.Check(t, qt.DeepEquals(readFile(t, cursorPath), original))
}

func assertCursorHasOnlyOurHook(t *testing.T, path string, installed bool) {
	t.Helper()
	var top map[string]json.RawMessage
	qt.Assert(t, qt.IsNil(json.Unmarshal(readFile(t, path), &top)))
	var hooks map[string]json.RawMessage
	qt.Assert(t, qt.IsNil(json.Unmarshal(top["hooks"], &hooks)))
	var sessionStart []cursorHookCommand
	qt.Assert(t, qt.IsNil(json.Unmarshal(hooks["sessionStart"], &sessionStart)))
	qt.Check(t, qt.HasLen(sessionStart, 1))
	if installed {
		var afterFileEdit []cursorHookCommand
		qt.Assert(t, qt.IsNil(json.Unmarshal(hooks[cursorEvent], &afterFileEdit)))
		qt.Assert(t, qt.HasLen(afterFileEdit, 1))
		qt.Check(t, qt.Equals(afterFileEdit[0].Command, binPath))
	} else {
		_, ok := hooks[cursorEvent]
		qt.Check(t, qt.IsFalse(ok))
	}
}

func assertCursorEventEmpty(t *testing.T, path string) {
	t.Helper()
	var top map[string]json.RawMessage
	qt.Assert(t, qt.IsNil(json.Unmarshal(readFile(t, path), &top)))
	var hooks map[string]json.RawMessage
	qt.Assert(t, qt.IsNil(json.Unmarshal(top["hooks"], &hooks)))
	raw, ok := hooks[cursorEvent]
	qt.Assert(t, qt.IsTrue(ok))
	qt.Check(t, qt.Equals(string(raw), "[]"))
}

func assertCursorHasNoVersion(t *testing.T, raw []byte) {
	t.Helper()
	var top map[string]json.RawMessage
	qt.Assert(t, qt.IsNil(json.Unmarshal(raw, &top)))
	_, ok := top["version"]
	qt.Check(t, qt.IsFalse(ok))
}

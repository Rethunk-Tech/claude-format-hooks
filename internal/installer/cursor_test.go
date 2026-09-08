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
	path := cursorFile(t, capturedCursorDoc)

	_, after, err := WireCursor(path, binPath)
	qt.Assert(t, qt.IsNil(err))

	version, ok := cursorVersion(t, after)
	qt.Assert(t, qt.IsTrue(ok))
	qt.Check(t, qt.Equals(version, 1))

	sessionStart := cursorEventHooks(t, after, "sessionStart")
	qt.Assert(t, qt.HasLen(sessionStart, 1))
	qt.Check(t, qt.Equals(sessionStart[0].Command, "session-start"))

	afterFileEdit := cursorEventHooks(t, after, cursorEvent)
	qt.Assert(t, qt.HasLen(afterFileEdit, 1))
	qt.Check(t, qt.Equals(afterFileEdit[0].Command, binPath))
	qt.Check(t, qt.Equals(afterFileEdit[0].Timeout, hookTimeout))
}

func TestWireCursorMissingFileCreatesVersionOne(t *testing.T) {
	path := cursorFile(t, "")

	_, after, err := WireCursor(path, binPath)
	qt.Assert(t, qt.IsNil(err))

	qt.Check(t, qt.DeepEquals(keysInOrder(t, after), []string{"hooks", "version"}))
	version, ok := cursorVersion(t, after)
	qt.Assert(t, qt.IsTrue(ok))
	qt.Check(t, qt.Equals(version, 1))
}

func TestVersionlessCursorFileStaysVersionless(t *testing.T) {
	path := cursorFile(t, `{"hooks":{"sessionStart":[{"command":"session-start","timeout":5}]}}`)

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

	sessionStart := cursorEventHooks(t, after, "sessionStart")
	qt.Check(t, qt.HasLen(sessionStart, 1))
	afterFileEdit := cursorEventHooks(t, after, cursorEvent)
	qt.Assert(t, qt.HasLen(afterFileEdit, 1))
	qt.Check(t, qt.Equals(afterFileEdit[0].Command, "format-docs"))
}

func TestUnwireCursorDoesNotMatchBinBasenameAlone(t *testing.T) {
	path := cursorFile(t, `{"hooks":{"afterFileEdit":[{"command":"/tmp/custom-hook","timeout":5}]}}`)

	_, after, err := UnwireCursor(path, "/opt/custom-hook")
	qt.Assert(t, qt.IsNil(err))
	entries := cursorEventHooks(t, after, cursorEvent)
	qt.Assert(t, qt.HasLen(entries, 1))
	qt.Check(t, qt.Equals(entries[0].Command, "/tmp/custom-hook"))
}

func TestInstallAndUninstallWithCursorHooks(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	cursorPath := filepath.Join(dir, "hooks.json")
	cursorBefore := capturedCursorDoc
	qt.Assert(t, qt.IsNil(os.WriteFile(cursorPath, []byte(cursorBefore), 0o600)))
	opts := Options{BinPath: binPath, SettingsPath: settingsPath, CursorHooksPath: cursorPath}

	var out strings.Builder
	qt.Assert(t, qt.IsNil(Install(opts, false, &out)))
	assertCursorHasOnlyOurHook(t, cursorPath, true)
	entries := settingsPostToolUse(t, readFile(t, settingsPath))
	qt.Assert(t, qt.HasLen(entries, 1))
	qt.Check(t, qt.Equals(entries[0].Hooks[0].Command, binPath))

	qt.Assert(t, qt.IsNil(Uninstall(opts, false, &out)))
	assertCursorHasOnlyOurHook(t, cursorPath, false)
	qt.Check(t, qt.Equals(settingsHasHooks(t, readFile(t, settingsPath)), false))
}

func TestUninstallMissingCursorLeavesFileMissing(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	cursorPath := filepath.Join(dir, "hooks.json")
	wireInstalled(t, settingsPath)

	var out strings.Builder
	opts := Options{BinPath: binPath, SettingsPath: settingsPath, CursorHooksPath: cursorPath}
	qt.Assert(t, qt.IsNil(Uninstall(opts, false, &out)))
	_, err := os.Stat(cursorPath)
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
	qt.Check(t, qt.Equals(settingsHasHooks(t, readFile(t, settingsPath)), false))
}

func TestUnwireCursorWithoutEventOmitsEventKey(t *testing.T) {
	path := cursorFile(t, capturedCursorDoc)

	_, after, err := UnwireCursor(path, binPath)
	qt.Assert(t, qt.IsNil(err))
	qt.Assert(t, qt.IsNil(os.WriteFile(path, after, 0o600)))
	assertCursorHasOnlyOurHook(t, path, false)
}

func TestUnwireCursorDropsAnEmptiedHooksContainer(t *testing.T) {
	for _, tc := range []struct{ name, doc string }{
		{"our hook was the only child", `{"version":1,"hooks":{"afterFileEdit":[{"command":"` + binPath + `","timeout":5}]}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := cursorFile(t, tc.doc)

			_, after, err := UnwireCursor(path, binPath)
			qt.Assert(t, qt.IsNil(err))
			var top map[string]json.RawMessage
			qt.Assert(t, qt.IsNil(json.Unmarshal(after, &top)))
			_, hasHooks := top["hooks"]
			qt.Check(t, qt.IsFalse(hasHooks), qt.Commentf("an emptied hooks container must be dropped"))

			version, ok := cursorVersion(t, after)
			qt.Assert(t, qt.IsTrue(ok))
			qt.Check(t, qt.Equals(version, 1))
		})
	}
}

func TestUnwireCursorKeepsPreexistingEmptyHooksObject(t *testing.T) {
	path := cursorFile(t, `{"version":1,"hooks":{}}`)

	_, after, err := UnwireCursor(path, binPath)
	qt.Assert(t, qt.IsNil(err))

	var top map[string]json.RawMessage
	qt.Assert(t, qt.IsNil(json.Unmarshal(after, &top)))
	hooksRaw, ok := top["hooks"]
	qt.Assert(t, qt.IsTrue(ok))
	var hooks map[string]json.RawMessage
	qt.Assert(t, qt.IsNil(json.Unmarshal(hooksRaw, &hooks)))
	qt.Assert(t, qt.HasLen(hooks, 0))
	var version int
	qt.Assert(t, qt.IsNil(json.Unmarshal(top["version"], &version)))
	qt.Check(t, qt.Equals(version, 1))
}

func TestInstallWithoutCursorPathDoesNotTouchCursor(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	cursorPath := filepath.Join(dir, "hooks.json")
	original := []byte(capturedCursorDoc)
	qt.Assert(t, qt.IsNil(os.WriteFile(cursorPath, original, 0o600)))

	var out strings.Builder
	qt.Assert(t, qt.IsNil(Install(Options{BinPath: binPath, SettingsPath: settingsPath}, false, &out)))
	qt.Check(t, qt.DeepEquals(readFile(t, cursorPath), original))
}

func assertCursorHasOnlyOurHook(t *testing.T, path string, installed bool) {
	t.Helper()
	raw := readFile(t, path)
	qt.Check(t, qt.HasLen(cursorEventHooks(t, raw, "sessionStart"), 1))

	afterFileEdit := cursorEventHooks(t, raw, cursorEvent)
	if !installed {
		qt.Check(t, qt.HasLen(afterFileEdit, 0), qt.Commentf("uninstall must leave no afterFileEdit hook"))
		return
	}
	qt.Assert(t, qt.HasLen(afterFileEdit, 1))
	qt.Check(t, qt.Equals(afterFileEdit[0].Command, binPath))
}

func assertCursorHasNoVersion(t *testing.T, raw []byte) {
	t.Helper()
	_, ok := cursorVersion(t, raw)
	qt.Check(t, qt.IsFalse(ok))
}

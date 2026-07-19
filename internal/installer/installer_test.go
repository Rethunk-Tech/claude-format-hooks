package installer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-quicktest/qt"
)

const binPath = "/home/user/.claude/hooks/format-dispatch"

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
	qt.Check(t, qt.Equals(entries[0].Matcher, "Write|Edit|NotebookEdit"))
	qt.Assert(t, qt.HasLen(entries[0].Hooks, 1))
	qt.Check(t, qt.Equals(entries[0].Hooks[0].Command, binPath))
	qt.Check(t, qt.Equals(entries[0].Hooks[0].Timeout, 30))
}

func TestWireMissingSettingsFile(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")

	_, after, err := Wire(settingsPath, binPath)
	qt.Assert(t, qt.IsNil(err))

	entries := settingsPostToolUse(t, after)
	qt.Assert(t, qt.HasLen(entries, 1))
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
	qt.Check(t, qt.Equals(entries[0].Matcher, "Write|Edit|NotebookEdit"))
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
				{"matcher": "Write|Edit|NotebookEdit", "hooks": [{"type": "command", "command": "` + binPath + `"}]}
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

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	qt.Assert(t, qt.IsNil(err))
	return raw
}

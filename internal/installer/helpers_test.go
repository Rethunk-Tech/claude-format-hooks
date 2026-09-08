package installer

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"

	"github.com/go-quicktest/qt"
)

// releaseConfig points upgradeWithConfig at a test release server serving
// this runtime's asset names.
func releaseConfig(server *httptest.Server) upgradeConfig {
	return upgradeConfig{
		client:     server.Client(),
		apiBaseURL: server.URL,
		goos:       runtime.GOOS,
		goarch:     runtime.GOARCH,
	}
}

// installedBinary writes content as the hook binary inside a fresh temp
// directory and returns its path.
func installedBinary(t *testing.T, content []byte, perm os.FileMode) string {
	t.Helper()
	target := HookBinaryPath(t.TempDir())
	qt.Assert(t, qt.IsNil(os.WriteFile(target, content, perm)))
	return target
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path) //nolint:gosec // path is a temp file the test wrote
	qt.Assert(t, qt.IsNil(err))
	return raw
}

// assertFileIs checks a file's whole content, naming what it was meant to
// prove when it does not match.
func assertFileIs(t *testing.T, path string, want []byte, why string) {
	t.Helper()
	qt.Check(t, qt.DeepEquals(readFile(t, path), want), qt.Commentf("%s", why))
}

// assertPerm checks a file's permission bits, skipping on Windows where
// they are not meaningful.
func assertPerm(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return
	}
	info, err := os.Stat(path)
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.Equals(info.Mode().Perm(), want))
}

// assertNoScratchLeftBehind checks the atomic replacement's temp sibling is
// gone, whichever way the upgrade ended.
func assertNoScratchLeftBehind(t *testing.T, target string) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Dir(target))
	qt.Assert(t, qt.IsNil(err))
	for _, entry := range entries {
		qt.Check(t, qt.Not(qt.Equals(filepath.Ext(entry.Name()), ".tmp")),
			qt.Commentf("leftover scratch file %q", entry.Name()))
	}
}

// assertRequests checks how many times the test release server was asked
// for one asset.
func assertRequests(t *testing.T, what string, counter *atomic.Int32, want int32) {
	t.Helper()
	qt.Check(t, qt.Equals(counter.Load(), want), qt.Commentf("%s requests", what))
}

// settingsFile returns a settings.json path inside a fresh temp directory,
// holding content -- or absent, when content is empty.
func settingsFile(t *testing.T, content string) string {
	t.Helper()
	return hookFile(t, "settings.json", content)
}

// cursorFile is settingsFile for Cursor's hooks.json.
func cursorFile(t *testing.T, content string) string {
	t.Helper()
	return hookFile(t, "hooks.json", content)
}

func hookFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if content != "" {
		qt.Assert(t, qt.IsNil(os.WriteFile(path, []byte(content), 0o600)))
	}
	return path
}

// settingsWithHook is a settings document whose only PostToolUse entry runs
// command.
func settingsWithHook(command string) string {
	return `{"hooks":{"PostToolUse":[{"matcher":"` + matcherAll +
		`","hooks":[{"type":"command","command":"` + command + `"}]}]}}`
}

// assertOnlyOurEntry checks a rendered document holds exactly one
// PostToolUse entry: ours, running command.
func assertOnlyOurEntry(t *testing.T, raw []byte, command string) {
	t.Helper()
	entries := settingsPostToolUse(t, raw)
	qt.Assert(t, qt.HasLen(entries, 1))
	qt.Check(t, qt.Equals(entries[0].Matcher, matcherAll))
	qt.Check(t, qt.Equals(entries[0].Hooks[0].Command, command))
}

// applyFixedChange runs applyChange over one before/after pair. The
// failure-path tests differ only in the filesystem they set up around it.
func applyFixedChange(settingsPath string, out io.Writer) error {
	return applyChange(
		Options{SettingsPath: settingsPath},
		[]byte(`{"before":true}`),
		[]byte(`{"after":true}`),
		false,
		out,
		"updated",
	)
}

// capturedCursorDoc is a real Cursor hooks.json holding one unrelated
// event, the shape these tests must leave untouched.
const capturedCursorDoc = `{"version":1,"hooks":{"sessionStart":[{"command":"session-start","timeout":5}]}}`

// cursorEventHooks decodes one event's hook list out of a rendered Cursor
// document, returning nil when the document has no such event.
func cursorEventHooks(t *testing.T, raw []byte, event string) []cursorHookCommand {
	t.Helper()
	var top map[string]json.RawMessage
	qt.Assert(t, qt.IsNil(json.Unmarshal(raw, &top)))
	var hooks map[string]json.RawMessage
	qt.Assert(t, qt.IsNil(json.Unmarshal(top["hooks"], &hooks)))
	rawEvent, ok := hooks[event]
	if !ok {
		return nil
	}
	var commands []cursorHookCommand
	qt.Assert(t, qt.IsNil(json.Unmarshal(rawEvent, &commands)))
	return commands
}

// cursorVersion returns the document's version and whether it carried one.
func cursorVersion(t *testing.T, raw []byte) (int, bool) {
	t.Helper()
	var top map[string]json.RawMessage
	qt.Assert(t, qt.IsNil(json.Unmarshal(raw, &top)))
	rawVersion, ok := top["version"]
	if !ok {
		return 0, false
	}
	var version int
	qt.Assert(t, qt.IsNil(json.Unmarshal(rawVersion, &version)))
	return version, true
}

// assertHooksGoneVersionKept checks an unwired Cursor document dropped its
// hooks container outright while keeping the version it came with.
func assertHooksGoneVersionKept(t *testing.T, raw []byte) {
	t.Helper()
	var top map[string]json.RawMessage
	qt.Assert(t, qt.IsNil(json.Unmarshal(raw, &top)))
	_, ok := top["hooks"]
	qt.Check(t, qt.IsFalse(ok), qt.Commentf("an emptied hooks container must be dropped"))

	version, ok := cursorVersion(t, raw)
	qt.Assert(t, qt.IsTrue(ok))
	qt.Check(t, qt.Equals(version, 1))
}

// wireInstalled wires the hook into settingsPath and persists the result --
// the starting state for anything testing a second pass over it.
func wireInstalled(t *testing.T, settingsPath string) []byte {
	t.Helper()
	_, wired, err := Wire(settingsPath, binPath)
	qt.Assert(t, qt.IsNil(err))
	qt.Assert(t, qt.IsNil(os.WriteFile(settingsPath, wired, 0o600)))
	return wired
}

// wireCursorInstalled is wireInstalled for Cursor's hooks.json.
func wireCursorInstalled(t *testing.T, cursorPath string) []byte {
	t.Helper()
	_, wired, err := WireCursor(cursorPath, binPath)
	qt.Assert(t, qt.IsNil(err))
	qt.Assert(t, qt.IsNil(os.WriteFile(cursorPath, wired, 0o600)))
	return wired
}

// settingsHooks decodes the hooks object out of a rendered settings document.
func settingsHooks(t *testing.T, raw []byte) map[string]json.RawMessage {
	t.Helper()
	var top map[string]json.RawMessage
	qt.Assert(t, qt.IsNil(json.Unmarshal(raw, &top)))
	var hooks map[string]json.RawMessage
	qt.Assert(t, qt.IsNil(json.Unmarshal(top["hooks"], &hooks)))
	return hooks
}

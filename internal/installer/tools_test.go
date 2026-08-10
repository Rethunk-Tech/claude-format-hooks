package installer

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/go-quicktest/qt"
)

// fakeBun puts a stub `bun` on PATH so provisioning can be exercised
// without touching the network or the operator's real global install.
// script is the body of the stub.
func fakeBun(t *testing.T, script string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake shell-script tools are POSIX-shell only")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "bun")
	qt.Assert(t, qt.IsNil(os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\n"), 0o755))) //nolint:gosec // test stub must be executable
	t.Setenv("PATH", dir)
}

// bunGlobalManifest points BUN_INSTALL at a temp tree and seeds the global
// manifest with content, returning the manifest path.
func bunGlobalManifest(t *testing.T, content string) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "install", "global")
	qt.Assert(t, qt.IsNil(os.MkdirAll(dir, 0o750)))
	path := filepath.Join(dir, "package.json")
	qt.Assert(t, qt.IsNil(os.WriteFile(path, []byte(content), 0o600)))
	t.Setenv("BUN_INSTALL", root)
	return path
}

func readManifest(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path) //nolint:gosec // path is the temp manifest this test wrote
	qt.Assert(t, qt.IsNil(err))
	var doc map[string]any
	qt.Assert(t, qt.IsNil(json.Unmarshal(raw, &doc)))
	return doc
}

func testBunGlobalOverrides(t *testing.T) {
	t.Helper()
	saved := bunGlobalOverrides
	bunGlobalOverrides = map[string]string{"test-dependency": "^1.2.3"}
	t.Cleanup(func() {
		bunGlobalOverrides = saved
	})
}

func TestProvisionToolsIsANoOpWithoutBun(t *testing.T) {
	// A machine with no bun is a supported configuration -- those formatters
	// are simply skipped at format time -- so --install must not complain.
	t.Setenv("PATH", t.TempDir())
	qt.Check(t, qt.IsNil(ProvisionTools(io.Discard)))
}

func TestProvisionToolsSurvivesAFailedInstall(t *testing.T) {
	// The settings.json wiring already succeeded by this point. A registry
	// outage must not turn that into a failed install.
	fakeBun(t, `echo "registry unreachable" >&2; exit 1`)
	qt.Check(t, qt.IsNil(ProvisionTools(io.Discard)))
}

func TestApplyBunGlobalOverridesPreservesExistingDependencies(t *testing.T) {
	// The global manifest is shared with whatever else the operator has
	// installed globally. Clobbering their dependencies to apply ours would
	// be a far worse bug than an override's intended dependency scope.
	testBunGlobalOverrides(t)
	fakeBun(t, "exit 0")
	manifest := bunGlobalManifest(t, `{"dependencies":{"some-other-tool":"^1.0.0"}}`)

	qt.Assert(t, qt.IsNil(applyBunGlobalOverrides(t.Context(), io.Discard)))

	doc := readManifest(t, manifest)
	deps, ok := doc["dependencies"].(map[string]any)
	qt.Assert(t, qt.IsTrue(ok))
	qt.Check(t, qt.Equals(deps["some-other-tool"], "^1.0.0"))

	overrides, ok := doc["overrides"].(map[string]any)
	qt.Assert(t, qt.IsTrue(ok))
	qt.Check(t, qt.Equals(overrides["test-dependency"], any(bunGlobalOverrides["test-dependency"])))
}

func TestApplyBunGlobalOverridesKeepsUnrelatedOverrides(t *testing.T) {
	testBunGlobalOverrides(t)
	fakeBun(t, "exit 0")
	manifest := bunGlobalManifest(t, `{"overrides":{"unrelated":"^2.0.0"}}`)

	qt.Assert(t, qt.IsNil(applyBunGlobalOverrides(t.Context(), io.Discard)))

	overrides, ok := readManifest(t, manifest)["overrides"].(map[string]any)
	qt.Assert(t, qt.IsTrue(ok))
	qt.Check(t, qt.Equals(overrides["unrelated"], "^2.0.0"))
	qt.Check(t, qt.Equals(overrides["test-dependency"], any(bunGlobalOverrides["test-dependency"])))
}

func TestApplyBunGlobalOverridesSkipsTheReinstallWhenAlreadyPinned(t *testing.T) {
	// Re-running --install is routine (it is how an operator picks up a new
	// binary), so the steady state must not pay for a reinstall every time.
	// The stub fails loudly to prove `bun install` was never invoked.
	testBunGlobalOverrides(t)
	fakeBun(t, `echo "bun install should not have run" >&2; exit 1`)
	pinned := `{"overrides":{"test-dependency":"` + bunGlobalOverrides["test-dependency"] + `"}}`
	bunGlobalManifest(t, pinned)

	qt.Check(t, qt.IsNil(applyBunGlobalOverrides(t.Context(), io.Discard)))
}

func TestApplyBunGlobalOverridesIsANoOpWhenEmpty(t *testing.T) {
	fakeBun(t, `echo "bun install should not have run" >&2; exit 1`)
	qt.Check(t, qt.IsNil(applyBunGlobalOverrides(t.Context(), io.Discard)))
}

func TestApplyBunGlobalOverridesReportsAMissingManifest(t *testing.T) {
	testBunGlobalOverrides(t)
	fakeBun(t, "exit 0")
	t.Setenv("BUN_INSTALL", t.TempDir())

	qt.Check(t, qt.IsNotNil(applyBunGlobalOverrides(t.Context(), io.Discard)))
}

func TestBunGlobalDir(t *testing.T) {
	t.Run("home fallback", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("BUN_INSTALL", "")
		t.Setenv("HOME", home)
		if runtime.GOOS == "windows" {
			t.Setenv("USERPROFILE", home)
		}

		got, err := bunGlobalDir()

		qt.Assert(t, qt.IsNil(err))
		qt.Check(t, qt.Equals(got, filepath.Join(home, ".bun", "install", "global")))
	})

	t.Run("missing home", func(t *testing.T) {
		keys := []string{"HOME", "USERPROFILE", "HOMEDRIVE", "HOMEPATH"}
		for _, key := range keys {
			t.Setenv(key, "")
		}

		t.Setenv("BUN_INSTALL", "")

		_, err := bunGlobalDir()

		qt.Check(t, qt.IsNotNil(err))
	})
}

func TestFirstLineTruncatesAtTheNewline(t *testing.T) {
	qt.Check(t, qt.Equals(string(firstLine([]byte("first\nsecond\nthird"))), "first"))
	qt.Check(t, qt.Equals(string(firstLine([]byte("only"))), "only"))
	qt.Check(t, qt.Equals(string(firstLine(nil)), ""))
}

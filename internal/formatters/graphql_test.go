package formatters

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/go-quicktest/qt"

	"github.com/Rethunk-Tech/claude-format-hooks/internal/config"
)

func TestGraphQLRouterName(t *testing.T) {
	qt.Check(t, qt.Equals(NewGraphQLRouter(config.Default()).Name(), "prettier"))
}

func TestGraphQLRouterUsesPrettierWithoutBiomeConfig(t *testing.T) {
	isolateDiskCache(t)
	dir := t.TempDir()
	marker := filepath.Join(dir, "marker")
	t.Setenv("GRAPHQL_MARKER", marker)
	writeGraphQLTools(t)

	abs := filepath.Join(dir, "schema.graphql")
	qt.Assert(t, qt.IsNil(os.WriteFile(abs, []byte("type Query { hello: String }"), 0o600)))

	res := NewGraphQLRouter(config.Default()).Format(t.Context(), dir, abs)
	qt.Assert(t, qt.IsNil(res.Err))
	qt.Check(t, qt.IsFalse(res.Skipped))

	got, err := os.ReadFile(marker)
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.Equals(string(got), "prettier"))
}

func TestGraphQLRouterUsesBiomeWhenConfigured(t *testing.T) {
	isolateDiskCache(t)
	dir := t.TempDir()
	marker := filepath.Join(dir, "marker")
	t.Setenv("GRAPHQL_MARKER", marker)
	writeFakeTool(t, "biome", `printf biome > "$GRAPHQL_MARKER"`)
	qt.Assert(t, qt.IsNil(os.WriteFile(filepath.Join(dir, "biome.json"), []byte("{}\n"), 0o600)))

	abs := filepath.Join(dir, "schema.gql")
	qt.Assert(t, qt.IsNil(os.WriteFile(abs, []byte("type Query { hello: String }"), 0o600)))

	res := NewGraphQLRouter(config.Default()).Format(t.Context(), dir, abs)
	qt.Assert(t, qt.IsNil(res.Err))
	qt.Check(t, qt.IsFalse(res.Skipped))

	got, err := os.ReadFile(marker)
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.Equals(string(got), "biome"))
}

func TestGraphQLRouterFallsBackWhenBiomeConfiguredButLaunchersMissing(t *testing.T) {
	isolateDiskCache(t)
	clearPath(t)
	dir := t.TempDir()
	qt.Assert(t, qt.IsNil(os.WriteFile(filepath.Join(dir, "biome.json"), []byte("{}\n"), 0o600)))

	abs := filepath.Join(dir, "schema.graphql")
	qt.Assert(t, qt.IsNil(os.WriteFile(abs, []byte("type Query { hello: String }"), 0o600)))

	res := NewGraphQLRouter(config.Default()).Format(t.Context(), dir, abs)
	qt.Check(t, qt.IsTrue(res.Skipped))
	qt.Check(t, qt.IsNil(res.Err))
}

func TestGraphQLRouterSkipsBiomeWhenDisabled(t *testing.T) {
	isolateDiskCache(t)
	dir := t.TempDir()
	marker := filepath.Join(dir, "marker")
	t.Setenv("GRAPHQL_MARKER", marker)
	writeGraphQLTools(t)
	qt.Assert(t, qt.IsNil(os.WriteFile(filepath.Join(dir, "biome.json"), []byte("{}\n"), 0o600)))

	cfg := config.Default()
	cfg.DisabledFormatters = []string{"BIOME"}
	abs := filepath.Join(dir, "schema.graphql")
	qt.Assert(t, qt.IsNil(os.WriteFile(abs, []byte("type Query { hello: String }"), 0o600)))

	res := NewGraphQLRouter(cfg).Format(t.Context(), dir, abs)
	qt.Assert(t, qt.IsNil(res.Err))
	qt.Check(t, qt.IsFalse(res.Skipped))

	got, err := os.ReadFile(marker)
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.Equals(string(got), "prettier"))
}

func writeGraphQLTools(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake shell-script tools are POSIX-shell only")
	}

	dir := t.TempDir()
	for _, name := range []string{"biome", "prettier"} {
		script := "#!/bin/sh\nprintf '" + name + "' > \"$GRAPHQL_MARKER\"\n"
		path := filepath.Join(dir, name)
		qt.Assert(t, qt.IsNil(os.WriteFile(path, []byte(script), 0o755))) //nolint:gosec // test fixture
	}
	t.Setenv("PATH", dir)
}

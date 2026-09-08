package main

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Rethunk-Tech/claude-format-hooks/internal/config"
	"github.com/Rethunk-Tech/claude-format-hooks/internal/dispatch"
	"github.com/go-quicktest/qt"
)

// unformattedJSON needs no external tool: JSON is formatted natively
// in-process, so these tests exercise --check without depending on bunx,
// sqlfluff, or anything else being installed on the runner.
const (
	unformattedJSON = "{\"a\":1,   \"b\":2}\n"
	formattedJSON   = "{\n  \"a\": 1,\n  \"b\": 2\n}\n"
)

// writeCheckFile is writeFile's dir+name form, returning the path so tests
// can assert on it. run_test.go's writeFile takes a full path.
func writeCheckFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	writeFile(t, path, content)
	return path
}

func TestCheckExitsZeroWhenEverythingIsFormatted(t *testing.T) {
	dir := t.TempDir()
	writeCheckFile(t, dir, "ok.json", formattedJSON)

	var out, errOut bytes.Buffer
	qt.Check(t, qt.Equals(runCheck([]string{dir}, &out, &errOut), 0))
}

func TestCheckExitsOneAndNamesTheFile(t *testing.T) {
	dir := t.TempDir()
	path := writeCheckFile(t, dir, "bad.json", unformattedJSON)

	var out, errOut bytes.Buffer
	qt.Assert(t, qt.Equals(runCheck([]string{dir}, &out, &errOut), 1))
	qt.Check(t, qt.StringContains(out.String(), path),
		qt.Commentf("the offending path must be named, or CI output is useless"))
}

func TestCheckNeverModifiesTheFilesItInspects(t *testing.T) {
	// The whole contract: a check that reformats what it inspects is not a
	// check. This is also why the copy is made beside the original rather
	// than the original being formatted and restored.
	dir := t.TempDir()
	path := writeCheckFile(t, dir, "bad.json", unformattedJSON)

	var out, errOut bytes.Buffer
	qt.Assert(t, qt.Equals(runCheck([]string{dir}, &out, &errOut), 1))

	after, err := os.ReadFile(path) //nolint:gosec // path is the temp file this test wrote
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.Equals(string(after), unformattedJSON))
}

func TestCheckLeavesNoScratchFilesBehind(t *testing.T) {
	// The scratch copy lands in the file's own directory (so config
	// discovery still works), which makes cleanup load-bearing -- a leaked
	// .fmtcheck-* file would show up in the operator's git status.
	dir := t.TempDir()
	writeCheckFile(t, dir, "bad.json", unformattedJSON)

	var out, errOut bytes.Buffer
	qt.Assert(t, qt.Equals(runCheck([]string{dir}, &out, &errOut), 1))

	entries, err := os.ReadDir(dir)
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.Equals(len(entries), 1))
}

func TestCheckAcceptsIndividualFiles(t *testing.T) {
	dir := t.TempDir()
	good := writeCheckFile(t, dir, "ok.json", formattedJSON)
	bad := writeCheckFile(t, dir, "bad.json", unformattedJSON)

	var out, errOut bytes.Buffer
	qt.Check(t, qt.Equals(runCheck([]string{good}, &out, &errOut), 0))

	out.Reset()
	qt.Check(t, qt.Equals(runCheck([]string{bad}, &out, &errOut), 1))
}

func TestWouldReformatComparesFormattedCopies(t *testing.T) {
	dir := t.TempDir()
	registry := dispatch.NewRegistry(config.Default())

	tests := []struct {
		name    string
		content string
		changed bool
	}{
		{name: "already formatted", content: formattedJSON},
		{name: "would change", content: unformattedJSON, changed: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeCheckFile(t, dir, tt.name+".json", tt.content)

			changed, err := wouldReformat(context.Background(), registry, dir, path, ".json")

			qt.Assert(t, qt.IsNil(err))
			qt.Check(t, qt.Equals(changed, tt.changed))
			qt.Check(t, qt.Equals(readFile(t, path), tt.content))
		})
	}
}

func TestWouldReformatReportsReadFileError(t *testing.T) {
	dir := t.TempDir()
	registry := dispatch.NewRegistry(config.Default())
	missing := filepath.Join(dir, "missing.json")

	changed, err := wouldReformat(context.Background(), registry, dir, missing, ".json")

	qt.Check(t, qt.Equals(changed, false))
	if err == nil {
		t.Fatal("wouldReformat must report a missing source file")
	}
}

func TestWouldReformatReturnsContextCauseAfterDispatch(t *testing.T) {
	dir := t.TempDir()
	path := writeCheckFile(t, dir, "bad.json", unformattedJSON)
	registry := dispatch.NewRegistry(config.Default())
	ctx, cancel := context.WithCancelCause(context.Background())
	cause := context.Canceled
	cancel(cause)

	changed, err := wouldReformat(ctx, registry, dir, path, ".json")

	qt.Check(t, qt.Equals(changed, false))
	qt.Check(t, qt.Equals(err, cause))
}

func TestWouldReformatReportsFormattedCopyReadError(t *testing.T) {
	if filepath.Separator == '\\' {
		t.Skip("fake formatter script is POSIX-shell only")
	}
	projectRoot := t.TempDir()
	path := writeCheckFile(t, projectRoot, "bad.ts", "const value=42\n")
	configPath := filepath.Join(t.TempDir(), "claude-format-hooks.json")
	writeFile(t, configPath, `{}`)
	toolDir := t.TempDir()
	script := "#!/bin/sh\nfor arg in \"$@\"; do\n  case \"$arg\" in\n    *.ts) rm -f \"$arg\"; exit 0 ;;\n  esac\ndone\nexit 1\n"
	for _, name := range []string{"biome", "bunx"} {
		tool := filepath.Join(toolDir, name)
		qt.Assert(t, qt.IsNil(os.WriteFile(tool, []byte(script), 0o755))) //nolint:gosec // test fixture
	}
	t.Setenv("CLAUDE_PROJECT_DIR", projectRoot)
	t.Setenv("CLAUDE_FORMAT_HOOKS_CONFIG", configPath)
	t.Setenv("CLAUDE_FORMAT_HOOKS_CACHE", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("PATH", toolDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	registry := dispatch.NewRegistry(config.Default())

	changed, err := wouldReformat(context.Background(), registry, projectRoot, path, ".ts")

	qt.Check(t, qt.Equals(changed, false))
	if err == nil {
		t.Fatal("wouldReformat must report a formatter that removes its scratch copy")
	}
}

func TestCopyBesideCreatesAndCleansSibling(t *testing.T) {
	dir := t.TempDir()
	abs := filepath.Join(dir, "target.json")
	content := []byte(unformattedJSON)

	path, cleanup, err := copyBeside(abs, content)

	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.Equals(filepath.Dir(path), dir))
	qt.Check(t, qt.IsTrue(strings.HasPrefix(filepath.Base(path), ".fmtcheck-")))
	qt.Check(t, qt.Equals(filepath.Ext(path), ".json"))
	qt.Check(t, qt.Equals(readFile(t, path), string(content)))

	cleanup()
	_, statErr := os.Stat(path)
	qt.Check(t, qt.IsTrue(os.IsNotExist(statErr)))
}

func TestCopyBesideReportsFileAsDirectoryWriteFailure(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "not-a-directory")
	qt.Assert(t, qt.IsNil(os.WriteFile(blocker, nil, 0o600))) //nolint:gosec // test fixture

	path, cleanup, err := copyBeside(filepath.Join(blocker, "target.json"), []byte(unformattedJSON))

	qt.Check(t, qt.Equals(path, ""))
	qt.Check(t, qt.IsNil(cleanup))
	if err == nil {
		t.Fatal("copyBeside must report a sibling write below a regular file")
	}
}

func TestCheckResolvesExtensionlessShellShebang(t *testing.T) {
	dir := t.TempDir()
	path := writeCheckFile(t, dir, "script", "#!/bin/sh\nif true;then echo hi;fi\n")

	var out, errOut bytes.Buffer
	qt.Check(t, qt.Equals(runCheck([]string{path}, &out, &errOut), 1))
	qt.Check(t, qt.StringContains(out.String(), path))
}

func TestCheckResolvesTerraformMultiDotExtensions(t *testing.T) {
	if filepath.Separator == '\\' {
		t.Skip("fake terraform script is POSIX-shell only")
	}
	projectRoot := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "claude-format-hooks.json")
	writeFile(t, configPath, `{}`)

	toolDir := t.TempDir()
	terraform := filepath.Join(toolDir, "terraform")
	script := "#!/bin/sh\nfor arg in \"$@\"; do\n  case \"$arg\" in\n    *.tftest.hcl|*.tfmock.hcl|*.tfquery.hcl) printf 'formatted\\n' > \"$arg\"; exit 0 ;;\n  esac\ndone\nexit 1\n"
	qt.Assert(t, qt.IsNil(os.WriteFile(terraform, []byte(script), 0o755))) //nolint:gosec // test fixture

	t.Setenv("CLAUDE_PROJECT_DIR", projectRoot)
	t.Setenv("CLAUDE_FORMAT_HOOKS_CONFIG", configPath)
	t.Setenv("PATH", toolDir)

	var paths []string
	for _, suffix := range []string{".tftest.hcl", ".tfmock.hcl", ".tfquery.hcl"} {
		paths = append(paths, writeCheckFile(t, projectRoot, "fixture"+suffix, "unformatted\n"))
	}

	var out, errOut bytes.Buffer
	qt.Check(t, qt.Equals(runCheck(paths, &out, &errOut), 1))
	for _, path := range paths {
		qt.Check(t, qt.StringContains(out.String(), path), qt.Commentf("path=%q", path))
	}
}

func TestCheckHonorsDisableConfig(t *testing.T) {
	const unformattedTS = "const value={answer:42}\n"
	const unformattedJSONC = `{"a":1}`

	tests := []struct {
		name          string
		files         map[string]string
		projectConfig string
		userConfig    string
		target        string
		checkRoot     bool
		wantExit      int
		wantReported  bool
	}{
		{
			name:          "project disables an extension",
			files:         map[string]string{"bad.json": unformattedJSON},
			projectConfig: `{"disabled": [".json"]}`,
			target:        "bad.json",
			checkRoot:     true,
		},
		{
			name:          "project disables a formatter by name",
			files:         map[string]string{"bad.ts": unformattedTS},
			projectConfig: `{"disabledFormatters":["BIOME"]}`,
			target:        "bad.ts",
		},
		{
			name:          "project disabling biome leaves the native JSON check on",
			files:         map[string]string{"bad.json": unformattedJSON, "biome.json": "{}\n"},
			projectConfig: `{"disabledFormatters":["biome"]}`,
			target:        "bad.json",
			wantExit:      1,
			wantReported:  true,
		},
		{
			name:          "project disabling biome skips JSONC entirely",
			files:         map[string]string{"bad.jsonc": unformattedJSONC},
			projectConfig: `{"disabledFormatters":["biome"]}`,
			target:        "bad.jsonc",
		},
		{
			name:       "user disables an extension",
			files:      map[string]string{"bad.json": unformattedJSON},
			userConfig: `{"disabled": [".json"]}`,
			target:     "bad.json",
			checkRoot:  true,
		},
		{
			name:       "user disables a formatter by name",
			files:      map[string]string{"bad.ts": unformattedTS},
			userConfig: `{"disabledFormatters":["biome"]}`,
			target:     "bad.ts",
			checkRoot:  true,
		},
		{
			name:         "user disabling biome leaves the native JSON check on",
			files:        map[string]string{"bad.json": unformattedJSON, "biome.json": "{}\n"},
			userConfig:   `{"disabledFormatters":["biome"]}`,
			target:       "bad.json",
			wantExit:     1,
			wantReported: true,
		},
		{
			name:       "user disables the json formatter",
			files:      map[string]string{"bad.json": unformattedJSON},
			userConfig: `{"disabledFormatters":["json"]}`,
			target:     "bad.json",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			projectRoot := t.TempDir()
			for name, content := range tc.files {
				writeCheckFile(t, projectRoot, name, content)
			}
			if tc.projectConfig != "" {
				writeCheckFile(t, projectRoot, projectConfigFile, tc.projectConfig)
			}
			userConfig := tc.userConfig
			if userConfig == "" {
				userConfig = `{}`
			}
			configPath := filepath.Join(t.TempDir(), "claude-format-hooks.json")
			writeFile(t, configPath, userConfig)

			t.Setenv("CLAUDE_PROJECT_DIR", projectRoot)
			t.Setenv("CLAUDE_FORMAT_HOOKS_CONFIG", configPath)

			target := filepath.Join(projectRoot, tc.target)
			arg := target
			if tc.checkRoot {
				arg = projectRoot
			}

			var out, errOut bytes.Buffer
			qt.Assert(t, qt.Equals(runCheck([]string{arg}, &out, &errOut), tc.wantExit))
			if tc.wantReported {
				qt.Check(t, qt.StringContains(out.String(), target))
				return
			}
			qt.Check(t, qt.Equals(readFile(t, target), tc.files[tc.target]),
				qt.Commentf("a disabled formatter must leave the file unreported and untouched"))
		})
	}
}

func TestCheckHonorsProjectConfigDisablesBiomeForGraphQLRouter(t *testing.T) {
	marker := fakeRouterTools(t)

	projectRoot := t.TempDir()
	writeCheckFile(t, projectRoot, "biome.json", "{}\n")
	writeCheckFile(t, projectRoot, projectConfigFile, `{"disabledFormatters":["biome"]}`)
	configPath := filepath.Join(t.TempDir(), "claude-format-hooks.json")
	writeFile(t, configPath, `{}`)
	t.Setenv("CLAUDE_PROJECT_DIR", projectRoot)
	t.Setenv("CLAUDE_FORMAT_HOOKS_CONFIG", configPath)

	for _, ext := range []string{".graphql", ".gql"} {
		t.Run(ext, func(t *testing.T) {
			path := writeCheckFile(t, projectRoot, "bad"+ext, "query { user { id } }\n")
			var out, errOut bytes.Buffer
			qt.Check(t, qt.Equals(runCheck([]string{path}, &out, &errOut), 0))
			qt.Check(t, qt.Equals(readFile(t, marker), "prettier"),
				qt.Commentf("project-disabled biome must route %s through prettier", ext))
		})
	}
}

func TestCheckReportsEveryJSONFileWhenProjectDisablesBiome(t *testing.T) {
	projectRoot := t.TempDir()
	firstPath := writeCheckFile(t, projectRoot, "first.json", unformattedJSON)
	secondPath := writeCheckFile(t, projectRoot, "second.json", unformattedJSON)
	writeCheckFile(t, projectRoot, "biome.json", "{}\n")
	writeCheckFile(t, projectRoot, projectConfigFile, `{"disabledFormatters":["biome"]}`)
	configPath := filepath.Join(t.TempDir(), "claude-format-hooks.json")
	writeFile(t, configPath, `{}`)

	t.Setenv("CLAUDE_PROJECT_DIR", projectRoot)
	t.Setenv("CLAUDE_FORMAT_HOOKS_CONFIG", configPath)

	var out, errOut bytes.Buffer
	qt.Check(t, qt.Equals(runCheck([]string{projectRoot}, &out, &errOut), 1))
	qt.Check(t, qt.StringContains(out.String(), firstPath))
	qt.Check(t, qt.StringContains(out.String(), secondPath))
}

func TestCheckUserDisableMixedWithEnabledExtension(t *testing.T) {
	// Both traversal orders must keep the disabled file silent while still
	// reporting the enabled native shell target — covers the nil-registry
	// to built-registry transition after the early IsDisabled skip.
	const unformattedShell = "#!/bin/sh\nif true;then echo hi;fi\n"

	for _, order := range []string{"disabled-first", "enabled-first"} {
		t.Run(order, func(t *testing.T) {
			projectRoot := t.TempDir()
			jsonPath := writeCheckFile(t, projectRoot, "bad.json", unformattedJSON)
			shellPath := writeCheckFile(t, projectRoot, "script.sh", unformattedShell)
			configPath := filepath.Join(t.TempDir(), "claude-format-hooks.json")
			writeFile(t, configPath, `{"disabled": [".json"]}`)

			t.Setenv("CLAUDE_PROJECT_DIR", projectRoot)
			t.Setenv("CLAUDE_FORMAT_HOOKS_CONFIG", configPath)

			paths := []string{jsonPath, shellPath}
			if order == "enabled-first" {
				paths = []string{shellPath, jsonPath}
			}

			var out, errOut bytes.Buffer
			qt.Check(t, qt.Equals(runCheck(paths, &out, &errOut), 1))
			qt.Check(t, qt.StringContains(out.String(), shellPath))
			qt.Check(t, qt.Equals(strings.Contains(out.String(), jsonPath), false),
				qt.Commentf("disabled json must not appear in --check output"))
			qt.Check(t, qt.Equals(readFile(t, jsonPath), unformattedJSON))
		})
	}
}

func TestCheckResolvesProjectRootPerPathArgument(t *testing.T) {
	firstRoot := t.TempDir()
	secondRoot := t.TempDir()
	firstPath := writeCheckFile(t, firstRoot, "bad.json", unformattedJSON)
	secondPath := writeCheckFile(t, secondRoot, "bad.json", unformattedJSON)
	writeCheckFile(t, firstRoot, projectConfigFile, `{"disabled": [".json"]}`)
	writeCheckFile(t, secondRoot, projectConfigFile, `{"disabled": [".json"]}`)
	configPath := filepath.Join(t.TempDir(), "claude-format-hooks.json")
	writeFile(t, configPath, `{}`)

	t.Setenv("CLAUDE_PROJECT_DIR", "")
	t.Setenv("CLAUDE_FORMAT_HOOKS_CONFIG", configPath)

	var out, errOut bytes.Buffer
	qt.Check(t, qt.Equals(runCheck([]string{firstRoot, secondRoot}, &out, &errOut), 0))
	qt.Check(t, qt.Equals(readFile(t, firstPath), unformattedJSON))
	qt.Check(t, qt.Equals(readFile(t, secondPath), unformattedJSON))
}

func TestCheckProjectConfigMalformedWarnsAndContinues(t *testing.T) {
	projectRoot := t.TempDir()
	path := writeCheckFile(t, projectRoot, "bad.json", unformattedJSON)
	writeCheckFile(t, projectRoot, projectConfigFile, "not valid json")
	configPath := filepath.Join(t.TempDir(), "claude-format-hooks.json")
	writeFile(t, configPath, `{}`)

	t.Setenv("CLAUDE_PROJECT_DIR", projectRoot)
	t.Setenv("CLAUDE_FORMAT_HOOKS_CONFIG", configPath)

	var out, errOut bytes.Buffer
	qt.Check(t, qt.Equals(runCheck([]string{projectRoot}, &out, &errOut), 1))
	qt.Check(t, qt.StringContains(errOut.String(), "project config:"))
	qt.Check(t, qt.StringContains(out.String(), path))
	qt.Check(t, qt.Equals(readFile(t, path), unformattedJSON))
}

func TestCheckProjectRoot(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*testing.T) ([]string, string, string)
	}{
		{
			name: "CLAUDE_PROJECT_DIR is absolute normalized",
			setup: func(t *testing.T) ([]string, string, string) {
				t.Helper()
				workingDir := t.TempDir()
				t.Chdir(workingDir)
				projectRoot := filepath.Join(workingDir, "project")
				qt.Assert(t, qt.IsNil(os.Mkdir(projectRoot, 0o750)))
				t.Setenv("CLAUDE_PROJECT_DIR", filepath.Join(".", "project"))
				abs := filepath.Join(projectRoot, "target.json")
				return nil, abs, projectRoot
			},
		},
		{
			name: "longest containing path wins and file paths use their directory",
			setup: func(t *testing.T) ([]string, string, string) {
				t.Helper()
				t.Setenv("CLAUDE_PROJECT_DIR", "")
				projectRoot := t.TempDir()
				nested := filepath.Join(projectRoot, "nested")
				qt.Assert(t, qt.IsNil(os.Mkdir(nested, 0o750)))
				abs := writeCheckFile(t, nested, "target.json", formattedJSON)
				return []string{projectRoot, abs}, abs, nested
			},
		},
		{
			name: "working directory wins when no path contains target",
			setup: func(t *testing.T) ([]string, string, string) {
				t.Helper()
				t.Setenv("CLAUDE_PROJECT_DIR", "")
				workingDir := t.TempDir()
				t.Chdir(workingDir)
				outside := t.TempDir()
				abs := filepath.Join(outside, "target.json")
				return nil, abs, workingDir
			},
		},
		{
			name: "directory fallback when working directory is unavailable",
			setup: func(t *testing.T) ([]string, string, string) {
				t.Helper()
				if filepath.Separator == '\\' {
					t.Skip("removing the current directory is not portable")
				}
				t.Setenv("CLAUDE_PROJECT_DIR", "")
				workingDir := t.TempDir()
				outside := t.TempDir()
				abs := filepath.Join(outside, "target.json")
				t.Chdir(workingDir)
				skipIfCwdSurvivesRemoval(t, workingDir)
				return nil, abs, filepath.Dir(abs)
			},
		},
		{
			name: "configured relative root is returned when working directory is unavailable",
			setup: func(t *testing.T) ([]string, string, string) {
				t.Helper()
				if filepath.Separator == '\\' {
					t.Skip("removing the current directory is not portable")
				}
				workingDir := t.TempDir()
				t.Chdir(workingDir)
				skipIfCwdSurvivesRemoval(t, workingDir)
				t.Setenv("CLAUDE_PROJECT_DIR", ".")
				abs := filepath.Join(t.TempDir(), "target.json")
				return nil, abs, "."
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			paths, abs, want := tc.setup(t)
			qt.Check(t, qt.Equals(checkProjectRoot(checkRootCandidates(paths), abs), want))
		})
	}
}

func TestCheckDispatchesNestedBiomeFromProjectRoot(t *testing.T) {
	if filepath.Separator == '\\' {
		t.Skip("fake formatter script is POSIX-shell only")
	}
	projectRoot := t.TempDir()
	path := writeCheckFile(t, filepath.Join(projectRoot, "nested"), "bad.ts", "const value={answer:42}\n")
	writeCheckFile(t, projectRoot, "biome.json", `{}`)
	configPath := filepath.Join(t.TempDir(), "claude-format-hooks.json")
	writeFile(t, configPath, `{}`)

	toolDir := t.TempDir()
	marker := filepath.Join(t.TempDir(), "formatter-cwd")
	script := "#!/bin/sh\nprintf '%s' \"$PWD\" > \"$FMTCHECK_MARKER\"\nfor arg in \"$@\"; do\n  case \"$arg\" in\n    *.ts) printf 'const value = { answer: 42 };\\n' > \"$arg\"; exit 0 ;;\n  esac\ndone\nexit 1\n"
	for _, name := range []string{"biome", "bunx"} {
		tool := filepath.Join(toolDir, name)
		qt.Assert(t, qt.IsNil(os.WriteFile(tool, []byte(script), 0o755))) //nolint:gosec // test fixture
	}

	t.Setenv("CLAUDE_PROJECT_DIR", projectRoot)
	t.Setenv("CLAUDE_FORMAT_HOOKS_CONFIG", configPath)
	t.Setenv("FMTCHECK_MARKER", marker)
	t.Setenv("PATH", toolDir)

	var out, errOut bytes.Buffer
	qt.Check(t, qt.Equals(runCheck([]string{projectRoot}, &out, &errOut), 1))
	qt.Check(t, qt.Equals(readFile(t, marker), projectRoot),
		qt.Commentf("nested checks must dispatch from the project root"))
	qt.Check(t, qt.StringContains(out.String(), path))

	hungPath := writeCheckFile(t, filepath.Join(projectRoot, "nested"), "hung.ts", "const value={answer:42}\n")
	writeFile(t, path, "const value = { answer: 42 };\n")
	timeoutScript := "#!/bin/sh\nprintf 'timeout:%s' \"$PWD\" > \"$FMTCHECK_MARKER\"\nexec /bin/sleep 30\n"
	for _, name := range []string{"biome", "bunx"} {
		tool := filepath.Join(toolDir, name)
		qt.Assert(t, qt.IsNil(os.WriteFile(tool, []byte(timeoutScript), 0o755))) //nolint:gosec // test fixture
	}
	var timeoutOut, timeoutErrOut bytes.Buffer
	started := time.Now()
	code := runCheck([]string{hungPath}, &timeoutOut, &timeoutErrOut)
	elapsed := time.Since(started)

	qt.Check(t, qt.Equals(code, 2))
	qt.Check(t, qt.IsTrue(elapsed < 10*time.Second),
		qt.Commentf("a single hung formatter must use the per-file timeout, not checkTimeout"))
	qt.Check(t, qt.Equals(readFile(t, marker), "timeout:"+projectRoot))
	qt.Check(t, qt.StringContains(timeoutErrOut.String(), hungPath))
	qt.Check(t, qt.StringContains(timeoutErrOut.String(), errFormatterTimeout.Error()))
}

func TestCollectCheckTargetsSkipsScratchFiles(t *testing.T) {
	dir := t.TempDir()
	keep := writeCheckFile(t, dir, "keep.json", formattedJSON)
	writeCheckFile(t, dir, ".fmtcheck-leftover-keep.json", formattedJSON)

	targets, err := collectCheckTargets([]string{dir})
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.DeepEquals(targets, []string{keep}))
}

func TestCheckIgnoresUnsupportedExtensions(t *testing.T) {
	dir := t.TempDir()
	writeCheckFile(t, dir, "notes.xyz", "whatever   \n")
	configPath := filepath.Join(t.TempDir(), "claude-format-hooks.json")
	writeFile(t, configPath, "not valid json")
	t.Setenv("CLAUDE_FORMAT_HOOKS_CONFIG", configPath)

	var out, errOut bytes.Buffer
	qt.Check(t, qt.Equals(runCheck([]string{dir}, &out, &errOut), 0))
	qt.Check(t, qt.Equals(errOut.String(), ""))
}

func TestCheckSkipsVendoredDirectories(t *testing.T) {
	// Formatting is never applied inside node_modules, so failing a build
	// over a dependency's formatting would be unfixable by design.
	dir := t.TempDir()
	vendored := filepath.Join(dir, "node_modules")
	qt.Assert(t, qt.IsNil(os.MkdirAll(vendored, 0o750)))
	writeCheckFile(t, vendored, "dep.json", unformattedJSON)

	var out, errOut bytes.Buffer
	qt.Check(t, qt.Equals(runCheck([]string{dir}, &out, &errOut), 0))
}

func TestCollectCheckTargetsSkipsDirectOutsideAndVendoredFiles(t *testing.T) {
	projectRoot := t.TempDir()
	outsideRoot := t.TempDir()
	vendored := writeCheckFile(t, filepath.Join(projectRoot, "node_modules", "pkg"), "dep.json", unformattedJSON)
	outside := writeCheckFile(t, outsideRoot, "outside.json", unformattedJSON)
	t.Setenv("CLAUDE_PROJECT_DIR", projectRoot)

	targets, err := collectCheckTargets([]string{vendored, outside, outsideRoot})
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.DeepEquals(targets, []string{}))
}

func TestCollectCheckTargetsSkipsVendoredDirWithoutAWorkingDirectory(t *testing.T) {
	if filepath.Separator == '\\' {
		t.Skip("removing the current directory is not portable")
	}
	t.Setenv("CLAUDE_PROJECT_DIR", "")
	outside := t.TempDir()
	vendored := filepath.Join(outside, "node_modules")
	writeFile(t, filepath.Join(vendored, "pkg", "f.json"), unformattedJSON)

	workingDir := t.TempDir()
	t.Chdir(workingDir)
	skipIfCwdSurvivesRemoval(t, workingDir)

	// With no project root and no usable cwd there is nothing to make the
	// path relative to, so the directory is judged on its own base name.
	got, err := collectCheckTargets([]string{vendored})
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.HasLen(got, 0), qt.Commentf("a vendored directory must be skipped even with no relative root"))
}

func TestCollectCheckTargetsSkipsVendoredDirectoryRoot(t *testing.T) {
	projectRoot := t.TempDir()
	vendored := filepath.Join(projectRoot, "node_modules")
	writeCheckFile(t, vendored, "dep.json", unformattedJSON)
	t.Setenv("CLAUDE_PROJECT_DIR", projectRoot)

	targets, err := collectCheckTargets([]string{vendored})

	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.DeepEquals(targets, []string{}))
}

func TestCollectCheckTargetsSkipsNewVendoredCacheDirectories(t *testing.T) {
	projectRoot := t.TempDir()
	keep := writeCheckFile(t, projectRoot, "keep.json", formattedJSON)
	for _, segment := range []string{
		".next",
		".yarn",
		".git",
		".agents",
		"dist",
		"build",
		"coverage",
		"test-results",
		"vendor",
		".venv",
		".terraform",
		"__pycache__",
		".ruff_cache",
		".mypy_cache",
		".pytest_cache",
		".tox",
	} {
		writeCheckFile(t, filepath.Join(projectRoot, segment), "ignored.json", unformattedJSON)
	}

	targets, err := collectCheckTargets([]string{projectRoot})
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.DeepEquals(targets, []string{keep}))
}

func TestCollectCheckTargetsSkipsScratchFilesDuringWalk(t *testing.T) {
	projectRoot := t.TempDir()
	nested := filepath.Join(projectRoot, "nested")
	keep := writeCheckFile(t, nested, "keep.json", formattedJSON)
	writeCheckFile(t, nested, ".fmtcheck-leftover.json", formattedJSON)
	t.Setenv("CLAUDE_PROJECT_DIR", projectRoot)

	targets, err := collectCheckTargets([]string{projectRoot})

	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.DeepEquals(targets, []string{keep}))
}

func TestCollectCheckTargetsReportsWalkError(t *testing.T) {
	if filepath.Separator == '\\' {
		t.Skip("directory permissions are not portable")
	}
	projectRoot := t.TempDir()
	unreadable := filepath.Join(projectRoot, "unreadable")
	writeCheckFile(t, unreadable, "hidden.json", formattedJSON)
	qt.Assert(t, qt.IsNil(os.Chmod(unreadable, 0o000))) //nolint:gosec // test fixture
	t.Cleanup(func() {
		_ = os.Chmod(unreadable, 0o750) //nolint:gosec // restore test fixture permissions
	})

	_, err := collectCheckTargets([]string{projectRoot})

	if err == nil {
		t.Skip("test process can read mode-000 directories")
	}
}

func TestCollectCheckTargetsReportsInjectedWalkError(t *testing.T) {
	projectRoot := t.TempDir()
	t.Setenv("CLAUDE_PROJECT_DIR", projectRoot)
	walkErr := errors.New("injected walk failure")
	originalWalkDir := checkWalkDir
	t.Cleanup(func() {
		checkWalkDir = originalWalkDir
	})
	checkWalkDir = func(_ string, _ fs.WalkDirFunc) error {
		return walkErr
	}

	_, err := collectCheckTargets([]string{projectRoot})

	qt.Check(t, qt.Equals(err, walkErr))
}

func TestCheckRejectsNoPaths(t *testing.T) {
	var out, errOut bytes.Buffer
	// 2, not 1: "you invoked me wrong" must be distinguishable from
	// "your files need formatting", or CI cannot tell a broken pipeline
	// from a legitimate failure.
	qt.Check(t, qt.Equals(runCheck(nil, &out, &errOut), 2))
}

func TestCheckReportsAMissingPath(t *testing.T) {
	var out, errOut bytes.Buffer
	missing := filepath.Join(t.TempDir(), "nope")
	qt.Check(t, qt.Equals(runCheck([]string{missing}, &out, &errOut), 2))
}

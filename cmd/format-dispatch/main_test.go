package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-quicktest/qt"

	"github.com/Rethunk-Tech/claude-format-hooks/internal/config"
)

func TestWithin(t *testing.T) {
	cases := []struct {
		name string
		abs  string
		root string
		want bool
	}{
		{"exact match", "/a/b", "/a/b", true},
		{"proper subpath", "/a/b/c.go", "/a/b", true},
		{"outside root", "/x/y", "/a/b", false},
		{
			"sibling dir sharing a string prefix is not within",
			"/a/b-evil/c.go", "/a/b", false,
		},
		{"trailing slash on root is normalized", "/a/b/c.go", "/a/b/", true},
		{"traversal above root is not within", "/a/b/../c", "/a/b", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			qt.Check(t, qt.Equals(within(tc.abs, tc.root), tc.want))
		})
	}
}

func TestWithinSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()

	t.Run("symlink inside root pointing outside is not within", func(t *testing.T) {
		target := filepath.Join(outside, "secret.txt")
		qt.Assert(t, qt.IsNil(os.WriteFile(target, []byte("x"), 0o600)))

		link := filepath.Join(root, "escape.txt")
		qt.Assert(t, qt.IsNil(os.Symlink(target, link)))

		qt.Check(t, qt.Equals(within(link, root), false))
	})

	t.Run("symlink whose target is still inside root passes", func(t *testing.T) {
		realSub := filepath.Join(root, "realsub")
		qt.Assert(t, qt.IsNil(os.Mkdir(realSub, 0o700)))
		file := filepath.Join(realSub, "file.txt")
		qt.Assert(t, qt.IsNil(os.WriteFile(file, []byte("x"), 0o600)))

		link := filepath.Join(root, "linksub")
		qt.Assert(t, qt.IsNil(os.Symlink(realSub, link)))

		qt.Check(t, qt.Equals(within(filepath.Join(link, "file.txt"), root), true))
	})
}

func TestResolveTarget(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*testing.T) (path, wantAbs, wantRoot, wantSkip string)
	}{
		{
			name: "missing file",
			setup: func(t *testing.T) (string, string, string, string) {
				t.Helper()
				t.Setenv("CLAUDE_PROJECT_DIR", "")
				path := filepath.Join(t.TempDir(), "missing.json")
				return path, path, "", "skip: stat failed or is a directory"
			},
		},
		{
			name: "directory",
			setup: func(t *testing.T) (string, string, string, string) {
				t.Helper()
				t.Setenv("CLAUDE_PROJECT_DIR", "")
				path := t.TempDir()
				return path, path, "", "skip: stat failed or is a directory"
			},
		},
		{
			name: "outside configured project root",
			setup: func(t *testing.T) (string, string, string, string) {
				t.Helper()
				projectRoot := t.TempDir()
				outside := filepath.Join(t.TempDir(), "outside.json")
				writeFile(t, outside, "content")
				t.Setenv("CLAUDE_PROJECT_DIR", projectRoot)
				return outside, outside, projectRoot, "skip: outside project root"
			},
		},
		{
			name: "outside working directory",
			setup: func(t *testing.T) (string, string, string, string) {
				t.Helper()
				t.Setenv("CLAUDE_PROJECT_DIR", "")
				workingDir := t.TempDir()
				t.Chdir(workingDir)
				outside := filepath.Join(t.TempDir(), "outside.json")
				writeFile(t, outside, "content")
				return outside, outside, workingDir, "skip: outside project root"
			},
		},
		{
			name: "vendored directory",
			setup: func(t *testing.T) (string, string, string, string) {
				t.Helper()
				projectRoot := t.TempDir()
				path := filepath.Join(projectRoot, "node_modules", "package", "file.json")
				writeFile(t, path, "content")
				t.Setenv("CLAUDE_PROJECT_DIR", projectRoot)
				return path, path, projectRoot, "skip: vendored directory"
			},
		},
		{
			name: "success resolves relative path against working directory",
			setup: func(t *testing.T) (string, string, string, string) {
				t.Helper()
				t.Setenv("CLAUDE_PROJECT_DIR", "")
				projectRoot := t.TempDir()
				t.Chdir(projectRoot)
				writeFile(t, filepath.Join(projectRoot, "target.json"), "content")
				path := "target.json"
				return path, filepath.Join(projectRoot, path), projectRoot, ""
			},
		},
		{
			name: "directory fallback when working directory is unavailable",
			setup: func(t *testing.T) (string, string, string, string) {
				t.Helper()
				if filepath.Separator == '\\' {
					t.Skip("removing the current directory is not portable")
				}
				t.Setenv("CLAUDE_PROJECT_DIR", "")
				workingDir := t.TempDir()
				outside := t.TempDir()
				path := filepath.Join(outside, "target.json")
				writeFile(t, path, "content")
				t.Chdir(workingDir)
				skipIfCwdSurvivesRemoval(t, workingDir)
				return path, path, outside, ""
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path, wantAbs, wantRoot, wantSkip := tc.setup(t)
			abs, projectRoot, skipReason := resolveTarget(path)
			qt.Check(t, qt.Equals(abs, wantAbs))
			qt.Check(t, qt.Equals(projectRoot, wantRoot))
			qt.Check(t, qt.Equals(skipReason, wantSkip))
		})
	}
}

func TestConfigPath(t *testing.T) {
	t.Run("env override wins", func(t *testing.T) {
		t.Setenv("CLAUDE_FORMAT_HOOKS_CONFIG", "/custom/path.json")
		qt.Check(t, qt.Equals(configPath(), "/custom/path.json"))
	})

	t.Run("defaults under the user's home directory", func(t *testing.T) {
		t.Setenv("CLAUDE_FORMAT_HOOKS_CONFIG", "")
		home, err := os.UserHomeDir()
		qt.Assert(t, qt.IsNil(err))
		want := filepath.Join(home, ".claude", "claude-format-hooks.json")
		qt.Check(t, qt.Equals(configPath(), want))
	})
}

func TestProjectRebuildsJSONRegistry(t *testing.T) {
	cases := []struct {
		name        string
		ext         string
		disabled    []string
		wantRebuild bool
	}{
		{"json with biome disabled", ".json", []string{"biome"}, true},
		{"json with trimmed biome disabled", ".json", []string{" biome "}, true},
		{"json with no disabled formatters", ".json", nil, false},
		{"uppercase json with biome disabled", ".JSON", []string{"biome"}, true},
		{"graphql with biome disabled", ".graphql", []string{"biome"}, true},
		{"gql with biome disabled", ".gql", []string{"biome"}, true},
		{"typescript with biome disabled", ".ts", []string{"biome"}, false},
		{"json with another formatter disabled", ".json", []string{"prettier"}, false},
		{"graphql with prettier disabled", ".graphql", []string{"prettier"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Config{DisabledFormatters: tc.disabled}
			qt.Check(t, qt.Equals(projectRebuildsJSONRegistry(tc.ext, cfg), tc.wantRebuild))
		})
	}
}

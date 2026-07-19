package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-quicktest/qt"
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

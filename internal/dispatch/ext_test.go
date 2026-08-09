package dispatch

import (
	"path/filepath"
	"testing"

	"github.com/go-quicktest/qt"
)

func TestResolveExtensionFallsBackToPathExtension(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"config.json", ".json"},
		{"config.JSON", ".JSON"},
		{"README", ""},
		{filepath.Join("nested.dir", "notes.unknown"), ".unknown"},
	}
	for _, tc := range cases {
		qt.Check(t, qt.Equals(ResolveExtension(tc.path), tc.want), qt.Commentf("path=%q", tc.path))
	}
}

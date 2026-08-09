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
		{"config.JSON", ".json"},
		{"README", ""},
		{filepath.Join("nested.dir", "notes.UNKNOWN"), ".UNKNOWN"},
	}
	for _, tc := range cases {
		qt.Check(t, qt.Equals(ResolveExtension(tc.path), tc.want), qt.Commentf("path=%q", tc.path))
	}
}

func TestResolveExtensionPrefersLongestRegisteredSuffix(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"fixture.tftest.hcl", ".tftest.hcl"},
		{"fixture.tfmock.hcl", ".tfmock.hcl"},
		{"fixture.TFQUERY.HCL", ".tfquery.hcl"},
		{"fixture.tf.json", ".json"},
	}
	for _, tc := range cases {
		qt.Check(t, qt.Equals(ResolveExtension(tc.path), tc.want), qt.Commentf("path=%q", tc.path))
	}
}

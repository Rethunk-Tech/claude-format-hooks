package dispatch

import (
	"path/filepath"
	"strings"
)

// ResolveExtension returns the registered extension with the longest matching
// suffix, or filepath.Ext when no registered multi-dot suffix matches.
func ResolveExtension(path string) string {
	base := strings.ToLower(filepath.Base(path))
	var match string
	for ext := range knownExtensions {
		if strings.HasSuffix(base, ext) && len(ext) > len(match) {
			match = ext
		}
	}
	if match != "" {
		return match
	}
	return filepath.Ext(path)
}

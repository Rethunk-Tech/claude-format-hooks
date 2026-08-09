package dispatch

import (
	"path/filepath"
	"strings"
)

// ResolveExtension returns the registered extension with the longest matching
// suffix, or filepath.Ext when no registered suffix matches.
func ResolveExtension(path string) string {
	base := strings.ToLower(filepath.Base(path))
	for _, ext := range registeredSuffixes {
		if strings.HasSuffix(base, ext) {
			return ext
		}
	}
	return filepath.Ext(path)
}

package installer

import (
	"path/filepath"
	"runtime"
)

// hookBinaryName is the executable's name without any platform suffix. It
// also prefixes the release asset names --upgrade downloads.
const hookBinaryName = "format-dispatch"

// HookBinaryBaseName returns the installed hook executable's platform-native
// basename.
func HookBinaryBaseName() string {
	return hookBinaryBaseName(runtime.GOOS)
}

func hookBinaryBaseName(goos string) string {
	if goos == "windows" {
		return hookBinaryName + ".exe"
	}
	return hookBinaryName
}

// HookBinaryPath returns the installed hook executable path under binDir.
func HookBinaryPath(binDir string) string {
	return filepath.Join(binDir, HookBinaryBaseName())
}

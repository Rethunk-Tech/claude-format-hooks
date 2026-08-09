package installer

import (
	"path/filepath"
	"runtime"
)

// HookBinaryBaseName returns the installed hook executable's platform-native
// basename.
func HookBinaryBaseName() string {
	return hookBinaryBaseName(runtime.GOOS)
}

func hookBinaryBaseName(goos string) string {
	if goos == "windows" {
		return "format-dispatch.exe"
	}
	return "format-dispatch"
}

// HookBinaryPath returns the installed hook executable path under binDir.
func HookBinaryPath(binDir string) string {
	return filepath.Join(binDir, HookBinaryBaseName())
}

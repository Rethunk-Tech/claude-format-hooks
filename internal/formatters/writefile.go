package formatters

import "os"

// writeFormatted writes out to abs, preserving abs's existing file mode
// (e.g. an executable bit on a shell script) rather than resetting it —
// falling back to defaultMode only when abs can't be stat'd (the file
// somehow vanished between the read and the write).
func writeFormatted(abs string, out []byte, defaultMode os.FileMode) error {
	mode := defaultMode
	if info, err := os.Stat(abs); err == nil {
		mode = info.Mode()
	}
	return os.WriteFile(abs, out, mode) //nolint:gosec // abs is the file this formatter was invoked to format, by design
}

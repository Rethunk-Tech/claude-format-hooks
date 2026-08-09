package formatters

import (
	"os"
	"path/filepath"
)

// writeFormatted writes out to abs, preserving abs's existing file mode
// (e.g. an executable bit on a shell script) rather than resetting it —
// falling back to defaultMode only when abs can't be stat'd (the file
// somehow vanished between the read and the write).
func writeFormatted(abs string, out []byte, defaultMode os.FileMode) error {
	mode := defaultMode
	if info, err := os.Stat(abs); err == nil {
		mode = info.Mode()
	}

	tmp, err := os.CreateTemp(filepath.Dir(abs), "."+filepath.Base(abs)+"-format-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()

	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(out); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	// The temporary file lives beside the destination, so rename is atomic
	// on the same filesystem and a partial write never reaches abs.
	return os.Rename(tmpName, abs) //nolint:gosec // abs is the file this formatter was invoked to format, by design
}

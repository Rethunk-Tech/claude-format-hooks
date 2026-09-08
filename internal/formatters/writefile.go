package formatters

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
)

// writeFormatted writes out to abs, preserving abs's existing file mode
// (e.g. an executable bit on a shell script) rather than resetting it —
// falling back to defaultMode only when abs can't be stat'd (the file
// somehow vanished between the read and the write).
func writeFormatted(abs string, source, out []byte, defaultMode os.FileMode) error {
	target, err := filepath.EvalSymlinks(abs)
	if err != nil {
		if info, lstatErr := os.Lstat(abs); lstatErr == nil && info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		if !os.IsNotExist(err) {
			return err
		}
		target = abs
	}

	mode := defaultMode
	if info, err := os.Stat(target); err == nil {
		mode = info.Mode()
	}

	tmp, err := os.CreateTemp(filepath.Dir(target), "."+filepath.Base(target)+"-format-*")
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
	// on the same filesystem and a partial write never reaches target.
	if source != nil {
		current, err := os.ReadFile(target)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if !bytes.Equal(current, source) {
			return nil
		}
	}
	return os.Rename(tmpName, target)
}

// writeIfChanged persists out when it differs from src, shaping the outcome
// as a Result. Every native formatter ends this way; only the mode differs.
func writeIfChanged(abs string, src, out []byte, perm os.FileMode) Result {
	if bytes.Equal(out, src) {
		return Result{}
	}
	if err := writeFormatted(abs, src, out, perm); err != nil {
		return Result{Err: fmt.Errorf("write: %w", err)}
	}
	return Result{}
}

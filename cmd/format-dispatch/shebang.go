package main

import (
	"os"
	"path/filepath"
	"strings"
)

const shebangPeekLimit = 256

func shebangExt(path string) (string, bool) {
	f, err := os.Open(path) //nolint:gosec // path is the hook/check target by design
	if err != nil {
		return "", false
	}
	defer func() { _ = f.Close() }()

	buf := make([]byte, shebangPeekLimit)
	n, err := f.Read(buf)
	if err != nil {
		return "", false
	}
	line, _, _ := strings.Cut(string(buf[:n]), "\n")
	line = strings.TrimRight(line, "\r")
	if !strings.HasPrefix(line, "#!") {
		return "", false
	}

	fields := strings.Fields(line[2:])
	if len(fields) == 0 {
		return "", false
	}
	interpreter := fields[0]
	if filepath.Base(interpreter) == "env" {
		if len(fields) < 2 {
			return "", false
		}
		interpreter = fields[1]
	}

	// python3.12 and similar version suffixes are the norm in shebangs, so
	// match on prefix for the interpreters that carry one.
	base := filepath.Base(interpreter)
	switch {
	case base == "bash", base == "sh", base == "zsh", base == "dash":
		return ".sh", true
	case strings.HasPrefix(base, "python"):
		return ".py", true
	case base == "ruby":
		return ".rb", true
	case base == "node", base == "bun":
		return ".js", true
	default:
		return "", false
	}
}

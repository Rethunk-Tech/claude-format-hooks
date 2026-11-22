package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/Rethunk-Tech/claude-format-hooks/internal/formatters"
)

const shebangPeekLimit = 256

func shebangExt(path string) (string, bool) {
	if !regularFile(path) {
		return "", false
	}
	f, err := os.Open(filepath.Clean(path))
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

	interpreter := shebangInterpreter(line)
	if interpreter == "" {
		return "", false
	}

	// python3.12 and php8.3 version suffixes are the norm in shebangs.
	// php-fpm / php-cgi are not language interpreters.
	base := filepath.Base(interpreter)
	switch base {
	case "bash", "sh", "zsh", "dash", "ksh":
		return ".sh", true
	case "ruby":
		return ".rb", true
	case "node", "bun":
		return ".js", true
	case "deno":
		return ".ts", true
	case "php":
		return ".php", true
	case "lua", "luajit":
		return ".lua", true
	default:
		if strings.HasPrefix(base, "python") {
			return ".py", true
		}
		if rest, ok := strings.CutPrefix(base, "php"); ok && len(rest) > 0 && rest[0] >= '0' && rest[0] <= '9' {
			return ".php", true
		}
		return "", false
	}
}

func shebangInterpreter(line string) string {
	return formatters.ShebangInterpreter(line)
}

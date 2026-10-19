package main

import (
	"os"
	"path/filepath"
	"strings"
)

const shebangPeekLimit = 256

func shebangExt(path string) (string, bool) {
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

// shebangInterpreter returns the program a #! line will exec. env's own
// flags (--split-string/-S, -u, -i, NAME=VALUE) sit between env and that
// program; treating the first token after env as the interpreter misses
// every `#!/usr/bin/env -S python3 -u` script.
func shebangInterpreter(line string) string {
	fields := strings.Fields(line[2:])
	if len(fields) == 0 {
		return ""
	}
	if filepath.Base(fields[0]) != "env" {
		return fields[0]
	}
	fields = fields[1:]
	for len(fields) > 0 {
		f := fields[0]
		if f == "-" {
			fields = fields[1:]
			continue
		}
		if strings.HasPrefix(f, "-") {
			name, val, eq := strings.Cut(f, "=")
			switch name {
			case "-S", "--split-string":
				fields = fields[1:]
				if eq {
					fields = append(strings.Fields(val), fields...)
				}
				if len(fields) == 0 {
					return ""
				}
				return fields[0]
			case "-u", "--unset", "-C", "--chdir":
				if eq {
					fields = fields[1:]
					continue
				}
				if len(fields) < 2 {
					return ""
				}
				fields = fields[2:]
				continue
			default:
				fields = fields[1:]
				continue
			}
		}
		if strings.Contains(f, "=") {
			fields = fields[1:]
			continue
		}
		return f
	}
	return ""
}

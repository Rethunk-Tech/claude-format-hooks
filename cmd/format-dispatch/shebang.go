package main

import (
	"os"
	"path/filepath"
	"strings"
)

const shebangPeekLimit = 256

func shellShebangExt(path string) (string, bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer func() { _ = f.Close() }()

	buf := make([]byte, shebangPeekLimit)
	n, err := f.Read(buf)
	if err != nil {
		return "", false
	}
	line := strings.SplitN(string(buf[:n]), "\n", 2)[0]
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

	switch filepath.Base(interpreter) {
	case "bash", "sh", "zsh", "dash":
		return ".sh", true
	default:
		return "", false
	}
}

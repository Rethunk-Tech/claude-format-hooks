package formatters

import (
	"context"
	"strings"
)

// notebookFormatter shells out to ruff or black for Jupyter notebooks.
// Ruff is preferred; black is a fallback for installations with notebook
// support enabled.
type notebookFormatter struct{}

// NewNotebook returns the notebookFormatter for .ipynb.
func NewNotebook() Formatter { return notebookFormatter{} }

func (notebookFormatter) Name() string { return "ruff/black-notebook" }

func (notebookFormatter) Format(ctx context.Context, projectRoot, abs string) Result {
	if _, err := lookPath("ruff"); err == nil {
		return runNotebookTool(ctx, projectRoot, "ruff", []string{"format", "--", abs})
	}
	if _, err := lookPath("black"); err == nil {
		ok, diagnostic, raw := runExternalOutput(ctx, projectRoot, "black", []string{"--quiet", "--", abs})
		if ok {
			return Result{}
		}
		if blackNotebookSupportMissing(diagnostic, raw) {
			return Result{Skipped: true}
		}
		return Result{Diagnostic: diagnostic}
	}
	return Result{Skipped: true}
}

func runNotebookTool(ctx context.Context, dir, name string, args []string) Result {
	ok, diagnostic := runExternal(ctx, dir, name, args)
	if !ok {
		return Result{Diagnostic: diagnostic}
	}
	return Result{}
}

func blackNotebookSupportMissing(diagnostic, raw string) bool {
	text := strings.ToLower(diagnostic + "\n" + raw)
	if strings.Contains(text, "black[jupyter]") {
		return true
	}

	const prefix = "no module named"
	for offset := 0; offset < len(text); {
		index := strings.Index(text[offset:], prefix)
		if index < 0 {
			return false
		}
		index += offset
		rest := text[index+len(prefix):]
		if len(rest) == 0 {
			return false
		}
		if isIdentifierByte(rest[0]) {
			offset = index + len(prefix)
			continue
		}
		for len(rest) > 0 && !isIdentifierByte(rest[0]) &&
			rest[0] != '\'' && rest[0] != '"' {
			rest = rest[1:]
		}
		if len(rest) == 0 {
			return false
		}
		if rest[0] == '\'' || rest[0] == '"' {
			quote := rest[0]
			for _, module := range []string{"nbformat", "jupyter"} {
				end := len(module) + 1
				if len(rest) > end && strings.HasPrefix(rest[1:], module) &&
					rest[end] == quote &&
					(end+1 == len(rest) || !isIdentifierByte(rest[end+1])) {
					return true
				}
			}
		} else {
			for _, module := range []string{"nbformat", "jupyter"} {
				if strings.HasPrefix(rest, module) {
					end := len(module)
					if end == len(rest) ||
						(!isIdentifierByte(rest[end]) && rest[end] != '.') {
						return true
					}
				}
			}
		}
		offset = index + len(prefix)
	}
	return false
}

func isIdentifierByte(value byte) bool {
	return value >= 'a' && value <= 'z' ||
		value >= '0' && value <= '9' ||
		value == '_'
}

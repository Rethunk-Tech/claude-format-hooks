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
	return strings.Contains(text, "jupyter") || strings.Contains(text, "notebook")
}

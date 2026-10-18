package formatters

import (
	"context"
	"path/filepath"
	"strings"
)

// notebookFormatter shells out to ruff or black for Jupyter notebooks.
// Ruff is preferred; black is a fallback for installations with notebook
// support enabled.
type notebookFormatter struct{}

// NewNotebook returns the notebookFormatter for .ipynb and .ipynb.example.
func NewNotebook() notebookFormatter { return notebookFormatter{} }

func (notebookFormatter) Name() string { return "ruff/black-notebook" }

func (notebookFormatter) Tools() []string { return []string{"ruff", "black"} }

func notebookExampleCopy(abs string) bool {
	return strings.HasSuffix(strings.ToLower(filepath.Base(abs)), ".ipynb.example")
}

func (notebookFormatter) Format(ctx context.Context, projectRoot, abs string) Result {
	named := ""
	if notebookExampleCopy(abs) {
		named = exampleStdinFilePath(abs)
	}
	if _, err := lookPath("ruff"); err == nil {
		if named != "" {
			return runExternalStdin(ctx, projectRoot, "ruff", []string{
				"format", "--stdin-filename", named, "-",
			}, abs)
		}
		return runExternalResult(ctx, projectRoot, "ruff", []string{"format", "--", abs})
	}
	if _, err := lookPath("black"); err == nil {
		if named != "" {
			return runExternalStdin(ctx, projectRoot, "black", []string{
				"--quiet", "--stdin-filename", named, "-",
			}, abs)
		}
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

// blackNotebookSupportMissingMarkers appear only when black cannot handle
// notebooks because its jupyter extra is absent. CPython always quotes the
// module in ModuleNotFoundError and black names the extra directly, so
// substring matching is enough -- the shape sqlfluff.go uses for its own
// parse-failure markers. Matching the quoted form is what keeps
// "no module named jupyterlab" from counting.
var blackNotebookSupportMissingMarkers = []string{
	"black[jupyter]",
	"no module named 'nbformat'",
	"no module named 'jupyter'",
}

func blackNotebookSupportMissing(diagnostic, raw string) bool {
	text := strings.ToLower(diagnostic + "\n" + raw)
	for _, marker := range blackNotebookSupportMissingMarkers {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

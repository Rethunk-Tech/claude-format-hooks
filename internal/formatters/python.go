package formatters

import (
	"context"
	"path/filepath"
	"strings"
)

// pythonFormatter shells out to ruff or black for .py, .pyi, and .py.example.
// Python has no Go equivalent, so this stays external. ruff format is
// preferred: it's a much faster, actively-developed drop-in for black's own
// formatting mode and has become the de facto standard; black is tried as a
// fallback for projects that only have it installed. Whichever is on PATH
// runs; if both are, ruff wins.
type pythonFormatter struct{}

// NewPython returns the pythonFormatter for .py, .pyi, and .py.example.
func NewPython() pythonFormatter { return pythonFormatter{} }

func (pythonFormatter) Name() string { return "ruff/black" }

func (pythonFormatter) Tools() []string { return []string{"ruff", "black"} }

func (pythonFormatter) Format(ctx context.Context, projectRoot, abs string) Result {
	// Black's include is *.py/*.pyi, so an in-place format of a path whose
	// final suffix is .example exits 0 having changed nothing. Stdin is
	// parsed as the language of --stdin-filename; dropping .example leaves
	// a .py name in the same directory. Ruff takes the same invocation.
	named := ""
	if pyExampleCopy(abs) {
		named = pyStdinFilePath(abs)
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
		return runExternalResult(ctx, projectRoot, "black", []string{"--quiet", "--", abs})
	}
	return Result{Skipped: true}
}

func pyExampleCopy(abs string) bool {
	return strings.HasSuffix(strings.ToLower(filepath.Base(abs)), ".py.example")
}

func pyStdinFilePath(abs string) string {
	base := filepath.Base(abs)
	trimmed := base[:len(base)-len(".example")]
	return filepath.Join(filepath.Dir(abs), trimmed)
}

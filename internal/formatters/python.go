package formatters

import "context"

// pythonFormatter shells out to ruff or black for .py — Python has no Go
// equivalent, so this stays external. ruff format is preferred: it's a
// much faster, actively-developed drop-in for black's own formatting
// mode and has become the de facto standard; black is tried as a
// fallback for projects that only have it installed. Whichever is on
// PATH runs; if both are, ruff wins.
type pythonFormatter struct{}

// NewPython returns the pythonFormatter for .py.
func NewPython() Formatter { return pythonFormatter{} }

func (pythonFormatter) Name() string { return "ruff/black" }

func (pythonFormatter) Format(ctx context.Context, projectRoot, abs string) Result {
	if _, err := lookPath("ruff"); err == nil {
		return runPythonTool(ctx, projectRoot, "ruff", []string{"format", "--", abs})
	}
	if _, err := lookPath("black"); err == nil {
		return runPythonTool(ctx, projectRoot, "black", []string{"--quiet", "--", abs})
	}
	return Result{Skipped: true}
}

func runPythonTool(ctx context.Context, dir, name string, args []string) Result {
	ok, diag := runExternal(ctx, dir, name, args)
	if !ok {
		return Result{Diagnostic: diag}
	}
	return Result{}
}

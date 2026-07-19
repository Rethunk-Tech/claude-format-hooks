package formatters

import (
	"bytes"
	"context"
	"fmt"
	"os"

	"mvdan.cc/sh/v3/syntax"

	"github.com/Rethunk-Tech/claude-format-hooks/internal/config"
)

// shellFormatter formats shell scripts in-process using mvdan.cc/sh/v3 —
// the same parser/printer package the shfmt binary itself is built on, so
// output matches `shfmt` exactly with no subprocess involved.
type shellFormatter struct{ cfg config.Config }

func NewShell(cfg config.Config) Formatter { return shellFormatter{cfg: cfg} }

func (shellFormatter) Name() string { return "shfmt" }

func (f shellFormatter) Format(_ context.Context, _, abs string) Result {
	src, err := os.ReadFile(abs) //nolint:gosec // abs is the file this formatter was invoked to format, by design
	if err != nil {
		return Result{Err: fmt.Errorf("read: %w", err)}
	}

	parser := syntax.NewParser(syntax.Variant(syntax.LangBash), syntax.KeepComments(true))
	file, err := parser.Parse(bytes.NewReader(src), abs)
	if err != nil {
		// Not valid shell syntax — not our job to fix a broken script.
		return Result{Skipped: true}
	}

	spec := config.ResolveShellIndent(f.cfg, abs)
	indentWidth := uint(max(spec.Size, 1))
	if spec.UseTabs {
		indentWidth = 0 // syntax.Indent(0) means "use tabs" per its own doc.
	}
	printer := syntax.NewPrinter(
		syntax.Indent(indentWidth),
		syntax.SwitchCaseIndent(f.cfg.Shell.SwitchCaseIndent),
	)
	var buf bytes.Buffer
	if err := printer.Print(&buf, file); err != nil {
		return Result{Err: fmt.Errorf("print: %w", err)}
	}

	out := buf.Bytes()
	if bytes.Equal(out, src) {
		return Result{}
	}

	info, statErr := os.Stat(abs)
	mode := os.FileMode(0o755)
	if statErr == nil {
		mode = info.Mode()
	}
	if err := os.WriteFile(abs, out, mode); err != nil {
		return Result{Err: fmt.Errorf("write: %w", err)}
	}
	return Result{}
}

package formatters

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"mvdan.cc/sh/v3/syntax"

	"github.com/Rethunk-Tech/claude-format-hooks/internal/config"
)

// shellFormatter formats shell scripts in-process using mvdan.cc/sh/v3 —
// the same parser/printer package the shfmt binary itself is built on, so
// output matches `shfmt` exactly with no subprocess involved.
type shellFormatter struct{ cfg config.Config }

// NewShell returns the native shellFormatter for .sh/.bash/.zsh/.ksh/.dash.
func NewShell(cfg config.Config) shellFormatter { return shellFormatter{cfg: cfg} }

func (shellFormatter) Name() string { return "shfmt" }

func (f shellFormatter) Format(_ context.Context, _, abs string) Result {
	src, err := os.ReadFile(filepath.Clean(abs))
	if err != nil {
		return Result{Err: fmt.Errorf("read: %w", err)}
	}

	parser := syntax.NewParser(syntax.Variant(dialect(abs, src)), syntax.KeepComments(true))
	file, err := parser.Parse(bytes.NewReader(src), abs)
	if err != nil {
		// Not valid shell syntax — not our job to fix a broken script.
		return Result{Skipped: true}
	}

	spec := config.ResolveShellIndent(f.cfg, abs)
	indentWidth := uint(clampedIndentWidth(spec.Size))
	if spec.UseTabs {
		indentWidth = 0 // syntax.Indent(0) means "use tabs" per its own doc.
	}
	printer := syntax.NewPrinter(
		syntax.Indent(indentWidth),
		syntax.SwitchCaseIndent(spec.CaseIndent),
	)
	var buf bytes.Buffer
	if err := printer.Print(&buf, file); err != nil {
		return Result{Err: fmt.Errorf("print: %w", err)}
	}

	out := buf.Bytes()
	return writeIfChanged(abs, src, out, 0o755)
}

// dialect picks the parser language from the extension, else the shebang;
// everything else stays bash.
func dialect(abs string, src []byte) syntax.LangVariant {
	if strings.HasSuffix(abs, ".zsh") || strings.HasSuffix(abs, ".zsh.example") {
		return syntax.LangZsh
	}
	if strings.HasSuffix(abs, ".ksh") || strings.HasSuffix(abs, ".ksh.example") {
		return syntax.LangMirBSDKorn
	}
	line, _, _ := bytes.Cut(src, []byte("\n"))
	if bytes.HasPrefix(line, []byte("#!")) {
		f := strings.Fields(string(line[2:]))
		if len(f) == 0 {
			return syntax.LangBash
		}
		interp := filepath.Base(f[len(f)-1]) // "#!/usr/bin/env zsh" names the shell last.
		switch interp {
		case "zsh":
			return syntax.LangZsh
		case "ksh", "mksh":
			return syntax.LangMirBSDKorn
		}
	}
	return syntax.LangBash
}

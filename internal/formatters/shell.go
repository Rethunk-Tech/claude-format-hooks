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
	// Dispatch already lowercases the extension; suffix checks must too or FILE.ZSH is bash.
	lower := strings.ToLower(abs)
	if strings.HasSuffix(lower, ".zsh") || strings.HasSuffix(lower, ".zsh.example") {
		return syntax.LangZsh
	}
	if strings.HasSuffix(lower, ".ksh") || strings.HasSuffix(lower, ".ksh.example") {
		return syntax.LangMirBSDKorn
	}
	line, _, _ := bytes.Cut(src, []byte("\n"))
	interp := filepath.Base(ShebangInterpreter(string(line)))
	switch interp {
	case "zsh":
		return syntax.LangZsh
	case "ksh", "mksh":
		return syntax.LangMirBSDKorn
	}
	return syntax.LangBash
}

// ShebangInterpreter returns the program a #! line will exec. env's own
// flags (--split-string/-S, -u, -i, NAME=VALUE) sit between env and that
// program; treating the first token after env as the interpreter misses
// every `#!/usr/bin/env -S python3 -u` script.
func ShebangInterpreter(line string) string {
	if !strings.HasPrefix(line, "#!") {
		return ""
	}
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

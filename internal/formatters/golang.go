package formatters

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"go/format"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// goFormatter reformats Go source. When gofumpt is on PATH it prints the
// formatted file to stdout (never -w) and the hook writes that result, so
// the bytes match what golangci-lint fmt would apply. goimports runs first
// only when it is also on PATH and a parent .golangci.yml lists it under
// formatters.enable. -extra=group_params is passed only when that same file
// sets formatters.settings.gofumpt.extra.group-params: true.
//
// Without gofumpt, formatting is go/format.Source, the engine gofmt uses.
// There is no Prober: a missing gofumpt is a fallback, not a skip, and
// --doctor would otherwise report the formatter as missing.
type goFormatter struct{}

// goFormatOpts is the slice of a parent .golangci.yml this formatter honors.
type goFormatOpts struct {
	goimports   bool
	groupParams bool
}

// NewGo returns the Go formatter for .go.
func NewGo() goFormatter { return goFormatter{} }

func (goFormatter) Name() string { return "gofmt" }

func (goFormatter) Format(ctx context.Context, _, abs string) Result {
	src, err := os.ReadFile(abs) //nolint:gosec // abs is the file this formatter was invoked to format, by design
	if err != nil {
		return Result{Err: fmt.Errorf("read: %w", err)}
	}

	// Same parser gofmt uses. A syntax error must stay a silent skip even
	// when an external tool would print the parse error on stderr.
	native, err := format.Source(src)
	if err != nil {
		return Result{Skipped: true}
	}

	if _, lookErr := lookPath("gofumpt"); lookErr != nil {
		return writeIfChanged(abs, src, native, 0o644)
	}

	out, diag := formatGoTools(ctx, abs, golangciGoFormat(abs))
	if diag != "" {
		return Result{Diagnostic: diag}
	}
	return writeIfChanged(abs, src, out, 0o644)
}

// formatGoTools runs goimports (when opts and PATH say so) and then gofumpt.
// gofumpt is last so its stricter rules are what lands on disk. Both tools
// print to stdout; the caller writes the file once.
func formatGoTools(ctx context.Context, abs string, opts goFormatOpts) ([]byte, string) {
	dir := filepath.Dir(abs)
	var piped []byte
	if opts.goimports {
		if _, err := lookPath("goimports"); err == nil {
			out, runErr := runToolStdout(ctx, dir, "goimports", []string{abs}, nil)
			if runErr != nil {
				return nil, toolDiagnostic(ctx, runErr)
			}
			if len(out) == 0 {
				return nil, "goimports produced no output"
			}
			piped = out
		}
	}

	args := make([]string, 0, 2)
	if opts.groupParams {
		args = append(args, "-extra=group_params")
	}
	if piped == nil {
		args = append(args, abs)
	}
	out, err := runToolStdout(ctx, dir, "gofumpt", args, piped)
	if err != nil {
		return nil, toolDiagnostic(ctx, err)
	}
	if len(out) == 0 {
		return nil, "gofumpt produced no output"
	}
	return out, ""
}

type toolRunError struct {
	err    error
	stderr []byte
}

func (e *toolRunError) Error() string { return e.err.Error() }

func (e *toolRunError) Unwrap() error { return e.err }

// runToolStdout runs name in dir and returns stdout. stdin is connected only
// when non-nil, so a tool that reads a path cannot consume the hook payload
// waiting on the process's inherited stdin.
func runToolStdout(ctx context.Context, dir, name string, args []string, stdin []byte) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // name is gofumpt or goimports; args are fixed flags plus the source path
	cmd.Dir = dir
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, &toolRunError{err: err, stderr: stderr.Bytes()}
	}
	return stdout.Bytes(), nil
}

func toolDiagnostic(ctx context.Context, err error) string {
	if ctx.Err() != nil {
		return context.Cause(ctx).Error()
	}
	var run *toolRunError
	if errors.As(err, &run) && len(run.stderr) > 0 {
		return truncate(run.stderr, 10, 500)
	}
	return err.Error()
}

// golangciGoFormat reads the nearest parent .golangci.yml of abs. A missing
// file, or one that does not mention these keys, leaves the zero value: no
// goimports, no group_params. The walk stops at the first config it can
// read, which is the file golangci-lint itself would use.
func golangciGoFormat(abs string) goFormatOpts {
	dir := filepath.Dir(abs)
	for {
		body, err := os.ReadFile(filepath.Join(dir, ".golangci.yml")) //nolint:gosec // dir is an ancestor of the source file being formatted
		if err == nil {
			return parseGolangciFormatters(body)
		}
		if !os.IsNotExist(err) {
			return goFormatOpts{}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return goFormatOpts{}
		}
		dir = parent
	}
}

type yamlFrame struct {
	indent int
	key    string
}

func parseGolangciFormatters(src []byte) goFormatOpts {
	var opts goFormatOpts
	var stack []yamlFrame
	for line := range strings.SplitSeq(string(src), "\n") {
		line = strings.TrimRight(line, "\r")
		line = stripYAMLComment(line)
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || trimmed == "---" || trimmed == "..." {
			continue
		}
		indent, ok := leadingSpaces(line)
		if !ok {
			continue
		}
		if strings.HasPrefix(trimmed, "- ") || trimmed == "-" {
			stack = popDeeper(stack, indent)
			if len(stack) == 0 || stack[len(stack)-1].indent != indent {
				stack = popAtOrDeeper(stack, indent)
			}
			item := strings.TrimSpace(strings.TrimPrefix(trimmed, "-"))
			if unquoteYAML(item) == "goimports" && inFormattersEnable(stack) {
				opts.goimports = true
			}
			continue
		}

		stack = popAtOrDeeper(stack, indent)
		key, val, isKey := splitYAMLKey(trimmed)
		if !isKey {
			continue
		}
		key = unquoteYAML(key)
		if val == "" {
			stack = append(stack, yamlFrame{indent: indent, key: key})
			continue
		}
		val = unquoteYAML(val)
		if key == "enable" && hasYAMLKey(stack, "formatters") && yamlListHas(val, "goimports") {
			opts.goimports = true
		}
		if key == "group-params" && underGofumptSettings(stack) && val == "true" {
			opts.groupParams = true
		}
	}
	return opts
}

func inFormattersEnable(stack []yamlFrame) bool {
	if len(stack) == 0 || stack[len(stack)-1].key != "enable" {
		return false
	}
	return hasYAMLKey(stack[:len(stack)-1], "formatters")
}

// underGofumptSettings reports whether the current line sits inside
// formatters.settings.gofumpt, in that order. A gofumpt block under linters
// is a different key and must not turn the flag on.
func underGofumptSettings(stack []yamlFrame) bool {
	stage := 0
	for _, frame := range stack {
		switch stage {
		case 0:
			if frame.key == "formatters" {
				stage = 1
			}
		case 1:
			if frame.key == "settings" {
				stage = 2
			}
		case 2:
			if frame.key == "gofumpt" {
				return true
			}
		}
	}
	return false
}

func hasYAMLKey(stack []yamlFrame, key string) bool {
	for _, frame := range stack {
		if frame.key == key {
			return true
		}
	}
	return false
}

func popDeeper(stack []yamlFrame, indent int) []yamlFrame {
	for len(stack) > 0 && stack[len(stack)-1].indent > indent {
		stack = stack[:len(stack)-1]
	}
	return stack
}

func popAtOrDeeper(stack []yamlFrame, indent int) []yamlFrame {
	for len(stack) > 0 && stack[len(stack)-1].indent >= indent {
		stack = stack[:len(stack)-1]
	}
	return stack
}

func leadingSpaces(line string) (int, bool) {
	n := 0
	for _, c := range line {
		switch c {
		case ' ':
			n++
		case '\t':
			return 0, false
		default:
			return n, true
		}
	}
	return n, true
}

func stripYAMLComment(line string) string {
	inSingle, inDouble := false, false
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case '#':
			if !inSingle && !inDouble {
				return line[:i]
			}
		}
	}
	return line
}

func splitYAMLKey(trimmed string) (string, string, bool) {
	key, val, ok := strings.Cut(trimmed, ":")
	if !ok || strings.TrimSpace(key) == "" {
		return "", "", false
	}
	return strings.TrimSpace(key), strings.TrimSpace(val), true
}

func unquoteYAML(s string) string {
	s = strings.TrimSpace(s)
	if len(s) < 2 {
		return s
	}
	if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
		return s[1 : len(s)-1]
	}
	return s
}

func yamlListHas(val, want string) bool {
	val = strings.TrimSpace(val)
	val = strings.TrimPrefix(val, "[")
	val = strings.TrimSuffix(val, "]")
	for part := range strings.SplitSeq(val, ",") {
		if unquoteYAML(part) == want {
			return true
		}
	}
	return false
}

package formatters

import (
	"context"
	"os"
	"os/exec"
)

// bunxFormatter runs a bunx-published CLI formatter against a single file
// from the project root, for tools with no practical native Go equivalent
// that preserves formatting fidelity (markdownlint-cli2's fix rules,
// taplo's TOML formatting, prettier's YAML/HTML formatting). Run from
// projectRoot so bunx resolves the repo's local devDependency version
// instead of fetching a fresh one from the registry each time.
type bunxFormatter struct {
	name string
	args func(abs string) []string
}

func NewMarkdown() Formatter {
	return bunxFormatter{name: "markdownlint-cli2", args: func(abs string) []string {
		return []string{"markdownlint-cli2", "--fix", "--", abs}
	}}
}

func NewTOML() Formatter {
	return bunxFormatter{name: "taplo", args: func(abs string) []string {
		return []string{"@taplo/cli", "format", "--", abs}
	}}
}

func NewPrettier() Formatter {
	return bunxFormatter{name: "prettier", args: func(abs string) []string {
		return []string{"prettier", "--write", "--", abs}
	}}
}

func (b bunxFormatter) Name() string { return b.name }

func (b bunxFormatter) Format(ctx context.Context, projectRoot, abs string) Result {
	if _, err := exec.LookPath("bunx"); err != nil {
		return Result{Skipped: true}
	}
	before, _ := os.ReadFile(abs) //nolint:gosec // abs is the file this formatter was invoked to format, by design
	ok, diag := runExternal(ctx, projectRoot, "bunx", b.args(abs))
	if !ok {
		return Result{Diagnostic: diag}
	}
	after, _ := os.ReadFile(abs) //nolint:gosec // abs is the file this formatter was invoked to format, by design
	return Result{Changed: string(before) != string(after)}
}

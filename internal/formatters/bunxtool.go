package formatters

import (
	"context"
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

// NewMarkdown returns the bunxFormatter for .md/.mdx, via markdownlint-cli2.
func NewMarkdown() Formatter {
	return bunxFormatter{name: "markdownlint-cli2", args: func(abs string) []string {
		return []string{"markdownlint-cli2", "--fix", "--", abs}
	}}
}

// NewTOML returns the bunxFormatter for .toml, via taplo.
func NewTOML() Formatter {
	return bunxFormatter{name: "taplo", args: func(abs string) []string {
		return []string{"@taplo/cli", "format", "--", abs}
	}}
}

// NewPrettier returns the bunxFormatter for .yaml/.yml/.html, via prettier.
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
	ok, diag := runExternal(ctx, projectRoot, "bunx", b.args(abs))
	if !ok {
		return Result{Diagnostic: diag}
	}
	return Result{}
}

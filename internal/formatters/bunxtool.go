package formatters

import "context"

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
//
// Passes the user-level base config (see markdownconfig.go) when one can be
// resolved, so a project with no markdownlint config of its own gets sane
// defaults instead of stock rules that fire on ordinary technical writing.
// A project's own config still layers over it and wins.
func NewMarkdown() Formatter {
	return bunxFormatter{name: "markdownlint-cli2", args: func(abs string) []string {
		args := []string{"markdownlint-cli2"}
		if cfg := userMarkdownlintConfig(); cfg != "" {
			args = append(args, "--config", cfg)
		}
		return append(args, "--fix", "--", abs)
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
	if _, err := lookPath("bunx"); err != nil {
		return Result{Skipped: true}
	}
	ok, diag := runExternal(ctx, projectRoot, "bunx", b.args(abs))
	if !ok {
		return Result{Diagnostic: diag}
	}
	return Result{}
}

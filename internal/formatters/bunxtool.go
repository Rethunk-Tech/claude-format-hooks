package formatters

import "context"

// bunxFormatter runs a bunx-published CLI formatter against a single file,
// preferring a provisioned CLI binary and falling back to bunx, for tools with
// no practical native Go equivalent that preserves formatting fidelity
// (markdownlint-cli2's fix rules, taplo's TOML formatting, prettier's YAML/HTML
// formatting). Run from projectRoot so bunx resolves the repo's local
// devDependency version instead of fetching a fresh one from the registry each
// time.
type bunxFormatter struct {
	name     string
	pathArgs func(abs string) []string
	bunxArgs func(abs string) []string
}

// NewMarkdown returns the bunxFormatter for .md/.mdx, via markdownlint-cli2.
//
// Passes the user-level base config (see markdownconfig.go) when one can be
// resolved, so a project with no markdownlint config of its own gets sane
// defaults instead of stock rules that fire on ordinary technical writing.
// A project's own config still layers over it and wins.
func NewMarkdown() Formatter {
	args := func(abs string) []string {
		args := []string{}
		if cfg := userMarkdownlintConfig(); cfg != "" {
			args = append(args, "--config", cfg)
		}
		return append(args, "--fix", "--", abs)
	}
	return bunxFormatter{
		name:     "markdownlint-cli2",
		pathArgs: args,
		bunxArgs: func(abs string) []string {
			return append([]string{"markdownlint-cli2"}, args(abs)...)
		},
	}
}

// NewTOML returns the bunxFormatter for .toml, via taplo.
func NewTOML() Formatter {
	return bunxFormatter{
		name:     "taplo",
		pathArgs: func(abs string) []string { return []string{"format", "--", abs} },
		bunxArgs: func(abs string) []string { return []string{"@taplo/cli", "format", "--", abs} },
	}
}

// NewPrettier returns the bunxFormatter for .yaml/.yml/.html, via prettier.
func NewPrettier() Formatter {
	return bunxFormatter{
		name:     "prettier",
		pathArgs: func(abs string) []string { return []string{"--write", "--", abs} },
		bunxArgs: func(abs string) []string { return []string{"prettier", "--write", "--", abs} },
	}
}

func (b bunxFormatter) Name() string { return b.name }

func (b bunxFormatter) Format(ctx context.Context, projectRoot, abs string) Result {
	return runPathOrBunx(ctx, projectRoot, b.name, b.pathArgs(abs), b.bunxArgs(abs))
}

func runPathOrBunx(ctx context.Context, cwd, pathName string, pathArgs, bunxArgs []string) Result {
	command, args := "bunx", bunxArgs
	if path, err := lookPath(pathName); err == nil {
		command, args = path, pathArgs
	} else if _, err := lookPath("bunx"); err != nil {
		return Result{Skipped: true}
	}

	ok, diag := runExternal(ctx, cwd, command, args)
	if !ok {
		return Result{Diagnostic: diag}
	}
	return Result{}
}

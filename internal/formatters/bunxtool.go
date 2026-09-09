package formatters

import (
	"context"
	"strings"
)

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
	// skipIf reclassifies a failure diagnostic as a skip. Set only where a
	// tool's failure can mean "this file isn't mine" rather than "this file
	// is broken".
	skipIf func(diagnostic string) bool
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

// prettierNoParserMarker appears when prettier recognizes the file but has
// no parser for it. .svelte and .astro are formatted by prettier plugins
// the project installs itself; without one, prettier fails on every write.
// That is "not my file", not a broken file, so it skips rather than
// reporting a diagnostic the operator cannot act on from here.
const prettierNoParserMarker = "No parser could be inferred"

// NewPrettier returns the bunxFormatter for .yaml/.yml/.html/.vue and the
// plugin-backed .svelte/.astro, via prettier.
func NewPrettier() Formatter {
	return bunxFormatter{
		name:     "prettier",
		pathArgs: func(abs string) []string { return []string{"--write", "--", abs} },
		bunxArgs: func(abs string) []string { return []string{"prettier", "--write", "--", abs} },
		skipIf: func(diagnostic string) bool {
			return strings.Contains(diagnostic, prettierNoParserMarker)
		},
	}
}

func (b bunxFormatter) Name() string { return b.name }

// Tools reports the provisioned CLI and the bunx fallback Format prefers
// between, in that order.
func (b bunxFormatter) Tools() []string { return []string{b.name, "bunx"} }

func (b bunxFormatter) Format(ctx context.Context, projectRoot, abs string) Result {
	res := runPathOrBunx(ctx, projectRoot, b.name, b.pathArgs(abs), b.bunxArgs(abs))
	if b.skipIf != nil && res.Diagnostic != "" && b.skipIf(res.Diagnostic) {
		return Result{Skipped: true}
	}
	return res
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

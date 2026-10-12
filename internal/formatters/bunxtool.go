package formatters

import (
	"context"
	"path/filepath"
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
	// stdinFromFile reports paths the tool will not rewrite in place. The
	// file is fed on stdin and stdout is written back.
	stdinFromFile func(abs string) bool
}

// NewMarkdown returns the bunxFormatter for .md/.mdx, via markdownlint-cli2.
//
// Passes the user-level base config (see markdownconfig.go) when one can be
// resolved, so a project with no markdownlint config of its own gets sane
// defaults instead of stock rules that fire on ordinary technical writing.
// A project's own config still layers over it and wins.
func NewMarkdown() bunxFormatter {
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

// NewTOML returns the bunxFormatter for .toml and .toml.example, via taplo.
func NewTOML() bunxFormatter {
	return bunxFormatter{
		name:     "taplo",
		pathArgs: tomlFormatArgs,
		bunxArgs: func(abs string) []string {
			return append([]string{"@taplo/cli"}, tomlFormatArgs(abs)...)
		},
		stdinFromFile: tomlExampleCopy,
	}
}

// tomlFormatArgs is taplo's format invocation. Taplo's default include is
// **/*.toml, so a copy whose final suffix is .example is collected and then
// excluded, and the command exits 0 having changed nothing. Stdin is parsed
// as TOML regardless of that suffix; --stdin-filepath keeps path-scoped
// format rules pointed at the real file.
func tomlFormatArgs(abs string) []string {
	if tomlExampleCopy(abs) {
		return []string{"format", "--stdin-filepath", abs, "-"}
	}
	return []string{"format", "--", abs}
}

func tomlExampleCopy(abs string) bool {
	return strings.HasSuffix(strings.ToLower(filepath.Base(abs)), ".toml.example")
}

// prettierNoParserMarker appears when prettier recognizes the file but has
// no parser for it. .svelte and .astro are formatted by prettier plugins
// the project installs itself; without one, prettier fails on every write.
// That is "not my file", not a broken file, so it skips rather than
// reporting a diagnostic the operator cannot act on from here.
const prettierNoParserMarker = "No parser could be inferred"

// NewPrettier returns the bunxFormatter for .yaml/.yml (including the
// .yaml.example/.yml.example copies), .html/.vue, and the plugin-backed
// .svelte/.astro, via prettier.
func NewPrettier() bunxFormatter {
	return bunxFormatter{
		name:     "prettier",
		pathArgs: prettierWriteArgs,
		bunxArgs: func(abs string) []string {
			return append([]string{"prettier"}, prettierWriteArgs(abs)...)
		},
		skipIf: func(diagnostic string) bool {
			return strings.Contains(diagnostic, prettierNoParserMarker)
		},
	}
}

// prettierWriteArgs is prettier's --write invocation. Prettier picks a parser
// from the final suffix, and .example is not one it knows, so a YAML copy
// has to name the parser or every write of it is reported as "no parser".
func prettierWriteArgs(abs string) []string {
	args := []string{"--write"}
	base := strings.ToLower(filepath.Base(abs))
	if strings.HasSuffix(base, ".yaml.example") || strings.HasSuffix(base, ".yml.example") {
		args = append(args, "--parser", "yaml")
	}
	return append(args, "--", abs)
}

func (b bunxFormatter) Name() string { return b.name }

// Tools reports the provisioned CLI and the bunx fallback Format prefers
// between, in that order.
func (b bunxFormatter) Tools() []string { return []string{b.name, "bunx"} }

func (b bunxFormatter) Format(ctx context.Context, projectRoot, abs string) Result {
	stdinPath := ""
	if b.stdinFromFile != nil && b.stdinFromFile(abs) {
		stdinPath = abs
	}
	res := runPathOrBunx(ctx, projectRoot, b.name, b.pathArgs(abs), b.bunxArgs(abs), stdinPath)
	if b.skipIf != nil && res.Diagnostic != "" && b.skipIf(res.Diagnostic) {
		return Result{Skipped: true}
	}
	return res
}

func runPathOrBunx(ctx context.Context, cwd, pathName string, pathArgs, bunxArgs []string, stdinPath string) Result {
	command, args := "bunx", bunxArgs
	if path, err := lookPath(pathName); err == nil {
		command, args = path, pathArgs
	} else if _, err := lookPath("bunx"); err != nil {
		return Result{Skipped: true}
	}
	if stdinPath != "" {
		return runExternalStdin(ctx, cwd, command, args, stdinPath)
	}

	ok, diag := runExternal(ctx, cwd, command, args)
	if !ok {
		return Result{Diagnostic: diag}
	}
	return Result{}
}

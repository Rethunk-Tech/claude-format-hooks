package formatters

import (
	"context"
	"path/filepath"
	"strings"
)

// systemFormatter runs the first available of one or more system binaries
// against a single file, skipping when none resolve. Every language whose
// canonical formatter is a plain PATH binary with an in-place flag reduces
// to this. The four that predate it -- rustfmt, buf, terraform, ruff/black
// -- keep their own files rather than being refactored into it.
type systemFormatter struct {
	name string
	// bins are candidate binaries in preference order; the first found runs.
	bins []string
	// args builds the argv tail for the binary that was found, since
	// alternatives for the same language rarely share a flag spelling.
	args func(bin, abs string) []string
	// onlyBase, when set, is the only basename this formatter may rewrite.
	// go mod edit and go work edit rewrite the path they are given, and
	// with no path they rewrite the module or workspace file found from
	// the working directory, so the name check is what keeps them off
	// every other file. A --check sibling copy
	// (.fmtcheck-<8 hex>-<base>) still counts as that basename.
	onlyBase string
}

func (s systemFormatter) Name() string { return s.name }

func (s systemFormatter) Tools() []string { return s.bins }

func (s systemFormatter) Format(ctx context.Context, projectRoot, abs string) Result {
	if s.onlyBase != "" && originalBase(abs) != s.onlyBase {
		return Result{Skipped: true}
	}
	for _, bin := range s.bins {
		if _, err := lookPath(bin); err != nil {
			continue
		}
		return runExternalResult(ctx, projectRoot, bin, s.args(bin, abs))
	}
	return Result{Skipped: true}
}

// originalBase is the file's basename. --check formats a sibling named
// .fmtcheck-<8 hex>-<original> (cmd/format-dispatch copyBeside) so the
// tool still sees the extension; that copy has to count as the original
// name or the check reports the tool as missing and never compares bytes.
func originalBase(abs string) string {
	base := filepath.Base(abs)
	rest, ok := strings.CutPrefix(base, ".fmtcheck-")
	if !ok {
		return base
	}
	hexPart, orig, ok := strings.Cut(rest, "-")
	if !ok || len(hexPart) != 8 || !isLowerHex(hexPart) {
		return base
	}
	return orig
}

func isLowerHex(s string) bool {
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f':
		default:
			return false
		}
	}
	return true
}

// inPlace is the argv shape shared by every formatter here that takes one
// in-place flag followed by the file.
func inPlace(flags ...string) func(bin, abs string) []string {
	return func(_, abs string) []string { return append(append([]string{}, flags...), abs) }
}

// NewClangFormat returns the C/C++/Objective-C formatter. clang-format is
// the only formatter in wide use for these languages and handles all of
// them from one binary, so there is no alternative to weigh.
func NewClangFormat() systemFormatter {
	return systemFormatter{name: "clang-format", bins: []string{"clang-format"}, args: inPlace("-i")}
}

// NewJava returns the .java formatter. google-java-format is the closest
// thing Java has to a canonical tool; clang-format also parses Java and is
// far more commonly already installed, so it stands in when the dedicated
// tool is absent.
func NewJava() systemFormatter {
	return systemFormatter{
		name: "google-java-format",
		bins: []string{"google-java-format", "clang-format"},
		args: func(bin, abs string) []string {
			if bin == "clang-format" {
				return []string{"-i", abs}
			}
			return []string{"--replace", abs}
		},
	}
}

// NewKotlin returns the .kt/.kts formatter, via ktlint's format mode.
func NewKotlin() systemFormatter {
	return systemFormatter{name: "ktlint", bins: []string{"ktlint"}, args: inPlace("--format", "--log-level=none")}
}

// NewSwift returns the .swift formatter. swift-format ships with the
// toolchain as of Swift 6; the third-party swiftformat predates it and is
// still what many projects have installed.
func NewSwift() systemFormatter {
	return systemFormatter{
		name: "swift-format",
		bins: []string{"swift-format", "swiftformat"},
		args: func(bin, abs string) []string {
			if bin == "swiftformat" {
				return []string{"--quiet", abs}
			}
			return []string{"format", "--in-place", abs}
		},
	}
}

// NewRuby returns the .rb/.rake/.gemspec formatter. Ruby has no canonical
// formatter, so this uses the autocorrector every Ruby project already
// runs. --fail-level fatal is load-bearing: without it rubocop exits
// non-zero whenever any offense it chose not to fix remains, which would
// report a correctly-rewritten file as a failed format on every write.
func NewRuby() systemFormatter {
	return systemFormatter{
		name: "rubocop",
		bins: []string{"rubocop", "standardrb"},
		args: func(bin, abs string) []string {
			if bin == "standardrb" {
				return []string{"--fix", "--no-fix-to-fail", abs}
			}
			return []string{"--autocorrect-all", "--fail-level", "fatal", "--format", "quiet", abs}
		},
	}
}

// NewPHP returns the .php formatter, via php-cs-fixer or Laravel's Pint.
func NewPHP() systemFormatter {
	return systemFormatter{
		name: "php-cs-fixer",
		bins: []string{"php-cs-fixer", "pint"},
		args: func(bin, abs string) []string {
			if bin == "pint" {
				return []string{"--quiet", abs}
			}
			return []string{"fix", "--quiet", abs}
		},
	}
}

// NewNix returns the .nix formatter. nixfmt is the format the Nix project
// itself settled on (RFC 166); alejandra and nixpkgs-fmt are the two
// pre-RFC tools still pinned by existing flakes.
func NewNix() systemFormatter {
	return systemFormatter{
		name: "nixfmt",
		bins: []string{"nixfmt", "alejandra", "nixpkgs-fmt"},
		args: func(_, abs string) []string { return []string{abs} },
	}
}

// NewLua returns the .lua formatter, via stylua.
func NewLua() systemFormatter {
	return systemFormatter{name: "stylua", bins: []string{"stylua"}, args: inPlace()}
}

// NewGoMod returns the go.mod formatter. The file path is always passed:
// without it, go mod edit -fmt rewrites the main module's go.mod.
func NewGoMod() systemFormatter {
	return systemFormatter{
		name:     "go mod edit",
		bins:     []string{"go"},
		args:     inPlace("mod", "edit", "-fmt"),
		onlyBase: "go.mod",
	}
}

// NewGoWork returns the go.work formatter. The file path is always passed:
// without it, go work edit -fmt walks parent directories for a go.work.
func NewGoWork() systemFormatter {
	return systemFormatter{
		name:     "go work edit",
		bins:     []string{"go"},
		args:     inPlace("work", "edit", "-fmt"),
		onlyBase: "go.work",
	}
}

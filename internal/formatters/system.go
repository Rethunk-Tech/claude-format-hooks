package formatters

import "context"

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
}

func (s systemFormatter) Name() string { return s.name }

func (s systemFormatter) Tools() []string { return s.bins }

func (s systemFormatter) Format(ctx context.Context, projectRoot, abs string) Result {
	for _, bin := range s.bins {
		if _, err := lookPath(bin); err != nil {
			continue
		}
		return runExternalResult(ctx, projectRoot, bin, s.args(bin, abs))
	}
	return Result{Skipped: true}
}

// inPlace is the argv shape shared by every formatter here that takes one
// in-place flag followed by the file.
func inPlace(flags ...string) func(bin, abs string) []string {
	return func(_, abs string) []string { return append(append([]string{}, flags...), abs) }
}

// NewClangFormat returns the C/C++/Objective-C formatter. clang-format is
// the only formatter in wide use for these languages and handles all of
// them from one binary, so there is no alternative to weigh.
func NewClangFormat() Formatter {
	return systemFormatter{name: "clang-format", bins: []string{"clang-format"}, args: inPlace("-i")}
}

// NewJava returns the .java formatter. google-java-format is the closest
// thing Java has to a canonical tool; clang-format also parses Java and is
// far more commonly already installed, so it stands in when the dedicated
// tool is absent.
func NewJava() Formatter {
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
func NewKotlin() Formatter {
	return systemFormatter{name: "ktlint", bins: []string{"ktlint"}, args: inPlace("--format", "--log-level=none")}
}

// NewSwift returns the .swift formatter. swift-format ships with the
// toolchain as of Swift 6; the third-party swiftformat predates it and is
// still what many projects have installed.
func NewSwift() Formatter {
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
func NewRuby() Formatter {
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
func NewPHP() Formatter {
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
func NewNix() Formatter {
	return systemFormatter{
		name: "nixfmt",
		bins: []string{"nixfmt", "alejandra", "nixpkgs-fmt"},
		args: func(_, abs string) []string { return []string{abs} },
	}
}

// NewLua returns the .lua formatter, via stylua.
func NewLua() Formatter {
	return systemFormatter{name: "stylua", bins: []string{"stylua"}, args: inPlace()}
}

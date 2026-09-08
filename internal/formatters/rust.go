package formatters

import "context"

// rustFormatter shells out to the system rustfmt binary for .rs — Rust
// has no Go equivalent, and rustfmt is the canonical formatter for Rust
// itself (the same relationship gofmt has to Go), so there's no fidelity
// question to weigh, just no in-process implementation to call.
type rustFormatter struct{}

// NewRust returns the rustFormatter for .rs.
func NewRust() Formatter { return rustFormatter{} }

func (rustFormatter) Name() string { return "rustfmt" }

func (rustFormatter) Format(ctx context.Context, projectRoot, abs string) Result {
	if _, err := lookPath("rustfmt"); err != nil {
		return Result{Skipped: true}
	}
	return runExternalResult(ctx, projectRoot, "rustfmt", []string{"--", abs})
}

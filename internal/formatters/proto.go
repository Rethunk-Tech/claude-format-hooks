package formatters

import "context"

// protoFormatter shells out to the system `buf` binary's `format`
// subcommand for .proto. buf is Go, but its formatter is not exposed as a
// stable importable package the way go/format is — and vendoring the
// toolchain to save one subprocess would trade a large dependency for a
// few milliseconds. Like `terraform fmt` and `rustfmt`, it is the single
// canonical formatter for the language, so there is no competing tool to
// weigh.
type protoFormatter struct{}

// NewProto returns the protoFormatter for .proto.
func NewProto() Formatter { return protoFormatter{} }

func (protoFormatter) Name() string { return "buf" }

func (protoFormatter) Tools() []string { return []string{"buf"} }

func (protoFormatter) Format(ctx context.Context, projectRoot, abs string) Result {
	if _, err := lookPath("buf"); err != nil {
		return Result{Skipped: true}
	}
	// -w rewrites in place. buf resolves any buf.yaml by walking up from the
	// file itself, so running from projectRoot does not hide a nested
	// module's config the way a fixed --path would.
	return runExternalResult(ctx, projectRoot, "buf", []string{"format", "-w", abs})
}

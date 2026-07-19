package formatters

import (
	"bytes"
	"context"
	"fmt"
	"go/format"
	"os"
)

// goFormatter reformats Go source in-process via go/format.Source — the
// same formatting engine `gofmt` itself is built on, so output matches
// gofmt exactly with no subprocess involved. Unlike JSON (see json.go's
// key-order rationale), Go source has exactly one canonical formatting,
// so there's no fidelity trade-off to weigh: this is a strictly safer
// native candidate than JSON was.
type goFormatter struct{}

// NewGo returns the native goFormatter for .go.
func NewGo() Formatter { return goFormatter{} }

func (goFormatter) Name() string { return "gofmt" }

func (goFormatter) Format(_ context.Context, _, abs string) Result {
	src, err := os.ReadFile(abs) //nolint:gosec // abs is the file this formatter was invoked to format, by design
	if err != nil {
		return Result{Err: fmt.Errorf("read: %w", err)}
	}

	out, err := format.Source(src)
	if err != nil {
		// Not valid Go syntax — not our job to fix a broken file.
		return Result{Skipped: true}
	}

	if bytes.Equal(out, src) {
		return Result{}
	}

	if err := writeFormatted(abs, out, 0o644); err != nil {
		return Result{Err: fmt.Errorf("write: %w", err)}
	}
	return Result{}
}

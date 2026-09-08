package formatters

import (
	"bytes"
	"context"
	"os/exec"
	"unicode/utf8"
)

// runExternal runs name with args in dir and, on failure, truncates its
// combined output to a safe diagnostic size.
func runExternal(ctx context.Context, dir, name string, args []string) (ok bool, diagnostic string) {
	ok, output, _ := runExternalOutput(ctx, dir, name, args)
	return ok, output
}

// runExternalOutput is runExternal plus the tool's raw combined output, for
// formatters that must inspect it to tell a real failure from a non-zero
// exit that just means "I ran, and some violations remain." A fixer that
// rewrote the file correctly and then exited non-zero over what it chose
// not to fix has not failed, and reporting it as a failure turns every
// affected write into a diagnostic the hook cannot act on.
//
// raw is empty when ok is true or when the process produced no output at
// all; diagnostic (returned as the second value when ok is false) is
// already truncated, raw is not.
func runExternalOutput(ctx context.Context, dir, name string, args []string) (ok bool, diagnostic string, raw string) {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // running an external formatter by design; args are our own construction, never a shell
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err == nil {
		return true, "", ""
	}
	if ctx.Err() != nil {
		// The process was killed because our timeout fired, not because it
		// failed on its own — report why, not the generic "signal: killed".
		return false, context.Cause(ctx).Error(), string(out)
	}
	if len(out) == 0 {
		return false, err.Error(), ""
	}
	return false, truncate(out, 10, 500), string(out)
}

func truncate(out []byte, maxLines, maxChars int) string {
	lines := bytes.SplitN(out, []byte("\n"), maxLines+1)
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	joined := bytes.Join(lines, []byte("\n"))
	if len(joined) <= maxChars {
		return string(joined)
	}
	// Back off to the nearest rune boundary so a multi-byte character
	// (e.g. in a non-ASCII linter message) isn't split mid-encoding.
	cut := maxChars
	for cut > 0 && !utf8.RuneStart(joined[cut]) {
		cut--
	}
	return string(joined[:cut])
}

// runExternalResult runs an external formatter and shapes the outcome as a
// Result: a failure carries its diagnostic, success carries nothing. Every
// tool that neither skips nor inspects the output ends this way.
func runExternalResult(ctx context.Context, dir, name string, args []string) Result {
	if ok, diagnostic := runExternal(ctx, dir, name, args); !ok {
		return Result{Diagnostic: diagnostic}
	}
	return Result{}
}

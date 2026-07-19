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
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // running an external formatter by design; args are our own construction, never a shell
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err == nil {
		return true, ""
	}
	if ctx.Err() != nil {
		// The process was killed because our timeout fired, not because it
		// failed on its own — report why, not the generic "signal: killed".
		return false, context.Cause(ctx).Error()
	}
	if len(out) == 0 {
		return false, err.Error()
	}
	return false, truncate(out, 10, 500)
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

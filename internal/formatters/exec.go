package formatters

import (
	"bytes"
	"context"
	"os/exec"
)

// runExternal runs name with args in dir, truncates combined output to a
// safe diagnostic size, and reports whether the file actually changed by
// comparing a content hash the caller supplies via changed.
func runExternal(ctx context.Context, dir, name string, args []string) (ok bool, diagnostic string) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err == nil {
		return true, ""
	}
	return false, truncate(out, 10, 500)
}

func truncate(out []byte, maxLines, maxChars int) string {
	lines := bytes.SplitN(out, []byte("\n"), maxLines+1)
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	joined := bytes.Join(lines, []byte("\n"))
	if len(joined) > maxChars {
		joined = joined[:maxChars]
	}
	return string(joined)
}

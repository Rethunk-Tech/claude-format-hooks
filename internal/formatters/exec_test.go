package formatters

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/go-quicktest/qt"
)

// A stdin tool that exits 0 without emitting the document declined the input.
// Writing its empty stdout back truncated the source to zero bytes: a stub
// taplo doing exactly that, and prettier for real on a .yaml.example.
func TestRunExternalStdinTreatsEmptyStdoutAsASkip(t *testing.T) {
	isolateDiskCache(t)
	argvPath := filepath.Join(t.TempDir(), "argv")
	t.Setenv("TAPLO_ARGV", argvPath)
	writeFakeTool(t, "taplo", `printf '%s\n' "$@" > "$TAPLO_ARGV"`)

	dir, abs := sourceFile(t, "config.toml.example", "a = 1\n")
	res := runExternalStdin(t.Context(), dir, "taplo", []string{"format", "--stdin-filepath", abs, "-"}, abs)

	qt.Check(t, qt.IsTrue(res.Skipped), qt.Commentf("silent success is a decline, not an empty document"))
	qt.Check(t, qt.Equals(res.Diagnostic, ""))
	qt.Check(t, qt.Equals(string(readSource(t, abs)), "a = 1\n"),
		qt.Commentf("an empty stdout must never be written over the source"))
}

func TestTruncateLineLimit(t *testing.T) {
	out := []byte("one\ntwo\nthree\nfour\nfive")
	got := truncate(out, 3, 500)
	qt.Check(t, qt.Equals(got, "one\ntwo\nthree"))
}

func TestTruncateCharLimitIsUTF8Safe(t *testing.T) {
	// "é" is 2 bytes (0xC3 0xA9); repeat it so a byte-level cut at an even
	// offset would land mid-rune.
	line := strings.Repeat("é", 10) // 20 bytes
	got := truncate([]byte(line), 10, 15)
	qt.Check(t, qt.IsTrue(utf8.ValidString(got)), qt.Commentf("truncated output is not valid UTF-8: %q", got))
	qt.Check(t, qt.IsTrue(len(got) <= 15), qt.Commentf("truncated output exceeds maxChars: %d bytes: %q", len(got), got))
}

func TestTruncateUnderLimitsUnchanged(t *testing.T) {
	out := []byte("short diagnostic")
	got := truncate(out, 10, 500)
	qt.Check(t, qt.Equals(got, "short diagnostic"))
}

func TestRunExternalReportsContextCause(t *testing.T) {
	cause := errors.New("simulated timeout")
	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(cause) // already done before runExternal ever starts the process

	ok, diag := runExternal(ctx, t.TempDir(), "does-not-matter", nil)
	qt.Check(t, qt.IsFalse(ok))
	qt.Check(t, qt.Equals(diag, cause.Error()), qt.Commentf("want the context's cause, not a generic process-killed error"))
}

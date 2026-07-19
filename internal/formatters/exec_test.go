package formatters

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/go-quicktest/qt"
)

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

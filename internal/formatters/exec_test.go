package formatters

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncateLineLimit(t *testing.T) {
	out := []byte("one\ntwo\nthree\nfour\nfive")
	got := truncate(out, 3, 500)
	want := "one\ntwo\nthree"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestTruncateCharLimitIsUTF8Safe(t *testing.T) {
	// "é" is 2 bytes (0xC3 0xA9); repeat it so a byte-level cut at an even
	// offset would land mid-rune.
	line := strings.Repeat("é", 10) // 20 bytes
	got := truncate([]byte(line), 10, 15)
	if !utf8.ValidString(got) {
		t.Fatalf("truncated output is not valid UTF-8: %q", got)
	}
	if len(got) > 15 {
		t.Fatalf("truncated output exceeds maxChars: %d bytes: %q", len(got), got)
	}
}

func TestTruncateUnderLimitsUnchanged(t *testing.T) {
	out := []byte("short diagnostic")
	got := truncate(out, 10, 500)
	if got != "short diagnostic" {
		t.Fatalf("got %q, want unchanged input", got)
	}
}

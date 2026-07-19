package formatters

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Rethunk-Tech/claude-format-hooks/internal/config"
)

// jsonFormatter reformats strict JSON (no comments) in place, in-process.
//
// It deliberately uses json.Indent rather than Unmarshal+Marshal:
// json.Indent re-indents at the byte level without building an object
// graph, so it preserves source key order exactly. A round-trip through
// map[string]interface{} would silently alphabetize every object's keys
// (encoding/json.Marshal sorts map keys), which is not what "format"
// means for a JSON file a human or another tool authored.
type jsonFormatter struct{ cfg config.Config }

func NewJSON(cfg config.Config) Formatter { return jsonFormatter{cfg: cfg} }

func (jsonFormatter) Name() string { return "json" }

func (f jsonFormatter) Format(_ context.Context, _, abs string) Result {
	src, err := os.ReadFile(abs)
	if err != nil {
		return Result{Err: fmt.Errorf("read: %w", err)}
	}

	spec := config.ResolveJSONIndent(f.cfg, abs)
	indent := "\t"
	if !spec.UseTabs {
		indent = strings.Repeat(" ", max(spec.Size, 1))
	}

	var buf bytes.Buffer
	if err := json.Indent(&buf, src, "", indent); err != nil {
		// Not valid strict JSON (e.g. actually JSONC, or malformed).
		// Not our job to fix malformed JSON — skip silently.
		return Result{Skipped: true}
	}

	// json.Indent passes through any insignificant whitespace already
	// trailing the top-level value in src verbatim (it does not strip
	// it). Trim before appending exactly one newline, or a file that
	// already ends in "}\n" would gain an extra blank line every single
	// time it's formatted — a non-idempotent, ever-growing bug.
	out := append(bytes.TrimRight(buf.Bytes(), "\n\t \r"), '\n')
	if bytes.Equal(out, src) {
		return Result{}
	}

	info, err := os.Stat(abs)
	mode := os.FileMode(0o644)
	if err == nil {
		mode = info.Mode()
	}
	if err := os.WriteFile(abs, out, mode); err != nil {
		return Result{Err: fmt.Errorf("write: %w", err)}
	}
	return Result{Changed: true}
}

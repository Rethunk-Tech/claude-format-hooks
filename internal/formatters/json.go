package formatters

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

// NewJSON returns the native jsonFormatter for .json.
func NewJSON(cfg config.Config) Formatter { return jsonFormatter{cfg: cfg} }

// jsonRouter sends .json to biome where the project has a biome config and to
// the native formatter everywhere else.
//
// Biome's config governs .json as much as it governs .ts — indent, line width
// and `expand`, which decides whether an object collapses onto one line. The
// native formatter cannot read any of that: json.Indent always expands every
// object. Formatting a project's own biome.json with it therefore produces a
// file that project's `biome check` rejects, so the config that opted in is the
// config that gets violated. Projects without biome keep the native formatter,
// which needs no bunx and no config of its own.
type jsonRouter struct {
	biome  Formatter
	native Formatter
}

// NewJSONRouter returns the .json formatter: biome when the project has a
// biome config, the dependency-free native formatter otherwise.
func NewJSONRouter(cfg config.Config) Formatter {
	return jsonRouter{biome: NewBiome(), native: NewJSON(cfg)}
}

func (jsonRouter) Name() string { return "json" }

func (r jsonRouter) Format(ctx context.Context, projectRoot, abs string) Result {
	// Without a usable Biome launcher the native formatter is still better than
	// leaving the file untouched, even though it ignores `expand`.
	if cachedFindUpward(filepath.Dir(abs), projectRoot, "biome.json", "biome.jsonc") != "" {
		if _, err := lookPath("biome"); err == nil {
			return r.biome.Format(ctx, projectRoot, abs)
		}
		if _, err := lookPath("bunx"); err == nil {
			return r.biome.Format(ctx, projectRoot, abs)
		}
	}
	return r.native.Format(ctx, projectRoot, abs)
}

func (jsonFormatter) Name() string { return "json" }

func (f jsonFormatter) Format(_ context.Context, _, abs string) Result {
	src, err := os.ReadFile(abs) //nolint:gosec // abs is the file this formatter was invoked to format, by design
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

	if err := writeFormatted(abs, src, out, 0o644); err != nil {
		return Result{Err: fmt.Errorf("write: %w", err)}
	}
	return Result{}
}

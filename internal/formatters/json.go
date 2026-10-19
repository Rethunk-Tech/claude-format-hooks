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
func NewJSON(cfg config.Config) jsonFormatter { return jsonFormatter{cfg: cfg} }

// jsonRouter sends .json to biome where the project has a biome config unless
// biome is disabled, and to the native formatter everywhere else. Its Name
// remains "json" even when Format delegates to biome, so callers applying
// project formatter opt-outs must account for the delegated formatter name.
//
// Biome's config governs .json as much as it governs .ts — indent, line width
// and `expand`, which decides whether an object collapses onto one line. The
// native formatter cannot read any of that: json.Indent always expands every
// object. Formatting a project's own biome.json with it therefore produces a
// file that project's biome formatting rejects, so the config that opted in is
// the config that gets violated. Projects without biome keep the native formatter,
// which needs no bunx and no config of its own.
type jsonRouter struct {
	biome         Formatter
	native        Formatter
	biomeDisabled bool
}

// NewJSONRouter returns the .json formatter: biome when the project has a
// biome config and biome is enabled, the dependency-free native formatter
// otherwise. Project-level "biome" disables therefore cannot be enforced by
// registry name filtering alone; hook and --check callers must preserve the
// router's delegated formatter opt-outs.
func NewJSONRouter(cfg config.Config) jsonRouter {
	return jsonRouter{
		biome:         NewBiome(),
		native:        NewJSON(cfg),
		biomeDisabled: cfg.IsFormatterDisabled("biome"),
	}
}

func (jsonRouter) Name() string { return "json" }

// Tools reports biome and bunx so --doctor does not label the router native.
// Format still falls back to encoding/json.Indent when those are absent.
func (jsonRouter) Tools() []string { return []string{"biome", "bunx"} }

func (r jsonRouter) Format(ctx context.Context, projectRoot, abs string) Result {
	// Biome exits 0 on a suffix it does not recognize (--no-errors-on-unmatched)
	// and leaves the file as it was. These copies are still JSON.
	if strings.HasSuffix(strings.ToLower(filepath.Base(abs)), ".json.example") {
		return r.native.Format(ctx, projectRoot, abs)
	}
	// Without a usable Biome launcher the native formatter is still better than
	// leaving the file untouched, even though it ignores `expand`.
	if biomeUsable(projectRoot, abs, r.biomeDisabled) {
		return r.biome.Format(ctx, projectRoot, abs)
	}
	return r.native.Format(ctx, projectRoot, abs)
}

func (jsonFormatter) Name() string { return "json" }

func (f jsonFormatter) Format(_ context.Context, _, abs string) Result {
	src, err := os.ReadFile(filepath.Clean(abs))
	if err != nil {
		return Result{Err: fmt.Errorf("read: %w", err)}
	}

	spec := config.ResolveJSONIndent(f.cfg, abs)
	indent := "\t"
	if !spec.UseTabs {
		indent = strings.Repeat(" ", clampedIndentWidth(spec.Size))
	}

	var buf bytes.Buffer
	if err := json.Indent(&buf, src, "", indent); err != nil {
		// Not valid strict JSON (e.g. actually JSONC, or malformed).
		// The hook surfaces Diagnostic; --check must fail rather than
		// treat an unchanged scratch copy as already formatted or as a
		// missing tool.
		return Result{Diagnostic: err.Error()}
	}

	// json.Indent passes through any insignificant whitespace already
	// trailing the top-level value in src verbatim (it does not strip
	// it). Trim before appending exactly one newline, or a file that
	// already ends in "}\n" would gain an extra blank line every single
	// time it's formatted — a non-idempotent, ever-growing bug.
	out := append(bytes.TrimRight(buf.Bytes(), "\n\t \r"), '\n')
	return writeIfChanged(abs, src, out, 0o644)
}

// maxIndentWidth caps spaces taken from untrusted config / .editorconfig.
// strings.Repeat and shfmt's Indent would otherwise accept a multi-gigabyte
// width from a hostile indentSize.
const maxIndentWidth = 32

func clampedIndentWidth(size int) int {
	if size < 1 {
		return 1
	}
	if size > maxIndentWidth {
		return maxIndentWidth
	}
	return size
}

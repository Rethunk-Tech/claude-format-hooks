// Package dispatch maps a file extension to the formatter that handles it,
// and routes a single file to that formatter.
package dispatch

import (
	"context"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Rethunk-Tech/claude-format-hooks/internal/config"
	"github.com/Rethunk-Tech/claude-format-hooks/internal/formatters"
)

// Registry holds one Formatter per supported extension, built once from
// the resolved Config so per-user Disabled entries only cost a map lookup,
// not a slice scan, on the hot path.
type Registry struct {
	byExt map[string]formatters.Formatter
}

// NewRegistry builds the extension -> Formatter map, honoring cfg.Disabled.
func NewRegistry(cfg config.Config) *Registry {
	json := formatters.NewJSON(cfg)
	shell := formatters.NewShell(cfg)
	golang := formatters.NewGo()
	biome := formatters.NewBiome()
	markdown := formatters.NewMarkdown()
	toml := formatters.NewTOML()
	prettier := formatters.NewPrettier()
	sql := formatters.NewSQLFluff()
	python := formatters.NewPython()
	rust := formatters.NewRust()
	terraform := formatters.NewTerraform()
	proto := formatters.NewProto()

	all := map[string]formatters.Formatter{
		".json": json,

		".sh":   shell,
		".bash": shell,

		".go": golang,

		".ts":  biome,
		".tsx": biome,
		".js":  biome,
		".jsx": biome,
		".mjs": biome,
		".cjs": biome,
		// .mts/.cts are TypeScript's explicit-module-type variants of
		// .mjs/.cjs — biome parses them the same way.
		".mts": biome,
		".cts": biome,
		".css": biome,
		// .jsonc keeps comments, which json.Indent can't handle safely —
		// route it to biome instead of the native json formatter.
		".jsonc": biome,

		".md":       markdown,
		".mdx":      markdown,
		".markdown": markdown,

		".toml": toml,

		".yaml": prettier,
		".yml":  prettier,
		".html": prettier,
		// biome's CSS parser doesn't support the SCSS/Less supersets;
		// prettier does natively.
		".scss": prettier,
		".less": prettier,
		// GraphQL has no dedicated tool integrated here; prettier supports
		// it natively at no extra cost.
		".graphql": prettier,
		".gql":     prettier,

		".sql": sql,

		".py":    python,
		".rs":    rust,
		".tf":    terraform,
		".proto": proto,
	}
	maps.DeleteFunc(all, func(ext string, _ formatters.Formatter) bool { return cfg.IsDisabled(ext) })
	return &Registry{byExt: all}
}

// vendoredDirs are path segments never worth formatting: build output,
// dependency trees, and VCS/tooling metadata.
var vendoredDirs = map[string]bool{
	"node_modules": true,
	".next":        true,
	".yarn":        true,
	".git":         true,
	".agents":      true,
	"dist":         true,
	"build":        true,
	"coverage":     true,
	"test-results": true,
	"vendor":       true,
	".venv":        true,
}

// Supported reports whether ext (as returned by filepath.Ext, e.g. ".ts")
// has a registered, enabled formatter. Callers should check this before
// doing any other work — an unsupported extension must be an instant
// no-op: this is a single map read, nothing else.
func (r *Registry) Supported(ext string) bool {
	_, ok := r.byExt[strings.ToLower(ext)]
	return ok
}

// knownExtensions is the full extension superset registered across every
// formatter, independent of any user or project config: Disabled can only
// remove entries from a Registry's byExt map, never add ones outside this
// set. Built once from a Registry constructed with an empty Config (so
// nothing is filtered out) — computing it this way, instead of duplicating
// the extension list from NewRegistry, means the two can never drift.
var knownExtensions = func() map[string]bool {
	r := NewRegistry(config.Config{})
	exts := make(map[string]bool, len(r.byExt))
	for ext := range r.byExt {
		exts[ext] = true
	}
	return exts
}()

// KnownExtension reports whether ext is ever handled by any formatter,
// regardless of user or project config. Unlike Supported, this needs no
// Registry (and so no config load) to answer — callers can use it to skip
// loading config entirely for an extension no formatter will ever touch,
// keeping that path a true instant no-op: no file read, not just no
// subprocess.
func KnownExtension(ext string) bool {
	return knownExtensions[strings.ToLower(ext)]
}

// InVendoredDir reports whether relPath (relative to the project root)
// passes through a directory that should never be auto-formatted.
func InVendoredDir(relPath string) bool {
	segs := strings.Split(filepath.ToSlash(relPath), "/")
	return slices.ContainsFunc(segs, func(seg string) bool { return vendoredDirs[seg] })
}

// Dispatch routes abs to the formatter registered for its extension.
// Callers must already have confirmed Supported(filepath.Ext(abs)) and
// !InVendoredDir(...); Dispatch does not re-check either.
func (r *Registry) Dispatch(ctx context.Context, projectRoot, abs string) formatters.Result {
	f := r.byExt[strings.ToLower(filepath.Ext(abs))]
	return f.Format(ctx, projectRoot, abs)
}

// Name returns the formatter name registered for ext, or "" if unsupported.
// Used only for diagnostic messages.
func (r *Registry) Name(ext string) string {
	if f, ok := r.byExt[strings.ToLower(ext)]; ok {
		return f.Name()
	}
	return ""
}

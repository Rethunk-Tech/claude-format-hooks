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
		".css": biome,
		// .jsonc keeps comments, which json.Indent can't handle safely —
		// route it to biome instead of the native json formatter.
		".jsonc": biome,

		".md":  markdown,
		".mdx": markdown,

		".toml": toml,

		".yaml": prettier,
		".yml":  prettier,
		".html": prettier,

		".sql": sql,
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

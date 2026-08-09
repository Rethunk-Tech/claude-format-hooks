// Package dispatch maps a file extension to the formatter that handles it,
// and routes a single file to that formatter.
package dispatch

import (
	"context"
	"maps"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/Rethunk-Tech/claude-format-hooks/internal/config"
	"github.com/Rethunk-Tech/claude-format-hooks/internal/formatters"
)

// Registry holds one Formatter per supported extension, built once from
// the resolved Config so per-user disabled entries only cost a map lookup,
// not a slice scan, on the hot path.
type Registry struct {
	byExt map[string]formatters.Formatter
}

// NewRegistry builds the extension -> Formatter map, honoring cfg.Disabled
// and cfg.DisabledFormatters.
func NewRegistry(cfg config.Config) *Registry {
	json := formatters.NewJSONRouter(cfg)
	shell := formatters.NewShell(cfg)
	golang := formatters.NewGo()
	biome := formatters.NewBiome()
	markdown := formatters.NewMarkdown()
	toml := formatters.NewTOML()
	prettier := formatters.NewPrettier()
	sql := formatters.NewSQLFluff()
	python := formatters.NewPython()
	notebook := formatters.NewNotebook()
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
		// biome unconditionally, since a project without a biome config still
		// needs something that parses comments.
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

		".py":          python,
		".pyi":         python,
		".ipynb":       notebook,
		".rs":          rust,
		".tf":          terraform,
		".tfvars":      terraform,
		".tftest.hcl":  terraform,
		".tfmock.hcl":  terraform,
		".tfquery.hcl": terraform,
		".proto":       proto,
	}
	maps.DeleteFunc(all, func(ext string, f formatters.Formatter) bool {
		return cfg.IsDisabled(ext) || cfg.IsFormatterDisabled(f.Name())
	})
	return &Registry{byExt: all}
}

// vendoredDirs are path segments never worth formatting: build output,
// dependency trees, language/tool caches, and VCS metadata.
var vendoredDirs = map[string]bool{
	"node_modules":  true,
	".next":         true,
	".yarn":         true,
	".git":          true,
	".agents":       true,
	"dist":          true,
	"build":         true,
	"coverage":      true,
	"test-results":  true,
	"vendor":        true,
	".venv":         true,
	".terraform":    true,
	"__pycache__":   true,
	".ruff_cache":   true,
	".mypy_cache":   true,
	".pytest_cache": true,
	".tox":          true,
}

// Supported reports whether ext (already resolved, e.g. ".ts" or
// ".tftest.hcl") has a registered, enabled formatter. Callers should check
// this before doing any other work — an unsupported extension must be an
// instant no-op: this is a single map read, nothing else.
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
var knownExtensions, registeredSuffixes = func() (map[string]bool, []string) {
	r := NewRegistry(config.Config{})
	exts := make(map[string]bool, len(r.byExt))
	suffixes := make([]string, 0, len(r.byExt))
	for ext := range r.byExt {
		exts[ext] = true
		suffixes = append(suffixes, ext)
	}
	sort.Slice(suffixes, func(i, j int) bool {
		if len(suffixes[i]) != len(suffixes[j]) {
			return len(suffixes[i]) > len(suffixes[j])
		}
		return suffixes[i] < suffixes[j]
	})
	return exts, suffixes
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

// Dispatch routes abs to the formatter registered for ext.
// Callers pass the already-resolved extension (which may differ from
// filepath.Ext(abs) for extensionless shell-shebang paths) and must
// already have confirmed Supported(ext) and !InVendoredDir(...);
// Dispatch does not re-check either.
func (r *Registry) Dispatch(ctx context.Context, projectRoot, abs, ext string) formatters.Result {
	if r == nil {
		return formatters.Result{Skipped: true}
	}
	f := r.byExt[strings.ToLower(ext)]
	if f == nil {
		return formatters.Result{Skipped: true}
	}
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

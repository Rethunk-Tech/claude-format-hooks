// Package config resolves per-formatter style settings (indent width,
// tabs-vs-spaces, etc.) from three layers, in increasing priority:
//
//  1. Built-in defaults (2-space indent, matching this repo's own style).
//  2. The installing user's own config file (~/.claude/claude-format-hooks.json),
//     for a personal preference that applies everywhere.
//  3. The target project's .editorconfig, if one covers the file being
//     formatted — a project's own declared convention wins over the
//     user's personal default, same as every editor and formatter that
//     honors EditorConfig.
//
// External formatters (biome, prettier, taplo, markdownlint-cli2,
// sqlfluff) already read their own project config files (biome.json,
// .prettierrc, .sqlfluff, ...) automatically because we invoke the real
// tool — nothing to do there. This package only matters for the two
// native, in-process formatters (JSON, shell), which otherwise have no
// project config file of their own to consult.
package config

import (
	"encoding/json"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"mvdan.cc/editorconfig"

	"github.com/Rethunk-Tech/claude-format-hooks/internal/diskcache"
)

// JSON holds the user's indent preference for the native JSON formatter.
type JSON struct {
	IndentSize int  `json:"indentSize"`
	UseTabs    bool `json:"useTabs"`
}

// Shell holds the user's indent and switch-case-indent preference for the
// native shell formatter.
type Shell struct {
	IndentSize       int  `json:"indentSize"`
	UseTabs          bool `json:"useTabs"`
	SwitchCaseIndent bool `json:"switchCaseIndent"`
}

// Config is the installing user's own preference, loaded once from
// ~/.claude/claude-format-hooks.json (or wherever CLAUDE_FORMAT_HOOKS_CONFIG
// points). Disabled lists extensions (e.g. ".sql") to skip entirely, while
// DisabledFormatters lists formatter names (e.g. "biome") for users who want
// fewer formatters running than the default set.
type Config struct {
	JSON               JSON     `json:"json"`
	Shell              Shell    `json:"shell"`
	Disabled           []string `json:"disabled"`
	DisabledFormatters []string `json:"disabledFormatters"`
}

// Default returns the built-in indent defaults (2-space, no tabs).
func Default() Config {
	return Config{
		JSON:  JSON{IndentSize: 2, UseTabs: false},
		Shell: Shell{IndentSize: 2, UseTabs: false, SwitchCaseIndent: true},
	}
}

// Load reads path over top of Default(); a missing file is not an error —
// it just means "use the defaults". A present-but-malformed file returns
// an error so the caller can decide whether to fail loudly or fall back;
// format-dispatch falls back, since a broken config must never turn a
// silent formatting hook into something that blocks every file write.
func Load(path string) (Config, error) {
	cfg := Default()
	raw, err := os.ReadFile(path) //nolint:gosec // path is the caller-controlled config location (env override or fixed default), by design
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return Default(), err
	}
	return cfg, nil
}

// IsDisabled reports whether ext is in the user's disabled list. Comparison
// is case-insensitive, since extensions are matched case-insensitively
// everywhere else (filepath.Ext preserves the source file's casing).
func (c Config) IsDisabled(ext string) bool {
	return slices.ContainsFunc(c.Disabled, func(d string) bool { return strings.EqualFold(d, ext) })
}

// IsFormatterDisabled reports whether name appears in a disabled formatter
// list. User-level and project configuration use this helper. Formatter names
// are matched case-insensitively.
func (c Config) IsFormatterDisabled(name string) bool {
	return slices.ContainsFunc(c.DisabledFormatters, func(d string) bool {
		return strings.EqualFold(strings.TrimSpace(d), name)
	})
}

// IndentSpec is a resolved indent width/style for one file.
type IndentSpec struct {
	Size    int
	UseTabs bool
}

// ResolveJSONIndent applies the three-layer priority described above for
// a JSON file at abs.
func ResolveJSONIndent(cfg Config, abs string) IndentSpec {
	return resolveIndent(abs, IndentSpec{Size: cfg.JSON.IndentSize, UseTabs: cfg.JSON.UseTabs})
}

// ResolveShellIndent applies the same priority for a shell script at abs.
func ResolveShellIndent(cfg Config, abs string) IndentSpec {
	return resolveIndent(abs, IndentSpec{Size: cfg.Shell.IndentSize, UseTabs: cfg.Shell.UseTabs})
}

// editorconfigCacheTTL is how long a resolved IndentSpec is trusted
// before resolveIndent re-walks and re-parses .editorconfig for it. A
// project's .editorconfig practically never changes mid-session, so a
// stale hit just costs one extra resolution after expiry, never a wrong
// result — same reasoning as findUpwardCacheTTL in
// internal/formatters/biome.go.
const editorconfigCacheTTL = 30 * time.Second

// resolveIndent is cached (via internal/diskcache) on (abs, fallback):
// each format-dispatch invocation is a fresh process (see AGENTS.md's
// cold-start rationale), and editorconfig.Find's directory walk
// otherwise repeats every time the same file is reformatted — a common
// pattern when an agent edits one file several times in quick
// succession.
//
// Measured: ~2.9us cached against ~9.2us for the bare walk three
// directories deep, ~2.8us against ~14.6us at eight. The key includes the
// file's absolute path, so entries accrue one per file formatted; that
// cost is bounded by pruneOnce (see internal/diskcache) and the cached
// path does not degrade as they accumulate. Deleting the cache has been
// proposed and measured down twice — it is worth its keep.
func resolveIndent(abs string, fallback IndentSpec) IndentSpec {
	cacheDir, ok := diskcache.Dir()
	if !ok {
		return resolveIndentUncached(abs, fallback)
	}

	key := diskcache.Key("editorconfig", abs, strconv.Itoa(fallback.Size), strconv.FormatBool(fallback.UseTabs))
	if cached, hit := diskcache.Get(cacheDir, key, editorconfigCacheTTL); hit {
		if spec, ok := decodeIndentSpec(cached); ok {
			return spec
		}
	}

	spec := resolveIndentUncached(abs, fallback)
	diskcache.Set(cacheDir, key, encodeIndentSpec(spec))
	return spec
}

func resolveIndentUncached(abs string, fallback IndentSpec) IndentSpec {
	section, err := editorconfig.Find(abs, nil)
	if err != nil {
		return fallback
	}
	style := section.Get("indent_style")
	if style == "" {
		// No EditorConfig section covers this file — the user's own
		// preference (or the built-in default) stands.
		return fallback
	}
	spec := fallback
	spec.UseTabs = style == "tab"
	if size := section.IndentSize(); size > 0 {
		spec.Size = size
	}
	return spec
}

// encodeIndentSpec/decodeIndentSpec serialize an IndentSpec to and from
// the plain "size,tabs" string diskcache stores it as — no need for
// JSON's overhead for a two-field value only this package ever reads.
func encodeIndentSpec(spec IndentSpec) string {
	tabs := "0"
	if spec.UseTabs {
		tabs = "1"
	}
	return strconv.Itoa(spec.Size) + "," + tabs
}

func decodeIndentSpec(s string) (IndentSpec, bool) {
	size, tabs, found := strings.Cut(s, ",")
	if !found {
		return IndentSpec{}, false
	}
	n, err := strconv.Atoi(size)
	if err != nil {
		return IndentSpec{}, false
	}
	return IndentSpec{Size: n, UseTabs: tabs == "1"}, true
}

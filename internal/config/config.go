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
	"strings"

	"mvdan.cc/editorconfig"
)

type JSON struct {
	IndentSize int  `json:"indentSize"`
	UseTabs    bool `json:"useTabs"`
}

type Shell struct {
	IndentSize       int  `json:"indentSize"`
	UseTabs          bool `json:"useTabs"`
	SwitchCaseIndent bool `json:"switchCaseIndent"`
}

// Config is the installing user's own preference, loaded once from
// ~/.claude/claude-format-hooks.json (or wherever CLAUDE_FORMAT_HOOKS_CONFIG
// points). Disabled lists extensions (e.g. ".sql") to skip entirely, for
// users who want fewer formatters running than the default set.
type Config struct {
	JSON     JSON     `json:"json"`
	Shell    Shell    `json:"shell"`
	Disabled []string `json:"disabled"`
}

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
	for _, d := range c.Disabled {
		if strings.EqualFold(d, ext) {
			return true
		}
	}
	return false
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

func resolveIndent(abs string, fallback IndentSpec) IndentSpec {
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

package formatters

import (
	_ "embed"
	"os"
	"path/filepath"
	"sync"
)

// markdownlintDefaults is the base configuration handed to every
// markdownlint-cli2 run. See markdownlint-defaults.jsonc for what it sets
// and why.
//
//go:embed markdownlint-defaults.jsonc
var markdownlintDefaults []byte

// userMarkdownlintConfigFile is the user-editable copy of the embedded
// defaults, materialized on first use so the settings are discoverable and
// tunable rather than locked inside the binary. Namespaced like the hook's
// other user config (~/.claude/claude-format-hooks.json) so it is obvious
// which tool owns it.
//
// The `.markdownlint-cli2.` infix is load-bearing, not decoration:
// markdownlint-cli2 picks the schema to parse a --config file by its NAME.
// A name matching `.markdownlint.<ext>` is read as a bare rules object,
// while `.markdownlint-cli2.<ext>` (and any unrecognized name) is read as
// the options object this file actually is -- the one with the "config" and
// "ignores" keys. Getting that backwards does not error: the file is
// silently discarded and markdownlint's stock rules apply instead.
const userMarkdownlintConfigFile = "claude-format-hooks.markdownlint-cli2.jsonc"

// markdownlintConfigOnce makes the existence check and first-use write
// happen once per process rather than once per file. A single Write/Edit
// only formats one file, but a multi-file edit reuses the same process for
// several.
var markdownlintConfigOnce sync.Once

// markdownlintConfigPath caches the resolved path, or "" when no user-level
// config could be resolved or written.
var markdownlintConfigPath string

// userMarkdownlintConfig returns the path to pass to markdownlint-cli2's
// --config, materializing the embedded defaults at
// ~/.claude/claude-format-hooks.markdownlint.jsonc the first time it is
// needed.
//
// Unlike sqlfluff, which merges ~/.sqlfluff before any project config,
// markdownlint-cli2 only discovers config by walking up from the linted file
// as far as the working directory -- it has no user-level location at all.
// Passing --config supplies the missing layer: markdownlint-cli2 treats it
// as the BASE configuration, so a project that ships its own config still
// overrides it rule by rule, and a project that ships none stops falling
// back to stock defaults that fire on ordinary technical writing.
//
// Returns "" if the home directory can't be resolved or the file can't be
// written; the caller then runs markdownlint-cli2 with no --config at all,
// which is exactly the previous behavior. A user-level default is a
// convenience, never a precondition for formatting.
func userMarkdownlintConfig() string {
	markdownlintConfigOnce.Do(func() {
		home, err := os.UserHomeDir()
		if err != nil {
			return
		}
		markdownlintConfigPath = resolveMarkdownlintConfig(home)
	})
	return markdownlintConfigPath
}

// resolveMarkdownlintConfig is userMarkdownlintConfig's logic with the home
// directory injected, so it can be exercised against a temp dir without the
// process-wide sync.Once getting in the way.
func resolveMarkdownlintConfig(home string) string {
	path := filepath.Join(home, ".claude", userMarkdownlintConfigFile)

	// An existing file is the user's, possibly edited -- never overwrite
	// it. Only its absence triggers a write.
	if _, err := os.Stat(path); err == nil {
		return path
	}

	if err := writeIfMissing(path, markdownlintDefaults); err != nil {
		return ""
	}
	return path
}

// writeIfMissing materializes content at path without clobbering a file
// that appeared in the meantime. It writes to a temporary file in the same
// directory and links it into place with O_EXCL semantics, so two hook
// processes racing on the first .md write of a session cannot interleave a
// partial file -- one wins, the other's rename is discarded and it reads
// the winner's copy on the next call.
func writeIfMissing(path string, content []byte) error {
	dir := filepath.Dir(path)
	// 0o750 rather than 0o755: this only applies when ~/.claude does not
	// already exist, and nothing outside the user's own tooling reads it.
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, ".markdownlint-defaults-*.jsonc")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		// Best-effort cleanup: harmless if the rename below already
		// consumed the file.
		_ = os.Remove(tmpName)
	}()

	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	// os.Link fails if path already exists, which is the atomicity we want
	// here -- os.Rename would silently replace a config the user just
	// created. A failed link because the file now exists is success as far
	// as this function is concerned: the file is there either way.
	if err := os.Link(tmpName, path); err != nil {
		if _, statErr := os.Stat(path); statErr == nil {
			return nil
		}
		return err
	}
	return nil
}

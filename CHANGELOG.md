# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Initial `format-dispatch` `PostToolUse` hook: native in-process formatters
  for JSON (`encoding/json.Indent`, key-order preserving) and shell scripts
  (`mvdan.cc/sh/v3`, matches `shfmt` output exactly), plus external
  formatters routed through `bunx` (biome, markdownlint-cli2, taplo,
  prettier) or a system binary (`sqlfluff`) for everything else.
- Three-layer indent configuration for the native formatters: built-in
  defaults, `~/.claude/claude-format-hooks.json`, then the target project's
  `.editorconfig`.
- `install.sh`: builds the binary and wires it into
  `~/.claude/settings.json` as a `PostToolUse` hook, replacing any prior
  narrower biome-only hook; pre-warms `bunx`'s package cache for the four
  bunx-invoked formatters so the first file write of a session doesn't pay
  a cold npm-registry fetch against the hook's timeout.
- GitHub Actions CI (`go build`, `go vet`, `gofmt`, `golangci-lint`,
  `go test -race -cover`, `govulncheck`) on every push and pull request to
  `main`.

### Fixed

- JSON formatter: a source file already ending in `}\n` gained an extra
  blank line on every reformat (non-idempotent, ever-growing) — trailing
  whitespace is now trimmed before appending exactly one newline.
- `biome` formatter now runs from `projectRoot` on its own built-in
  defaults when no `biome.json`/`biome.jsonc` exists upward, instead of
  skipping the file entirely — consistent with how the other bunx-invoked
  formatters (prettier, taplo, markdownlint-cli2) already format any
  project unconditionally.

[Unreleased]: https://github.com/Rethunk-Tech/claude-format-hooks/commits/main

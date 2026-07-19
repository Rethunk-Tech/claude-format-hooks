# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed

- `biome` formatter and `install.sh`'s bunx prewarm invoked the npm package
  literally named `biome` — an unrelated, abandoned (2016, v0.3.3)
  environment-variable manager — instead of `@biomejs/biome`. Every
  `.ts`/`.tsx`/`.js`/`.jsx`/`.mjs`/`.cjs`/`.css`/`.jsonc` write was
  dispatching to the wrong CLI.
- External formatter invocations (`biome`, `markdownlint-cli2`, `taplo`,
  `prettier`, `sqlfluff`) now pass `--` before the target file path, so a
  file name beginning with `-` can't be parsed as a flag by the underlying
  CLI (argument injection).
- `truncate()` could split a multi-byte UTF-8 character when cutting a
  diagnostic to `maxChars`, emitting invalid UTF-8 to stderr for non-ASCII
  formatter/linter output; it now backs off to the nearest rune boundary.

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
  `main`, backed by a minimal `.golangci.yml`.
- Repo-hygiene doc set: `CHANGELOG.md`, `SECURITY.md`, `CODE_OF_CONDUCT.md`.
- Split the README into tiered docs: `HUMANS.md` (install, configuration,
  supported extensions, troubleshooting, uninstall), `AGENTS.md`
  (architecture, design rationale, package layout, invariants), and
  `CONTRIBUTING.md` (prerequisites, build/test/lint, PR workflow); added a
  `CLAUDE.md` symlink to `AGENTS.md`. README trimmed to a pitch,
  highlights, and a documentation table, and conformed to the standard
  7-part README structure (centered title, badges, quick start section).
- GitHub meta: `.github/CODEOWNERS`, `.github/dependabot.yml` (weekly
  `gomod` + `github-actions` updates), issue templates (bug report,
  feature request), and a pull request template.

### Fixed

- JSON formatter: a source file already ending in `}\n` gained an extra
  blank line on every reformat (non-idempotent, ever-growing) — trailing
  whitespace is now trimmed before appending exactly one newline.
- `biome` formatter now runs from `projectRoot` on its own built-in
  defaults when no `biome.json`/`biome.jsonc` exists upward, instead of
  skipping the file entirely — consistent with how the other bunx-invoked
  formatters (prettier, taplo, markdownlint-cli2) already format any
  project unconditionally.
- `dispatch.NewRegistry` duplicated `Config.IsDisabled`'s filtering logic
  inline instead of calling it; now calls the method (made
  case-insensitive to match how extensions are compared everywhere else).
- `runExternal` returned an empty diagnostic when a command failed with no
  captured output; it now falls back to the process error itself.
- `govulncheck` pinned to `v1.6.0` in CI instead of floating on `@latest`,
  so a new govulncheck release can't fail a PR with no corresponding code
  change.

[Unreleased]: https://github.com/Rethunk-Tech/claude-format-hooks/commits/main

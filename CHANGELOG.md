# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- `format-dispatch --uninstall` (with `--dry-run` support), removing the
  `PostToolUse` entry pointing at the installed binary — no more hand-
  editing `settings.json` to uninstall.
- The installer now backs up `settings.json` to a sibling `.bak` (a
  single rolling backup) before any real write.
- Test coverage for the five external-tool formatters (biome,
  markdownlint-cli2, taplo, prettier, sqlfluff) and `runExternal`,
  previously all at 0% since CI installs neither `bunx` nor `sqlfluff`;
  and for `cmd/format-dispatch`'s core `run()` dispatch path (extension
  gate, project-root/vendored-dir checks, dispatch, diagnostics),
  previously untested beyond its pure helpers.

### Changed

- `internal/installer` no longer round-trips `settings.json` through
  `map[string]json.RawMessage` + `json.Marshal`, which silently
  alphabetized every top-level key and every `hooks.*` entry on each
  `--install` run. A new order-preserving `orderedMap` keeps every
  untouched key exactly where it was.
- All tests now use `github.com/go-quicktest/qt` for uniformity; a few
  packages still used plain `if`/`t.Errorf`/`t.Fatalf` assertions.

### Fixed

- `cmd/format-dispatch`'s `run()` took stdin as a hardcoded `os.Stdin`
  read, making its core dispatch logic untestable; it now takes an
  `io.Reader` parameter.
- Removed `Result.Changed`, a field the external formatters computed via
  a wasted before/after file read on every invocation but no caller ever
  read.
- `--install`/`--uninstall` are mutually exclusive top-level subcommands
  instead of `--uninstall` being a modifier taken after `--install`.

## [0.1.0] - 2026-07-19

### Added

- Unit tests for `internal/installer`'s `settings.json` mutation logic:
  fresh install, missing settings file, idempotent re-install, replacing
  the old narrow biome-only hook, and preserving unrelated `hooks.*`
  entries — none of which had coverage under the old `jq` implementation.
- Unit tests for `internal/dispatch`, `internal/config`, `internal/hookio`
  (0% -> 100% each), `cmd/format-dispatch`'s `within()`/`configPath()`
  helpers, and a shell-formatter idempotency suite mirroring the existing
  JSON one. Uses `github.com/go-quicktest/qt` (already present via
  `mvdan.cc/sh/v3`'s own test dependencies) for the new packages.
- CI now enforces a 45% total-coverage floor (`go tool cover -func`) so
  this gap can't silently recur.
- `gosec` added to `.golangci.yml`'s linter set, given this tool's entire
  job is subprocess execution and file writes from external input.
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

### Changed

- `install.sh`'s `settings.json` wiring (the `jq` filter that removed a
  stale/old-biome-only `PostToolUse` entry and appended the new one) moved
  into a native `format-dispatch --install [--dry-run]` Go subcommand
  using `encoding/json`, matching the "native Go over external tools" bar
  the rest of this project holds itself to. `install.sh` is now a thin
  wrapper: it still builds the binary and pre-warms `bunx`'s cache (no Go
  equivalent for that step), then delegates to `--install`. `jq` is no
  longer a prerequisite.
- README: dropped the meta/narrative "generalizes a hand-written
  per-project hook" framing in favor of describing what the tool does.
- De-duplicated the build/vet/lint/test command block that had drifted
  out of sync between AGENTS.md and CONTRIBUTING.md (CONTRIBUTING.md's
  copy didn't mention the new coverage floor); AGENTS.md § Commands is
  now the single canonical copy, CONTRIBUTING.md points to it.
- Added GitHub repo topics (`claude-code`, `hooks`, `formatter`, `linter`,
  `golang`, `developer-tools`, `cli`, `biome`, `prettier`) — previously
  unset.

### Fixed

- `cmd/format-dispatch/main.go`'s `within()` boundary check now resolves
  symlinks (`filepath.EvalSymlinks`) on both the target path and
  `$CLAUDE_PROJECT_DIR` before comparing, so a symlink inside the project
  that points outside it can no longer slip past the boundary check as a
  pure string-prefix match.
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

[Unreleased]: https://github.com/Rethunk-Tech/claude-format-hooks/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/Rethunk-Tech/claude-format-hooks/releases/tag/v0.1.0

# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Native in-process Go formatter (`.go`, via stdlib `go/format.Source`) —
  the same engine `gofmt` itself is built on, so output matches exactly
  with no subprocess. Unlike JSON, there's no fidelity trade-off to weigh:
  Go source has exactly one canonical formatting.
- Extensions already covered by an integrated tool, added at zero
  integration cost: `.mts`/`.cts` (biome, alongside the existing
  `.mjs`/`.cjs`), `.markdown` (markdownlint-cli2, alongside `.md`), and
  `.scss`/`.less`/`.graphql`/`.gql` (prettier, which supports all four
  natively; biome has no SCSS/Less support).

### Fixed

- `format-dispatch --install`/`--uninstall` silently ignored any argument
  after the subcommand other than exactly `--dry-run` (e.g. a typo like
  `--dryrun`, or a stray extra argument) instead of erroring — it now
  prints usage and exits 1, matching the top-level unrecognized-flag
  behavior.

## [0.2.0] - 2026-07-19

### Added

- `format-dispatch --uninstall` (with `--dry-run` support), removing the
  `PostToolUse` entry pointing at the installed binary — no more hand-
  editing `settings.json` to uninstall.
- The installer now backs up `settings.json` to a sibling `.bak` (a
  single rolling backup) before any real write.
- Test coverage for the five external-tool formatters (biome,
  markdownlint-cli2, taplo, prettier, sqlfluff) and `runExternal`,
  previously all at 0% since CI installs neither `bunx` nor `sqlfluff`.
  `internal/formatters` coverage: 43.6% -> 85.6%.
- Test coverage for `cmd/format-dispatch`'s core `run()` dispatch path
  (extension gate, project-root/vendored-dir checks, dispatch,
  diagnostics), previously untested beyond its pure helpers.
  `cmd/format-dispatch` coverage: 20.0% -> 65.7%; total: 54.2% -> 79.4%.
- Test coverage for `installer.DefaultOptions`, `cmd/format-dispatch`'s
  `runInstall` (including its `installer.DefaultOptions` error path), the
  `json`/`shell` formatters' `Name()` methods, and the shared
  `bunxFormatter.Format` success/failure paths — all previously at 0-33%.
  Total coverage: 78.9% -> 85.3%.
- `format-dispatch --version` (via `runtime/debug.ReadBuildInfo`, no
  ldflags needed) and `format-dispatch --help`/`-h`. An unrecognized flag
  now prints usage to stderr and exits 1 instead of silently falling
  through to reading stdin, which would otherwise hang forever in an
  interactive terminal. Total coverage: 85.3% -> 84.4% (new CLI surface
  outpaced its own test coverage slightly).

### Changed

- `cmd/format-dispatch`'s `run()` now takes stdin as an `io.Reader`
  parameter instead of a hardcoded `os.Stdin` read, enabling the
  dispatch-path test coverage above.
- All tests standardized on `github.com/go-quicktest/qt` (a few packages
  still used plain `if`/`t.Errorf`/`t.Fatalf` assertions); no behavior
  change.
- CI's pinned `golangci-lint` version: v2.9.0 -> v2.12.2, so local runs
  and CI use the same linter build by default.
- Modernized manual loops onto the standard `slices`/`maps` packages
  (available since Go 1.21; go.mod is on 1.26.5): `dispatch.NewRegistry`'s
  disabled-extension filter now uses `maps.DeleteFunc`;
  `dispatch.InVendoredDir`, `config.IsDisabled`, `biome.findUpward`, and
  `installer`'s `hasBin`/`keepEntry` now use `slices.ContainsFunc`; and
  `installer.Wire`/`Unwire`'s filter-by-append loops (with the
  `entries[:0:0]` zero-capacity idiom) now use `slices.DeleteFunc`. No
  behavior change.
- `installer.DefaultOptions`'s two env-var-with-fallback assignments now
  use `cmp.Or` (Go 1.21) instead of an `if val == "" { val = fallback }`
  pair.
- A per-file formatter timeout now uses `context.WithTimeoutCause`
  instead of `context.WithTimeout`; `runExternal` prefers
  `context.Cause(ctx)` for its diagnostic when the timeout actually
  fired, instead of a generic "signal: killed".
- All tests now use `t.Context()` (Go 1.24) instead of
  `context.Background()`, so each test's context is canceled at its own
  cleanup instead of never.
- CI: added an explicit least-privilege `permissions: contents: read`
  block, a `concurrency` group to cancel superseded runs, and pinned
  `actions/checkout`, `actions/setup-go`, and `golangci-lint-action` to
  commit SHAs instead of mutable version tags. The coverage floor was
  raised from 45% to 75% (actual coverage was already 85.3%, so the old
  floor gated nothing).
- `.golangci.yml`: enabled `contextcheck`, `errorlint`, `gocritic`,
  `predeclared`, `revive`, `unconvert`, and `wastedassign` alongside the
  existing `standard` + `gosec` set; added the missing doc comments
  `revive`'s exported-symbol check surfaced on 11 previously-undocumented
  exported types/functions.

### Removed

- Dead `Result.Changed` field, and the wasted before/after file reads
  each external formatter (biome, bunx-based, sqlfluff) performed solely
  to populate it — no caller ever read `.Changed`, only `.Err` and
  `.Diagnostic`.

### Fixed

- `internal/installer.Wire` no longer round-trips `settings.json` through
  `map[string]json.RawMessage` + `json.Marshal`, which silently
  alphabetized every top-level key and every `hooks.*` entry on each
  `--install` run. A new order-preserving `orderedMap` keeps every
  untouched key exactly where it was.
- `--install`/`--uninstall` are now mutually exclusive top-level
  subcommands, instead of `--uninstall` being a modifier taken after
  `--install` (`--install --uninstall`), which read like two
  contradictory flags.
- Shell formatter: a non-positive `shell.indentSize` (via user config or
  `.editorconfig`) converted straight to `uint`, wrapping around to a huge
  value instead of erroring — clamp to a 1-space minimum, matching the
  JSON formatter's existing `max(spec.Size, 1)`.

## [0.1.0] - 2026-07-19

Initial release.

[Unreleased]: https://github.com/Rethunk-Tech/claude-format-hooks/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/Rethunk-Tech/claude-format-hooks/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/Rethunk-Tech/claude-format-hooks/releases/tag/v0.1.0

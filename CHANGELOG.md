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
- Project-level formatter opt-out: a `.claude-format-hooks.json` at the
  project root (same schema as the user-level config; only `disabled` is
  consulted) lets a project skip a specific formatter for itself, without
  every operator changing their global config. A malformed project
  config is ignored (diagnostic to stderr) rather than blocking, same as
  a malformed user config.
- Opt-in troubleshooting log: when `$CLAUDE_FORMAT_HOOKS_LOG` names a
  file, one line (timestamp, path, formatter, outcome, duration) is
  appended per invocation — for diagnosing "why didn't my file get
  formatted" (and a slow-feeling hook, usually a cold `bunx` fetch)
  without changing the hook's default silent-on-success behavior. Off
  unless set; a logging failure never affects the hook's exit code.
- Native-adjacent Python (`.py`) support: `ruff format` (preferred) or
  `black` as a fallback, whichever is on `PATH` — ruff wins if both are.
  Neither has a Go equivalent, same reasoning as `sqlfluff`/SQL.
- Rust (`.rs`) support via `rustfmt` — no Go equivalent, and (like Go
  itself) no fidelity trade-off to weigh: rustfmt is the canonical
  formatter for Rust, the same relationship gofmt has to Go.
- Terraform/HCL (`.tf`) support via `terraform fmt` — surfaced by a fleet
  survey as the one clean, low-effort gap worth closing immediately (a
  single canonical formatter, real hand-authored `.tf` files already
  present in this fleet); Kotlin/XML/Lua/Gradle were also surveyed and
  passed over (fragmented tooling, too rare, or generated/config-
  sensitive content), and `.proto` was flagged as a near-term watch item
  once protobuf usage spreads past its one current repo.
- `internal/formatters/binpath.go`: a disk-backed, 30-second-TTL negative
  cache for "is this external tool installed" (`lookPath`, replacing
  every formatter's direct `exec.LookPath` call). Each format-dispatch
  invocation is a fresh, short-lived process, so an in-memory cache
  wouldn't survive between file writes — without this, a project missing
  `bunx`/`sqlfluff`/`ruff`/`black`/`rustfmt` re-walked `$PATH` on every
  single file write. The check reruns after the TTL, so installing the
  missing tool mid-session is picked up without restarting.
- `internal/diskcache`: extracted the disk-backed TTL cache underlying
  `lookPath` into its own package (shared by `internal/formatters` and
  `internal/config`, which may not import each other in the direction
  this would otherwise require), then used it for two more hot,
  repeats-every-invocation paths that were flagged and initially passed
  over as too marginal on their own, but were worth doing once the
  cache mechanism itself was already shared infrastructure: biome's
  upward walk for `biome.json`/`biome.jsonc` (`cachedFindUpward` in
  `internal/formatters/biome.go` — also caches the common "no config
  anywhere" case) and EditorConfig resolution for the two native
  formatters (`internal/config/config.go`'s `resolveIndent`, keyed on
  the file path so re-editing the same file benefits immediately). Both
  use the same reasoning as the binary-lookup cache: the underlying
  result rarely changes mid-session, so a stale hit just costs one
  extra recompute after the TTL, never a wrong one.
- `internal/formatters/writefile.go`: a shared `writeFormatted` helper
  (stat the existing file for its mode, fall back to a default, write)
  replacing three near-identical copies of the same block in `json.go`,
  `golang.go`, and `shell.go` — and, since none of the three had a test
  actually asserting the mode-preservation behavior they all claimed,
  added one (`TestShellFormatterPreservesExecutableBit`) that would have
  caught a regression here.
- `.github/workflows/release.yml`: a `v*` tag push now cross-compiles
  `format-dispatch` for linux/darwin (amd64+arm64) from a single
  `ubuntu-latest` runner (pure Go, no cgo) and publishes a GitHub Release
  with the binaries and their sha256sums via `gh release create` — an
  install path with no local Go toolchain required, alongside
  `install.sh`'s existing build-from-source path.

### Changed

- `cmd/format-dispatch`'s `run()` had grown to 106 lines / cyclomatic
  complexity 19 (over `cyclop`'s default threshold of 10) after this
  batch's additions. Extracted `buildRegistry`, `resolveTarget`, and
  `projectDisables` so each guard is independently testable and `run()`
  reads as a sequence of checks. No behavior change.
- CI now runs build/vet/gofmt/test across a
  `ubuntu-latest`/`macos-latest`/`windows-latest` matrix instead of
  `ubuntu-latest` only, catching platform-specific bugs (path
  separators, symlink/file-mode differences) in code that already had
  Windows-aware branches with zero Windows test coverage. `golangci-lint`
  and `govulncheck` were split into their own `ubuntu-latest`-only job,
  since static analysis and vulnerability data don't vary by OS — running
  them per matrix leg would have tripled their cost for no signal.

### Fixed

- `internal/installer`'s writes to `settings.json` (and its `.bak`
  backup) went through a plain `os.WriteFile`, which isn't atomic — a
  process killed or crashing mid-write could leave the operator's entire
  Claude Code hook configuration (every hook, not just this one)
  truncated. Both writes now go through a new `writeAtomic` (temp file in
  the same directory, then `os.Rename`), which on both POSIX and Windows
  either fully replaces the target or doesn't touch it at all.
- Added `.gitattributes` (`* text=auto eol=lf`): the new `windows-latest`
  CI leg failed immediately at `gofmt`, since Windows Git's default
  `core.autocrlf` checked out every tracked file as CRLF with no
  `.gitattributes` forcing normalization, and `gofmt` is line-ending-
  sensitive. Verified locally (clone with `-c core.autocrlf=true`
  before/after) rather than assumed.
- Several tests carried POSIX-only assumptions that only broke on the
  new `windows-latest` CI leg: the `payload()` test helper built its
  JSON literal by raw string concatenation, corrupting it on any
  Windows path (backslashes need escaping) — now uses `json.Marshal`;
  two tests hardcoded POSIX-style path literals instead of building
  them with `filepath.Join`; and two tests simulated "no home
  directory" by clearing only `$HOME`, which `os.UserHomeDir()` never
  reads on Windows (it reads `%USERPROFILE%`) — now clears both.
- `format-dispatch --install`/`--uninstall` silently ignored any argument
  after the subcommand other than exactly `--dry-run` (e.g. a typo like
  `--dryrun`, or a stray extra argument) instead of erroring — it now
  prints usage and exits 1, matching the top-level unrecognized-flag
  behavior.
- `run()` loaded the user's config file (an `os.ReadFile`) on *every*
  invocation before checking whether the extension had a formatter at
  all — so every write to a `.py`, `.rs`, or other never-formatted file
  paid a file read the documented "an unsupported extension is an
  instant no-op: one `filepath.Ext` call and one map lookup, nothing
  else" contract explicitly promised it wouldn't. Added
  `dispatch.KnownExtension`, a config-independent check against the
  static extension superset (derived from a `Registry` built with an
  empty `Config`, so it can't drift from `NewRegistry`'s own list), and
  moved it before `buildRegistry()`. Config is now loaded only for
  extensions that could possibly be handled; a since-disabled extension
  gets a new, more accurate invocation-log outcome
  (`skip: disabled by config`) instead of being lumped in with
  `skip: unsupported extension`.

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

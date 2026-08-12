# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- PostToolUse stdin accepts `tool_result.filePath` as well as
  `tool_response.filePath`; when both are set, `tool_response` wins.
- Terraform-owned suffixes use `tofu fmt` when `terraform` is not on
  `PATH`. `disabledFormatters: ["terraform"]` still skips both binaries.
- Route `.tftest.hcl`, `.tfmock.hcl`, and `.tfquery.hcl` through Terraform
  formatting; bare `.hcl`, `.tf.json`, and `.tfvars.json` are not registered
  Terraform extensions.
- `--upgrade` downloads and verifies the latest platform release binary, then
  atomically replaces the installed hook without rewriting `settings.json`;
  `--upgrade --dry-run` previews the planned asset and path offline without
  writing.
- `.proto` formatting via `buf format -w`, following the same
  system-binary pattern as `terraform fmt` and `rustfmt` (skipped silently
  when `buf` is not on `PATH`).
- Jupyter Notebook (`.ipynb`) formatting via `ruff format` (preferred) or
  `black` when its notebook support is installed; both tools remain optional
  and the extension is skipped silently when neither is available.
- Extensionless shell-shebang scripts are formatted by the native shell
  formatter when their shebang names `bash`, `sh`, `zsh`, or `dash`.
- `format-dispatch --check PATH...`, a non-mutating CI gate that reports
  which files a formatter would change and exits 1 if any would. It answers
  the same question every local write answers, so a repo can enforce
  formatting in CI without installing the formatters — a file whose tool is
  absent is reported as fine rather than failing the build. Exit 2 is
  reserved for bad invocation so a broken pipeline stays distinguishable
  from a real failure.
- Windows release binaries (amd64 + arm64), with
  `format-dispatch-windows-*.exe` assets and `format-dispatch.exe` as the
  installed Windows basename. CI already ran the full test matrix on
  `windows-latest`, so the platform was being validated on every push and
  then withheld from every release.
- `--install` now provisions the bunx-dispatched formatters globally
  (replacing `install.sh`'s cache pre-warm), so no registry fetch ever
  happens inside the per-file budget.
- `disabledFormatters` provides case-insensitive formatter-name opt-outs in
  user and project config. Disabling `biome` covers all Biome-owned
  extensions while `.json` falls back to native formatting; `--check`
  applies the same opt-outs as the live hook.

### Changed

- `--install` writes a PostToolUse matcher of
  `Write|Edit|MultiEdit|NotebookEdit`. Re-install replaces the previous
  format-dispatch entry.
- Biome-backed extensions run `biome format --write` instead of
  `biome check --write`, so lint-only findings no longer fail the hook.
  Legacy settings entries that contain `biome check --write` are still
  replaced on `--install`.
- `--check` reuses the project-biome-disabled registry per project root,
  avoiding a `NewRegistry` rebuild for each `.json` file.

### Fixed

- gosec CI clears: `checkProjectRoot` stats the Abs root (G703), shebang
  peek documents the intentional Open of the hook target (G304), and
  `--upgrade` creates its binary parent directory as `0o750` (G301).
- Portable CI tests: darwin skips non-portable cwd-removal cases; Windows
  skips Unix permission asserts on upgrade and POSIX-shell notebook
  fixtures.
- `disabledFormatters` entries are trimmed before matching.
- `--check` now resolves multi-dot registered suffixes and extensionless
  shell shebangs the same way the live hook does, so CI no longer skips
  those targets.
- Black's notebook fallback only treats missing-jupyter-extra diagnostics
  as a silent skip; unrelated failures that merely mention "notebook" still
  surface as diagnostics.
- `Registry.Dispatch` returns `Skipped` instead of panicking when called
  with an unregistered extension.
- `--install` no longer rewrites an existing `"args": []` as `"args": null`
  in `settings.json`. The key is now omitted entirely, which is what the
  schema expects for a hook that takes no arguments and the only form that
  survives a rewrite unchanged.
- The per-file formatter budget (25s) exceeded the timeout the installer
  wrote into `settings.json` (30s) and both exceeded the intended 5s, so on
  a correctly-configured install Claude Code killed the process before the
  hook's own timeout could report why. Now 4s under a 5s harness limit.

- User-level markdownlint defaults. markdownlint-cli2 discovers config only
  by walking up from the linted file as far as the working directory, so a
  project with none of its own fell back to stock rules — several of which
  (notably `MD013` line-length) fire constantly on technical writing and
  cannot be auto-fixed, turning every `.md` write into a diagnostic nothing
  could act on. The hook now passes a base config via `--config`,
  materialized once at
  `~/.claude/claude-format-hooks.markdownlint-cli2.jsonc` so it stays
  editable rather than locked in the binary. A project's own config still
  layers on top and wins rule by rule.

  This closes the gap that made projects carry a `.markdownlint-cli2.jsonc`
  purely to silence defaults.

- SQL defaults. On the first `.sql` write, format-dispatch materializes
  `~/.sqlfluff` with an ANSI dialect when it does not already exist. Existing
  user configuration is preserved, and project configuration still wins.

- `markdownlint-cli2` now resolves the patched `js-yaml` 5.2.2 release, so
  the installer no longer needs a global dependency override for
  GHSA-pm4m-ph32-ghv5.

### Documentation

- Documents conditional Biome routing for `.json`, native Go and protobuf
  support, Windows release targets, installer `ProvisionTools`, and the
  `--check` CI gate.
- Documents `disabledFormatters` configuration in `HUMANS.md` for user- and
  project-level formatter opt-outs.

## [0.3.0] - 2026-07-19

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
- `cmd/format-dispatch`'s `versionString` split into a thin wrapper plus
  `versionStringFrom(info *debug.BuildInfo)`, the one seam
  `debug.ReadBuildInfo` offers — its revision-truncation, dirty-suffix,
  and no-VCS-metadata branches previously depended on whatever happened
  to be embedded in the test binary itself (in practice, never populated
  under `go test`), making them untestable without this. No behavior
  change.
- Closed a batch of genuinely easy test-coverage gaps across
  `internal/diskcache`, `internal/config`, `internal/formatters`, and
  `internal/installer` — mostly the "point at a directory instead of a
  file" trick this repo already used to distinguish "missing" from
  "exists but unreadable" error branches, plus a couple of malformed-JSON
  cases in `orderedMap.UnmarshalJSON` pinned via a spike (calling it
  through `json.Unmarshal`, which fully validates input before ever
  delegating to a custom `Unmarshaler`, silently never reached the code
  under test — verified before writing the assertions). Total coverage:
  87.8% -> 92.9%.

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
- `run()` loaded the user's config file (an `os.ReadFile`) on _every_
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

[Unreleased]: https://github.com/Rethunk-Tech/claude-format-hooks/compare/v0.3.0...HEAD
[0.3.0]: https://github.com/Rethunk-Tech/claude-format-hooks/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/Rethunk-Tech/claude-format-hooks/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/Rethunk-Tech/claude-format-hooks/releases/tag/v0.1.0

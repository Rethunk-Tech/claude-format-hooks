# TODO — Follow-on and carry-forward

Planning-only backlog. Items below are not yet tracked elsewhere (no open
GitHub issues; this file is the ledger). Prefer landing one coherent unit
per commit; do not treat this as a mandate to expand scope mid-PR.

Wave 1 (2026-08-09) landed: Round 1+8 docs; `--check` opt-out/root/
`.fmtcheck`/per-file timeout + vendored/`within` guards; `.tfvars`/`.pyi`;
expanded `vendoredDirs`; PATH-first bunx fallback; atomic native writes
(symlink-safe + stale-skip); diskcache prune; sqlfluff `~/.sqlfluff`
bootstrap. Multi-dot Terraform suffixes stayed deferred (below).

Wave 2 (2026-08-09) landed: Windows `format-dispatch.exe` basename +
legacy Wire/Unwire dedupe; extensionless shell-shebang peek (incl. CRLF)

+ `Dispatch(ext)` glue; `.ipynb` via ruff/black-notebook; docs/tests
aligned.

Wave 3 (2026-08-09) landed: longest-suffix `ResolveExtension`; Terraform
`.tftest.hcl`/`.tfmock.hcl`/`.tfquery.hcl`; `--check` shebang parity;
nil-safe `Dispatch`; black notebook-missing heuristic tighten;
`TestExternalFormatterNames` notebook row; `installer.Upgrade` +
`--upgrade` CLI glue; HUMANS/CHANGELOG. Fixup: unique `writeAtomic`
temps, offline `--upgrade --dry-run`, Windows in-use note, coverage for
multi-dot `--check` / nil Dispatch / upgrade asset errors.

Wave 4 (2026-08-09) landed: named checksum lines + 64 MiB upgrade
download cap + `CLAUDE_FORMAT_HOOKS_RELEASE_API`; black notebook
ModuleNotFoundError-shaped skip; precomputed `registeredSuffixes`;
`--check` lazy registry; CLI `--upgrade` happy-path test;
`install.sh --upgrade` (+ dry-run) with unrecognized-arg reject; HUMANS
env docs. Fixup: document release API override trust.

Wave 5 (2026-08-09) landed: `fetchHTTP` prefers HTTP status on oversized
error bodies + non-2xx unit coverage; CLI `--upgrade --dry-run` happy
path; black notebook dotted-module reject + ModuleNotFoundError
fixtures; `ResolveExtension("")` row; `--check` user-disable before
registry build. Fixup: single config load for `--check` + mixed
disabled/enabled order coverage.

Wave 6 (2026-08-09) landed: capped oversized non-2xx error-body test;
`disabledFormatters` name-based opt-out (registry + jsonRouter biome
skip + hook/`--check` + HUMANS/CHANGELOG); drop empty `js-yaml`
`bunGlobalOverrides` after markdownlint-cli2 0.23.2 → js-yaml 5.2.2;
fleet survey all no-go (`.vue`/`.svelte`/`.astro`/`.nix`/`.zig` = 0).
Fixup: canonical Name() list + notebook distinction; json/jsonc
disabledFormatters tests; EditorConfig numbering glue.

Wave 7 (2026-08-10) landed: `IsFormatterDisabled` trims list entries;
caller-agnostic godoc; CHANGELOG Documentation bullet for
`disabledFormatters`; router project-disable invariant comments on
`jsonRouter` / `registryWithProjectConfig`; whitespace-only entry test.
Audit: 0 must-fix; memoization / remote CI / fleet resurvey stay
deferred.

Wave 8 (2026-08-10) landed: `--check` reuses the project-biome-disabled
registry per project root; CHANGELOG and fleet resurvey updated.
Wave 8b polish: `.proto`/`buf` + native `Name()` test coverage;
CHANGELOG Fixed bullet for `disabledFormatters` trim. Audit fixup:
shared `projectRebuildsJSONRegistry` predicate + `registryForCheck`
godoc; Wave-9 audit follow-ons tracked below.

Wave 9 (2026-08-10) landed: direct `registryForCheck` memoization proof
with per-project-root pointer reuse; multi-JSON memo coverage with an
upward `biome.json` sibling; CHANGELOG Fixed note. Audit fixup: removed
the stale Wave-8 pointer; per-project-root isolation remains the deliberate
test floor.

Wave 10 (2026-08-10) landed: negative `registryForCheck` non-cache path;
direct `projectRebuildsJSONRegistry` table; drop test-only CHANGELOG Fixed
bullet; fleet resurvey wave-10 (all zero / no-go). Audit fixup: assert
memo path rebuilds a distinct registry; `.JSON` non-cache case; trimmed
biome disable through the predicate.

Wave 11 (2026-08-10) landed: direct `checkProjectRoot` /
`resolveTarget` tables; dangling-symlink `writeFormatted` no-op;
Terraform multi-dot external paths; `TestName` parity via shared
`supportedExtensions`; `bunGlobalDir` HOME fallback. Audit fixup:
`t.Setenv` missing-home isolation; optionals tracked below.

Wave 12 (2026-08-10) landed: gosec G703/G304/G301 CI clears
(`Stat(root)`, shebang nolint, upgrade `MkdirAll` `0o750`); darwin
cwd-gone skips + nil paths for directory fallback; Windows upgrade mode
assert gates + notebook POSIX-stub skip; dangling-symlink
`Readlink`/`ReadFile` asserts. Audit fixup: `ReadFile(link)` not-exist.
Local `go vet` / `go test -race` / `golangci-lint` green; remote badge
still needs an operator push of ahead `main`.

Wave 13 (2026-08-10) landed: coverage floors for
`writeFormatted`/`writeIfMissing`, diskcache `prune`/`cacheEntryExpired`,
installer `writeAtomic`/`parseSettings`/`applyChange`, and `--check`
`wouldReformat`/`copyBeside`/`collectCheckTargets`. Audit fixup: drop
unsafe Write + flaky Close descriptor races; chmod-denial fixtures probe
effective writability before asserting; prune refresh asserts `Get`
returns fresh. Quick CI (`go build`/`go vet`/`gofmt -l`) green; remote
badge still needs an operator push of ahead `main`.

---

## Residual — Wave-11 / Wave-12 deferred

### `resolveTarget` Rel-error skip

`filepath.Rel` failure still returns `skip: relative path error` and
stays uncovered. Wave contract excludes mock-based `Abs`/`Rel` failure
tests; reopen only with a portable fixture that forces `Rel` to fail
without production hooks.

### Darwin cwd-gone skip is blanket `GOOS`

Wave 12 skipped cwd-removal subtests on `darwin` rather than probing
`Getwd` after `RemoveAll`. Correct for current macOS; coarser than a
probe. Optional: shared helper + probe if darwin ever stops succeeding
`Getwd` after unlink.

### Upgrade parent-dir mode `0750` unasserted

`upgrade.go` creates the binary parent with `0o750`; no test checks the
directory mode after a fresh install. Low risk; add only if mode
regressions become a concern.

---

## Residual — Ops

### Confirm green CI after push of Wave-13 tip

Local `main` is ahead of `origin/main` with Wave-13 coverage + audit
fixups (`7ca342c` tip at closeout). Push is not authorized from this
session. After an operator push, confirm ubuntu lint +
ubuntu/macOS/Windows test are green on the new tip (prior red was run
`31445363724` on `36d7fe3`).

**Acceptance**

+ Remote CI green on the Wave-13 tip for lint + test matrix.

### Fleet re-survey (periodic)

Wave-6 survey across `/usr/local/src/com.github/Rethunk-Tech/` found
**zero** hand-authored `.vue`/`.svelte`/`.astro`/`.nix`/`.zig` under
vendored-dir exclusions — all **no-go**. Waves 7–10 resurveys reconfirmed
zero; see `.orchestrate/fleet-survey-wave10.md`.
Re-run when the fleet gains candidate sources; any go still needs
dispatch registration + HUMANS row together. For `.nix`, pick one of
`nixfmt`/`alejandra` by PATH dominance.

---

## Residual — Wave-13 deferred (coverage / fixture optionals)

Wave-13 closed the four coverage follow-ons. Remaining branches need a
deterministic seam or stay acknowledged gaps — no descriptor races, no
`unsafe`, no Abs/Rel production hooks.

### `writeAtomic` Write / Close error branches

Dropped flaky unsafe Write and `/proc/self/fd` Close fixtures. Reopen
only with a portable deterministic fault (e.g. RLIMIT_FSIZE-style Write
or an injectable close seam). Owns: `internal/installer/installer_test.go`.

### `writeAtomic` Chmod fault injection (optional)

`TestWriteAtomicChmodFailure` still races raw descriptors and may write
large buffers. Prefer a deterministic chmod-error seam or keep as
acknowledged optional coverage. Owns: `installer_test.go`.

### `copyBeside` `crypto/rand.Read` error

Unreachable under Go stdlib (entropy failure aborts). Do not mock; leave
below 90% unless a production seam appears. Owns:
`cmd/format-dispatch/check_test.go`.

### Native write Chmod / Close fault seams

`writeFormatted` Chmod/Close failures still need filesystem-specific or
injected faults. Owns: `internal/formatters/writefile_test.go`.

### `collectCheckTargets` WalkDir error on privileged runners

`TestCollectCheckTargetsReportsWalkError` skips when mode-000 stays
readable. Keep skip or add a controlled walk-error seam. Owns:
`check_test.go`.

---

## Explicitly out of scope (do not re-open without new evidence)

+ CLI framework (Cobra/urfave/kong/ffcli) — decided v0.2.0; see AGENTS.md.
+ Native YAML/TOML/HTML Go formatters — fidelity failures already recorded
  in AGENTS.md Architecture.
+ Kotlin / XML / Lua / Gradle formatters — surveyed and passed over in
  0.3.0.
+ Requiring per-tool project config before formatting — removed for biome;
  do not reintroduce.
+ Shared `internal/atomicfile` extract for installer + formatters — both
  paths are atomic independently; extract only if a third caller appears.
+ Overloading bare formatter names into `disabled` (wave 6 chose separate
  `disabledFormatters` key).
+ `runCheck` NewRegistry call-count / shared-cache integration proof —
  Wave-9 unit floor is deliberate; Wave-10 closed the complementary
  negative path without production instrumentation.

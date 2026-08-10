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
CHANGELOG Fixed bullet for `disabledFormatters` trim. Audit: 0 must-fix /
0 should-fix; optional carry-forwards below.

---

## Residual — Wave-8 audit carry-forwards (optional)

### Prove `--check` registry memoization (not only output parity)

`TestCheckReusesProjectDisabledBiomeRegistryForMultipleJSONFiles` asserts
both unformatted `.json` files would reformat under project-disabled
biome. Contracts chose behavior-first; removing the cache would still
pass. Optional: a call-count or equivalent harness that fails if
`NewRegistry` rebuilds per file for the same `projectRoot`.

**Acceptance**

+ Test fails if the per-`projectRoot` cache is removed, or leave-as-is note
  that behavioral parity remains the deliberate floor.

### Godoc on `registryForCheck`

`registryForCheck` caches only the project-biome-disabled `.json`
`NewRegistry` rebuild, keyed by `projectRoot`. A one-line godoc would
match `registryWithProjectConfig` style.

**Acceptance**

+ Short godoc on `registryForCheck`, or leave-as-is.

---

## Residual — Ops

### Diagnose and clear red CI on `origin/main`

Remote tip is still `2fd5ab2`, with failed CI workflow run
`30131468133` (test×3 + lint); job logs remain **expired / unavailable**.
Local `main` is ahead with green `go build`/`go vet`/scoped race tests.
Clearing the badge still needs an explicit operator push of the ahead local
`main` (not authorized). A later Dependabot `go_modules` run on the same SHA
succeeded and does not clear the CI workflow failure.

**Acceptance**

+ Identified failing check and root cause recorded (logs gone — stale tip).
+ A subsequent `main` push is green on ubuntu/macOS/Windows test + lint.

### Fleet re-survey (periodic)

Wave-6 survey across `/usr/local/src/com.github/Rethunk-Tech/` found
**zero** hand-authored `.vue`/`.svelte`/`.astro`/`.nix`/`.zig` under
vendored-dir exclusions — all **no-go**. Wave-7 audit and the Wave-8
resurvey reconfirmed zero; see `.orchestrate/fleet-survey-wave8.md`.
Re-run when the fleet gains candidate sources; any go still needs
dispatch registration + HUMANS row together. For `.nix`, pick one of
`nixfmt`/`alejandra` by PATH dominance.

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

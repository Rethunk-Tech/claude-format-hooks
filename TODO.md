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

---

## Residual — Wave-6 audit carry-forwards (optional)

### Trim whitespace in `disabledFormatters` entries

`IsFormatterDisabled` uses `EqualFold` without `TrimSpace`, so a value
like `"biome "` silently fails to match.

**Acceptance**

+ Leading/trailing whitespace on list entries is ignored (or rejected
  loudly at Load with a clear diagnostic).

### CHANGELOG Documentation bullet for `disabledFormatters`

Unreleased Documentation section still omits the new opt-out; Added
already covers the feature.

**Acceptance**

+ Documentation bullet mentions `disabledFormatters` / HUMANS config.

### Clarify `IsFormatterDisabled` comment (user vs project)

Helper is reused for project configs via `projectDisables`; comment still
says "user".

**Acceptance**

+ Comment names both callers (or is caller-agnostic).

### Cache project-biome-disabled registry in `--check`

`registryWithProjectConfig` rebuilds `NewRegistry` per `.json` when the
project disables biome. Fine for small trees; optional memoization if
`--check` on large monorepos shows cost.

**Acceptance**

+ Same merged registry reused across files that share the same user+project
  disable set, or measured no-op leave-as-is note.

### Router-style formatters need explicit project-disable wiring

`jsonRouter.Name()` is `"json"` while biome may run underneath, so
project-level `"biome"` disable needs `registryWithProjectConfig`. Any
future router that delegates to another `Name()` must get the same hook/
`--check` treatment — do not assume registry name-filter alone is enough.

**Acceptance**

+ Documented invariant near `jsonRouter` / `registryWithProjectConfig`, or
  a shared helper that future routers must call.

---

## Residual — Ops

### Diagnose and clear red CI on `origin/main`

Remote tip `2fd5ab2` still shows failed CI workflow run
`30131468133` (test×3 + lint). Job logs are **expired / unavailable**.
Local `main` is ahead with green `go build`/`go vet`/scoped race tests;
clearing the badge needs an explicit operator push (not authorized). A
later Dependabot `go_modules` run on the same SHA succeeded and does not
clear the CI workflow failure.

**Acceptance**

+ Identified failing check and root cause recorded (logs gone — stale tip).
+ A subsequent `main` push is green on ubuntu/macOS/Windows test + lint.

### Fleet re-survey (periodic)

Wave-6 survey across `/usr/local/src/com.github/Rethunk-Tech/` found
**zero** hand-authored `.vue`/`.svelte`/`.astro`/`.nix`/`.zig` under
vendored-dir exclusions — all **no-go**. Re-run when the fleet gains
candidate sources; any go still needs dispatch registration + HUMANS row
together. For `.nix`, pick one of `nixfmt`/`alejandra` by PATH dominance.

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

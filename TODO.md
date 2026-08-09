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

---

## Residual — Wave-4 audit carry-forwards (optional)

### Prefer HTTP status over size-limit on oversized error bodies

`fetchHTTP` can return a size-limit error for a non-2xx body larger than
the cap before surfacing `HTTP 403`/`404`. Reorder or separately cap
error-body reads.

### Tighten black skip around dotted module names

`no module named nbformat.core` can still classify as missing notebook
extras (prefix match before `.`). Prefer end-of-token only for the exact
module names.

### CLI `--upgrade --dry-run` success path

Happy-path covers live upgrade via `dispatchArgs`; add a parallel
`--upgrade --dry-run` CLI test (library dry-run already covered).

### fetchHTTP non-2xx unit coverage

Oversized bodies are tested; small 403/404 bodies are not.

### Notebook ModuleNotFoundError fixture lines

Runtime lowercasing matches `ModuleNotFoundError: No module named …`;
committed tests do not include that prefix.

### ResolveExtension empty-path case

`""` falls through to `filepath.Ext`; add an explicit unit row.

### `--check` skip config for disabled-but-known extensions

`KnownExtension` still loads the registry before `Supported` returns
false for user-disabled extensions. CI cost only; matches the hook.

---

## Residual — Ops / next-fleet survey

### Diagnose and clear red CI on `origin/main`

MCP `repo_status` previously reported CI failure on `main` at `2fd5ab2`.
Local gates are green on commits ahead of `origin/main` — verify against
the failing run before any history rewrite (none authorized). Push still
requires an explicit operator go.

**Acceptance**

+ Identified failing check and root cause recorded.
+ A subsequent `main` push is green on ubuntu/macOS/Windows test + lint.

### Retire `js-yaml` global override when upstream is fixed

`bunGlobalOverrides` in `internal/installer/tools.go` pins `js-yaml` to
`^5.2.2` because markdownlint-cli2 0.23.1 pins vulnerable `5.2.1`
(GHSA-pm4m-ph32-ghv5). Drop once markdownlint-cli2 ships against the patch.

**Acceptance**

+ Override map empty (or without `js-yaml`) only after confirming the
  published dependency tree; CHANGELOG notes the pin removal.

### Fleet re-survey for the next zero-cost extensions

Measure real fleet file counts before registering:

| Candidate | Likely tool | Notes |
| --- | --- | --- |
| `.vue` / `.svelte` / `.astro` | prettier | Confirm biome does not own these in fleet |
| `.nix` | `nixfmt` / `alejandra` | Only if one canonical binary dominates |
| `.zig` | `zig fmt` | Same pattern as rustfmt/gofmt |

Re-reject Kotlin/XML/Lua/Gradle unless fleet evidence changed.

**Acceptance**

+ Short survey note with counts and go/no-go per candidate.
+ Any "go" lands with dispatch registration + HUMANS row together.

### Config: disable by formatter name

Today `disabled` is extension-only. A parallel `disabledFormatters:
["biome"]` (or bare names in `disabled`) would match operator mental models.

**Traps**

+ `jsonRouter.Name()` returns `"json"` while biome may run underneath —
  disabling `"biome"` must still skip the biome branch for `.json`, or
  document extension-only opt-out for `.json`.

**Acceptance**

+ One config key disables all extensions registered to that formatter.
+ HUMANS example shows disabling biome without enumerating eight extensions.

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

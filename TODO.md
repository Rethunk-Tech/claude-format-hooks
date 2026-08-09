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
aligned. Deferred: multi-dot Terraform, `disabledFormatters`, `--check`
shebang parity (below).

---

## Residual — Formatter coverage

### Multi-dot Terraform suffixes (deferred from `.tfvars` wave)

`filepath.Ext("x.tftest.hcl")` is `.hcl`, not `.tftest.hcl`. Upstream
`terraform fmt` also formats `.tftest.hcl`, `.tfmock.hcl`, and
`.tfquery.hcl`. A naive `".hcl"` registration would hit Packer/Nomad HCL.

Absorb extension resolution into a shared `dispatch` helper (longest
suffix + shebang already peeks in `main`) so `main.go` / `check.go` stop
re-deriving `filepath.Ext` independently.

**Traps**

+ Longest-suffix match; keep `disabled: [".hcl"]` vs
  `disabled: [".tftest.hcl"]` coherent with `KnownExtension`.
+ Do not format `.tf.json` / `.tfvars.json`.

**Acceptance**

+ Multi-dot Terraform suffixes format via `terraform fmt`, or stay an
  explicit won't-fix with the `filepath.Ext` trap recorded in HUMANS.

### Retire `js-yaml` global override when upstream is fixed

`bunGlobalOverrides` in `internal/installer/tools.go` pins `js-yaml` to
`^5.2.2` because markdownlint-cli2 0.23.1 pins vulnerable `5.2.1`
(GHSA-pm4m-ph32-ghv5). Drop once markdownlint-cli2 ships against the patch.

**Acceptance**

+ Override map empty (or without `js-yaml`) only after confirming the
  published dependency tree; CHANGELOG notes the pin removal.

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

### Release-binary upgrade path

No `format-dispatch --upgrade` / install.sh path that fetches the latest
release artifact, verifies sha256, and replaces the installed hook binary.

**Traps**

+ Must verify sha256; reuse `installer.HookBinaryBaseName` /
  `HookBinaryPath` for Windows `.exe` vs POSIX naming.
+ Do not rewrite `settings.json` if already wired to the same path.

**Acceptance**

+ Documented, tested download + hash verify + install; HUMANS no-Go links it.

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

## Residual — Hook / check parity

### `--check` extensionless shebang parity

Hook formats extensionless shell-shebang paths; `runCheck` still uses
`filepath.Ext` only, so CI skips those files. Wave-2 contract deferred
this to the same extension-resolution pass as multi-dot Terraform.

**Acceptance**

+ `--check` and the hook agree on extensionless shell-shebang targets.

---

## Residual — Hardening (wave-2 audit carry-forwards)

### Defensive nil skip in `Registry.Dispatch`

`Dispatch` indexes `byExt[ext]` with no nil guard; a caller that skips
`Supported` panics the process. Prefer `Result{Skipped: true}` (or a
clear Err) over nil deref.

### Tighten black notebook-missing heuristic

`blackNotebookSupportMissing` substring-matches `jupyter`/`notebook` in
any diagnostic, which can silence unrelated black failures. Prefer
exit-code/message patterns that only mean the jupyter extra is absent.

### Include notebook in `TestExternalFormatterNames`

`external_test.go` lists external formatter names but omits
`NewNotebook` / `ruff/black-notebook`.

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

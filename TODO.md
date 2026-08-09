# TODO — Follow-on and carry-forward

Planning-only backlog. Items below are not yet tracked elsewhere (no open
GitHub issues; this file is the ledger). Prefer landing one coherent unit
per commit; do not treat this as a mandate to expand scope mid-PR.

Wave 1 (2026-08-09) landed: Round 1+8 docs; `--check` opt-out/root/
`.fmtcheck`/per-file timeout + vendored/`within` guards; `.tfvars`/`.pyi`;
expanded `vendoredDirs`; PATH-first bunx fallback; atomic native writes
(symlink-safe + stale-skip); diskcache prune; sqlfluff `~/.sqlfluff`
bootstrap. Multi-dot Terraform suffixes stayed deferred (below).

---

## Residual — Formatter coverage

### Multi-dot Terraform suffixes (deferred from `.tfvars` wave)

`filepath.Ext("x.tftest.hcl")` is `.hcl`, not `.tftest.hcl`. Upstream
`terraform fmt` also formats `.tftest.hcl`, `.tfmock.hcl`, and
`.tfquery.hcl`. A naive `".hcl"` registration would hit Packer/Nomad HCL.

**Traps**

- Longest-suffix match; keep `disabled: [".hcl"]` vs
  `disabled: [".tftest.hcl"]` coherent with `KnownExtension`.
- Do not format `.tf.json` / `.tfvars.json`.

**Acceptance**

- Multi-dot Terraform suffixes format via `terraform fmt`, or stay an
  explicit won't-fix with the `filepath.Ext` trap recorded in HUMANS.

### Retire `js-yaml` global override when upstream is fixed

`bunGlobalOverrides` in `internal/installer/tools.go` pins `js-yaml` to
`^5.2.2` because markdownlint-cli2 0.23.1 pins vulnerable `5.2.1`
(GHSA-pm4m-ph32-ghv5). Drop once markdownlint-cli2 ships against the patch.

**Acceptance**

- Override map empty (or without `js-yaml`) only after confirming the
  published dependency tree; CHANGELOG notes the pin removal.

---

## Residual — Ops / next-fleet survey

### Diagnose and clear red CI on `origin/main`

MCP `repo_status` previously reported CI failure on `main` at `2fd5ab2`.
Local gates are green on commits ahead of `origin/main` — verify against
the failing run before any history rewrite (none authorized). Push still
requires an explicit operator go.

**Acceptance**

- Identified failing check and root cause recorded.
- A subsequent `main` push is green on ubuntu/macOS/Windows test + lint.

### Release-binary upgrade path

No `format-dispatch --upgrade` / install.sh path that fetches the latest
release artifact, verifies sha256, and replaces
`~/.claude/hooks/format-dispatch`.

**Traps**

- Must verify sha256; Windows `.exe` naming must share the helper with
  DefaultOptions (see Windows install path below).
- Do not rewrite `settings.json` if already wired to the same path.

**Acceptance**

- Documented, tested download + hash verify + install; HUMANS no-Go links it.

### Fleet re-survey for the next zero-cost extensions

Measure real fleet file counts before registering:

| Candidate | Likely tool | Notes |
| --- | --- | --- |
| `.vue` / `.svelte` / `.astro` | prettier | Confirm biome does not own these in fleet |
| `.nix` | `nixfmt` / `alejandra` | Only if one canonical binary dominates |
| `.zig` | `zig fmt` | Same pattern as rustfmt/gofmt |

Re-reject Kotlin/XML/Lua/Gradle unless fleet evidence changed.

**Acceptance**

- Short survey note with counts and go/no-go per candidate.
- Any "go" lands with dispatch registration + HUMANS row together.

### Config: disable by formatter name

Today `disabled` is extension-only. A parallel `disabledFormatters:
["biome"]` (or bare names in `disabled`) would match operator mental models.

**Traps**

- `jsonRouter.Name()` returns `"json"` while biome may run underneath —
  disabling `"biome"` must still skip the biome branch for `.json`, or
  document extension-only opt-out for `.json`.

**Acceptance**

- One config key disables all extensions registered to that formatter.
- HUMANS example shows disabling biome without enumerating eight extensions.

---

## Residual — Hook claim vs capability

### Format notebooks (`.ipynb`) on NotebookEdit

Matcher advertises `NotebookEdit`; `.ipynb` is not in the registry.
Prefer ruff-then-black (ruff formats notebooks by default since 0.6.0).

**Traps**

- Do not JSON round-trip the notebook; shell out.
- Black without jupyter extra → skip, not spam.
- Fleet-count first; if rare, document matcher/extension mismatch instead.

**Acceptance**

- NotebookEdit `.ipynb` formats Python cells when ruff/black available;
  silent skip otherwise; HUMANS + registry + CHANGELOG together.

### Windows install path must use `format-dispatch.exe`

`DefaultOptions` / `install.sh` always build bare `format-dispatch`.
Release assets are `format-dispatch-windows-*.exe`. Uninstall matches by
exact string — basename mismatch breaks Wire/Unwire.

**Acceptance**

- On Windows, `BinPath` ends in `format-dispatch.exe`; HUMANS names the
  `.exe` asset; shared helper with any future `--upgrade`.

---

## Residual — Extension refinements

### Extensionless shebang scripts (optional)

Agents write `bin/do-thing` with `#!/usr/bin/env bash` and no extension →
instant no-op. Narrow peek only when `ext == ""`.

**Traps**

- Read ≤256 bytes; only clear shell shebangs to `shellFormatter`.
- Keep unsupported-extension fast path for every real extension.
- Fleet count first; if rare, documented won't-fix.

**Acceptance**

- Extensionless shell-shebang hook paths format via shfmt; `.ts`/`.md`
  still zero I/O before map lookup.

---

## Explicitly out of scope (do not re-open without new evidence)

- CLI framework (Cobra/urfave/kong/ffcli) — decided v0.2.0; see AGENTS.md.
- Native YAML/TOML/HTML Go formatters — fidelity failures already recorded
  in AGENTS.md Architecture.
- Kotlin / XML / Lua / Gradle formatters — surveyed and passed over in
  0.3.0.
- Requiring per-tool project config before formatting — removed for biome;
  do not reintroduce.
- Shared `internal/atomicfile` extract for installer + formatters — both
  paths are atomic independently; extract only if a third caller appears.

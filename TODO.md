# TODO — Follow-on and carry-forward

Planning-only backlog. Items below are not yet tracked elsewhere (no open
GitHub issues; this file is the ledger). Prefer landing one coherent unit
per commit; do not treat this as a mandate to expand scope mid-PR.

---

## Round 1 — Docs / layout drift (post Unreleased)

### Document `.json` biome routing in operator docs

`jsonRouter` (`internal/formatters/json.go`) sends `.json` to biome when a
`biome.json`/`biome.jsonc` is found upward and `bunx` is on `PATH`;
otherwise it keeps the native `encoding/json.Indent` path. Registered via
`formatters.NewJSONRouter` in `internal/dispatch/dispatch.go`.

HUMANS.md's supported-extensions table still lists `.json` as native-only
(`encoding/json.Indent`). Operators reading that table will misdiagnose
biome/`expand` fights and the "missing bunx still formats JSON" story.

**Traps**

- Do not claim every `.json` is biome — the native path is still the
  default without a biome config or without `bunx`.
- `--check` on a biome project with bunx will exercise biome for `.json`;
  without bunx it exercises native. Document both.
- README Highlights still say "Native … for JSON" without the router
  caveat — keep README short, put the full rule in HUMANS.

**Acceptance**

- HUMANS supported-extensions row for `.json` names both backends and the
  switch condition.
- Troubleshooting mentions "my `biome.json` got expanded" → expected
  unless bunx+biome config are present (router should prevent that; if it
  still happens, point at cache TTL / missing bunx).
- CHANGELOG Unreleased notes the doc catch-up if behavior already shipped.

### Catch CONTRIBUTING / AGENTS / install.sh up to shipped surface

Drift sites:

| Site | Stale claim | Current truth |
| --- | --- | --- |
| `CONTRIBUTING.md` § Releasing | release builds `linux/darwin` only | `.github/workflows/release.yml` also ships Windows amd64+arm64 |
| `AGENTS.md` § Layout (`internal/formatters/`) | external list ends at `terraform.go` | also `proto.go` |
| `install.sh` header comment | "pre-warms bunx's package cache" | provisioning lives in `installer.ProvisionTools` (`internal/installer/tools.go`) |
| `HUMANS.md` no-Go install note | "bunx cache pre-warming" skipped | same — provision step, not cache warm |

**Traps**

- Layout table is LLM-facing; leaving `proto.go` out will make the next
  formatter author miss the system-binary pattern already established.
- Do not reintroduce cache-warm language anywhere; the whole point of
  global `bun add -g` was to put binaries on `PATH`.

**Acceptance**

- CONTRIBUTING release sentence lists linux/darwin/windows.
- AGENTS formatters bullet includes `proto.go`.
- `install.sh` / HUMANS no-Go path wording matches `ProvisionTools`.

### Remove or explain the empty `hooks/` directory

Repo-root `hooks/` is an empty tracked directory (no README, no files).
Easy to confuse with Claude Code's `~/.claude/hooks` install target.

**Traps**

- Deleting may be intentional scaffolding — confirm nothing in CI or docs
  references it before `rmdir`.

**Acceptance**

- Directory gone, or a one-line README stating it is unused / reserved.

---

## Round 2 — `--check` fidelity vs the live hook

### Honor project-level opt-out in `--check`

Hook path: `projectDisables` in `cmd/format-dispatch/main.go` reads
`.claude-format-hooks.json` at `$CLAUDE_PROJECT_DIR` and skips disabled
extensions. `runCheck` / `wouldReformat` in `cmd/format-dispatch/check.go`
never call it — CI can fail on files the hook would silently leave alone.

**Traps**

- Opt-out is per project root, not per file directory. For
  `format-dispatch --check .`, resolve the same root the hook would
  (`CLAUDE_PROJECT_DIR` if set, else the path argument / cwd) — do not
  invent a per-subdir config walk unless product intent changes.
- Malformed project config must not fail `--check` (hook ignores and
  continues); mirror that, or document a deliberate stricter CI stance.

**Acceptance**

- A tree with `{ "disabled": [".json"] }` and an unformatted `.json` exits
  0 from `--check` for that extension (same as the hook).
- HUMANS `--check` section mentions project opt-out parity.

### Pass a real project root into `--check` dispatches

`wouldReformat` calls `registry.Dispatch(ctx, filepath.Dir(abs), scratch)`.
The hook passes `$CLAUDE_PROJECT_DIR` (or cwd). Biome's
`cachedFindUpward` (`internal/formatters/biome.go`) stops at that root —
so for `apps/web/foo.ts` with `biome.json` at the repo root, `--check`
stops at `apps/web` and may format under the wrong (or empty) config
while the hook formats under the real one.

Also affects `jsonRouter`'s biome-vs-native decision (same find-upward
bound).

**Traps**

- Scratch copies must stay beside the real file (config discovery by
  walking from the file) — only the `projectRoot` argument is wrong, not
  the copy location.
- Vendored-dir collection already relativizes to the walk root; don't
  conflate that with Dispatch's `projectRoot`.
- When checking multiple path args from different trees, pick a root per
  file (containing check-arg / `CLAUDE_PROJECT_DIR`) consistently.

**Acceptance**

- Nested package file with only a repo-root `biome.json` yields the same
  "would reformat" answer under `--check` as a hook invocation with
  `CLAUDE_PROJECT_DIR` set to the repo root.
- Comment in `check.go` states why `projectRoot` is not `Dir(abs)`.

---

## Round 3 — Formatter coverage and install hardening

### Extend Terraform coverage past `.tf`

`terraformFormatter` (`internal/formatters/terraform.go`) registers only
`.tf`. Upstream `terraform fmt` also formats `.tfvars`, `.tftest.hcl`,
`.tfmock.hcl`, and `.tfquery.hcl`.

**Traps**

- `filepath.Ext("x.tftest.hcl")` is `.hcl`, not `.tftest.hcl`. A naive
  `".hcl"` registration would also hit Packer/Nomad/other HCL and is
  unsafe. Need suffix matching (or a small `matchExt` helper) that checks
  longest known suffixes first — and `KnownExtension` / disabled-list
  semantics must stay coherent (`disabled: [".hcl"]` vs
  `disabled: [".tftest.hcl"]`).
- `.tfvars` is a clean `filepath.Ext` add; land it even if multi-dot
  suffixes wait.
- Do not format `.tf.json` / `.tfvars.json` — `terraform fmt` leaves
  those alone by design.

**Acceptance**

- `.tfvars` writes go through `terraform fmt` when `terraform` is on
  `PATH`.
- Multi-dot Terraform suffixes either work via suffix match or are an
  explicit deferred bullet with the `filepath.Ext` trap recorded.
- HUMANS table + dispatch registry updated together.

### Prefer provisioned binaries on `PATH` over `bunx`

`--install` globally installs biome / prettier / taplo / markdownlint-cli2
(`internal/installer/tools.go`), yet `biome.go` / `bunxtool.go` still
always invoke `bunx <pkg> …`. That re-pays bunx resolution on every file
inside the 4s `formatterTimeout`.

**Traps**

- Version skew: `PATH` binary may be older than what `bunx @scope/pkg`
  would fetch — prefer PATH only when present; keep bunx as fallback so
  machines without global install still work.
- Binary names: `@biomejs/biome` → `biome`, `@taplo/cli` → `taplo`,
  `markdownlint-cli2` → `markdownlint-cli2`, `prettier` → `prettier`.
- Negative `lookPath` cache (`binpath.go`) must key on the actual binary
  tried first, or a missing `biome` will mask a working `bunx` for 30s if
  ordered wrong — try PATH tool, then bunx, and don't cache a miss for
  the whole formatter on the first miss alone.

**Acceptance**

- With `biome` on `PATH` and no network, a `.ts` write formats without
  invoking `bunx`.
- Without the PATH binary but with `bunx`, behavior unchanged.
- Name() / invocation log still identify the logical formatter (biome,
  prettier, …), not the launcher.

### Retire `js-yaml` global override when upstream is fixed

`bunGlobalOverrides` in `internal/installer/tools.go` pins `js-yaml` to
`^5.2.2` because markdownlint-cli2 0.23.1 pins vulnerable `5.2.1`
(GHSA-pm4m-ph32-ghv5). Comment already says drop once markdownlint-cli2
ships against the patch.

**Traps**

- Removing the override while the vulnerable pin remains re-exposes every
  `--install` operator's global bun tree — verify the markdownlint-cli2
  release's lock/manifest before deleting the map entry.
- `bun add -g` still drops `overrides`; if other pins appear later, keep
  the merge-then-`bun install` flow even when this one pin is gone.

**Acceptance**

- Override map empty (or without `js-yaml`) only after confirming the
  published markdownlint-cli2 dependency tree.
- Tools tests updated; CHANGELOG notes the pin removal.

### Materialize a user-level sqlfluff base config (dialect)

Markdown got `markdownconfig.go` because markdownlint-cli2 has no
user-level discovery. sqlfluff *does* read `~/.sqlfluff`, but without any
reachable config it hard-errors ("No dialect was specified") — every
`.sql` write becomes a diagnostic. HUMANS documents "create one"; nothing
bootstraps it the way markdown defaults are materialized on first use.

**Traps**

- Do not invent a wrong dialect. A safe bootstrap is a commented template
  or a deliberately conservative default (e.g. `ansi`) plus HUMANS
  guidance to edit — matching "editable, never overwritten once present"
  from markdown.
- Never clobber an existing `~/.sqlfluff`.
- Project `.sqlfluff` must still win; sqlfluff's native merge order is the
  feature — prefer writing `~/.sqlfluff` over inventing a `--config` pass
  that fights the tool.

**Acceptance**

- First `.sql` write on a machine with sqlfluff but no config either
  formats under a materialized user default or skips with a single clear
  diagnostic pointing at the file to edit — never a raw "No dialect"
  wall of text every time.
- Existing `~/.sqlfluff` untouched.

---

## Round 4 — Ops / next-fleet survey

### Diagnose and clear red CI on `origin/main`

MCP `repo_status` reports CI failure on `main` at `2fd5ab2` (test matrix +
lint); logs were unavailable to this session. Local `go test -race ./...`
and `format-dispatch --check .` are green on the 4 commits ahead of
`origin/main`.

**Traps**

- Do not "fix" by skipping the dogfood `--check` step in
  `.github/workflows/ci.yml` without evidence.
- Unpushed local commits (json router, deps, agents note) may already
  contain the fix — verify against the failing run before rewriting
  history or force-pushing (none authorized).

**Acceptance**

- Identified failing check and root cause recorded (issue comment or
  CHANGELOG Fixed).
- A subsequent `main` push is green on ubuntu/macOS/Windows test + lint.

### Release-binary upgrade path

HUMANS documents download-from-release then `--install` for operators
without Go. There is no `format-dispatch --upgrade` / install.sh path that
fetches the latest release artifact and replaces `~/.claude/hooks/
format-dispatch` with checksum verification.

**Traps**

- Must verify sha256 against the release asset; do not curl|sh.
- Windows `.exe` naming and overwrite-while-running differ from POSIX —
  match `release.yml` artifact names.
- `--upgrade` should not rewrite `settings.json` if the hook entry
  already points at the same path (install is idempotent; keep that).

**Acceptance**

- Documented, tested path: download matching GOOS/GOARCH asset, verify
  hash, install into `CLAUDE_HOOKS_BIN_DIR`, run `--install` only if
  unwired.
- HUMANS no-Go section links it.

### Fleet re-survey for the next zero-cost extensions

Previous survey (CHANGELOG 0.3.0) added `.tf`, deferred Kotlin/XML/Lua/
Gradle, and flagged `.proto` (now shipped). Next pass should measure
real fleet file counts before registering:

| Candidate | Likely tool | Notes |
| --- | --- | --- |
| `.vue` / `.svelte` / `.astro` | prettier (parser already present) | Same bunx/prettier path as `.html`; confirm biome does not own these in fleet projects or double-format |
| `.tfvars` | `terraform fmt` | See Round 3; count before multi-dot HCL |
| `.nix` | `nixfmt` / `alejandra` | Fragmented tools — only if one canonical binary dominates |
| `.zig` | `zig fmt` | Single canonical formatter; same pattern as rustfmt/gofmt |

**Traps**

- Re-reject Kotlin/XML/Lua/Gradle unless fleet evidence changed — do not
  re-litigate without counts.
- Prettier-on-Vue in a biome project may fight biome's HTML/JS embeds —
  prefer project opt-out documentation over clever dual routing unless a
  real conflict appears.
- Instant no-op invariant: unknown extensions stay one map lookup.

**Acceptance**

- Short survey note (CHANGELOG or this file) with counts and a go/no-go
  per candidate.
- Any "go" lands with dispatch registration + HUMANS row in the same PR.

### Config: disable by formatter name (optional enhancement)

Today `disabled` is extension-only (`config.Config.Disabled`). Opting out
of biome requires listing every biome extension (`.ts`, `.tsx`, …). A
parallel `disabledFormatters: ["biome"]` (or accepting bare names in
`disabled`) would match how operators think about double-running with
pre-commit.

**Traps**

- `jsonRouter.Name()` returns `"json"` while biome may run underneath —
  disabling `"biome"` must still skip the biome branch for `.json`, or
  document that `.json` opt-out is extension-only.
- Project + user config merge: project `disabled` already overlays via a
  separate file; keep one schema.

**Acceptance**

- One config key disables all extensions registered to that formatter
  instance.
- HUMANS example shows disabling biome without enumerating eight
  extensions.

---

## Explicitly out of scope (do not re-open without new evidence)

- CLI framework (Cobra/urfave/kong/ffcli) — decided v0.2.0; see AGENTS.md.
- Native YAML/TOML/HTML Go formatters — fidelity failures already recorded
  in AGENTS.md Architecture.
- Kotlin / XML / Lua / Gradle formatters — surveyed and passed over in
  0.3.0.
- Requiring per-tool project config before formatting — removed for biome;
  do not reintroduce.

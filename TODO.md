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

Repo-root `hooks/` is an empty directory (currently untracked; no README,
no files). Easy to confuse with Claude Code's `~/.claude/hooks` install
target.

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
inside the 4s `formatterTimeout`. `jsonRouter` (`json.go`) also gates the
biome branch on `lookPath("bunx")` alone — once PATH preference lands, that
gate must follow the same order or `.json` keeps paying bunx while `.ts`
does not.

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
- Same for a `.json` under a biome config (`jsonRouter` biome branch).
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

## Round 5 — Hook claim vs capability

### Format notebooks (`.ipynb`) on NotebookEdit

Installer matcher is `Write|Edit|NotebookEdit` (`internal/installer/
installer.go`), and `hookio.Payload.FilePath` already falls back to
`tool_input.notebook_path`. `.ipynb` is not in `dispatch.NewRegistry`, so
every NotebookEdit is an instant no-op after `KnownExtension` — the
matcher advertises a path the binary never takes.

Upstream: `ruff format` formats notebooks by default (since 0.6.0); Black
needs `black[jupyter]` and detects `.ipynb` by extension. Prefer the same
ruff-then-black order as `python.go`, not a third tool.

**Traps**

- Do not round-trip the whole notebook through `encoding/json` — that
  reorders cell/metadata keys and can drop fields formatters preserve.
  Shell out; let ruff/black own the cell walk.
- Black without the jupyter extra exits non-zero on `.ipynb` — treat that
  as skip (or a clear diagnostic once), not a repeating fixer-failed
  spam; ruff-first avoids the common case.
- `--check` scratch copies must keep the `.ipynb` extension (`copyBeside`
  already preserves the basename).
- Fleet count before landing: if NotebookEdit traffic is rare, document
  the matcher/extension mismatch in HUMANS instead of adding a formatter.

**Acceptance**

- A NotebookEdit payload whose path ends in `.ipynb` formats Python cells
  when `ruff` (or jupyter-capable `black`) is on `PATH`.
- Without either tool, silent skip (same as `.py` today).
- HUMANS table + dispatch registry updated together; CHANGELOG notes the
  matcher/extension gap closing.

### Windows install path must use `format-dispatch.exe`

`installer.DefaultOptions` and `install.sh` always build
`$BIN_DIR/format-dispatch` with no `.exe`. Release assets are
`format-dispatch-windows-amd64.exe` (see `.github/workflows/release.yml`).
`Wire`/`Unwire` match `HookCommand.Command` by exact string equality
(`hasBin`), so a Windows operator who renames the release binary correctly
still gets a settings entry that does not match what `DefaultOptions`
later uninstalls — and a from-source `install.sh` on Windows produces a
non-`.exe` name CreateProcess may not resolve the way operators expect.

**Traps**

- Only append `.exe` when `runtime.GOOS == "windows"` (or when the built
  artifact already has it) — never on linux/darwin.
- Uninstall must resolve the same basename Install wired; a one-time
  migration that also removes a bare `format-dispatch` entry on Windows
  avoids leaving a duplicate matcher.
- Round 4 `--upgrade` must share this naming helper — do not fork a third
  copy of the basename rule.

**Acceptance**

- On Windows, `DefaultOptions().BinPath` ends in `format-dispatch.exe`.
- `install.sh` (or a small Go helper it calls) writes that name when
  building on Windows.
- HUMANS no-Go download blurb names the `.exe` asset and the on-disk
  basename.

---

## Round 6 — Write durability and `--check` hygiene

### Atomic writes for native formatters

`formatters.writeFormatted` (`writefile.go`) uses `os.WriteFile` in place.
`installer.writeAtomic` already does temp-file + rename so a kill cannot
truncate `settings.json`. A SIGKILL mid-format on a large `.json`/`.sh`/
`.go` can leave the operator's source truncated — the same class of bug,
on a hotter path.

**Traps**

- Temp file must stay in `filepath.Dir(abs)` so rename does not cross
  filesystems (installer already documents this).
- Preserve mode: stat first, write temp with that mode, rename — same
  contract `writeFormatted` claims today (`TestShellFormatterPreservesExecutableBit`).
- Do not change external formatters' own in-place writes (`biome
  --write`, `prettier --write`, …); only the native helper.
- Extracting a shared helper into e.g. `internal/atomicfile` is fine if
  `installer` and `formatters` both need it — avoid an import cycle
  through `diskcache`/`config`.

**Acceptance**

- Killing the process between temp write and rename leaves the original
  file bytes intact.
- Mode-preservation test still passes on Unix; Windows coverage unchanged
  in spirit.

### Ignore leftover `.fmtcheck-*` scratch files

`wouldReformat` / `copyBeside` (`check.go`) write
`.fmtcheck-<rand>-<base>` beside the target and delete on defer. A
hard-kill mid-check leaves orphans; the next `format-dispatch --check .`
can collect them as ordinary targets (they keep the real extension) and
report bogus "would reformat" noise — or worse, format the orphan in
place via a later hook write if someone opens it.

**Traps**

- Match the `.fmtcheck-` prefix on the basename only, not anywhere in
  the path (a legitimate `fmtcheck-report.json` must not be skipped).
- Cleanup-on-start is optional; skip-on-collect is the minimum.
- Do not add a broad `.*` ignore — only this known scratch prefix.

**Acceptance**

- `collectCheckTargets` never returns a path whose basename starts with
  `.fmtcheck-`.
- Document the prefix in the `--check` comment block so a future
  scratch scheme does not collide.

### Bound each `--check` file with `formatterTimeout`

`runCheck` uses one `checkTimeout` (15m) for the whole walk. A single
hung `bunx`/networked tool can burn the entire budget and fail the job
with a generic deadline, with no indication which file stuck. The hook
path already uses `formatterTimeout` (4s) per file for this reason.

**Traps**

- Nested contexts: per-file timeout under the run-wide timeout; either
  firing must surface which limit hit (file path + cause).
- Absent-tool skips must stay fast — do not wait the full 4s on a
  negative `lookPath` cache hit.
- Keep 15m (or similar) as the wall clock for large trees; only the
  per-file bound is new.

**Acceptance**

- A deliberately hung formatter on one file fails/skips that file (or
  ends the run with that path named) without consuming the full 15m.
- Well-behaved trees still exit 0/1 as today within budget.

### Prune expired `diskcache` entries

`diskcache.Get` treats expired entries as misses but never deletes them.
`editorconfig` keys include the absolute file path, so a long-lived
cache dir accumulates one file per edited path forever. Same for
find-upward keys under busy monorepos.

**Traps**

- Prune must stay best-effort and never fail a format — same swallow
  policy as `Set`.
- Do not hold a lock across the whole directory; a simple "on Set/Get,
  occasionally sweep entries older than max(TTLs)" or size cap is enough.
- `$CLAUDE_FORMAT_HOOKS_CACHE` overrides must still work; never wipe a
  directory that is not clearly ours (stick to files matching `Key`'s
  `namespace-` prefix pattern).

**Acceptance**

- After TTL expiry, a subsequent Get/Set eventually removes the stale
  file (or a documented explicit sweep runs at most once per process).
- HUMANS troubleshooting "delete the cache dir" remains valid as a
  manual escape hatch.

---

## Round 7 — Extension and skip-list refinements

### Register `.pyi` on the Python formatter

`pythonFormatter` (`python.go`) only registers `.py`. Ruff's formatter
explicitly treats `.pyi` stub spacing; Black formats stubs too. Stub
writes from agents are common in typed Python trees and currently no-op.

**Traps**

- Same ruff-then-black PATH order; no new tool.
- Do not add `.pyw` / `.pyx` without evidence — `.pyx` is Cython, not
  safe for black/ruff format.
- Project opt-out `disabled: [".py"]` must not silently disable `.pyi`
  unless we document them as one formatter family (ties to Round 4
  disable-by-formatter-name).

**Acceptance**

- `.pyi` writes go through `ruff format` / `black` when present.
- HUMANS table lists `.pyi` beside `.py`.

### Extend `vendoredDirs` with safe language caches

`dispatch.vendoredDirs` covers JS/Next/yarn/git/agents/dist/build/
coverage/test-results/vendor/.venv. After adding `.tf`/`.rs`/`.py`
formatters, generated trees those ecosystems create are still walked:

| Segment | Why |
| --- | --- |
| `.terraform` | provider plugins + state-shaped JSON/HCL noise |
| `__pycache__` | `.pyc` adjacent; agents sometimes drop `.py` beside caches |
| `.ruff_cache` / `.mypy_cache` / `.pytest_cache` / `.tox` | tool caches |

**Traps**

- Do **not** add bare `target` — Rust's `target/` collides with ordinary
  package names (`app/target/...`) and would false-skip real sources.
  Revisit only with a Rust-specific heuristic (e.g. `target/debug`) if
  fleet evidence demands it.
- Keep segment-exact matching (`InVendoredDir`); no substring matches.
- Update AGENTS invariants list + `TestInVendoredDir` cases in the same
  change.

**Acceptance**

- Paths under the new segments are skipped by the hook and by
  `--check` collection/pruning.
- HUMANS / AGENTS vendored list matches `vendoredDirs`.

### Extensionless shebang scripts (optional, Ext-empty only)

Agents often write `bin/do-thing` with a `#!/usr/bin/env bash` shebang and
no extension. Today `filepath.Ext` is `""`, `KnownExtension` is false →
instant no-op. A narrow peek when — and only when — `ext == ""` would
keep the unsupported-extension fast path intact for every real
extension.

**Traps**

- Read at most one small prefix (e.g. 256 bytes); never full-file parse
  before the gate.
- Only route clear shell shebangs (`bash`/`sh`/`dash`) to `shellFormatter`;
  do not guess Python/Ruby from shebang into other formatters in v1.
- `LangBash` parser already used for `.sh`/`.bash` — same variant.
- `--check` and `KnownExtension` need a coherent story: either
  `KnownExtension("")` stays false and only `run`/`wouldReformat` special-
  case empty ext after a shebang sniff, or document that extensionless
  files are never collected by `--check` directory walks unless named
  explicitly.

**Acceptance**

- An executable without an extension whose first line is a shell shebang
  formats via shfmt when invoked as a hook path.
- A `.ts` / `.md` / unknown-ext write still does zero I/O before the map
  lookup.
- Explicit go/no-go after a short fleet count of extensionless shebang
  writes; if rare, keep as documented won't-fix rather than code.

---

## Round 8 — Doc catch-up beyond Round 1

### SECURITY.md command-injection list missing `buf`

Scope bullet lists external formatters through `terraform` and omits
`buf` (added with `.proto`). Same class of omission Round 1 already
flags for AGENTS layout / CONTRIBUTING release matrix.

**Traps**

- List tools, not every flag — keep the bullet readable.
- Do not claim formatters are sandboxed; the point is injection via
  *our* argument construction.

**Acceptance**

- `buf` appears beside `terraform` / `rustfmt` in SECURITY.md Scope.

### HUMANS "Other flags" omits `--check`

`### Other flags` still only shows `--version` / `--help`, while a full
`--check` section exists below. Operators scanning the flag list miss the
CI gate.

**Traps**

- One-line pointer is enough; do not duplicate the whole `--check`
  section under Other flags.

**Acceptance**

- Other flags names `--check PATH...` and points at the CI section.

### Package comment drift in `cmd/format-dispatch`

`main.go`'s file comment still says native formatters are "JSON, shell"
only — Go (`golang.go`) has been native since 0.3.0. LLM-facing and
human-facing skim both start there.

**Traps**

- Keep the comment short; full rationale stays in AGENTS Architecture.

**Acceptance**

- Comment lists JSON, shell, and Go as in-process.

### README Highlights omit `--check`

README sells install + silent hook behavior but never mentions
`format-dispatch --check`, which is now the CI dogfood path
(`.github/workflows/ci.yml`) and the main consumer-facing gate. Operators
discovering the project via README alone will not know the non-mutating
mode exists.

**Traps**

- One bullet; link or point to HUMANS — do not paste the exit-code matrix
  into README.
- Round 1 already covers the JSON-router caveat for Highlights; do not
  thrash that bullet while adding this one.

**Acceptance**

- Highlights (or Quick start) names `--check` as the CI-oriented,
  non-mutating gate.

---

## Explicitly out of scope (do not re-open without new evidence)

- CLI framework (Cobra/urfave/kong/ffcli) — decided v0.2.0; see AGENTS.md.
- Native YAML/TOML/HTML Go formatters — fidelity failures already recorded
  in AGENTS.md Architecture.
- Kotlin / XML / Lua / Gradle formatters — surveyed and passed over in
  0.3.0.
- Requiring per-tool project config before formatting — removed for biome;
  do not reintroduce.

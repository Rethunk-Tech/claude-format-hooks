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

- `Dispatch(ext)` glue; `.ipynb` via ruff/black-notebook; docs/tests
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

Wave 14 (2026-08-10) landed: upgrade parent-dir `0o750` assert;
`writeAtomic` + `writeFormatted` chmod/close seams with deterministic
fault tests (installer Write via child-helper prlimit); `checkWalkDir`
injection for `collectCheckTargets`; cwd-gone `Getwd` probe helper
replacing darwin `GOOS` blanket. Audit fixup: route `writeAtomic`
Write/Chmod error-path closes through `writeAtomicClose`. Quick CI
green; remote badge still needs an operator push of ahead `main`.

---

## Residual — Wave-11 / Wave-12 deferred

### `resolveTarget` Rel-error skip

`filepath.Rel` failure still returns `skip: relative path error` and
stays uncovered. Wave contract excludes mock-based `Abs`/`Rel` failure
tests; reopen only with a portable fixture that forces `Rel` to fail
without production hooks.

---

## Residual — Ops

### Unbreak Windows `TestWriteAtomicCloseFailure` (CI red)

`origin/main` is at `b53285f` (markdownlint-defaults JSONC fix). Ubuntu
lint + ubuntu/macOS test are not the remaining gap: run
`31548185995` fails **test (windows-latest)** on
`TestWriteAtomicCloseFailure` (`internal/installer/installer_test.go`).
The close seam returns an error without closing the handle; Windows
cannot `Remove` an open file, so the deferred temp cleanup leaves a
`.tmp` and the "directory empty" assert fails. POSIX hides this because
unlink-while-open works.

This is the same class of Windows handle lifetime already skipped in
`writefile_test.go` / upgrade mode asserts — the installer close-fault
fixture was not gated.

**Owns:** `internal/installer/installer.go` (`writeAtomic`,
`writeAtomicClose`), `internal/installer/installer_test.go`.

**Trap:** stubbing `Close` to fail *and* skip the real close leaks the
handle on Windows. Either close the real `*os.File` then return the
injected error, or skip the empty-dir assert on Windows the way
chmod-denial fixtures already skip when still writable.

**Acceptance**

- `go test -race -run TestWriteAtomicCloseFailure ./internal/installer`
  passes on Windows.
- Remote `test (windows-latest)` green on the tip that lands the fix.
- Failed close still returns an error and does not rename the temp over
  the destination (POSIX and Windows).

### Fleet re-survey (periodic)

Wave-6 survey across `/usr/local/src/com.github/Rethunk-Tech/` found
**zero** hand-authored `.vue`/`.svelte`/`.astro`/`.nix`/`.zig` under
vendored-dir exclusions — all **no-go**. Waves 7–10 resurveys reconfirmed
zero; see `.orchestrate/fleet-survey-wave10.md`.
Re-run when the fleet gains candidate sources; any go still needs
dispatch registration + HUMANS row together. For `.nix`, pick one of
`nixfmt`/`alejandra` by PATH dominance.

---

## Residual — Wave-13 / Wave-14 deferred (optionals)

### `copyBeside` `crypto/rand.Read` error

Unreachable under Go stdlib (entropy failure aborts). Do not mock; leave
below 90% unless a production seam appears. Owns:
`cmd/format-dispatch/check_test.go`.

### `withFileSizeLimit` third-copy extract

Identical helper in `installer_test.go` and `writefile_test.go`. Below
the three-site consolidation threshold; extract a shared test helper
only when a third copy appears.

### Formatters prlimit isolation (optional)

Installer Write-fault uses a child-process helper so race testlog stays
writable; formatters still apply prlimit in-process. Align only if
in-process prlimit starts failing the race harness.

### Seam globals vs `t.Parallel`

Package-level seam vars rely on `t.Cleanup` restore and no
`t.Parallel()` today. If parallel subtests are added in these packages,
gate seams with a mutex or per-test wiring.

### Seam declaration style (cosmetic)

Installer uses method values; formatters use func literals. Runtime
defaults are equivalent — unify only if a third seam package appears.

---

## Wave 15 — 2026-08-12 planning

Planning-only. Do not implement from this section without picking a
unit. shadcn/UX registry does not apply (no UI). Context7: Claude Code
hook matcher now includes `MultiEdit` and documents `tool_result`;
Biome GraphQL formatter is on by default; Ruff formats `.ipynb` by
default (already how `notebook.go` invokes `ruff format`).

### Round 1 — Hook envelope

#### Match `MultiEdit` on install

Claude Code's current PostToolUse matcher used by first-party hooks is
`Edit|Write|MultiEdit|NotebookEdit`. Installer still writes
`Write|Edit|NotebookEdit` (`matcherAll` in
`internal/installer/installer.go`). MultiEdit writes never invoke this
hook, so those files stay unformatted until a later Write/Edit.

**Owns:** `internal/installer/installer.go` (`matcherAll`, `keepEntry`,
`matcherOld`), `internal/installer/installer_test.go`, HUMANS/README
matcher prose, CHANGELOG.

**Trap:** `keepEntry` already replaces the binary's own entry on
`--install`, so existing operators pick this up only after re-install
(or a settings rewrite). Do not treat `matcherOld` (`Write|Edit`) as
the MultiEdit-less current matcher — that string is reserved for the
legacy inline biome hook. MultiEdit's `tool_input` still uses
`file_path` (same as Edit); do not invent a multi-path loop unless a
captured payload shows an `edits` array of distinct paths.

**Acceptance**

- Fresh `--install` writes a matcher that includes `MultiEdit`.
- Re-`--install` replaces the previous format-dispatch matcher (no
  duplicate PostToolUse entries).
- A stdin payload with `tool_name` MultiEdit and `tool_input.file_path`
  formats that file the same as Edit.

#### Accept `tool_result` as well as `tool_response`

`internal/hookio/payload.go` reads `tool_response.filePath` first.
Current Claude Code hook docs name the sibling field `tool_result`. If
the runtime stopped emitting `tool_response`, the hook still works via
`tool_input.file_path` — unless a future tool only puts the resolved
path on `tool_result`. Cheap to accept both; do not drop
`tool_response` until a captured payload proves it is gone.

**Owns:** `internal/hookio/payload.go`, `internal/hookio/payload_test.go`.

**Trap:** field names differ (`filePath` camelCase on the response
object vs `file_path` on input). Decode both objects; keep the existing
fallback order, inserting `tool_result` next to `tool_response`.

**Acceptance**

- Payload with only `tool_result.filePath` resolves.
- Payload with both `tool_response` and `tool_result` is deterministic
  (document which wins).
- Malformed JSON still yields an empty path (exit-0 no-op).

### Round 2 — Formatter routing

#### `biome format --write` instead of `biome check --write`

`internal/formatters/biome.go` runs `biome check --write
--no-errors-on-unmatched`. `--no-errors-on-unmatched` only covers
unknown paths. Lint findings still fail the process, so a file that
formatted cleanly can still emit a truncated diagnostic on every write
— the same class of noise sqlfluff and markdownlint already special-case.
Biome's GraphQL/HTML/CSS docs and the formatter guide apply formatting
via `biome format --write`.

**Owns:** `internal/formatters/biome.go`, `internal/formatters/json.go`
(router still delegates to NewBiome), HUMANS supported-extensions
table, installer `oldBiomeMark` (legacy detection string must keep
matching old settings entries).

**Trap:** `oldBiomeMark = "biome check --write"` is how Wire recognizes
the pre-Go-installer biome-only hook. Changing the *live* argv must not
stop replacing that legacy command. Projects that relied on the hook as
a stealth linter would lose that; the invariant is format-on-write, not
lint-on-write (`HUMANS.md` `--check` already says CI checks formatting
not lint conformance).

**Acceptance**

- Biome-backed extensions invoke `format --write` (PATH and bunx argv).
- A file with lint-only findings and a successful format does not print
  a fixer-failed diagnostic.
- Legacy settings entries containing `biome check --write` are still
  replaced on `--install`.

#### Route `.graphql`/`.gql` through Biome when Biome is the project formatter

Biome's GraphQL formatter is stable and enabled by default
(v1.9+; disable via `graphql.formatter.enabled`). Dispatch currently
sends both suffixes to prettier (`internal/dispatch/dispatch.go`) with
a comment that GraphQL has no dedicated tool. That comment is stale.

**Owns:** `internal/dispatch/dispatch.go`, a router or biome registration
alongside prettier fallback, HUMANS table, CHANGELOG.

**Trap:** prettier-plugin-graphql / `.prettierrc` GraphQL options will
diverge from Biome output. Do not steal files from a repo that has no
`biome.json`/`biome.jsonc` — mirror the `.json` router: Biome only when
an upward config exists and a launcher is available; otherwise keep
prettier. `disabledFormatters: ["biome"]` must leave prettier in
place. Fleet `.vue`/`.svelte`/`.astro` remain no-go (Wave-6–10); this
is a routing change for an already-supported extension.

**Acceptance**

- Project with `biome.json` + biome on PATH formats `.graphql`/`.gql`
  via biome.
- Project with neither biome config nor biome binary still uses
  prettier (or skips if prettier/bunx absent).
- Disabling `biome` does not disable prettier for those suffixes.

#### `tofu fmt` when `terraform` is missing

OpenTofu's `tofu fmt` is the same canonical formatter for `.tf` /
`.tfvars` / the registered multi-dot HCL suffixes. `terraform.go`
LookPaths only `terraform` and skips otherwise.

**Owns:** `internal/formatters/terraform.go`, HUMANS prerequisites +
table, `disabledFormatters` name (keep `terraform` unless a distinct
name is required — prefer one name so opt-out covers both binaries).

**Trap:** both binaries on PATH — pick one (terraform first, matching
today) and do not run both. `tofu fmt` argv is `fmt <path>`, same as
terraform. Do not register bare `.hcl` / `.tf.json` / `.tfvars.json`
(explicitly out of scope below).

**Acceptance**

- `terraform` present → unchanged.
- Only `tofu` present → those extensions format.
- Neither present → skip, no diagnostic.
- `disabledFormatters: ["terraform"]` skips both binaries.

### Round 3 — Skip list / dual harness

#### Vendored-dir additions (evidence-gated)

`dispatch.vendoredDirs` / AGENTS.md invariants omit common toolchain
output that is not `node_modules`/`dist`/`build`: `.turbo`,
`.svelte-kit`, `.nuxt`, `.output` (Nitro), `.parcel-cache`, `.nox`.
Formatting generated files there is wasted budget and noisy diffs.

**Owns:** `internal/dispatch/dispatch.go` (`vendoredDirs`),
`internal/dispatch/dispatch_test.go`, AGENTS.md invariants list,
HUMANS pointer.

**Trap:** do not add bare `target/` (Rust/Java/CMake all use it; too
many false positives). Do not add `.vue`/`.svelte`/`.astro` *formatters*
here — that is still the Wave-6 fleet no-go. Re-run the fleet survey
under `/usr/local/src/com.github/Rethunk-Tech/` and only add a segment
that actually appears as generated output.

**Acceptance**

- Each added segment is skipped by `InVendoredDir` anywhere in the
  relative path (same as existing entries).
- HUMANS/AGENTS lists stay in lockstep with the map.
- No new formatter registrations in this unit.

#### Optional: wire Cursor `hooks.json` as well as Claude `settings.json`

This binary is Claude Code–shaped (`CLAUDE_PROJECT_DIR`,
`~/.claude/settings.json`). Cursor sessions inherit skills/MCP from
`~/.claude/` but keep a separate hooks file. Operators in Cursor never
get PostToolUse formatting unless they duplicate the entry by hand.

**Owns:** `internal/installer` (second settings path or a Cursor-shaped
writer), HUMANS install/uninstall, env override sibling to
`CLAUDE_SETTINGS_FILE`.

**Trap:** Cursor hook JSON is not guaranteed to be the Claude
`hooks.PostToolUse[]` document — do not assume `Wire` can append
blindly. Matcher tool names also differ (Cursor Write/StrReplace vs
Claude Write/Edit/MultiEdit). If the schemas diverge, ship a distinct
writer or drop this item rather than corrupting `~/.cursor/hooks.json`.
shadcn/component registries are irrelevant to this path.

**Acceptance**

- Documented, tested round-trip against a captured Cursor hooks file
  (or explicit no-go in HUMANS if the schema cannot be reused).
- Claude `settings.json` behavior unchanged.
- Uninstall removes only the format-dispatch entry from whichever file
  was wired.

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
- Overloading bare formatter names into `disabled` (wave 6 chose separate
  `disabledFormatters` key).
- `runCheck` NewRegistry call-count / shared-cache integration proof —
  Wave-9 unit floor is deliberate; Wave-10 closed the complementary
  negative path without production instrumentation.

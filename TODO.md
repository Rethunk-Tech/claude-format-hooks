# TODO — Follow-on and carry-forward

Planning-only backlog. Items below are not yet tracked elsewhere (no open
GitHub issues; this file is the ledger). Prefer landing one coherent unit
per commit; do not treat this as a mandate to expand scope mid-PR.

## Coverage

### `resolveTarget` Rel-error skip

`filepath.Rel` failure still returns `skip: relative path error` and
stays uncovered. Reopen only with a portable fixture that forces `Rel`
to fail without production hooks.

### `copyBeside` `crypto/rand.Read` error

Unreachable under Go stdlib (entropy failure aborts). Do not mock; leave
below 90% unless a production seam appears. Owns:
`cmd/format-dispatch/check_test.go`.

### `--check` cache reuse for GraphQL

`TestCheckReusesProjectDisabledBiomeRegistryForMultipleJSONFiles`
covers `.json` only. Add `.graphql`/`.gql` under the same
`projectRoot` cache key only if a regression appears.

## Extract / unify (threshold-gated)

### `withFileSizeLimit` third-copy extract

Identical helper in `installer_test.go` and `writefile_test.go`. Below
the three-site consolidation threshold; extract a shared test helper
only when a third copy appears.

### Unify `writeGraphQLTools` with `writeFakeTool`

`writeGraphQLTools` is a local POSIX/`.cmd` helper. Extract only if a
third fake-tool helper appears (`writeFakeTool` still skips Windows).

### `writeAtomicClose` stub via saved `original`

`TestWriteAtomicCloseFailure` calls `f.Close()` then injects the error.
`TestWriteFormattedReturnsCloseError` routes through the saved method
value and propagates its error. Equivalent for default `Close`; switch
the installer stub only if the seam is wrapped.

### Formatters prlimit isolation

Installer Write-fault uses a child-process helper so race testlog stays
writable; formatters still apply prlimit in-process. Align only if
in-process prlimit starts failing the race harness.

### Seam globals vs `t.Parallel`

Package-level seam vars rely on `t.Cleanup` restore and no
`t.Parallel()` today. If parallel subtests are added in these packages,
gate seams with a mutex or per-test wiring.

### Seam declaration style

Installer uses method values; formatters use func literals. Runtime
defaults are equivalent — unify only if a third seam package appears.

## Cursor installer leftovers

### Installer dry-run with `CursorHooksPath`

Package tests pass `Options` without `CursorHooksPath`; CLI dry-run
covers both files. Extend installer dry-run tests only if a regression
appears.

### Pre-existing empty `afterFileEdit` array

`renderCursorHooks` deletes the key when `kept` is empty, including a
file that already had `"afterFileEdit": []`. No fixture covers that
input. Add one only if a regression appears. Owns:
`internal/installer/cursor_test.go`.

### Unwire when `afterFileEdit` is the only hooks child

Uninstall then leaves `"hooks": {}`. HUMANS documents omitting the
event key, not collapsing an empty `hooks` object. Collapse only if
operators report the empty object as noise. Owns:
`internal/installer/cursor.go`.

### CHANGELOG omit-empty `afterFileEdit`

Unreleased Added already covers Cursor wiring. Record that the last
`afterFileEdit` entry omits the key (not `[]`) on the next changelog
pass. Owns: `CHANGELOG.md`.

## Ops

### Fleet re-survey (periodic)

Last survey of `/usr/local/src/com.github/Rethunk-Tech/` found **zero**
hand-authored `.vue`/`.svelte`/`.astro`/`.nix`/`.zig` and **zero**
`.turbo` / `.svelte-kit` / `.nuxt` / `.output` / `.parcel-cache` /
`.nox` directories under vendored-dir exclusions. Re-run when the fleet
gains candidate sources. Any go still needs dispatch registration +
HUMANS row together. For `.nix`, pick one of `nixfmt`/`alejandra` by
PATH dominance.

### Vendored-dir additions (evidence-gated)

Do not add `.turbo`, `.svelte-kit`, `.nuxt`, `.output`,
`.parcel-cache`, or `.nox` until one appears as generated output. Do
not add bare `target/`. If adding, also update
`cmd/format-dispatch/check_test.go`
`TestCollectCheckTargetsSkipsNewVendoredCacheDirectories` (duplicate
segment list).

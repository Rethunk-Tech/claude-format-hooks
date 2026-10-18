# TODO

Work that is decided but not built. `AGENTS.md` and `HUMANS.md` describe what
exists; this file is the only place that describes what does not.

Sections are unordered.

## install.sh has no --uninstall

`./install.sh` builds and then runs `--install` or `--upgrade` (`install.sh:24`).
The binary already has `--uninstall` (`cmd/format-dispatch/main.go:93`) and
`HUMANS.md` tells the operator to invoke that binary. The shell wrapper is
the documented entry for install and upgrade, so uninstall is the missing
sibling.

`--uninstall` must not rebuild first. The current script builds whenever
`$ACTION` is not `--upgrade` against an existing binary (`install.sh:51`).
Uninstalling after a compile is a different, slower tool than unwiring the
hook that is already on disk. When `$BIN_PATH` is executable, call
`"$BIN_PATH" --uninstall` (and `--uninstall --dry-run`). When it is missing,
print that there is nothing to unwire and exit 0 — do not run `go build`
to produce a binary whose only job is to remove itself.

Rejected: adding `--uninstall` to the `case` at `:24` and falling through
to the existing build. That is the trap this section exists to prevent.

This extends the usage line at `install.sh:9` and `:28`. It does not change
`--install`, `--upgrade`, or the binary's own `--uninstall`.

### Read

1. `install.sh:24` `case "$ACTION"` — today `""`, `--dry-run`, `--upgrade`.
2. `install.sh:51` — the skip-build predicate, `--upgrade` and `-x "$BIN_PATH"`.
   Uninstall uses the same "binary already there" test and never the else
   branch that `go build`s.
3. `install.sh:64` — `--upgrade`'s `DRY_RUN` handling. Copy that for
   `--uninstall`.
4. `cmd/format-dispatch/main.go:93` `runInstall(..., true)` — the action
   the script delegates to.
5. `HUMANS.md:123` — operator uninstall today. Point it at `./install.sh
   --uninstall` once the script can do that without a compile.

### Traps

- **A missing binary is success, not a build.** `--upgrade` compiles when
  nothing is installed yet because it needs a binary to replace. Uninstall
  does not.
- **`--uninstall extra` must fail before any work**, the way `--upgrade`
  rejects a second argument that is not `--dry-run` (`install.sh:73`).
- **Do not delete `$BIN_PATH`.** The binary's `--uninstall` only rewrites
  settings. `HUMANS.md:124` is a separate `rm`. Keep that split.

### Acceptance

- `./install.sh --uninstall` with an existing hooked binary unwires Claude
  and Cursor and does not run `go build`.
- `./install.sh --uninstall --dry-run` prints the same preview as
  `format-dispatch --uninstall --dry-run` and writes nothing.
- `./install.sh --uninstall` with no `$BIN_PATH` exits 0 and prints that
  there is nothing to unwire.
- `./install.sh --uninstall --bogus` exits 2 and does not compile.
- `gate` stays green. Windows CI does not have to execute the script.

## Remaining source types have no .example sibling

`NewRegistry` (`internal/dispatch/dispatch.go:53`) registers `.json.example`,
`.jsonc.example`, `.md.example`, `.toml.example`, `.yaml.example`,
`.yml.example`, `.sql.example`, and `.py.example`. A bare `.example` is
still unregistered (`dispatch.go:55`). `.html` (`:111`), `.php` (`:167`),
`.lua` (`:169`), `.go`, `.ts`/`.js` and the rest of the map have no
`.*.example` key.

Each existing `.example` formatter has a reason it cannot format the copy
in place (biome/taplo/prettier/sqlfluff/black inspect the final suffix).
Adding the rest is that same stdin-or-rename pattern per tool, not a
one-line map insert.

Rejected: registering every remaining extension's `.example` in one
undifferentiated loop. Prettier HTML, biome TS, and native Go each need
the same kind of per-tool stdin name the existing copies already have.

This is new files the hook would start rewriting. That is a contract
change for any repo that keeps `foo.ts.example` next to `foo.ts`.

### Read

1. `internal/dispatch/dispatch.go:53` — the map. Each `.example` key sits
   beside its real extension with a comment naming the tool restriction.
2. `internal/formatters/json.go:68`, `bunxtool.go:82` / `:151`,
   `python.go` (the `.py.example` stdin rename) — the three shapes a new
   copy has to pick from.
3. `internal/dispatch/ext.go` `ResolveExtension` — longest registered
   suffix. A new `.ts.example` must be a real map key, not a suffix
   guess.
4. `internal/dispatch/ext_test.go:38` and `dispatch_test.go` name map —
   every new key lands in both.

### Traps

- **A bare `.example` stays unregistered.** `notes.example` is the
  documented no-op (`HUMANS.md` supported extensions, `ext_test.go:18`).
- **In-place biome/prettier/black on `*.example` is a miss**, not a
  simplification. The file is skipped or parsed as the wrong language.
- **Do not add this as `--check` only.** Hook and `--check` share
  `KnownExtension`.

### Acceptance

- Each new `.ext.example` is formatted by the same formatter as `.ext`,
  via that formatter's existing stdin-or-rename path.
- `notes.example` is still unknown. `foo.ts` behaviour is unchanged.
- `go test ./internal/dispatch/... ./internal/formatters/...` and `gate`
  stay green.

## Versioned php shebangs stay unknown

`shebangExt` maps exact `php` to `.php` (`cmd/format-dispatch/shebang.go:46`)
and prefix-matches `python` (`:51`) so `python3.12` works. `php8.3` and
`php-fpm` both start with `php`. A prefix match would format fpm config
scripts as PHP.

Decided shape: treat `php` plus a version suffix the way `python3.12` is
treated, and no other `php*` basename. `php8.3` → `.php`. `php-fpm`,
`php-cgi` stay unknown.

Rejected: `strings.HasPrefix(base, "php")`. That is the fpm trap.

### Read

1. `cmd/format-dispatch/shebang.go:46` — exact `php`.
2. `cmd/format-dispatch/shebang.go:51` — the python prefix this copies,
   including the comment at `:34` about version suffixes.
3. `cmd/format-dispatch/shebang_test.go` — add `php8.3` ok and `php-fpm`
   not ok next to the existing `php` and `unknown interpreter` (perl) cases.

### Traps

- **`php-fpm` is not PHP source.** A dash after `php` is a different
  program. Require the next character to be a digit (`php8.3`), not just
  a longer string.
- **Do not add perl.** There is no perl formatter. The perl cases in
  `shebang_test.go` stay `ok=false`.

### Acceptance

- `#!/usr/bin/env php8.3` on an extensionless file dispatches as `.php`.
- `#!/usr/bin/env php-fpm` stays unknown.
- Exact `php` still maps to `.php`. `gate` stays green.

## --check and --doctor have no machine-readable output

`--check` prints `would reformat` / `reformatted` lines and a summary
(`cmd/format-dispatch/check.go` after the job loop). `--doctor` prints a
tabwriter table (`cmd/format-dispatch/doctor.go:47`). CI that wants to
consume either has to scrape prose.

A `--json` flag on each, off by default, writing one JSON document to
stdout and keeping the current text as the default, would complete the
operator surface without changing the default contract.

This is a new public flag and a schema. It is not a one-function fill.

### Read

1. `cmd/format-dispatch/check.go` `runCheck` — the human lines and exit
   codes 0/1/2. JSON must not change those codes.
2. `cmd/format-dispatch/check.go` `parseCheckArgs` — `--write` is the only
   accepted dash-prefix today. `--json` would be a second, accepted
   anywhere among the paths the same way.
3. `cmd/format-dispatch/doctor.go:27` `runDoctor` — table, disabled list,
   `reportWiring`. JSON is those three structures, not a new probe.
4. `cmd/format-dispatch/main.go:99` — `--doctor` currently takes no args
   (`parseNoExtra`). `--json` is the exception that function would grow.

### Traps

- **Default output and exit codes stay.** A scraper that already greps
  `would reformat` must keep working when `--json` is absent.
- **`--doctor --json` is the one extra argument `--doctor` accepts.**
  `--doctor --bogus` stays a usage error.
- **Do not print JSON and the table together.** One or the other.

### Acceptance

- `format-dispatch --check --json PATH` exits 1 with a JSON document
  naming the files that would change, and 0 when none would.
- `format-dispatch --doctor --json` exits 0 with formatters, tools, and
  wiring.
- Neither flag is required. Existing tests keep passing. `gate` stays
  green.

## Subcommands do not take --help

Top-level `--help`/`-h` prints `usage` and exits 0
(`cmd/format-dispatch/main.go:110`). `--install --help` is an unrecognized
argument (`parseDryRun`). `--check --help` is now an unrecognized dash-prefix
(`parseCheckArgs`). `--doctor --help` is rejected by `parseNoExtra`.

`--install --help`, `--uninstall --help`, `--upgrade --help`,
`--check --help`, `--doctor --help` should print the same `usage` and
exit 0, without writing settings or walking paths.

Rejected: a per-subcommand help string. One usage block is what `--help`
already prints.

### Read

1. `cmd/format-dispatch/main.go:110` — the existing help path.
2. `cmd/format-dispatch/main.go` `parseDryRun` / `parseNoExtra` — each
   needs `--help`/`-h` as a distinguished success, not a typo.
3. `cmd/format-dispatch/check.go` `parseCheckArgs` — `--help`/`-h` among
   paths must not be a collectable path and must not look like
   `--write`.
4. `cmd/format-dispatch/run_test.go` `unrecognizedArgCases` — `--help` is
   not one of those cases; do not fold it in.

### Traps

- **`--help` must not install or check.** Exit before `runInstall` /
  `collectCheckTargets`.
- **`--check PATH --help` is still help**, matching `--check PATH --write`
  accepting the flag anywhere. Do not format `PATH` and then show usage.
- **Unknown flags stay errors.** Only `--help` and `-h` become success.

### Acceptance

- Each of `--install --help`, `--check --help`, `--doctor --help`,
  `--upgrade -h` prints `Usage:` and exits 0.
- `--install --help` does not write `settings.json`.
- `--check --bogus` still exits 2. `gate` stays green.

## install.sh --dry-run ignores a trailing extra argument

`ACTION="${1:-}"` and `DRY_RUN="${2:-}"` (`install.sh:21`). When `$ACTION`
is `--dry-run`, the script never inspects `$DRY_RUN` (`install.sh:77`) and
runs `--install --dry-run` even if argv was `./install.sh --dry-run extra`.
`--upgrade extra` is already rejected (`:73`).

Reject any argv the usage line does not name, before `go` is required, so
a typo does not compile first.

Rejected: treating a second argument to `--dry-run` as `--uninstall` or
as a path. The usage line names no such form.

### Read

1. `install.sh:21` — `ACTION` / `DRY_RUN`.
2. `install.sh:24` and `:28` — the allowed first words and the usage
   string.
3. `install.sh:77` — the `--dry-run` branch that swallows `$2`.
4. `install.sh:36` — `command -v go`. Validation must run above this so
   a typo is cheap.

### Traps

- **Windows CI does not run this script.** A Go test that `exec`s `bash`
  install.sh must skip on `windows`. Prefer asserting the `case` by
  extracting it, or document that `shellcheck` is the check.
- **Do not move `--upgrade`'s `DRY_RUN` check later.** It already errors;
  only `--dry-run extra` is the hole.

### Acceptance

- `./install.sh --dry-run extra` exits 2, prints usage, and does not
  invoke `go`.
- `./install.sh --dry-run` still previews `--install`.
- `./install.sh --upgrade --dry-run` is unchanged. `gate` stays green.

## Minified TypeScript is not in the generated-file skip list

`skipFilePatterns` (`internal/dispatch/dispatch.go:221`) skips `*.min.js`,
`*.min.mjs`, `*.min.cjs`, and `*.min.css`. Biome also owns `.ts`/`.tsx`
(`dispatch.go:71`). A `bundle.min.ts` in a source directory is formatted.

Adding `*.min.ts` and `*.min.tsx` matches the existing minified-JS rule.
It changes the live hook: files that are formatted today would start
being skipped. That is why this is not a silent fill.

Rejected: `*.min.*`. That would skip `foo.min.json` config the operator
wrote by hand.

### Read

1. `internal/dispatch/dispatch.go:221` `skipFilePatterns` — append next
   to `*.min.css`.
2. `internal/dispatch/dispatch.go:235` `InSkippedFile` — case-insensitive
   `path.Match`; tests already cover `Package-Lock.json`.
3. Hook and `--check` both call `InSkippedFile`. One list change covers
   both.

### Traps

- **This skips files that are currently formatted.** Call it out in
  CHANGELOG. Do not sneak it into an unrelated formatter change.
- **`.min.mts` / `.min.cts` are the same question.** Add them in the same
  commit if `.mts`/`.cts` stay biome-registered, or leave them out
  together. Do not add only `.ts`.

### Acceptance

- `app.min.ts` and `app.min.tsx` are skipped by the hook and by
  `--check`. `app.ts` is unchanged.
- User `skipFiles` still extends the list. `gate` stays green.

# TODO

Work that is decided but not built. `AGENTS.md` and `HUMANS.md` describe what
exists; this file is the only place that describes what does not.

Sections are unordered.

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

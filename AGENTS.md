# AGENTS.md — Developer onboarding

`claude-format-hooks` is a Claude Code `PostToolUse` hook: global Go binary
`format-dispatch` formats files after Write/Edit/MultiEdit/NotebookEdit.
Operators: [HUMANS.md](HUMANS.md).

## Stack

Go 1.27+. CI ([`.github/workflows/ci.yml`](.github/workflows/ci.yml)): the
`test` matrix (build, vet, race tests, 75% coverage floor, `--check`)
runs on `ubuntu-latest`/`windows-latest`. The ubuntu `lint` job runs
`golangci-lint` / `golangci-lint fmt` (gofumpt), govulncheck, then `gate`
(actionlint, shellcheck, gitleaks, and the restated `.gate.toml` checks).
Tag `v*` triggers
[`.github/workflows/release.yml`](.github/workflows/release.yml) cross-compile
release binaries.

## Commands

```bash
go build ./...
go vet ./...
golangci-lint fmt
golangci-lint run ./...
go test -race -cover ./...
```

Single package: `go test -race -v ./internal/formatters/...`.

## Layout

| Path | Role |
| ---- | ---- |
| [`cmd/format-dispatch/`](cmd/format-dispatch/) | Entrypoint, `--install`/`--uninstall`/`--upgrade`/`--check` (`--write`)/`--doctor`/`--version`/`--help`/`-h` |
| [`internal/hookio/`](internal/hookio/) | Stdin → file path (Claude + Cursor schemas) |
| [`internal/diskcache/`](internal/diskcache/) | TTL disk cache (`$CLAUDE_FORMAT_HOOKS_CACHE` or OS user cache/`claude-format-hooks`) |
| [`internal/config/`](internal/config/) | Indent: defaults → user config (`json`/`shell`) → `.editorconfig`; also `disabled`, `disabledFormatters`, `skipDirs`, `skipFiles` |
| [`internal/dispatch/`](internal/dispatch/) | Extension registry, non-source skip list (built-in + config `skipDirs`/`skipFiles`) |
| [`internal/formatters/`](internal/formatters/) | Per-type formatters (native + external) |
| [`internal/installer/`](internal/installer/) | Hook wiring, upgrade, bunx tool provisioning (`$CLAUDE_HOOKS_BIN_DIR`, `$CLAUDE_SETTINGS_FILE`, `$CURSOR_HOOKS_FILE`, `$CLAUDE_FORMAT_HOOKS_RELEASE_API`) |

`dispatch.NewRegistry` extensions: `.json` (biome when the project has a
biome config and a usable launcher, otherwise native `encoding/json.Indent`),
`.json.example` (always native JSON), `.sh`/`.bash`/`.zsh`/`.ksh`/`.dash`
(native `mvdan.cc/sh/v3`), `.go` (`gofumpt` on `PATH`, otherwise
`go/format.Source`; `goimports` runs first when it is on `PATH` and a parent
`.golangci.yml` lists it), `.mod`/`go.mod` via `go mod edit -fmt` and
`.work`/`go.work` via `go work edit -fmt` on those exact filenames, biome
for `.ts`/`.tsx`/`.js`/`.jsx`/`.mjs`/`.cjs`/`.mts`/`.cts`/`.css`/`.jsonc`/
`.jsonc.example` (stdin under a `.jsonc` name), markdownlint-cli2 for
`.md`/`.mdx`/`.markdown`/`.mdc`/`.md.example`, taplo for `.toml`/
`.toml.example` (`--stdin-filepath`), prettier for `.yaml`/`.yml`/
`.yaml.example`/`.yml.example` (`--parser yaml`), `.html`/`.scss`/`.less`/
`.vue`/`.svelte`/`.astro`, GraphQL router (biome or prettier) for
`.graphql`/`.gql`, sqlfluff for `.sql`/`.sql.example` (`--stdin-filename`),
ruff/black for `.py`/`.py.example`/`.pyi` (stdin under a `.py` name) and
notebooks (`.ipynb`), rustfmt (`.rs`), terraform/tofu (`.tf`/`.tfvars`/
`.tftest.hcl`/`.tfmock.hcl`/`.tfquery.hcl`), buf (`.proto`), clang-format
(`.c`/`.h`/`.cc`/`.cpp`/`.cxx`/`.hpp`/`.hh`/`.hxx`/`.m`/`.mm`), Java
(`.java`), Kotlin (`.kt`/`.kts`), Swift (`.swift`), Ruby (`.rb`/`.rake`/
`.gemspec`), PHP (`.php`), Nix (`.nix`), Lua (`.lua`). A bare `.example`
is not registered. External tools invoked via `bunx` or system `PATH`.
A formatter that shells out reports its candidate binaries via the optional
`formatters.Prober` interface, which is what `--doctor` reads; without one a
formatter reads as native. `json` implements `Prober` (biome, bunx) and
still falls back to the native formatter.

## Invariants

- Silent on success; truncated stderr diagnostic on formatter failure,
  plus one `hookSpecificOutput.additionalContext` JSON line on stdout so
  the model sees it (Claude payloads only -- Cursor does not define that
  channel). Nothing else may write to stdout on the hook path.
- Hook path always exits 0 (`--check` is the CI exception).
- Unsupported extension: instant no-op — no stat, exec, or config read.
  An extensionless file, or a hidden basename whose only suffix is the
  whole name (`.bashrc`), is first peeked at for a shebang naming a
  shell, python, ruby, node, bun, deno, php, or lua interpreter. `env -S` and other env flags
  are skipped so the real interpreter is seen.
- Missing external tools cached 30s (`internal/diskcache`); self-heals within TTL.
- Skips `node_modules/`, `.git/`, `vendor/`, `.venv/`, and peers; files outside `$CLAUDE_PROJECT_DIR`.
- Skips generated files by name (`*.min.js`, `*.min.ts`, `*-lock.json`, peers) even in a
  source directory. Config `skipDirs` / `skipFiles` extend both lists.
- No formatter requires project config to exist first, except prettier: it runs
  only where the file's project has a prettier config (`.prettierrc*`,
  `prettier.config.*`, or a package.json `prettier` key up to the project root).

## Conventions

- New formatters implement `formatters.Formatter`, register in `dispatch.NewRegistry`.
  External ones also implement `formatters.Prober` beside their `lookPath` calls.
  A PATH-binary-plus-in-place-flag tool needs no new type: add a constructor to
  [`internal/formatters/system.go`](internal/formatters/system.go).
- No drive-by refactors; match file style.
- Build and PR workflow: [CONTRIBUTING.md](CONTRIBUTING.md).

## Gate budget

Measured 2026-10-09 at load 5-30 on 32 cores. Detection found build, golangci-lint (which covers gofumpt and vet), `go test ./...`, govulncheck, actionlint and shellcheck. CI also runs the suite with `-race` and a 75% coverage floor, `go mod verify` and a `--check` dogfood of the freshly built binary, so `.gate.toml` replaces the plain test with the race and coverage run and adds the other two. Warm: 7.8 s wall and 16 s CPU (coverage run uncached; measured at load 21). Cold (fresh copy, throwaway Go build, module and lint caches, so the go1.27.2 toolchain download is included): 25 s wall and 193 s CPU at load 26-33. Warm is under the 10 s bar; cold is under 30 s wall but its CPU is the toolchain download and a full `-race` build, which a second cold run would not repeat.

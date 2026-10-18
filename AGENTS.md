# AGENTS.md — Developer onboarding

Global `format-dispatch` PostToolUse hook. Operators: [HUMANS.md](HUMANS.md).

## Stack

Go 1.27+. [`.github/workflows/ci.yml`](.github/workflows/ci.yml) and
[`.github/workflows/release.yml`](.github/workflows/release.yml) (`v*` tags).

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
| [`cmd/format-dispatch/`](cmd/format-dispatch/) | Entrypoint (`--help`) |
| [`internal/hookio/`](internal/hookio/) | Stdin → file path (Claude + Cursor) |
| [`internal/diskcache/`](internal/diskcache/) | TTL cache (`$CLAUDE_FORMAT_HOOKS_CACHE`) |
| [`internal/config/`](internal/config/) | Indent + `disabled*` / `skip*` |
| [`internal/dispatch/`](internal/dispatch/) | `NewRegistry`; skip lists |
| [`internal/formatters/`](internal/formatters/) | Native + external |
| [`internal/installer/`](internal/installer/) | Wire, upgrade, bunx |

Traps in `dispatch.NewRegistry`: `.json` uses biome only with a project
biome config and a usable launcher (else native Indent); `.json.example`
always native; `go.mod`/`go.work` only under those names; `gofumpt` else
native `gofmt`, with `goimports` first when PATH has it and a parent
`.golangci.yml` lists it; prettier only with a project prettier config;
bare `.example` unregistered; JSONC stdin named `.jsonc`, Python stdin
`.py`; sqlfluff `--stdin-filename`; taplo `--stdin-filepath`. External
via `bunx` or `PATH`. Shell-out formatters implement `formatters.Prober`
(`--doctor`); `json` probes biome/bunx and still falls back native.
PATH-binary-plus-in-place-flag: [`internal/formatters/system.go`](internal/formatters/system.go).

## Invariants

- Silent on success. Formatter failure: truncated stderr plus one
  `hookSpecificOutput.additionalContext` stdout line (Claude payloads
  only). Nothing else on stdout on the hook path.
- Hook path always exits 0 (`--check` is the CI exception).
- Unsupported extension: no-op (no stat/exec/config). Extensionless or
  hidden basename (`.bashrc`): peek shebang for shell, python, ruby,
  node, bun, deno, php, lua; skip `env -S` and other env flags.
- Missing tools cached 30s (`internal/diskcache`).
- Skip `node_modules/`, `.git/`, `vendor/`, `.venv/`, peers; files
  outside `$CLAUDE_PROJECT_DIR`; generated names (`*.min.js`,
  `*.min.ts`, `*-lock.json`, peers). `skipDirs`/`skipFiles` extend both.
- Prettier is the only formatter that requires project config
  (`.prettierrc*`, `prettier.config.*`, or package.json `prettier`).

## Conventions

New formatters: `formatters.Formatter` + `dispatch.NewRegistry`;
external also `formatters.Prober` beside `lookPath`. No drive-by
refactors. PRs: [CONTRIBUTING.md](CONTRIBUTING.md).

## Gate budget

Measured 2026-10-09, load 5-30, 32 cores. Warm: 7.8 s wall / 16 s CPU
(coverage uncached, load 21). Cold (fresh copy, throwaway Go build,
module and lint caches, go1.27.2 toolchain download): 25 s wall /
193 s CPU at load 26-33. Warm < 10 s; cold < 30 s wall. `.gate.toml`
restates CI's race+coverage, `go mod verify`, and `--check` dogfood.

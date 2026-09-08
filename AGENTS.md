# AGENTS.md — Developer onboarding

`claude-format-hooks` is a Claude Code `PostToolUse` hook: global Go binary
`format-dispatch` formats files after Write/Edit/MultiEdit/NotebookEdit.
Operators: [HUMANS.md](HUMANS.md).

## Stack

Go 1.27+. CI ([`.github/workflows/ci.yml`](.github/workflows/ci.yml)): build,
vet, gofmt, test (75% coverage floor), golangci-lint, govulncheck on
`ubuntu-latest`/`macos-latest`/`windows-latest`. Tag `v*` triggers
[`.github/workflows/release.yml`](.github/workflows/release.yml) cross-compile
release binaries.

## Commands

```bash
go build ./...
go vet ./...
gofmt -l .
golangci-lint run ./...
go test -race -cover ./...
```

Single package: `go test -race -v ./internal/formatters/...`.

## Layout

| Path | Role |
| ---- | ---- |
| [`cmd/format-dispatch/`](cmd/format-dispatch/) | Entrypoint, `--install`/`--uninstall`/`--upgrade`/`--check` |
| [`internal/hookio/`](internal/hookio/) | Stdin → file path (Claude + Cursor schemas) |
| [`internal/diskcache/`](internal/diskcache/) | TTL disk cache shared by config + formatters |
| [`internal/config/`](internal/config/) | Indent: defaults → user config → `.editorconfig` |
| [`internal/dispatch/`](internal/dispatch/) | Extension registry, vendored-dir skip list |
| [`internal/formatters/`](internal/formatters/) | Per-type formatters (native + external) |
| [`internal/installer/`](internal/installer/) | Hook wiring, upgrade, bunx tool provisioning |

Native formatters: JSON (`encoding/json.Indent`), shell (`mvdan.cc/sh/v3`),
Go (`go/format.Source`). External tools invoked via `bunx` or system `PATH`.

## Invariants

- Silent on success; truncated stderr diagnostic on formatter failure.
- Hook path always exits 0 (`--check` is the CI exception).
- Unsupported extension: instant no-op — no stat, exec, or config read.
  An extensionless file is first peeked at for a shell shebang.
- Missing external tools cached briefly (`internal/diskcache`); self-heals within TTL.
- Skips `node_modules/`, `.git/`, `vendor/`, `.venv/`, and peers; files outside `$CLAUDE_PROJECT_DIR`.
- No formatter requires project config to exist first.

## Conventions

- New formatters implement `formatters.Formatter`, register in `dispatch.NewRegistry`.
- No drive-by refactors; match file style.
- Build and PR workflow: [CONTRIBUTING.md](CONTRIBUTING.md).

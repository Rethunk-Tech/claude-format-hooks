# AGENTS.md - Developer / LLM Onboarding

`claude-format-hooks` is a [Claude Code](https://claude.com/claude-code)
`PostToolUse` hook: a single global Go binary (`format-dispatch`) that
formats/lints a file right after Write/Edit/NotebookEdit writes it, with no
per-repo setup required. **Operators:** see [HUMANS.md](HUMANS.md).

## Commands

```bash
go build ./...
go vet ./...
gofmt -l .
golangci-lint run ./...
go test -race -cover ./...
```

Single package: `go test -race -v ./internal/formatters/...`. Single test:
`go test -race -v -run TestJSONFormatterIdempotent ./internal/formatters`.

CI ([`.github/workflows/ci.yml`](.github/workflows/ci.yml)) runs build/vet/
gofmt/test plus a 75% total-coverage floor (`go tool cover -func`) across a
`ubuntu-latest`/`macos-latest`/`windows-latest` matrix; `golangci-lint` and
`govulncheck` run once, on `ubuntu-latest` only, since static analysis and
vulnerability data don't vary by OS. Runs on every push and pull request to
`main`.

A tag push matching `v*` triggers
[`.github/workflows/release.yml`](.github/workflows/release.yml): cross-
compiles `format-dispatch` for linux/darwin/windows (amd64+arm64) from a
single `ubuntu-latest` runner (pure Go, no cgo) and publishes a GitHub
Release with the binaries and their sha256sums via `gh release create`.
The target list mirrors the OS matrix `ci.yml` tests on — a platform that
is worth testing every push is worth shipping.

This is the canonical command reference — [CONTRIBUTING.md](CONTRIBUTING.md)
points here instead of repeating it.

## Architecture

This hook runs on _every_ file write in a session. Its own dispatch logic
(read stdin, extract a path, switch on extension) is trivial — the real
cost is process startup, which is repeated every single invocation:

| Runtime          | Cold-start overhead                       |
| ---------------- | ----------------------------------------- |
| Go (this binary) | ~1–3ms                                    |
| Bash + jq        | ~5–15ms                                   |
| Python           | ~30–60ms (worse behind a venv/pyenv shim) |

Three formatters are implemented **natively in-process**, skipping the
subprocess entirely:

- **JSON** — `encoding/json.Indent`, chosen deliberately over
  Unmarshal+Marshal: `Indent` re-indents at the byte level without
  building an object graph, so it preserves source key order exactly. A
  round-trip through `map[string]interface{}` would silently alphabetize
  every object's keys, since `encoding/json.Marshal` sorts map keys —
  that's not what "format" means for a file a human authored. An upward
  `biome.json`/`biome.jsonc` plus a resolvable `biome` binary routes `.json`
  through Biome instead; otherwise this native path applies.
- **Shell scripts** — [`mvdan.cc/sh/v3`](https://pkg.go.dev/mvdan.cc/sh/v3),
  the actual parser/printer package the `shfmt` binary itself is built on.
  Output matches `shfmt` exactly; there's no subprocess to spawn at all.
- **Go** — stdlib `go/format.Source`, the same formatting engine `gofmt`
  itself is built on. Unlike JSON, there's no fidelity trade-off to weigh
  at all: Go source has exactly one canonical formatting, so this is an
  even safer native candidate than JSON was.

Everything else stays external, on purpose, after actually testing the
native alternatives rather than assuming:

- **YAML** — `gopkg.in/yaml.v3`'s `Node` round-trip was prototyped against
  a real workflow file in this fleet. It stripped every blank line and
  re-indented list items under mapping keys — a real, surprising diff, not
  a safe formatting operation. Stays on `prettier`.
- **TOML** — no mature Go library does comment/structure-preserving
  reformatting the way `taplo` does; a naive parse+re-encode would lose
  comments and reorder keys. Stays on `taplo` via `bunx`.
- **HTML** — reformatting through any tree-based parser risks restructuring
  markup per the HTML5 tree-construction algorithm (implied tag closing,
  element repositioning). `prettier` already has this exact failure mode
  against hand-authored custom-element markup — a native Go attempt would
  not improve on it. Stays on `prettier`.
- **TS/TSX/JS/JSX/CSS/JSONC** — `biome` is Rust with no Go bindings.
- **SQL** — `sqlfluff` is Python; no Go equivalent exists.
- **Python** — `ruff format` (preferred) or `black` as a fallback; both
  are Python tools, no Go equivalent exists. Whichever is on `PATH` runs;
  ruff wins if both are present.
- **Jupyter notebooks (`.ipynb`)** — `ruff format` (preferred) or `black`
  with notebook support; subprocess only.
- **Rust** — `rustfmt`; no Go equivalent exists, and unlike JSON there's
  no fidelity trade-off to weigh — rustfmt is the canonical formatter for
  Rust itself, the same relationship gofmt has to Go.
- **Terraform/HCL** — `terraform fmt`; no Go equivalent invokable
  in-process without vendoring HashiCorp's own `hclwrite`, and (like
  rustfmt) there's no competing tool to weigh.
- **Protobuf** — `buf format -w`. buf is Go, but its formatter isn't
  exposed as a stable importable package the way `go/format` is, and
  vendoring the toolchain to save one subprocess would trade a large
  dependency for a few milliseconds. Like terraform/rustfmt, it's the
  canonical formatter for the language.

External formatters already read their own project config (`biome.json`,
`.prettierrc`, `.sqlfluff`, ...) automatically, since we invoke the real
tool. Two exceptions needed a config story of their own (see
[HUMANS.md](HUMANS.md#configuration)):

- The two **native** formatters have no project config file to consult at
  all, so `internal/config` resolves their indent settings.
- **markdownlint-cli2** discovers config only by walking up from the linted
  file as far as the working directory — unlike `sqlfluff`, which merges
  `~/.sqlfluff` before any project config, it has no user-level location.
  `internal/formatters/markdownconfig.go` supplies the missing layer by
  passing `--config` with a user-level base config, which a project's own
  config still overrides rule by rule.

## Layout

| Path                                           | Role                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      |
| ---------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| [`cmd/format-dispatch/`](cmd/format-dispatch/) | Entrypoint: stdin parsing, extension gate, vendored-dir/project-root checks, project-level formatter opt-out, timeout, exit-0 contract; also dispatches `--install`/`--uninstall`/`--upgrade` to `internal/installer`. `check.go` implements `--check`, the one path that deliberately does NOT exit 0 — it formats a copy beside each file and compares bytes, so it needs no per-tool dry-run flag and matches the hook's behavior exactly                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| [`internal/hookio/`](internal/hookio/)         | Decodes the `PostToolUse` JSON payload into a file path                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| [`internal/diskcache/`](internal/diskcache/)   | Small disk-backed key/value cache with TTL-based expiry, shared by `internal/config` and `internal/formatters` (neither may import the other in the direction this package would require) — every result that's expensive to recompute on every invocation but rarely changes mid-session goes through here                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
| [`internal/config/`](internal/config/)         | Resolves per-file indent settings: built-in defaults -> user config -> `.editorconfig`; the `.editorconfig` resolution itself is cached via `internal/diskcache`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| [`internal/dispatch/`](internal/dispatch/)     | Extension -> `Formatter` registry, vendored-dir list, disabled-extension filtering                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| [`internal/formatters/`](internal/formatters/) | One `Formatter` implementation per file type (native: `json.go`, `shell.go`, `golang.go`; external: `biome.go`, `bunxtool.go`, `sqlfluff.go`, `python.go`, `rust.go`, `terraform.go`, `proto.go`); `exec.go` holds the shared subprocess-run + diagnostic-truncation helper; `binpath.go` holds the shared, cached `lookPath` every external formatter uses instead of calling `exec.LookPath` directly; `writefile.go` holds the shared mode-preserving write every native formatter uses instead of its own stat-then-write; `markdownconfig.go` + `markdownlint-defaults.jsonc` supply markdownlint-cli2's missing user-level config layer                                                                                                                                                                                                                                             |
| [`internal/installer/`](internal/installer/)   | Wires/unwires format-dispatch's `PostToolUse` hook in `~/.claude/settings.json` (`format-dispatch --install`/`--uninstall`), replacing `install.sh`'s old `jq` filter; `upgrade.go` downloads and verifies the latest platform release binary (`format-dispatch --upgrade`), supports `--upgrade --dry-run`, and atomically replaces the installed binary without rewriting `settings.json`; `tools.go` globally provisions the bunx-dispatched formatters at install time (and pins transitive deps carrying an unpatched advisory) so no registry fetch happens inside the per-file budget; `orderedmap.go` preserves the file's existing key order across the rewrite and a `.bak` backup is written before any real change; both settings and binary writes go through `writeAtomic` (unique same-dir temp + rename) so a kill mid-write can never truncate the operator's live files |

## Invariants

Unchanged from the hand-written per-project hooks this replaces:

- **Silent on success** — nothing printed, saves tokens in the transcript.
- **On failure**, a truncated (≤10 lines / 500 chars) diagnostic goes to
  stderr so a broken fixer is still debuggable; if the failing command
  produced no output at all, the diagnostic falls back to the process
  error itself rather than an empty string.
- **Always exits 0.** A `PostToolUse` hook runs after the tool call
  already succeeded — it must never be the reason a Write/Edit/
  NotebookEdit call reports failure. `--check` is the one deliberate
  exception: it is not a hook invocation, and exists precisely to fail a
  build. It must never share the hook's exit path.
- **An unsupported real extension is an instant no-op** — one
  `dispatch.ResolveExtension` call (longest registered suffix, else
  `filepath.Ext`) and one map lookup, nothing else — no `stat`, no
  `exec.LookPath`, no subprocess, no config read. An extensionless path is
  the only exception: the hook may read at most 256 bytes to detect a
  supported shell shebang before deciding whether to route it as `.sh`.
- **Anything expensive that rarely changes mid-session is cached for a
  short TTL via `internal/diskcache`**, not recomputed on every
  invocation: a missing external tool binary (`binPathCacheTTL`,
  `internal/formatters/binpath.go` — every external formatter must go
  through `formatters.lookPath`, never a bare `exec.LookPath`), biome's
  resolved config directory (`findUpwardCacheTTL`,
  `internal/formatters/biome.go`), and EditorConfig resolution
  (`editorconfigCacheTTL`, `internal/config/config.go`). Each check
  reruns after its TTL, so a change made mid-session (installing the
  missing tool, adding a config file) is picked up without restarting
  Claude Code.
- **Every formatter runs unconditionally within scope** — no formatter
  requires its own project config file to exist first. A `.ts` file in a
  project with no `biome.json` still gets formatted with biome's built-in
  defaults, the same way a `.md` file with no `.markdownlint.json` gets
  formatted against the user-level base config
  (`internal/formatters/markdownconfig.go`). Do not reintroduce a
  config-presence gate on any formatter (biome had one; it was removed —
  see `CHANGELOG.md`).
- Files under `node_modules/`, `.next/`, `.yarn/`, `.git/`, `.agents/`,
  `dist/`, `build/`, `coverage/`, `test-results/`, `vendor/`, `.venv/`,
  `.terraform/`, `__pycache__/`, `.ruff_cache/`, `.mypy_cache/`,
  `.pytest_cache/`, or `.tox/` (anywhere in the path), or outside
  `$CLAUDE_PROJECT_DIR`, are always skipped (`dispatch.InVendoredDir`,
  `main.within`).

## Conventions

- No drive-by refactors; match the style of the file being touched.
- New formatters implement the `formatters.Formatter` interface
  ([`formatter.go`](internal/formatters/formatter.go)) and register in
  `dispatch.NewRegistry` ([`dispatch.go`](internal/dispatch/dispatch.go)).
- Prefer a native implementation only after actually testing it against a
  real file from this fleet, per the Architecture rationale above — do not
  assume a Go library is formatting-fidelity-safe without checking.
- Commit conventions, PR checklist: [CONTRIBUTING.md](CONTRIBUTING.md).

## No CLI framework (decided v0.2.0, 2026-07-19)

`cmd/format-dispatch/main.go` keeps its hand-rolled `dispatchArgs` switch. Cobra (+pflag), urfave/cli v3, alecthomas/kong, and peterbourgon/ff/ffcli were all researched and rejected.

Cobra was the strongest candidate — the only one with real shell completion at no extra deps beyond pflag — for a measured **+28% binary size** (3008 KB → ~3860 KB) plus a 60–80 line rewrite and test restructuring.

Rejected anyway because the flag surface (`--install` / `--uninstall` / `--dry-run` / `--version` / `--help`) is a rarely-used ops side door: ~99% of invocations are the zero-arg `PostToolUse` hook path reading stdin JSON, which no framework touches. There are no POSIX combined short-flags to parse, man-page generation is a build-time side tool under every candidate, and completion has little value for a binary Claude Code invokes programmatically.

**Don't re-propose these** without a materially new argument — e.g. the CLI surface grows past ~3–4 flat subcommands, or users actually ask for completion.

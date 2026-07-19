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
compiles `format-dispatch` for linux/darwin (amd64+arm64) from a single
`ubuntu-latest` runner (pure Go, no cgo) and publishes a GitHub Release
with the binaries and their sha256sums via `gh release create`.

This is the canonical command reference — [CONTRIBUTING.md](CONTRIBUTING.md)
points here instead of repeating it.

## Architecture

This hook runs on *every* file write in a session. Its own dispatch logic
(read stdin, extract a path, switch on extension) is trivial — the real
cost is process startup, which is repeated every single invocation:

| Runtime | Cold-start overhead |
| --- | --- |
| Go (this binary) | ~1–3ms |
| Bash + jq | ~5–15ms |
| Python | ~30–60ms (worse behind a venv/pyenv shim) |

Three formatters are implemented **natively in-process**, skipping the
subprocess entirely:

- **JSON** — `encoding/json.Indent`, chosen deliberately over
  Unmarshal+Marshal: `Indent` re-indents at the byte level without
  building an object graph, so it preserves source key order exactly. A
  round-trip through `map[string]interface{}` would silently alphabetize
  every object's keys, since `encoding/json.Marshal` sorts map keys —
  that's not what "format" means for a file a human authored.
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
- **Rust** — `rustfmt`; no Go equivalent exists, and unlike JSON there's
  no fidelity trade-off to weigh — rustfmt is the canonical formatter for
  Rust itself, the same relationship gofmt has to Go.

External formatters already read their own project config (`biome.json`,
`.prettierrc`, `.sqlfluff`, ...) automatically, since we invoke the real
tool. Only the two native formatters needed their own config story (see
[HUMANS.md](HUMANS.md#configuration)).

## Layout

| Path | Role |
| --- | --- |
| [`cmd/format-dispatch/`](cmd/format-dispatch/) | Entrypoint: stdin parsing, extension gate, vendored-dir/project-root checks, project-level formatter opt-out, timeout, exit-0 contract; also dispatches `--install`/`--uninstall` to `internal/installer` |
| [`internal/hookio/`](internal/hookio/) | Decodes the `PostToolUse` JSON payload into a file path |
| [`internal/config/`](internal/config/) | Resolves per-file indent settings: built-in defaults -> user config -> `.editorconfig` |
| [`internal/dispatch/`](internal/dispatch/) | Extension -> `Formatter` registry, vendored-dir list, disabled-extension filtering |
| [`internal/formatters/`](internal/formatters/) | One `Formatter` implementation per file type (native: `json.go`, `shell.go`, `golang.go`; external: `biome.go`, `bunxtool.go`, `sqlfluff.go`, `python.go`, `rust.go`); `exec.go` holds the shared subprocess-run + diagnostic-truncation helper; `binpath.go` holds the shared, disk-cached `lookPath` every external formatter uses instead of calling `exec.LookPath` directly |
| [`internal/installer/`](internal/installer/) | Wires/unwires format-dispatch's `PostToolUse` hook in `~/.claude/settings.json` (`format-dispatch --install`/`--uninstall`), replacing `install.sh`'s old `jq` filter; `orderedmap.go` preserves the file's existing key order across the rewrite and a `.bak` backup is written before any real change |

## Invariants

Unchanged from the hand-written per-project hooks this replaces:

- **Silent on success** — nothing printed, saves tokens in the transcript.
- **On failure**, a truncated (≤10 lines / 500 chars) diagnostic goes to
  stderr so a broken fixer is still debuggable; if the failing command
  produced no output at all, the diagnostic falls back to the process
  error itself rather than an empty string.
- **Always exits 0.** A `PostToolUse` hook runs after the tool call
  already succeeded — it must never be the reason a Write/Edit/
  NotebookEdit call reports failure.
- **An unsupported extension is an instant no-op** — one `filepath.Ext`
  call and one map lookup, nothing else — no `stat`, no `exec.LookPath`,
  no subprocess, no config read.
- **A missing external tool binary is remembered for a short TTL**
  (`binPathCacheTTL`, `internal/formatters/binpath.go`) so a burst of
  file writes doesn't re-walk `$PATH` for every one; the check reruns
  after the TTL, so installing the tool mid-session is picked up without
  restarting Claude Code. Every external formatter must go through
  `formatters.lookPath`, never a bare `exec.LookPath`, to get this.
- **Every formatter runs unconditionally within scope** — no formatter
  requires its own project config file to exist first. A `.ts` file in a
  project with no `biome.json` still gets formatted with biome's built-in
  defaults, the same way a `.md` file with no `.markdownlint.json` gets
  formatted with markdownlint-cli2's defaults. Do not reintroduce a
  config-presence gate on any formatter (biome had one; it was removed —
  see `CHANGELOG.md`).
- Files under `node_modules/`, `.next/`, `.yarn/`, `.git/`, `.agents/`,
  `dist/`, `build/`, `coverage/`, `test-results/`, `vendor/`, or `.venv/`
  (anywhere in the path), or outside `$CLAUDE_PROJECT_DIR`, are always
  skipped (`dispatch.InVendoredDir`, `main.within`).

## Conventions

- No drive-by refactors; match the style of the file being touched.
- New formatters implement the `formatters.Formatter` interface
  ([`formatter.go`](internal/formatters/formatter.go)) and register in
  `dispatch.NewRegistry` ([`dispatch.go`](internal/dispatch/dispatch.go)).
- Prefer a native implementation only after actually testing it against a
  real file from this fleet, per the Architecture rationale above — do not
  assume a Go library is formatting-fidelity-safe without checking.
- Commit conventions, PR checklist: [CONTRIBUTING.md](CONTRIBUTING.md).

# claude-format-hooks

A [Claude Code](https://claude.com/claude-code) `PostToolUse` hook that
formats/lints a file right after Write/Edit/NotebookEdit writes it —
generalized from a hand-written per-project hook into a single global Go
binary (`format-dispatch`), with no per-repo setup required.

## Why a compiled binary instead of a shell script

This hook runs on *every* file write in a session. Its own dispatch logic
(read stdin, extract a path, switch on extension) is trivial — the real
cost is process startup, which is repeated every single invocation:

| Runtime | Cold-start overhead |
|---|---|
| Go (this binary) | ~1–3ms |
| Bash + jq | ~5–15ms |
| Python | ~30–60ms (worse behind a venv/pyenv shim) |

Two formatters are also implemented **natively in-process**, skipping the
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

External formatters already read their own project config (`biome.json`,
`.prettierrc`, `.sqlfluff`, ...) automatically, since we invoke the real
tool. Only the two native formatters needed their own config story — see
below.

## Supported extensions

| Extension | Formatter | Native? |
|---|---|---|
| `.json` | `encoding/json.Indent` | yes |
| `.sh`, `.bash` | `mvdan.cc/sh/v3` | yes |
| `.ts`, `.tsx`, `.js`, `.jsx`, `.mjs`, `.cjs`, `.css`, `.jsonc` | `biome check --write` | no (bunx) |
| `.md`, `.mdx` | `markdownlint-cli2 --fix` | no (bunx) |
| `.toml` | `taplo format` | no (bunx) |
| `.yaml`, `.yml`, `.html` | `prettier --write` | no (bunx) |
| `.sql` | `sqlfluff fix` | no (system binary) |

Any other extension is an instant no-op: one `filepath.Ext` call and one
map lookup, nothing else — no `stat`, no `exec.LookPath`, no subprocess.

Files under `node_modules/`, `.next/`, `.yarn/`, `.git/`, `.agents/`,
`dist/`, `build/`, `coverage/`, `test-results/`, `vendor/`, or `.venv/`
(anywhere in the path) are always skipped, as are files outside
`$CLAUDE_PROJECT_DIR`.

## Configuration

Three layers, in increasing priority:

1. **Built-in defaults** — 2-space indent for both native formatters.
2. **Your own config** — `~/.claude/claude-format-hooks.json` (or
   `$CLAUDE_FORMAT_HOOKS_CONFIG` to point elsewhere):

   ```json
   {
     "json": { "indentSize": 2, "useTabs": false },
     "shell": { "indentSize": 2, "useTabs": false, "switchCaseIndent": true },
     "disabled": [".sql"]
   }
   ```

   `disabled` lists extensions to skip entirely, even if their formatter
   is installed.

3. **The target project's `.editorconfig`** — if a section covers the
   file being formatted, its `indent_style`/`indent_size` win over your
   personal config, the same way every editor and formatter that honors
   EditorConfig behaves. A missing or malformed config file never blocks
   a file write — the hook falls back to defaults and says why on stderr.

## Design contract

Unchanged from the hand-written per-project hooks this replaces:

- **Silent on success** — nothing printed, saves tokens in the transcript.
- **On failure**, a truncated (≤10 lines / 500 chars) diagnostic goes to
  stderr so a broken fixer is still debuggable.
- **Always exits 0.** A `PostToolUse` hook runs after the tool call
  already succeeded — it must never be the reason a Write/Edit/
  NotebookEdit call reports failure.

## Install

```bash
git clone git@github.com:Rethunk-Tech/claude-format-hooks.git
cd claude-format-hooks
./install.sh              # builds the binary, wires ~/.claude/settings.json
./install.sh --dry-run    # preview the settings.json diff, write nothing
```

The installer builds `format-dispatch` to `~/.claude/hooks/format-dispatch`
and adds (or replaces an existing narrower biome-only hook with) a
`PostToolUse` entry for `Write|Edit|NotebookEdit`, invoked via the hook
schema's exec form (`command` + `args: []`) — no shell spawned to launch
it, just the binary directly.

If Claude Code is already running, open `/hooks` once (or restart) to pick
up the change — the settings watcher only watches directories that had a
settings file when the session started.

## Development

```bash
go build ./...
go vet ./...
gofmt -l .
```

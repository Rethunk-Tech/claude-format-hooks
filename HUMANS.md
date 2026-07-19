# HUMANS.md — Using claude-format-hooks

This file covers **operator** setup, configuration, and troubleshooting.
For **developer/architecture context**, see [AGENTS.md](AGENTS.md).

## What it does

A [Claude Code](https://claude.com/claude-code) `PostToolUse` hook that
formats/lints a file right after Write/Edit/NotebookEdit writes it — a
single global Go binary (`format-dispatch`), with no per-repo setup
required.

## Quick start

### Prerequisites

- [Go](https://go.dev/) (see the `go` version in [go.mod](go.mod))
- Optional: [`bun`](https://bun.sh/) (provides `bunx`) for the
  bunx-dispatched formatters — biome, prettier, taplo, markdownlint-cli2.
  Without it, files handled by those formatters are silently skipped;
  JSON, shell, and (if installed) `sqlfluff` still work.
- Optional: [`sqlfluff`](https://sqlfluff.com/) (system binary, e.g. via
  `pipx install sqlfluff`) for `.sql` formatting.

### Installation

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

If `bunx` is on `PATH`, the installer also pre-warms its package cache for
biome, prettier, taplo, and markdownlint-cli2 (`bunx <pkg> --version`) —
each is unpinned (floats to whatever version bunx resolves) and would
otherwise pay a cold npm-registry fetch on the first file write of a
session, against the hook's 25s per-file timeout. This is best-effort: a
failed pre-warm never fails the install, since the same fetch just retries
on first use.

Every real write to `settings.json` (install or uninstall) first backs up
its current content to a sibling `settings.json.bak` — a single rolling
backup of the last-known-good state, overwritten on each subsequent write,
not a history. Re-running with no actual change to make (e.g. installing
when already installed) skips the write, and the backup, entirely.

If Claude Code is already running, open `/hooks` once (or restart) to pick
up the change — the settings watcher only watches directories that had a
settings file when the session started.

Env overrides (mainly for testing): `CLAUDE_HOOKS_BIN_DIR` (default
`~/.claude/hooks`), `CLAUDE_SETTINGS_FILE` (default
`~/.claude/settings.json`).

### Uninstall

```bash
~/.claude/hooks/format-dispatch --uninstall              # remove the PostToolUse entry
~/.claude/hooks/format-dispatch --uninstall --dry-run    # preview the diff, write nothing
rm ~/.claude/hooks/format-dispatch                        # then delete the binary
```

`--uninstall` removes only the `PostToolUse` entry whose `command` points
at `~/.claude/hooks/format-dispatch` (or `$CLAUDE_HOOKS_BIN_DIR`'s
binary); every other key and hook entry is left exactly as it was. A
settings.json with no such entry is a no-op.

### Other flags

```bash
format-dispatch --version    # print version and build info (for bug reports)
format-dispatch --help       # usage
```

An unrecognized flag prints usage to stderr and exits 1, rather than
hanging on stdin — safe to run by hand while debugging.

## Supported extensions

| Extension | Formatter | Native? |
| --- | --- | --- |
| `.json` | `encoding/json.Indent` | yes |
| `.sh`, `.bash` | `mvdan.cc/sh/v3` | yes |
| `.go` | `go/format.Source` | yes |
| `.ts`, `.tsx`, `.js`, `.jsx`, `.mjs`, `.cjs`, `.css`, `.jsonc` | `biome check --write` | no (bunx) |
| `.md`, `.mdx` | `markdownlint-cli2 --fix` | no (bunx) |
| `.toml` | `taplo format` | no (bunx) |
| `.yaml`, `.yml`, `.html` | `prettier --write` | no (bunx) |
| `.sql` | `sqlfluff fix` | no (system binary) |

Any other extension is an instant no-op. Vendored/build directories and
anything outside `$CLAUDE_PROJECT_DIR` are always skipped — see
[AGENTS.md § Invariants](AGENTS.md#invariants) for the exact list. None of
the formatters require their own project config file to exist first — a
`.ts` file in a project with no `biome.json` still gets formatted, using
biome's built-in defaults.

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

## Troubleshooting

**Nothing happened after I wrote a file.** That's the default, silent
success — check the extension is in the [supported table](#supported-extensions)
above and not in your `disabled` list. If the file lives under a
vendored directory, it's skipped on purpose.

**A diagnostic showed up on stderr.** The formatter ran and failed (e.g.
malformed syntax it couldn't safely fix). The message is truncated to 10
lines / 500 characters — run the underlying tool directly on the file for
the full error. The hook itself always exits 0; a formatter failure never
blocks your Write/Edit.

**The hook doesn't seem to be running at all.** Re-run `./install.sh` and
confirm the `PostToolUse` entry is in `~/.claude/settings.json`, then open
`/hooks` once (or restart Claude Code) — the settings watcher only watches
directories that had a settings file when the session started.

**A `.yaml`/`.toml`/`.md` file wasn't formatted even though the table says
it should be.** Confirm `bunx` is on `PATH` (`command -v bunx`); those
formatters are silently skipped without it. A first-ever `bunx` fetch on a
machine can also be slow enough to hit the hook's 25s timeout — re-running
`install.sh` pre-warms the cache for this.

## See also

- [AGENTS.md](AGENTS.md) — architecture and developer reference
- [CONTRIBUTING.md](CONTRIBUTING.md) — build, test, and pull request workflow
- [SECURITY.md](SECURITY.md) — vulnerability reporting

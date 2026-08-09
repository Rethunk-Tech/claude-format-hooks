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
- Optional: [`ruff`](https://docs.astral.sh/ruff/) or
  [`black`](https://black.readthedocs.io/) (system binary) for `.py`
  formatting — ruff is tried first if both are installed.
- Optional: [`rustfmt`](https://github.com/rust-lang/rustfmt) (installed
  with the Rust toolchain via `rustup component add rustfmt`) for `.rs`
  formatting.
- Optional: [`terraform`](https://developer.hashicorp.com/terraform)
  (system binary) for `.tf` formatting.
- Optional: [`buf`](https://buf.build/) (system binary) for `.proto`
  formatting.

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

If `bun` is on `PATH`, `--install` also installs biome, prettier, taplo, and
markdownlint-cli2 globally (`bun add -g`), putting their binaries on `PATH`
where `bunx` reaches them immediately. Otherwise the first file write of a
session pays a cold npm-registry fetch against the hook's 4s per-file
timeout, which it will not fit inside. The same step pins transitive
dependencies carrying an unpatched advisory, merging into bun's global
manifest rather than replacing it, so anything else you installed globally
is left alone.

This is best-effort: a failed install never fails the install as a whole,
since `bunx` still falls back to fetching on demand.

**No Go toolchain?** Download the `linux`/`darwin`/`windows` (amd64 or
arm64) binary from the [latest release](https://github.com/Rethunk-Tech/claude-format-hooks/releases/latest)
instead of building from source, place it at
`~/.claude/hooks/format-dispatch`, `chmod +x` it, then run
`~/.claude/hooks/format-dispatch --install` yourself. The binary's
`ProvisionTools` step still runs when `bun` is available; only `install.sh`'s
Go build wrapper is skipped.

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

For CI formatting checks, see [`--check PATH...`](#checking-formatting-in-ci).

An unrecognized flag prints usage to stderr and exits 1, rather than
hanging on stdin — safe to run by hand while debugging.

## Supported extensions

| Extension | Formatter | Native? |
| --- | --- | --- |
| `.json` | `biome check --write` when an upward `biome.json`/`biome.jsonc` is found and `biome` is available through `bunx`/`PATH`; otherwise `encoding/json.Indent` | conditional |
| `.sh`, `.bash` | `mvdan.cc/sh/v3` | yes |
| `.go` | `go/format.Source` | yes |
| `.ts`, `.tsx`, `.js`, `.jsx`, `.mjs`, `.cjs`, `.mts`, `.cts`, `.css`, `.jsonc` | `biome check --write` | no (bunx) |
| `.md`, `.mdx`, `.markdown` | `markdownlint-cli2 --fix` | no (bunx) |
| `.toml` | `taplo format` | no (bunx) |
| `.yaml`, `.yml`, `.html`, `.scss`, `.less`, `.graphql`, `.gql` | `prettier --write` | no (bunx) |
| `.sql` | `sqlfluff fix` | no (system binary) |
| `.py` | `ruff format` (preferred) or `black` | no (system binary) |
| `.rs` | `rustfmt` | no (system binary) |
| `.tf` | `terraform fmt` | no (system binary) |
| `.proto` | `buf format -w` | no (system binary) |

Any other extension is an instant no-op. Vendored/build directories and
anything outside `$CLAUDE_PROJECT_DIR` are always skipped — see
[AGENTS.md § Invariants](AGENTS.md#invariants) for the exact list. None of
the formatters require their own project config file to exist first — a
`.ts` file in a project with no `biome.json` still gets formatted, using
biome's built-in defaults.

## Checking formatting in CI (`--check`)

`format-dispatch --check PATH...` reports which files a formatter *would*
change, without changing them, and exits 1 if there are any. Directories are
walked; vendored directories and unsupported extensions are skipped.

```bash
format-dispatch --check .              # whole tree
format-dispatch --check src docs/a.md  # specific paths
```

Exit codes are `0` (all formatted), `1` (some files need formatting), and
`2` (bad invocation, e.g. no paths or a path that does not exist) — so a
broken pipeline is distinguishable from a real failure.

Two properties make this usable in a consumer's CI without provisioning a
toolchain:

- **A missing tool is not a failure.** A file whose formatter is not
  installed is reported as fine, because nothing could have changed it. A
  runner with no `bunx` still meaningfully checks `.json`, `.sh`, and `.go`.
- **It checks formatting, not lint conformance.** The question is "would the
  hook rewrite this file", the same one every local write answers. A
  formatter that deliberately leaves violations it will not auto-fix does
  not fail the build over them — otherwise CI would gate on something no
  local write could ever repair.

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

### Markdown and SQL defaults

These two external tools get a user-level base config, so a project does
not need to carry its own just to avoid noise:

- **Markdown** — `~/.claude/claude-format-hooks.markdownlint-cli2.jsonc` is
  written on first use and passed to every run via `--config`. Edit it
  freely; it is never overwritten once it exists, and deleting it restores
  the built-in defaults on the next `.md` write. It turns off the rules
  that fire constantly on technical writing and cannot be auto-fixed
  (notably `MD013` line-length). A project's own
  `.markdownlint-cli2.jsonc` layers on top and wins rule by rule.

  The filename matters: markdownlint-cli2 picks a schema from it, and a
  `.markdownlint.` infix means "bare rules object" while
  `.markdownlint-cli2.` means "options object". Renaming the file to the
  wrong shape makes it silently ignored, with no error.

- **SQL** — `sqlfluff` reads `~/.sqlfluff` natively, before any project
  config, so no hook support is needed. Create one to set at minimum a
  `dialect`; without any reachable config `sqlfluff` hard-errors with "No
  dialect was specified" and every `.sql` write reports that instead of
  formatting.

### Project-level formatter opt-out

A project can opt a specific formatter out for itself — e.g. it already
runs its own pre-commit `prettier` with different rules and doesn't want
this hook's `biome` double-running — without every operator changing
their global config. Drop a `.claude-format-hooks.json` at the project
root (same schema as your own config above; only `disabled` is
consulted — `json`/`shell` indent settings there are ignored, since
`.editorconfig` already owns that layer):

```json
{ "disabled": [".ts", ".tsx"] }
```

A missing file is normal (no project-level opt-out). A malformed one is
ignored — a diagnostic goes to stderr, and formatting proceeds as if it
weren't there, same as a malformed user config.

## Troubleshooting

**Nothing happened after I wrote a file.** That's the default, silent
success — check the extension is in the [supported table](#supported-extensions)
above and not in your `disabled` list or the project's own
`.claude-format-hooks.json`. If the file lives under a vendored
directory, it's skipped on purpose.

**A `.json` file used the native formatter.** JSON uses `biome check --write`
when an upward `biome.json` or `biome.jsonc` is found and `biome` is
available through `bunx` or `PATH`. Otherwise it uses native
`encoding/json.Indent`, which preserves source key order and applies the
configured indentation.

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
machine is easily slow enough to blow the hook's 4s timeout — re-running
`format-dispatch --install` provisions the tools so no format-time fetch is
needed.

**I just installed `ruff`/`black`/`rustfmt`/`sqlfluff`/`bunx`, but the
hook still skipped a file.** A missing tool's absence is cached on disk
for up to 30 seconds, so a burst of file writes doesn't re-walk `$PATH`
for every one — it self-heals within that window with no restart needed.
The cache lives under your OS cache directory (`$XDG_CACHE_HOME`,
`%LocalAppData%`, etc.) in a `claude-format-hooks` subfolder; it's safe to
delete by hand if you want the next write to recheck immediately.

**Still can't tell what happened.** Set `CLAUDE_FORMAT_HOOKS_LOG` to a
file path before starting Claude Code, and every invocation appends one
line there (timestamp, file path, formatter, outcome, and how long it
took — e.g. `skip: vendored directory`, `skip: disabled by project
config`, `fixer failed`, `ok`) without changing the hook's normal silent
output. The duration field also helps spot a slow-feeling hook — usually
a cold `bunx` fetch (see the timeout note above). Unset it when done —
the file isn't rotated or truncated, so it grows unbounded; this is
meant for a short debugging session, not to be left on permanently.

## See also

- [AGENTS.md](AGENTS.md) — architecture and developer reference
- [CONTRIBUTING.md](CONTRIBUTING.md) — build, test, and pull request workflow
- [SECURITY.md](SECURITY.md) — vulnerability reporting

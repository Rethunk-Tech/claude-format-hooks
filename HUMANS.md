# HUMANS.md — Using claude-format-hooks

Operator setup and troubleshooting. Developers: [AGENTS.md](AGENTS.md).

## Install

### Prerequisites

Go ([go.mod](go.mod)); optional `bun` (bunx formatters),
`sqlfluff`, `ruff`/`black`, `rustfmt`, `terraform`/`tofu`, `buf`,
`clang-format`, `google-java-format`, `ktlint`, `swift-format`/`swiftformat`,
`rubocop`/`standardrb`, `php-cs-fixer`/`pint`, `nixfmt`, `stylua`.
`format-dispatch --doctor` reports which of these are actually reachable.

```bash
git clone git@github.com:Rethunk-Tech/claude-format-hooks.git
cd claude-format-hooks
./install.sh              # build, wire Claude + Cursor hooks
./install.sh --dry-run    # preview hook diffs
./install.sh --upgrade    # latest release binary
```

Without Go: download the platform binary from
[releases](https://github.com/Rethunk-Tech/claude-format-hooks/releases/latest),
place at `~/.claude/hooks/format-dispatch` (`chmod +x`), run `--install`.
The `.sha256` sidecar ships in the same release as the binary, so it proves
transit only. Check where the bytes were built before trusting them, with
`gh attestation verify <binary> --repo Rethunk-Tech/claude-format-hooks`.

`--install` wires `PostToolUse` in `~/.claude/settings.json` and
`afterFileEdit` in `~/.cursor/hooks.json`. With `bun` on PATH, it also
provisions biome/prettier/taplo/markdownlint-cli2 globally. Restart Claude
Code or open `/hooks` after install.

Rewriting either file sorts its top-level keys alphabetically. Values and
unrelated keys are preserved exactly; only their order changes, once.

## Env

`CLAUDE_HOOKS_BIN_DIR`, `CLAUDE_SETTINGS_FILE`, `CURSOR_HOOKS_FILE`,
`CLAUDE_FORMAT_HOOKS_RELEASE_API` (test/mirror override).
`CLAUDE_FORMAT_HOOKS_LOG=/path/to/log` for per-invocation debug lines.

## Usage

Formats supported extensions on each Write/Edit/MultiEdit/NotebookEdit.
When a formatter reports a real failure (a syntax error, say), the message
goes to stderr for you and back to Claude as hook context, so the model
learns the file it just wrote is broken. A skipped file -- unsupported type,
missing tool -- stays silent.
Unsupported extensions and skipped paths are no-ops — see
[AGENTS.md § Invariants](AGENTS.md#invariants).

```bash
format-dispatch --version
format-dispatch --check .              # CI: exit 1 if formatting needed
format-dispatch --doctor               # which formatters, extensions, and tools you have
format-dispatch --upgrade [--dry-run]
```

`--upgrade` also refreshes the globally-provisioned bunx formatters, so the
binary and the tools it shells out to move together.

`--upgrade` checks the release `.sha256` and then requires GitHub to hold a
build-provenance attestation for the downloaded bytes; a binary the release
workflow did not attest is refused rather than installed. `--dry-run` prints
the planned asset and path without going near the network.

**Config:** `~/.claude/claude-format-hooks.json` (`$CLAUDE_FORMAT_HOOKS_CONFIG`);
project `.claude-format-hooks.json` for `disabled` / `disabledFormatters` /
`skipDirs` (extra generated-directory names, added to the built-in list);
`.editorconfig` wins indent. Markdown base:
`~/.claude/claude-format-hooks.markdownlint-cli2.jsonc` (created on first use).
SQL: `~/.sqlfluff` bootstrap on first `.sql` write.

## Supported extensions

JSON/shell/Go native; TS/CSS/JSONC via biome; MD via markdownlint-cli2; TOML
via taplo; YAML/HTML/etc via prettier; SQL via sqlfluff; Python via ruff/black;
notebooks via ruff/black; Rust via rustfmt; Terraform via terraform/tofu; proto
via buf. Non-native formatters need `bunx` or the tool on `PATH`.

## Verify

```bash
format-dispatch --version
./install.sh --dry-run
format-dispatch --check path/to/file
```

## Uninstall

```bash
~/.claude/hooks/format-dispatch --uninstall [--dry-run]
rm ~/.claude/hooks/format-dispatch
```

Removes only this binary's hook entries from Claude and Cursor settings.

## Troubleshooting

- **Nothing formatted:** extension unsupported/disabled, skipped dir, or missing
  formatter (`bunx`/tool not on PATH; 30s tool-absence cache).
- **stderr diagnostic:** formatter failed; hook still exits 0. Run the underlying
  tool for full output.
- **Hook not running:** re-run `./install.sh`, restart Claude Code.

## See also

[AGENTS.md](AGENTS.md), [CONTRIBUTING.md](CONTRIBUTING.md), [SECURITY.md](SECURITY.md).

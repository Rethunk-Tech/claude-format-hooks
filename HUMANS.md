# HUMANS.md — Using claude-format-hooks

Operator setup. Developers: [AGENTS.md](AGENTS.md). Flags: `--help`.

## Install

```bash
brew install rethunk-tech/tap/format-dispatch
```

Windows:

```powershell
scoop bucket add rethunk https://github.com/Rethunk-Tech/scoop-bucket
scoop install format-dispatch
```

Go ([go.mod](go.mod)). Reachable formatters: `--doctor`.

```bash
git clone git@github.com:Rethunk-Tech/claude-format-hooks.git
cd claude-format-hooks
./install.sh              # build, wire Claude + Cursor hooks
./install.sh --dry-run    # preview hook diffs
./install.sh --upgrade    # latest release binary
./install.sh --uninstall  # unwire hooks (no rebuild)
```

Without Go: release binary at `~/.claude/hooks/format-dispatch`
(`chmod +x`), then `--install`. `.sha256` proves transit only; check
build provenance with
`gh attestation verify <binary> --repo Rethunk-Tech/claude-format-hooks`.

`--install` wires `PostToolUse` (`~/.claude/settings.json`) and
`afterFileEdit` (`~/.cursor/hooks.json`). `bun` on PATH also provisions
biome/prettier/taplo/markdownlint-cli2. Restart Claude Code or open
`/hooks`. Rewriting either file sorts top-level keys; values stay.

## Env

| Variable | Default / role |
| -------- | -------------- |
| `CLAUDE_HOOKS_BIN_DIR` | `~/.claude/hooks` |
| `CLAUDE_SETTINGS_FILE` | `~/.claude/settings.json` |
| `CURSOR_HOOKS_FILE` | `~/.cursor/hooks.json` |
| `CLAUDE_FORMAT_HOOKS_RELEASE_API` | GitHub Releases API (test/mirror) |
| `CLAUDE_PROJECT_DIR` | set by Claude Code; files outside skipped |
| `CLAUDE_FORMAT_HOOKS_CONFIG` | `~/.claude/claude-format-hooks.json` |
| `CLAUDE_FORMAT_HOOKS_CACHE` | OS user cache/`claude-format-hooks` |
| `CLAUDE_FORMAT_HOOKS_LOG` | if set, one debug line per invocation |

## Usage

Hook path: silent on success, always exit 0. Formatter failure → stderr
and, for Claude payloads, hook context to the model. Skips stay silent.

`--upgrade` refreshes bunx formatters with the binary. It checks
`.sha256`, then a build-provenance attestation bound to
`.github/workflows/release.yml`; anything else is refused. Authenticated
`gh` also runs `gh attestation verify --signer-workflow` (DSSE, Fulcio,
Rekor) and that verdict decides. Without `gh` (token required even for
a public repo) the attestation check trusts TLS to `api.github.com`.
`--upgrade` says which applied. A custom `CLAUDE_FORMAT_HOOKS_RELEASE_API`
skips `gh`. `--dry-run` prints asset and path, no network.

**Config** (`$CLAUDE_FORMAT_HOOKS_CONFIG` or
`~/.claude/claude-format-hooks.json`; same shape as project
`.claude-format-hooks.json`): `disabled` (extensions),
`disabledFormatters` (`--doctor` names; `"biome"` skips biome-owned
types), `skipDirs`/`skipFiles` (add to built-in lists) at both levels.
Native indent is user-only; project indent keys are ignored;
`.editorconfig` wins over the user file. Markdown base
`~/.claude/claude-format-hooks.markdownlint-cli2.jsonc` on first use;
`~/.sqlfluff` on first `.sql` write.

## Supported extensions

[AGENTS.md](AGENTS.md#layout) and `--doctor`. Traps: `.json.example`
always native JSON; `go.mod`/`go.work` only under those names; a bare
`notes.example` is not formatted; extensionless/hidden rc files need a
supported shebang (`env -S` flags skipped); prettier only with a project
prettier config.

## Verify

```bash
format-dispatch --version
./install.sh --dry-run
format-dispatch --check path/to/file
```

## Uninstall

```bash
./install.sh --uninstall [--dry-run]
rm ~/.claude/hooks/format-dispatch
```

Removes this binary's hook entries only.

## Troubleshooting

- **Nothing formatted:** unsupported/disabled, skipped dir, or missing
  tool (30s absence cache).
- **stderr:** formatter failed; hook still exits 0.
- **Hook not running:** `./install.sh`, restart Claude Code.

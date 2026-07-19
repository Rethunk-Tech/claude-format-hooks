# claude-format-hooks

A [Claude Code](https://claude.com/claude-code) `PostToolUse` hook that
formats/lints a file right after Write/Edit/NotebookEdit writes it —
generalized from a hand-written per-project hook into a single global Go
binary (`format-dispatch`), with no per-repo setup required.

## Highlights

- **Native, in-process formatting for JSON and shell scripts** — no
  subprocess, and (for JSON) source key order is preserved exactly instead
  of alphabetized by a naive Unmarshal+Marshal round-trip.
- **Everything else routed to the real tool** — biome, markdownlint-cli2,
  taplo, and prettier via `bunx`; `sqlfluff` for SQL — so project config
  files are honored automatically, with no config-presence gate: every
  supported extension formats in every project.
- **Silent on success, exits 0 always** — a `PostToolUse` hook must never
  be the reason a Write/Edit/NotebookEdit call reports failure; failures
  surface as a truncated stderr diagnostic instead.
- **One `./install.sh`, no per-repo setup** — builds the binary and wires
  it into `~/.claude/settings.json` globally.

## Documentation

| Document | Purpose |
| --- | --- |
| [Humans guide](HUMANS.md) | Install, configuration, supported extensions, troubleshooting |
| [Agents guide](AGENTS.md) | Architecture, design rationale, package layout, invariants |
| [Contributing](CONTRIBUTING.md) | Build, test, and pull request workflow |
| [Security](SECURITY.md) | Vulnerability reporting |
| [Changelog](CHANGELOG.md) | Release history (Keep a Changelog style) |
| [Code of conduct](CODE_OF_CONDUCT.md) | Community standards and enforcement |
| [License](LICENSE) | MIT License |

## License

Copyright (c) 2026 Rethunk Tech. Licensed under the MIT License — see
[LICENSE](LICENSE) for details.

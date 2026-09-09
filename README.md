<h1 align="center">claude-format-hooks</h1>

<div align="center">

[![Go 1.27+](https://img.shields.io/badge/go-1.27+-blue.svg)](go.mod)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![CI](https://github.com/Rethunk-Tech/claude-format-hooks/actions/workflows/ci.yml/badge.svg)](https://github.com/Rethunk-Tech/claude-format-hooks/actions/workflows/ci.yml)

A [Claude Code](https://claude.com/claude-code) `PostToolUse` hook that
formats/lints a file right after Write/Edit/MultiEdit/NotebookEdit writes it

</div>

---

Single global Go binary (`format-dispatch`), no per-repo setup. Native
in-process formatters for JSON, shell, and Go; everything else routes to
biome, prettier, taplo, markdownlint-cli2, sqlfluff, ruff/black, rustfmt,
`terraform fmt` (or `tofu fmt`), and buf.

## Quick start

```bash
git clone git@github.com:Rethunk-Tech/claude-format-hooks.git
cd claude-format-hooks
./install.sh
```

Install, prerequisites, config, and uninstall: [HUMANS.md](HUMANS.md).

## Highlights

- **Native JSON, shell, Go** — in-process; JSON preserves key order.
- **`--check` for CI** — reports files needing format; distinct exit codes.
- **`--doctor`** — every formatter, the extensions it owns, and whether its tool is installed.
- **Real tools for everything else** — honors project config; no config-presence gate.
- **Silent on success, always exits 0** — formatter failures go to stderr only.
- **One `./install.sh`** — wires Claude and Cursor hooks globally.

## Documentation

| Document | Purpose |
| -------- | ------- |
| [Humans guide](HUMANS.md) | Install, configuration, troubleshooting |
| [Agents guide](AGENTS.md) | Architecture, layout, invariants |
| [Contributing](CONTRIBUTING.md) | Build, test, and pull request workflow |
| [Security](SECURITY.md) | Vulnerability reporting |
| [Changelog](CHANGELOG.md) | Release history |
| [Code of conduct](CODE_OF_CONDUCT.md) | Community standards |
| [License](LICENSE) | MIT License |

## License

Copyright (c) 2026 Rethunk Tech. Licensed under the MIT License — see
[LICENSE](LICENSE).

<h1 align="center">claude-format-hooks</h1>

<div align="center">

[![Go 1.26+](https://img.shields.io/badge/go-1.26+-blue.svg)](go.mod)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![CI](https://github.com/Rethunk-Tech/claude-format-hooks/actions/workflows/ci.yml/badge.svg)](https://github.com/Rethunk-Tech/claude-format-hooks/actions/workflows/ci.yml)

A [Claude Code](https://claude.com/claude-code) `PostToolUse` hook that
formats/lints a file right after Write/Edit/NotebookEdit writes it

</div>

---

`claude-format-hooks` is a single global Go binary (`format-dispatch`),
with no per-repo setup required. Three formatters (JSON, shell, Go) run
natively in-process; everything else routes to the real,
project-config-aware tool: biome, prettier, taplo, and markdownlint-cli2
from `PATH` first, then via `bunx`; `sqlfluff`, `ruff`/`black`, `rustfmt`,
`terraform fmt`, and `buf format` as system binaries.

## Quick start

```bash
git clone git@github.com:Rethunk-Tech/claude-format-hooks.git
cd claude-format-hooks
./install.sh
```

Full install steps, prerequisites, and configuration: [HUMANS.md](HUMANS.md).

## Highlights

- **Native, in-process formatting for JSON, shell, and Go** — no
  subprocess, and (for JSON) source key order is preserved exactly instead
  of alphabetized by a naive Unmarshal+Marshal round-trip.
- **CI formatting checks with `--check`** — reports files a formatter would
  change without mutating them, with distinct exit codes for clean, changed,
  and invalid invocations.
- **Everything else routed to the real tool** — biome, markdownlint-cli2,
  taplo, and prettier from `PATH` first, then via `bunx`; `sqlfluff`,
  `ruff`/`black`, `rustfmt`,
  `terraform fmt`, and `buf format` as system binaries — so project config
  files are
  honored automatically, with no config-presence gate: every supported
  extension formats in every project.
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

<h1 align="center">claude-format-hooks</h1>

<div align="center">

[![Go 1.27+](https://img.shields.io/badge/go-1.27+-blue.svg)](go.mod)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![CI](https://github.com/Rethunk-Tech/claude-format-hooks/actions/workflows/ci.yml/badge.svg)](https://github.com/Rethunk-Tech/claude-format-hooks/actions/workflows/ci.yml)

A [Claude Code](https://claude.com/claude-code) `PostToolUse` hook that
formats a file right after Write/Edit/MultiEdit/NotebookEdit writes it

</div>

---

Single global Go binary (`format-dispatch`). Native JSON, shell, and Go;
everything else shells out. No per-repo setup.

## Installation

```bash
brew install rethunk-tech/tap/format-dispatch
```

Windows:

```powershell
scoop bucket add rethunk https://github.com/Rethunk-Tech/scoop-bucket
scoop install format-dispatch
```

From source:

```bash
git clone git@github.com:Rethunk-Tech/claude-format-hooks.git
cd claude-format-hooks
./install.sh
```

Config and uninstall: [HUMANS.md](HUMANS.md).

Each `v*` release attaches `format-dispatch-completions-man.tar.gz` (listed in
`SHA256SUMS`). After extracting it:

```bash
install -Dm644 format-dispatch.bash ~/.local/share/bash-completion/completions/format-dispatch
```

```zsh
install -Dm644 _format-dispatch ~/.local/share/zsh/site-functions/_format-dispatch
```

```fish
install -Dm644 format-dispatch.fish ~/.config/fish/completions/format-dispatch.fish
```

```powershell
$dest = Join-Path (Split-Path $PROFILE) 'Completions'
New-Item -ItemType Directory -Force $dest | Out-Null
Copy-Item format-dispatch.ps1 (Join-Path $dest 'format-dispatch.ps1')
Add-Content $PROFILE ". '$(Join-Path $dest 'format-dispatch.ps1')'"
```

## Highlights

- **`--check` / `--write`** — CI: exit 1 if formatting is needed, 2 on
  usage/I/O error.
- **`--doctor`** — formatters, tools, and whether the hook is wired.
- **Prettier only with a project prettier config.** Other tools honor
  theirs without requiring one to exist first.
- **Hook path silent on success, always exits 0.** Failures reach the
  operator and, on Claude payloads, the model.

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

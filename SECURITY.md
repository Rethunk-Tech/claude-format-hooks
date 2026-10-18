# Security Policy

## Reporting a Vulnerability

Do not open a public GitHub issue. Submit a
**[private security advisory](https://github.com/Rethunk-Tech/claude-format-hooks/security/advisories/new)**
(preferred). Include affected versions, reproduction, impact, and
remediation if known.

Acknowledgement within 48 hours; triage within 7 days for confirmed
issues.

Code of Conduct reports: [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md)
(private channels only).

## Supported Versions

Latest minor only.

| Version | Supported |
| -------- | ------------------ |
| latest | :white_check_mark: |
| < latest | :x: |

## Scope

In scope:

- A crafted file path, file content, or hook payload that makes
  `format-dispatch` read, write, or execute outside the file it was
  invoked on or the formatter it dispatches to.
- Command injection via arguments passed to external formatters.
- Path traversal past the `$CLAUDE_PROJECT_DIR` boundary
  (`cmd/format-dispatch/main.go`).

Out of scope: bugs in the external formatters (report upstream), and
running project-configured formatters against files in that project —
that is the tool's job.

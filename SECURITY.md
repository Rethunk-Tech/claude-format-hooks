# Security Policy

## Reporting a Vulnerability

Rethunk-Tech takes security seriously. To report a vulnerability in
claude-format-hooks:

- **Do not** open a public GitHub issue for security vulnerabilities.
- Submit a **[private security advisory](https://github.com/Rethunk-Tech/claude-format-hooks/security/advisories/new)**
  on GitHub (preferred).
- Include as much detail as possible: affected versions, reproduction
  steps, potential impact, and suggested remediation if known.

We will acknowledge receipt within 48 hours and aim to triage and respond
within 7 days for confirmed issues.

## Reporting a Code of Conduct Violation

Code of Conduct violations should be reported privately — **not** via
public issues or security advisories.

- Contact a [Rethunk-Tech organization owner](https://github.com/orgs/Rethunk-Tech/people)
  on GitHub through a private message, or
- Use any **Contact maintainers** link shown on the
  [repository home page](https://github.com/Rethunk-Tech/claude-format-hooks).

All complaints will be reviewed and investigated promptly and fairly. See
[CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) for enforcement guidelines.

## Supported Versions

This project follows semantic versioning. Only the latest minor version
receives security fixes. Older versions should be upgraded to receive
patches.

| Version  | Supported          |
| -------- | ------------------ |
| latest   | :white_check_mark: |
| < latest | :x:                |

## Scope

claude-format-hooks is a local developer tool: a `PostToolUse` hook binary
that runs on every Write/Edit/NotebookEdit call in a Claude Code session,
reads the file just written, and (for supported extensions) reformats it
in place — either in-process or by shelling out to a project-local or
`bunx`-resolved formatter. In scope:

- Anything that lets a crafted file path, file content, or hook payload
  cause `format-dispatch` to read, write, or execute outside the file it
  was invoked on or the formatter it dispatches to.
- Command injection via the arguments passed to external formatters
  (`biome`, `prettier`, `taplo`, `markdownlint-cli2`, `sqlfluff`, `ruff`,
  `black`, `rustfmt`, `terraform`, `buf`).
- Path traversal past the `$CLAUDE_PROJECT_DIR` boundary check in
  `cmd/format-dispatch/main.go`.

Out of scope: bugs in the external formatters themselves (report those
upstream), and the hook's designed behavior of running arbitrary
project-configured formatters against files in that project — that is the
tool's job, not a vulnerability.

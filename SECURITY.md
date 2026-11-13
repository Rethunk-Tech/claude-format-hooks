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

## Release signing

Releases from the first one after this section landed carry `SHA256SUMS.asc`, a detached signature over `SHA256SUMS` made with the Rethunk.Tech release signing key. Earlier releases are unsigned.

- Fingerprint: `827D 0553 6BF3 3C8F 634E  DBE3 E60A 9B0F 6E3F 276E`
- Verify: `gpg --import rethunk-release-signing.pub.asc && gpg --verify SHA256SUMS.asc SHA256SUMS`

```text
-----BEGIN PGP PUBLIC KEY BLOCK-----

mDMEavZr2xYJKwYBBAHaRw8BAQdAuTAn1oKhKC13SOyJMlYmaoGGQtVRyZ5yEpxT
yZBxH/q0NFJldGh1bmsuVGVjaCByZWxlYXNlIHNpZ25pbmcgPHJlbGVhc2VzQHJl
dGh1bmsudGVjaD6ImQQTFgoAQRYhBIJ9BVNr8zyPY07b4+YKmw9uPyduBQJq9mvb
AhsDBQkDwmcABQsJCAcCAiICBhUKCQgLAgQWAgMBAh4HAheAAAoJEOYKmw9uPydu
7WIA+wYeW352nuKph1uJ+XJPPpgmkPyYujZWiRBZIUUA5hfwAQDY6ru4vNL8TfQ5
5rddKHoSKh1nMIdiLWn9ukkyWW+MCg==
=meeO
-----END PGP PUBLIC KEY BLOCK-----
```

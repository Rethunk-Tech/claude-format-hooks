# Contributing

Thanks for helping improve **claude-format-hooks**.

## Before you start

- **Architecture and conventions:** [AGENTS.md](AGENTS.md)
- **Operators (install, config, troubleshooting):** [HUMANS.md](HUMANS.md)

## Prerequisites

- [Go](https://go.dev/) (see the `go` version in [go.mod](go.mod))
- [`golangci-lint`](https://golangci-lint.run/) v2.12.2+ (CI pins this
  version; see [`.golangci.yml`](.golangci.yml))
- Optional: `bun` (`bunx`) and `sqlfluff` if you're changing an external
  formatter and want to exercise it locally — see
  [HUMANS.md](HUMANS.md#prerequisites)

## Build and test

Build, vet, format, lint, and test commands (plus what CI runs and how to
scope a single package/test) are canonical in
[AGENTS.md § Commands](AGENTS.md#commands) — don't duplicate them here.

## Pull requests

Use the PR template. No unrelated refactors or scope creep — match the
style of the file being touched (see [AGENTS.md](AGENTS.md#conventions)).
If you add or change a formatter, update the
[supported-extensions table](HUMANS.md#supported-extensions) in
HUMANS.md and the [layout table](AGENTS.md#layout) in AGENTS.md if the
package layout changed, and add a `CHANGELOG.md` entry under
`[Unreleased]`.

## Releasing

Update `CHANGELOG.md` (move `[Unreleased]` into a new dated version
section, add the compare links at the bottom) and push a matching
annotated `vX.Y.Z` tag. The tag push triggers
[`.github/workflows/release.yml`](.github/workflows/release.yml), which
cross-compiles `format-dispatch` for linux/darwin/windows (amd64+arm64) and
publishes a GitHub Release with the binaries and their sha256sums — no
manual build/upload step.

## Security

Report vulnerabilities via [SECURITY.md](SECURITY.md) (private advisory),
not a public issue.

## License

By contributing, you agree that your contributions are licensed under the
[MIT License](LICENSE) that applies to this project.

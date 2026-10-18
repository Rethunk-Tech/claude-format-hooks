# Contributing

## Prerequisites

- [Go](https://go.dev/) ([go.mod](go.mod))
- [`golangci-lint`](https://golangci-lint.run/) v2 ([`.golangci.yml`](.golangci.yml);
  CI version from `Rethunk-Tech/gh-actions/setup-go`)
- Optional formatters when changing one: [HUMANS.md](HUMANS.md#install)

## Build and test

[AGENTS.md § Commands](AGENTS.md#commands).

## Pull requests

Use the PR template. Match the file being touched
([AGENTS.md](AGENTS.md#conventions)). Formatter or install changes:
[HUMANS.md](HUMANS.md#supported-extensions) and, if the package layout
changed, [AGENTS.md](AGENTS.md#layout). `CHANGELOG.md` under `[Unreleased]`.

## Releasing

Move `[Unreleased]` in `CHANGELOG.md` into a dated version (compare links
at the bottom) and push annotated tag `vX.Y.Z`. That triggers
[`.github/workflows/release.yml`](.github/workflows/release.yml)
(linux/darwin/windows, amd64+arm64, sha256sums).

## Security

[SECURITY.md](SECURITY.md), not a public issue.

## License

Contributions are [MIT](LICENSE).

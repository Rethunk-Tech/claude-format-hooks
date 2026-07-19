#!/usr/bin/env bash
# Installer for claude-format-hooks: builds the format-dispatch binary and
# pre-warms bunx's package cache, then delegates the settings.json wiring
# to the binary's own --install subcommand (native Go via encoding/json,
# no jq dependency).
#
# Usage:
#   ./install.sh              # build + install
#   ./install.sh --dry-run    # print the settings.json diff, write nothing
#
# Env overrides (mainly for testing):
#   CLAUDE_HOOKS_BIN_DIR   default: ~/.claude/hooks
#   CLAUDE_SETTINGS_FILE   default: ~/.claude/settings.json
set -euo pipefail

DRY_RUN="${1:-}"

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN_DIR="${CLAUDE_HOOKS_BIN_DIR:-$HOME/.claude/hooks}"
BIN_PATH="$BIN_DIR/format-dispatch"

command -v go >/dev/null 2>&1 || {
  echo "error: go is required to build format-dispatch" >&2
  exit 1
}

echo "==> Building format-dispatch..."
mkdir -p "$BIN_DIR"
(cd "$REPO_DIR" && go build -o "$BIN_PATH" ./cmd/format-dispatch)
chmod +x "$BIN_PATH"
echo "==> Built: $BIN_PATH"

# Pre-warm bunx's package cache for the four bunx-invoked formatters so the
# first real Write/Edit in a session doesn't pay a cold npm-registry fetch
# against the hook's 25s per-file timeout. Best-effort: a network hiccup
# here must never fail the install, since these tools are re-fetched (from
# cache) on every subsequent run regardless.
if command -v bunx >/dev/null 2>&1; then
  echo "==> Pre-warming bunx cache for biome, prettier, taplo, markdownlint-cli2..."
  for pkg in @biomejs/biome prettier @taplo/cli markdownlint-cli2; do
    bunx "$pkg" --version >/dev/null 2>&1 || echo "    (skipped $pkg: fetch failed, will retry on first use)"
  done
fi

if [ "$DRY_RUN" = "--dry-run" ]; then
  "$BIN_PATH" --install --dry-run
else
  "$BIN_PATH" --install
  echo "==> If Claude Code is already running, open /hooks once (or restart) to pick up the change."
fi

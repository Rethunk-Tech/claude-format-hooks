#!/usr/bin/env bash
# Installer for claude-format-hooks: builds the format-dispatch binary, then
# delegates settings.json wiring and ProvisionTools to the binary's own
# --install subcommand (native Go via encoding/json, no jq dependency).
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

# Provisioning the bunx-dispatched formatters moved into `--install` below,
# where it can also pin their transitive dependencies -- see
# internal/installer/tools.go. It replaces the cache pre-warm that used to
# live here, which left the tools resolvable but not on PATH.

if [ "$DRY_RUN" = "--dry-run" ]; then
  "$BIN_PATH" --install --dry-run
else
  "$BIN_PATH" --install
  echo "==> If Claude Code is already running, open /hooks once (or restart) to pick up the change."
fi

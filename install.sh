#!/usr/bin/env bash
# Installer for claude-format-hooks: builds the format-dispatch binary, then
# delegates settings.json wiring and ProvisionTools to the binary's own
# --install subcommand (native Go via encoding/json, no jq dependency).
#
# Usage:
#   ./install.sh              # build + install
#   ./install.sh --dry-run    # print the settings.json diff, write nothing
#   ./install.sh --upgrade    # build + upgrade to the latest release
#   ./install.sh --upgrade --dry-run # build + preview the latest release upgrade
#
# Env overrides (mainly for testing):
#   CLAUDE_HOOKS_BIN_DIR   default: ~/.claude/hooks
#   CLAUDE_SETTINGS_FILE   default: ~/.claude/settings.json
#   CLAUDE_FORMAT_HOOKS_RELEASE_API
#       override GitHub Releases API base URL for --upgrade (default:
#       https://api.github.com); intended for tests/mirrors. The configured
#       host is fully trusted for release metadata and asset URLs.
set -euo pipefail

ACTION="${1:-}"
DRY_RUN="${2:-}"

case "$ACTION" in
  "" | --dry-run | --upgrade)
    ;;
  *)
    echo "usage: $0 [--dry-run | --upgrade [--dry-run]]" >&2
    exit 2
    ;;
esac

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN_DIR="${CLAUDE_HOOKS_BIN_DIR:-$HOME/.claude/hooks}"

command -v go >/dev/null 2>&1 || {
  echo "error: go is required to build format-dispatch" >&2
  exit 1
}

GOOS="$(go env GOOS)"
BIN_NAME="format-dispatch"
if [ "$GOOS" = "windows" ]; then
  BIN_NAME="${BIN_NAME}.exe"
fi
BIN_PATH="$BIN_DIR/$BIN_NAME"

echo "==> Building format-dispatch..."
mkdir -p "$BIN_DIR"
(cd "$REPO_DIR" && go build -o "$BIN_PATH" ./cmd/format-dispatch)
chmod +x "$BIN_PATH"
echo "==> Built: $BIN_PATH"

# Provisioning the bunx-dispatched formatters runs in `--install` below --
# see internal/installer/tools.go.

if [ "$ACTION" = "--upgrade" ]; then
  case "$DRY_RUN" in
    "")
      "$BIN_PATH" --upgrade
      ;;
    "--dry-run")
      "$BIN_PATH" --upgrade --dry-run
      ;;
    *)
      echo "error: --upgrade accepts only an optional --dry-run" >&2
      exit 2
      ;;
  esac
elif [ "$ACTION" = "--dry-run" ]; then
  "$BIN_PATH" --install --dry-run
else
  "$BIN_PATH" --install
  echo "==> If Claude Code is already running, open /hooks once (or restart) to pick up the change."
fi

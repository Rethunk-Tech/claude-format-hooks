#!/usr/bin/env bash
# Installer for claude-format-hooks: builds the format-dispatch binary, then
# delegates settings.json wiring and ProvisionTools to the binary's own
# --install subcommand (native Go via encoding/json, no jq dependency).
#
# Usage:
#   ./install.sh              # build + install
#   ./install.sh --dry-run    # preview Claude and Cursor hook diffs, write nothing
#   ./install.sh --upgrade    # latest release binary (builds from source if none is installed yet)
#   ./install.sh --upgrade --dry-run # preview the latest release upgrade
#   ./install.sh --uninstall  # unwire Claude and Cursor hooks (no rebuild)
#   ./install.sh --uninstall --dry-run # preview uninstall without writing
#
# Env overrides (mainly for testing; CLAUDE_SETTINGS_FILE and CURSOR_HOOKS_FILE
# are read by format-dispatch, not this script):
#   CLAUDE_HOOKS_BIN_DIR   default: ~/.claude/hooks
#   CLAUDE_SETTINGS_FILE   default: ~/.claude/settings.json
#   CURSOR_HOOKS_FILE      default: ~/.cursor/hooks.json
#   CLAUDE_FORMAT_HOOKS_RELEASE_API
#       override GitHub Releases API base URL for --upgrade (default:
#       https://api.github.com); intended for tests/mirrors. The configured
#       host is fully trusted for release metadata and asset URLs.
set -euo pipefail

ACTION="${1:-}"
DRY_RUN="${2:-}"
USAGE="usage: $0 [--dry-run | --upgrade [--dry-run] | --uninstall [--dry-run]]"

case "$ACTION" in
  "" | --dry-run | --upgrade | --uninstall)
    ;;
  *)
    echo "$USAGE" >&2
    exit 2
    ;;
esac

case "$ACTION" in
  --upgrade | --uninstall)
    case "$DRY_RUN" in
      "" | --dry-run)
        ;;
      *)
        echo "error: $ACTION accepts only an optional --dry-run" >&2
        exit 2
        ;;
    esac
    ;;
  *)
    if [ -n "$DRY_RUN" ]; then
      echo "$USAGE" >&2
      exit 2
    fi
    ;;
esac

BIN_DIR="${CLAUDE_HOOKS_BIN_DIR:-$HOME/.claude/hooks}"
BIN_NAME="format-dispatch"
if [ -x "$BIN_DIR/format-dispatch.exe" ] && [ ! -x "$BIN_DIR/format-dispatch" ]; then
  BIN_NAME="format-dispatch.exe"
fi
BIN_PATH="$BIN_DIR/$BIN_NAME"

if [ "$ACTION" = "--uninstall" ]; then
  if [ ! -x "$BIN_PATH" ]; then
    echo "==> Nothing to unwire: $BIN_PATH is not installed"
    exit 0
  fi
  if [ "$DRY_RUN" = "--dry-run" ]; then
    "$BIN_PATH" --uninstall --dry-run
  else
    "$BIN_PATH" --uninstall
  fi
  exit 0
fi

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

command -v go >/dev/null 2>&1 || {
  echo "error: go is required to build format-dispatch" >&2
  exit 1
}

GOOS="$(go env GOOS)"
if [ "$GOOS" = "windows" ]; then
  BIN_NAME="format-dispatch.exe"
  BIN_PATH="$BIN_DIR/$BIN_NAME"
fi

# --upgrade replaces the installed binary with a published release, so
# building first would only discard the build. The build still runs when
# there is nothing installed yet, since --upgrade is run by that binary.
# A dry run writes nothing, so it previews with a throwaway build; the preview
# still names BIN_PATH, which the installer reads from CLAUDE_HOOKS_BIN_DIR.
RUN_PATH=$BIN_PATH
if [ "$ACTION" = "--upgrade" ] && [ -x "$BIN_PATH" ]; then
  echo "==> Upgrading in place: $BIN_PATH"
else
  if [ "$ACTION" = "--dry-run" ] || [ -n "$DRY_RUN" ]; then
    BUILD_DIR=$(mktemp -d "${TMPDIR:-/tmp}/format-dispatch.XXXXXX")
    trap 'rm -rf "$BUILD_DIR"' EXIT
    RUN_PATH="$BUILD_DIR/$BIN_NAME"
  fi
  echo "==> Building format-dispatch..."
  mkdir -p "$(dirname "$RUN_PATH")"
  (cd "$REPO_DIR" && go build -o "$RUN_PATH" ./cmd/format-dispatch)
  chmod +x "$RUN_PATH"
  echo "==> Built: $RUN_PATH"
fi

# Provisioning the bunx-dispatched formatters runs in `--install` below --
# see internal/installer/tools.go.

if [ "$ACTION" = "--upgrade" ]; then
  if [ "$DRY_RUN" = "--dry-run" ]; then
    "$RUN_PATH" --upgrade --dry-run
  else
    "$BIN_PATH" --upgrade
  fi
elif [ "$ACTION" = "--dry-run" ]; then
  "$RUN_PATH" --install --dry-run
else
  "$BIN_PATH" --install
  echo "==> If Claude Code is already running, open /hooks once (or restart) to pick up the change."
fi

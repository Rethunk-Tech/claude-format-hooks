#!/usr/bin/env bash
# Installer for claude-format-hooks: builds the format-dispatch binary and
# wires it into ~/.claude/settings.json as a PostToolUse hook for
# Write/Edit/NotebookEdit, replacing the narrower inline biome-only hook if
# present.
#
# Usage:
#   ./install.sh              # build + install
#   ./install.sh --dry-run    # print the settings.json diff, write nothing
#
# Env overrides (mainly for testing):
#   CLAUDE_HOOKS_BIN_DIR   default: ~/.claude/hooks
#   CLAUDE_SETTINGS_FILE   default: ~/.claude/settings.json
set -euo pipefail

DRY_RUN=0
[ "${1:-}" = "--dry-run" ] && DRY_RUN=1

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN_DIR="${CLAUDE_HOOKS_BIN_DIR:-$HOME/.claude/hooks}"
BIN_PATH="$BIN_DIR/format-dispatch"
SETTINGS="${CLAUDE_SETTINGS_FILE:-$HOME/.claude/settings.json}"

command -v go >/dev/null 2>&1 || {
  echo "error: go is required to build format-dispatch" >&2
  exit 1
}
command -v jq >/dev/null 2>&1 || {
  echo "error: jq is required to wire up settings.json" >&2
  exit 1
}

echo "==> Building format-dispatch..."
mkdir -p "$BIN_DIR"
(cd "$REPO_DIR" && go build -o "$BIN_PATH" ./cmd/format-dispatch)
chmod +x "$BIN_PATH"
echo "==> Built: $BIN_PATH"

[ -f "$SETTINGS" ] || echo '{}' >"$SETTINGS"

# Remove any PostToolUse entry that either (a) already points at our own
# binary (idempotent re-install: drop the stale copy before re-adding), or
# (b) is the old inline biome-only hook this replaces (matcher "Write|Edit"
# running a "biome check --write" command directly in settings.json).
# Then append the new entry, invoked via exec form (no shell spawn).
NEW_SETTINGS="$(
  jq --arg bin "$BIN_PATH" '
    .hooks //= {} |
    .hooks.PostToolUse //= [] |
    .hooks.PostToolUse |= [
      .[] | select(
        (([.hooks[]?.command // empty] | index($bin)) == null)
        and (
          (.matcher != "Write|Edit")
          or (([.hooks[]?.command // ""] | map(test("biome check --write")) | any) | not)
        )
      )
    ] |
    .hooks.PostToolUse += [{
      "matcher": "Write|Edit|NotebookEdit",
      "hooks": [{
        "type": "command",
        "command": $bin,
        "args": [],
        "timeout": 30,
        "statusMessage": "format-dispatch..."
      }]
    }]
  ' "$SETTINGS"
)"

if [ "$DRY_RUN" = "1" ]; then
  echo "==> --dry-run: settings.json diff (not written):"
  diff -u "$SETTINGS" <(printf '%s\n' "$NEW_SETTINGS") || true
  exit 0
fi

printf '%s\n' "$NEW_SETTINGS" >"$SETTINGS"
echo "==> Wired PostToolUse hook into $SETTINGS"
echo "==> If Claude Code is already running, open /hooks once (or restart) to pick up the change."

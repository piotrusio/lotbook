#!/usr/bin/env bash
# Notification hook: Claude is blocked on a permission or a question — tap the human.
# Input: the hook JSON on stdin. Output: a macOS notification, nothing else.
set -uo pipefail

input=$(cat)
root=${CLAUDE_PROJECT_DIR:-$(pwd)}
title=$(basename "$root")
msg=$(jq -r '.message // "Claude czeka na odpowiedź"' <<<"$input" | head -c 200)

osascript \
  -e 'on run argv' \
  -e 'display notification (item 1 of argv) with title (item 2 of argv) sound name "Ping"' \
  -e 'end run' \
  -- "$msg" "$title" >/dev/null 2>&1 || true

exit 0

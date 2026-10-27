#!/usr/bin/env bash
# PostToolUse hook (Edit|Write): record the touched file for the Stop gate,
# then fast-lint just that file. Exit 2 feeds the failure back to the agent
# inside the same turn; the full gate runs once, on Stop.
set -uo pipefail

input=$(cat)
root=${CLAUDE_PROJECT_DIR:-$(pwd)}
file=$(jq -r '.tool_input.file_path // .tool_response.filePath // empty' <<<"$input")
sid=$(jq -r '.session_id // "unknown"' <<<"$input")

[ -n "$file" ] || exit 0
case "$file" in
  "$root"/*) rel=${file#"$root"/} ;;
  *) exit 0 ;; # outside the repository: scratchpad, memory
esac
state_dir="/tmp/claude-hooks-$(basename "$root")"
mkdir -p "$state_dir"
echo "$rel" >>"$state_dir/$sid.touched"

case "$rel" in
*.go)
  if command -v gofmt >/dev/null; then
    unformatted=$(gofmt -l "$file" 2>&1) || exit 0
    if [ -n "$unformatted" ]; then
      echo "gofmt: $rel is not gofmt-formatted (run make fmt)" >&2
      exit 2
    fi
  fi
  ;;
esac

exit 0

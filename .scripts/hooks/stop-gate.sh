#!/usr/bin/env bash
# Stop hook: the turn is over. If it touched Go code, run the done gate
# (make lint test), then notify. A red gate blocks the stop once — the agent
# gets the output and keeps fixing; on the second consecutive stop it only
# notifies, so the loop cannot spin.
#
# One red is legitimate: the flow's first commit is the tests, red, before the
# implementation exists. That state is recognized from git and from the gate
# halves, never from a flag the session could set for itself — see
# test_step_commit below.
set -uo pipefail

input=$(cat)
root=${CLAUDE_PROJECT_DIR:-$(pwd)}
name=$(basename "$root")
sid=$(jq -r '.session_id // "unknown"' <<<"$input")
active=$(jq -r '.stop_hook_active // false' <<<"$input")
state="/tmp/claude-hooks-$name/$sid.touched"

notify() { # message, sound
  osascript \
    -e 'on run argv' \
    -e 'display notification (item 1 of argv) with title (item 2 of argv) sound name (item 3 of argv)' \
    -e 'end run' \
    -- "$1" "$name" "$2" >/dev/null 2>&1 || true
}

if [ ! -s "$state" ]; then
  notify "czeka na Ciebie" "Ping"
  exit 0
fi

if ! grep -qE '(\.go|^go\.(mod|sum)|^\.golangci\.yml)$' "$state"; then
  # only docs or other non-Go files touched — nothing to gate
  rm -f "$state"
  notify "czeka na Ciebie (bez bramek)" "Ping"
  exit 0
fi

# test_step_commit reports whether HEAD is the flow's test commit: a `test`
# commit that stages nothing but *_test.go, with a clean tree behind it.
#
# The clean tree is the load-bearing part. It cannot be satisfied by wanting to
# be at the test step — only by actually being there, with the tests committed
# and nothing else in hand.
test_step_commit() {
  git -C "$root" diff --quiet 2>/dev/null || return 1
  git -C "$root" diff --cached --quiet 2>/dev/null || return 1
  [ -z "$(git -C "$root" ls-files --others --exclude-standard)" ] || return 1

  git -C "$root" log -1 --pretty=%s | grep -qE '^test(\([a-z0-9-]+\))?: ' || return 1

  local files
  files=$(git -C "$root" show --pretty=format: --name-only HEAD | grep -v '^$')
  [ -n "$files" ] || return 1
  ! grep -qv '_test\.go$' <<<"$files"
}

log=$(mktemp)
failed=false
test_step=false

if test_step_commit; then
  # The test step. Two questions, and the second is the one that makes
  # test-first checkable rather than claimed: the implementation must be
  # clean, and the tests must actually fail. A test that passes before its
  # implementation exists states nothing.
  test_step=true
  if ! make -C "$root" lint-impl >>"$log" 2>&1; then
    failed=true
  elif make -C "$root" test >>"$log" 2>&1; then
    {
      echo "--- the test step's tests pass with no implementation behind them ---"
      echo "A test that is green before the thing it tests exists is not a test."
      echo "Either it asserts nothing, or the implementation is already there and"
      echo "this is not the test step."
    } >>"$log"
    failed=true
  fi
elif ! make -C "$root" lint test >>"$log" 2>&1; then
  failed=true
fi

if [ "$failed" = true ]; then
  # Red: keep the state so the gate runs again next turn — the work is not done.
  notify "bramka czerwona" "Basso"
  if [ "$active" != "true" ]; then
    echo "Done gate failed." >&2
    echo "--- last lines of gate output ---" >&2
    tail -n 60 "$log" >&2
    rm -f "$log"
    exit 2
  fi
  rm -f "$log"
  exit 0
fi

rm -f "$log"
if [ "$test_step" = true ]; then
  # The state stays. The next turn is the implementation, the whole gate runs
  # again, and then it has to be green. The red is spent, not forgiven.
  notify "czerwone testy zgodnie z flow" "Glass"
  exit 0
fi

rm -f "$state"
notify "done — bramka zielona" "Glass"
exit 0

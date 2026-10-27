#!/bin/sh
# Validates a commit message (or a PR title, with --title) against
# Conventional Commits. PRs are squash-merged with the PR title as the
# commit subject, so CI holds the title to the same rule.

if [ "$1" = "--title" ]; then
  msg=$2
else
  msg=$(head -1 "$1")
fi

pattern='^(feat|fix|refactor|test|build|ci|docs|chore)(\([a-z0-9-]+\))?!?: .{1,72}$'

echo "$msg" | grep -qE "$pattern" && exit 0

echo ""
echo "✗ Bad commit message: $msg"
echo ""
echo "  Expected format: <type>(<scope>): <description>   (scope optional)"
echo "  Allowed types:   feat|fix|refactor|test|build|ci|docs|chore"
echo ""
echo "  Examples:"
echo "    feat: trace a lot downstream to deliveries"
echo "    fix(ingest): reject bookings without a lot number"
echo "    ci: pin golangci-lint to v2.15.0"
echo ""
exit 1

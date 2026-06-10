#!/usr/bin/env bash
set -euo pipefail

echo "==> go build"
go build ./...

echo "==> go test (unit)"
go test ./...

echo "==> go vet"
go vet ./...

echo "==> golangci-lint (if installed)"
if command -v golangci-lint >/dev/null 2>&1; then
  golangci-lint run ./...
else
  echo "  (skipped — install with: brew install golangci-lint)"
fi

echo "==> check-comment-density"
"$(dirname "$0")/check-comment-density.sh"

echo "==> check-no-bare-sleep"
"$(dirname "$0")/check-no-bare-sleep.sh"

# Catch the space-separated `closes #N #M` form GitHub silently drops
# (only the first issue auto-closes). Skips when no open PR matches HEAD.
echo "==> pr-body close-keyword form"
SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
if command -v gh >/dev/null 2>&1; then
  PR_NUM=$(gh pr view --json number --jq '.number' 2>/dev/null || true)
  if [ -n "$PR_NUM" ]; then
    "$SCRIPT_DIR/check-pr-body-close-keywords.sh" --pr "$PR_NUM"
  else
    echo "  (skipped — no PR for current branch)"
  fi
else
  echo "  (skipped — gh not installed)"
fi

echo "==> check-doc-links"
"$(dirname "$0")/check-doc-links.sh"

echo "==> check-base-fresh"
"$(dirname "$0")/check-base-fresh.sh"

echo "==> all checks passed"

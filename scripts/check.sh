#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)

go build ./... || { echo "build failed"; exit 1; }

# Serial: feature-completeness gates placeholder ships before tests run.
"$SCRIPT_DIR/check-feature-completeness.sh" || { echo "feature-completeness failed"; exit 1; }

go test -race -count=1 ./... || { echo "test failed"; exit 1; }

LOG=$(mktemp -d -t leah-check-XXXXXX)
trap 'rm -rf "$LOG"' EXIT

PIDS=()
NAMES=()
go vet ./...                           >"$LOG/vet"      2>&1 & PIDS+=($!); NAMES+=(vet)
if command -v golangci-lint >/dev/null 2>&1; then
  golangci-lint run ./...              >"$LOG/lint"     2>&1 & PIDS+=($!); NAMES+=(lint)
else
  echo "=== skipped: golangci-lint not installed (brew install golangci-lint) ==="
fi
"$SCRIPT_DIR/check-comment-density.sh" >"$LOG/density"  2>&1 & PIDS+=($!); NAMES+=(density)
"$SCRIPT_DIR/check-no-bare-sleep.sh"   >"$LOG/sleep"    2>&1 & PIDS+=($!); NAMES+=(sleep)
"$SCRIPT_DIR/check-doc-links.sh"       >"$LOG/doclinks" 2>&1 & PIDS+=($!); NAMES+=(doclinks)

FAILED=0
for i in "${!PIDS[@]}"; do
  if ! wait "${PIDS[$i]}"; then
    FAILED=1
    echo "stage failed: ${NAMES[$i]}"
  fi
done

for f in "$LOG"/*; do
  [ -s "$f" ] && { echo "--- $(basename "$f") ---"; cat "$f"; }
done

# Network-dependent: kept serial after parallel block. Catches the
# space-separated `closes #N #M` form GitHub silently drops.
if command -v gh >/dev/null 2>&1; then
  PR_NUM=$(gh pr view --json number --jq '.number' 2>/dev/null || true)
  if [ -n "$PR_NUM" ]; then
    "$SCRIPT_DIR/check-pr-body-close-keywords.sh" --pr "$PR_NUM" || FAILED=1
  fi
fi

# W90 base-staleness gate: serial — needs git fetch + ancestry walks.
"$SCRIPT_DIR/check-base-fresh.sh" || FAILED=1

exit "$FAILED"

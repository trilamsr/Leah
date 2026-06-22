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
# Hard-fail on missing golangci-lint — silent-skip shipped PR #278 CI-red.
# `make check`/`make lint` auto-install via the ensure-lint target.
if ! command -v golangci-lint >/dev/null 2>&1; then
  echo "golangci-lint not installed; run \`make lint\` or \`go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2\`"
  exit 1
fi
golangci-lint run --timeout=5m ./...    >"$LOG/lint"     2>&1 & PIDS+=($!); NAMES+=(lint)
"$SCRIPT_DIR/check-comment-density.sh" >"$LOG/density"  2>&1 & PIDS+=($!); NAMES+=(density)
"$SCRIPT_DIR/check-no-bare-sleep.sh"   >"$LOG/sleep"    2>&1 & PIDS+=($!); NAMES+=(sleep)
"$SCRIPT_DIR/check-doc-links.sh"       >"$LOG/doclinks" 2>&1 & PIDS+=($!); NAMES+=(doclinks)
# MAY-16: guard the destructive-regression class on the worktree janitor.
# Mutation tests confirm both the prefix-check and the merged/deleted
# guard are load-bearing — dropping either prunes live agent work.
"$SCRIPT_DIR/leah-worktree-janitor_test.sh" >"$LOG/janitor"  2>&1 & PIDS+=($!); NAMES+=(janitor)
"$SCRIPT_DIR/check-handoff-continuity_test.sh" >"$LOG/handoff" 2>&1 & PIDS+=($!); NAMES+=(handoff)
"$SCRIPT_DIR/audit-stale-specs_test.sh" >"$LOG/stalespecs" 2>&1 & PIDS+=($!); NAMES+=(stalespecs)
# MAY-13: stale-APPROVE guard tests pin the timestamp comparison + fixture routing.
"$SCRIPT_DIR/check-amend-after-approve_test.sh" >"$LOG/amend"  2>&1 & PIDS+=($!); NAMES+=(amend)

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
# LEAH_CI_GATES=skip is the documented emergency operator override; an
# audit-row appears in /tmp/cicheck.log so the skip is traceable.
if [ "${LEAH_CI_GATES:-}" = "skip" ]; then
  echo "=== LEAH_CI_GATES=skip — pr-body gate bypassed ==="
elif command -v gh >/dev/null 2>&1; then
  PR_NUM=$(gh pr view --json number --jq '.number' 2>/dev/null || true)
  if [ -n "$PR_NUM" ]; then
    "$SCRIPT_DIR/check-pr-body-close-keywords.sh" --pr "$PR_NUM" || FAILED=1
  fi
fi

# W90 base-staleness gate: serial — needs git fetch + ancestry walks.
"$SCRIPT_DIR/check-base-fresh.sh" || FAILED=1

# MAY-V6: warn-only on first ship — flip to FAILED=1 once 7 consecutive
# days of feat/* PRs land with zero new-placeholder hits.
if ! "$SCRIPT_DIR/check-placeholders.sh"; then
  echo "::warning::check-placeholders introduced new markers (warn-only)"
fi

exit "$FAILED"

#!/usr/bin/env bash
# WHY: reviewer APPROVE at sha A + author amend to sha B = APPROVE is for old code.
# Block merge unless HEAD commit timestamp <= latest REVIEWER APPROVE comment timestamp.
set -eu

if [ "$#" -lt 1 ] && [ -z "${PR:-}" ]; then
  echo "usage: check-amend-after-approve.sh <pr-number>  (or PR=<n>)" >&2
  exit 2
fi
PR="${1:-${PR}}"

REPO_FLAG=""
[ -n "${REPO:-}" ] && REPO_FLAG="--repo $REPO"

VIEW="$(gh pr view "$PR" $REPO_FLAG --json comments,headRefOid)"

HEAD_SHA="$(printf '%s' "$VIEW" | jq -r '.headRefOid')"

# WHY: pick the most recent REVIEWER APPROVE; older approvals are superseded.
APPROVE_TS="$(printf '%s' "$VIEW" \
  | jq -r '[.comments[] | select(.body | test("REVIEWER APPROVE")) | .createdAt] | max // empty')"

if [ -z "$APPROVE_TS" ]; then
  # WHY: no APPROVE recorded — this guard has nothing to enforce; merge gate lives elsewhere.
  exit 0
fi

OWNER_REPO="${REPO:-}"
if [ -z "$OWNER_REPO" ]; then
  OWNER_REPO="$(gh repo view --json nameWithOwner -q .nameWithOwner 2>/dev/null || echo "")"
fi
API_PATH="repos/${OWNER_REPO}/commits/${HEAD_SHA}"
[ -z "$OWNER_REPO" ] && API_PATH="repos/{owner}/{repo}/commits/${HEAD_SHA}"

HEAD_TS="$(gh api "$API_PATH" -q '.commit.author.date')"

# WHY: lexical ISO-8601 UTC compares chronologically; both come from GH normalized.
if [ "$HEAD_TS" \> "$APPROVE_TS" ]; then
  echo "approve-was-for-prior-sha: APPROVE at $APPROVE_TS, current head $HEAD_SHA committed at $HEAD_TS" >&2
  exit 1
fi

exit 0

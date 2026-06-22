#!/usr/bin/env bash
# WHY: reviewer APPROVE at sha A + author amend to sha B = APPROVE is for old code.
# Block merge unless the latest VALID reviewer APPROVE was posted at/after HEAD commit time.
# "Valid" = (a) author.login matches the cavecrew-reviewer agent-id shape per CLAUDE.md,
# AND (b) "REVIEWER APPROVE" appears at the start of a line (anchored — defeats the
# "quote the prior approval" bypass).
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

# WHY: author-login regex matches CLAUDE.md canonical shape — author cannot self-approve
# under their own login. Split body on newlines + anchor "^REVIEWER APPROVE" rejects
# quoted prior approvals ("> REVIEWER APPROVE ..."). max() over the filtered set picks
# the most recent valid APPROVE.
APPROVE_TS="$(printf '%s' "$VIEW" | jq -r '
  [ .comments[]
    | select(.author.login | test("^(a[0-9a-f]{16}|cavecrew-reviewer-[a-z0-9-]+)$"))
    | select(.body | split("\n") | any(test("^REVIEWER APPROVE")))
    | .createdAt
  ] | max // empty
')"

if [ -z "$APPROVE_TS" ]; then
  # WHY: no valid reviewer APPROVE — this guard has nothing to enforce; merge gate
  # (reviewer-required) lives elsewhere.
  exit 0
fi

OWNER_REPO="${REPO:-}"
if [ -z "$OWNER_REPO" ]; then
  OWNER_REPO="$(gh repo view --json nameWithOwner -q .nameWithOwner 2>/dev/null || echo "")"
fi
API_PATH="repos/${OWNER_REPO}/commits/${HEAD_SHA}"
[ -z "$OWNER_REPO" ] && API_PATH="repos/{owner}/{repo}/commits/${HEAD_SHA}"

HEAD_TS="$(gh api "$API_PATH" -q '.commit.author.date')"

# WHY: GitHub returns ISO-8601 but format varies (Z vs +00:00 vs +0000). Lexical compare
# inverts chronology across formats. Convert both to epoch seconds — GNU `date -d` first
# then BSD `date -j -f` fallback. Normalize "+HH:MM" → "+HHMM" for BSD strptime.
to_epoch() {
  local ts="$1"
  date -u -d "$ts" +%s 2>/dev/null && return
  local norm
  norm="$(printf '%s' "$ts" | sed -E 's/Z$/+0000/; s/([+-][0-9]{2}):([0-9]{2})$/\1\2/')"
  date -u -j -f "%Y-%m-%dT%H:%M:%S%z" "$norm" +%s
}

HEAD_EPOCH="$(to_epoch "$HEAD_TS")"
APPROVE_EPOCH="$(to_epoch "$APPROVE_TS")"

if [ "$HEAD_EPOCH" -gt "$APPROVE_EPOCH" ]; then
  echo "approve-was-for-prior-sha: APPROVE at $APPROVE_TS, current head $HEAD_SHA committed at $HEAD_TS" >&2
  exit 1
fi

exit 0

#!/usr/bin/env bash
# check-reviewer-verdict_test.sh asserts the gate fails closed on
# load-bearing PRs without a valid APPROVE recommendation, and passes
# the documented happy path. Minimal smoke port from regatta.

set -u
# pipefail intentionally omitted: tests assert the gate exits non-zero
# and pipe the output into grep — pipefail would propagate the gate's
# failure exit through the pipeline regardless of grep's match result.

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
GATE="$SCRIPT_DIR/check-reviewer-verdict.sh"
PASS=0
FAIL=0

fail() {
  echo "FAIL: $1"
  FAIL=$((FAIL + 1))
}

pass() {
  echo "ok: $1"
  PASS=$((PASS + 1))
}

# Negative case (brief-required): missing token on load-bearing → exit 1.
run_case_load_bearing_missing_token() {
  local body
  body=$(mktemp)
  cat > "$body" <<'EOF'
## Summary

Changes internal/adapters/confluence/instrumentation.go

```release-notes
[FEAT] thing
```
EOF
  "$GATE" --body-file "$body" --load-bearing >/dev/null 2>&1
  local rc=$?
  if [ "$rc" -eq 1 ]; then
    pass "load-bearing without token exits 1"
  else
    fail "load-bearing without token should exit 1 (got $rc)"
  fi
  rm -f "$body"
}

# Positive case (brief-required): valid APPROVE + cavecrew-reviewer id → exit 0.
run_case_load_bearing_approve_passes() {
  local body
  body=$(mktemp)
  cat > "$body" <<'EOF'
## Summary

Changes internal/adapters/confluence/instrumentation.go

Reviewer-agent-id: cavecrew-reviewer-abc123
Reviewer-recommendation: APPROVE

```release-notes
[FEAT] thing
```
EOF
  if "$GATE" --body-file "$body" --load-bearing --pr-author trilamsr >/dev/null 2>&1; then
    pass "load-bearing + APPROVE + valid reviewer-id exits 0"
  else
    fail "load-bearing + APPROVE + valid reviewer-id should exit 0"
  fi
  rm -f "$body"
}

# Self-tag: PR author == Reviewer-agent-id → exit 1.
run_case_self_tag_rejected() {
  local body
  body=$(mktemp)
  cat > "$body" <<'EOF'
Reviewer-agent-id: cavecrew-reviewer-trilamsr
Reviewer-recommendation: APPROVE
EOF
  "$GATE" --body-file "$body" --load-bearing --pr-author cavecrew-reviewer-trilamsr >/dev/null 2>&1
  local rc=$?
  if [ "$rc" -eq 1 ]; then
    pass "self-tag (author == reviewer-id) exits 1"
  else
    fail "self-tag should exit 1 (got $rc)"
  fi
  rm -f "$body"
}

run_case_load_bearing_missing_token
run_case_load_bearing_approve_passes
run_case_self_tag_rejected

echo "---"
echo "passed: $PASS  failed: $FAIL"
if [ "$FAIL" -gt 0 ]; then
  exit 1
fi

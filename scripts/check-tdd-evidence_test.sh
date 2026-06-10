#!/usr/bin/env bash
# check-tdd-evidence_test.sh — assert the gate enforces TDD-evidence
# presence on feat/* branches while permitting non-feat branches
# unconditionally.
#
# Driven by audit-session findings (2026-06-10): PRs #162, #199, #219
# landed as `feat(*)` without a pre-impl failing-test capture in body.
# CLAUDE.md "TDD + review" mandates RED-first capture — operator was
# enforcing by hand. This gate makes the rule machine-enforced.

set -u

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
GATE="$SCRIPT_DIR/check-tdd-evidence.sh"
PASS=0
FAIL=0

pass() { echo "ok: $1"; PASS=$((PASS + 1)); }
fail() { echo "FAIL: $1"; FAIL=$((FAIL + 1)); }

# feat/ branch, body LACKS TDD evidence → exit 1.
run_case_feat_missing_evidence() {
  local body
  body=$(mktemp)
  cat > "$body" <<'EOF'
## Summary
Adds a new adapter.

```release-notes
[FEAT] new adapter
```
EOF
  "$GATE" --branch feat/foo-bar --body-file "$body" >/dev/null 2>&1
  local rc=$?
  if [ "$rc" -eq 1 ]; then
    pass "feat/ branch w/o TDD evidence exits 1"
  else
    fail "feat/ branch w/o TDD evidence should exit 1 (got $rc)"
  fi
  rm -f "$body"
}

# feat/ branch, body HAS TDD evidence section → exit 0.
run_case_feat_with_evidence() {
  local body
  body=$(mktemp)
  cat > "$body" <<'EOF'
## Summary
Adds adapter X.

## TDD evidence
Pre-impl failing test output:

```
--- FAIL: TestAdapterX (0.00s)
    adapter_test.go:42: expected ok, got nil
FAIL
```
EOF
  if "$GATE" --branch feat/foo --body-file "$body" >/dev/null 2>&1; then
    pass "feat/ branch with TDD evidence exits 0"
  else
    fail "feat/ branch with TDD evidence should exit 0"
  fi
  rm -f "$body"
}

# feat/ branch, body lacks section heading but mentions "failing test" → exit 0.
# Token-permissive: RED→GREEN / red-first / test-first phrasings also accepted.
run_case_feat_token_variants() {
  for token in "failing test output" "RED→GREEN trace" "red-first commit" "test-first capture"; do
    local body
    body=$(mktemp)
    printf '## Summary\n\n%s\n' "$token" > "$body"
    if "$GATE" --branch feat/foo --body-file "$body" >/dev/null 2>&1; then
      pass "token variant accepted: $token"
    else
      fail "token variant should pass: $token"
    fi
    rm -f "$body"
  done
}

# Non-feat branch (fix/, refactor/, ci/, docs/) → exit 0 regardless of body.
run_case_non_feat_branch_skips() {
  local body
  body=$(mktemp)
  echo "no tdd here" > "$body"
  for br in fix/x refactor/y ci/z docs/w chore/q main; do
    if "$GATE" --branch "$br" --body-file "$body" >/dev/null 2>&1; then
      pass "non-feat branch $br exits 0"
    else
      fail "non-feat branch $br should exit 0"
    fi
  done
  rm -f "$body"
}

# Operator escape: `<!-- tdd-skip-justified: <reason ≥4 chars> -->` → exit 0
# on feat/ branch even without TDD section. Audit-row mandatory.
run_case_skip_marker() {
  local body
  body=$(mktemp)
  cat > "$body" <<'EOF'
## Summary
Pure rename — no behavior change.

<!-- tdd-skip-justified: dependency-bump only, no logic touched -->
EOF
  if "$GATE" --branch feat/dep-bump --body-file "$body" >/dev/null 2>&1; then
    pass "tdd-skip-justified marker exits 0"
  else
    fail "tdd-skip-justified marker should exit 0"
  fi
  rm -f "$body"
}

# Skip marker with reason <4 chars → still fails (audit-trail integrity).
run_case_skip_marker_too_short() {
  local body
  body=$(mktemp)
  cat > "$body" <<'EOF'
## Summary

<!-- tdd-skip-justified: x -->
EOF
  "$GATE" --branch feat/x --body-file "$body" >/dev/null 2>&1
  local rc=$?
  if [ "$rc" -eq 1 ]; then
    pass "skip marker with short reason exits 1"
  else
    fail "skip marker with short reason should exit 1 (got $rc)"
  fi
  rm -f "$body"
}

# --skip flag → exit 0 immediately (emergency operator override).
run_case_skip_flag() {
  local body
  body=$(mktemp)
  echo "" > "$body"
  if "$GATE" --branch feat/x --body-file "$body" --skip >/dev/null 2>&1; then
    pass "--skip flag exits 0"
  else
    fail "--skip flag should exit 0"
  fi
  rm -f "$body"
}

# Missing --body-file AND --pr → usage error exit 2.
run_case_usage_error() {
  "$GATE" --branch feat/x >/dev/null 2>&1
  local rc=$?
  if [ "$rc" -eq 2 ]; then
    pass "missing body source exits 2"
  else
    fail "missing body source should exit 2 (got $rc)"
  fi
}

run_case_feat_missing_evidence
run_case_feat_with_evidence
run_case_feat_token_variants
run_case_non_feat_branch_skips
run_case_skip_marker
run_case_skip_marker_too_short
run_case_skip_flag
run_case_usage_error

echo "---"
echo "passed: $PASS  failed: $FAIL"
if [ "$FAIL" -gt 0 ]; then
  exit 1
fi

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

# feat/ branch, body HAS TDD evidence heading PAIRED with failing output → exit 0.
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
    pass "feat/ branch with TDD evidence heading + FAIL exits 0"
  else
    fail "feat/ branch with TDD evidence heading + FAIL should exit 0"
  fi
  rm -f "$body"
}

# Bare `## TDD` heading with NO failing-output token → exit 1 (tightened
# from v1 — heading alone used to pass).
run_case_feat_bare_heading_rejected() {
  local body
  body=$(mktemp)
  cat > "$body" <<'EOF'
## Summary

## TDD
notes go here but no fail trace
EOF
  "$GATE" --branch feat/foo --body-file "$body" >/dev/null 2>&1
  local rc=$?
  if [ "$rc" -eq 1 ]; then
    pass "bare ## TDD heading w/o FAIL exits 1"
  else
    fail "bare ## TDD heading w/o FAIL should exit 1 (got $rc)"
  fi
  rm -f "$body"
}

# Explicit RED→GREEN / red-first / test-first token alone → exit 0.
run_case_feat_token_variants() {
  for token in "RED→GREEN trace" "red-first commit" "test-first capture"; do
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

# Operator escape: `<!-- tdd-skip-justified: <reason ≥32 chars> -->` → exit 0
# on feat/ branch even without TDD section. Audit-row mandatory.
run_case_skip_marker() {
  local body
  body=$(mktemp)
  cat > "$body" <<'EOF'
## Summary
Pure rename — no behavior change.

<!-- tdd-skip-justified: dependency-bump only, no logic touched in this PR -->
EOF
  if LEAH_STATE_DIR=$(mktemp -d) "$GATE" --branch feat/dep-bump --body-file "$body" >/dev/null 2>&1; then
    pass "tdd-skip-justified marker (≥32 chars) exits 0"
  else
    fail "tdd-skip-justified marker should exit 0"
  fi
  rm -f "$body"
}

# Skip marker with reason <32 chars → still fails (audit-trail integrity).
# 4-char v1 floor let `noop`/`wip!`/`skip` pass; raised to 32.
run_case_skip_marker_too_short() {
  local body
  body=$(mktemp)
  cat > "$body" <<'EOF'
## Summary

<!-- tdd-skip-justified: noop -->
EOF
  "$GATE" --branch feat/x --body-file "$body" >/dev/null 2>&1
  local rc=$?
  if [ "$rc" -eq 1 ]; then
    pass "skip marker with short reason (<32 chars) exits 1"
  else
    fail "skip marker with short reason should exit 1 (got $rc)"
  fi
  rm -f "$body"
}

# Bare --skip (no reason) → exit 2 (usage error, no silent bypass).
run_case_skip_flag_bare_rejected() {
  local body
  body=$(mktemp)
  echo "" > "$body"
  "$GATE" --branch feat/x --body-file "$body" --skip >/dev/null 2>&1
  local rc=$?
  if [ "$rc" -eq 2 ]; then
    pass "bare --skip (no reason) exits 2"
  else
    fail "bare --skip should exit 2 (got $rc)"
  fi
  rm -f "$body"
}

# --skip <reason ≥32> → exit 0 + audit-row emitted.
run_case_skip_flag_with_reason() {
  local body audit_dir
  body=$(mktemp)
  audit_dir=$(mktemp -d)
  echo "" > "$body"
  if LEAH_STATE_DIR="$audit_dir" "$GATE" --branch feat/x --body-file "$body" \
      --skip "emergency override: hotfix path, no test possible" >/dev/null 2>&1; then
    if [ -s "$audit_dir/audit.jsonl" ] && grep -q '"kind":"ci_gate_skipped"' "$audit_dir/audit.jsonl"; then
      pass "--skip <reason> exits 0 + writes audit row"
    else
      fail "--skip should write audit.jsonl row"
    fi
  else
    fail "--skip <reason ≥32 chars> should exit 0"
  fi
  rm -rf "$body" "$audit_dir"
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
run_case_feat_bare_heading_rejected
run_case_feat_token_variants
run_case_non_feat_branch_skips
run_case_skip_marker
run_case_skip_marker_too_short
run_case_skip_flag_bare_rejected
run_case_skip_flag_with_reason
run_case_usage_error

echo "---"
echo "passed: $PASS  failed: $FAIL"
if [ "$FAIL" -gt 0 ]; then
  exit 1
fi

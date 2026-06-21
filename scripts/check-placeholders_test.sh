#!/usr/bin/env bash
# check-placeholders_test.sh — assert the gate flags placeholder markers
# only when introduced by the current diff on feat/* branches, never on
# pre-existing code or on non-feat branches.
#
# Rationale (MAY-V6): feat/* PRs were landing with TODO/FIXME breadcrumbs
# and `panic("not implemented")` stubs disguised as ship-ready code.
# Diff-scoped because the whole-tree scan would trip on the ~80 legit
# placeholders already living in internal/recommend (sql `?` placeholders)
# and internal/voice/listener (W11→W12 stub trail).

set -u

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
GATE="$SCRIPT_DIR/check-placeholders.sh"
PASS=0
FAIL=0

pass() { echo "ok: $1"; PASS=$((PASS + 1)); }
fail() { echo "FAIL: $1"; FAIL=$((FAIL + 1)); }

# Each test spins a throwaway git repo with a `main` baseline and a
# `feat/x` branch carrying the introduction under test. Keeps the host
# repo's diff state out of the fixture.
mkrepo() {
  local dir
  dir=$(mktemp -d)
  (
    cd "$dir"
    git init -q -b main
    git config user.email t@t
    git config user.name t
    printf 'package p\n\nfunc Existing() int { return 1 }\n' > base.go
    git add base.go
    git commit -q -m base
    git checkout -q -b feat/x
  )
  echo "$dir"
}

# feat/* branch introducing a NEW TODO → exit 1.
case_new_todo_fails() {
  local dir; dir=$(mkrepo)
  (
    cd "$dir"
    printf 'package p\n\n// TODO: wire this up\nfunc New() int { return 2 }\n' > new.go
    git add new.go
    git commit -q -m add
  )
  "$GATE" --repo "$dir" --base main --branch feat/x >/dev/null 2>&1
  local rc=$?
  if [ "$rc" -eq 1 ]; then pass "new TODO on feat/ exits 1"
  else fail "new TODO on feat/ should exit 1 (got $rc)"; fi
  rm -rf "$dir"
}

# feat/* introducing FIXME → exit 1.
case_new_fixme_fails() {
  local dir; dir=$(mkrepo)
  (
    cd "$dir"
    printf 'package p\n\nfunc F() { /* FIXME: broken */ }\n' > f.go
    git add f.go
    git commit -q -m add
  )
  "$GATE" --repo "$dir" --base main --branch feat/x >/dev/null 2>&1
  [ $? -eq 1 ] && pass "new FIXME exits 1" || fail "new FIXME should exit 1"
  rm -rf "$dir"
}

# All-caps PLACEHOLDER — reviewer caught the regex missing it.
case_new_all_caps_placeholder_fails() {
  local dir; dir=$(mkrepo)
  (
    cd "$dir"
    printf 'package p\n\n// PLACEHOLDER: wire later\nfunc Z() {}\n' > z.go
    git add z.go
    git commit -q -m add
  )
  "$GATE" --repo "$dir" --base main --branch feat/x >/dev/null 2>&1
  [ $? -eq 1 ] && pass "new PLACEHOLDER (caps) exits 1" \
    || fail "new PLACEHOLDER should exit 1"
  rm -rf "$dir"
}

# feat/* introducing panic("not implemented") → exit 1.
case_new_panic_not_impl_fails() {
  local dir; dir=$(mkrepo)
  (
    cd "$dir"
    printf 'package p\n\nfunc Stub() { panic("not implemented") }\n' > stub.go
    git add stub.go
    git commit -q -m add
  )
  "$GATE" --repo "$dir" --base main --branch feat/x >/dev/null 2>&1
  [ $? -eq 1 ] && pass "new panic(not implemented) exits 1" || fail "new panic should exit 1"
  rm -rf "$dir"
}

# feat/* with NO new placeholders (clean addition) → exit 0.
case_clean_addition_passes() {
  local dir; dir=$(mkrepo)
  (
    cd "$dir"
    printf 'package p\n\nfunc Clean() int { return 42 }\n' > clean.go
    git add clean.go
    git commit -q -m add
  )
  "$GATE" --repo "$dir" --base main --branch feat/x >/dev/null 2>&1
  local rc=$?
  if [ "$rc" -eq 0 ]; then pass "clean addition exits 0"
  else fail "clean addition should exit 0 (got $rc)"; fi
  rm -rf "$dir"
}

# Pre-existing TODO carried over from base, no new placeholder → exit 0.
# This is the load-bearing case — we MUST NOT trip on baseline debt.
case_preexisting_todo_ignored() {
  local dir; dir=$(mktemp -d)
  (
    cd "$dir"
    git init -q -b main
    git config user.email t@t
    git config user.name t
    printf 'package p\n\n// TODO: legacy debt\nfunc Old() {}\n' > base.go
    git add base.go
    git commit -q -m base
    git checkout -q -b feat/x
    printf 'package p\n\nfunc Added() int { return 7 }\n' > new.go
    git add new.go
    git commit -q -m add
  )
  "$GATE" --repo "$dir" --base main --branch feat/x >/dev/null 2>&1
  local rc=$?
  if [ "$rc" -eq 0 ]; then pass "pre-existing TODO not flagged"
  else fail "pre-existing TODO should not flag (got $rc)"; fi
  rm -rf "$dir"
}

# Non-feat branch (fix/, refactor/, chore/, main) → exit 0 regardless.
case_non_feat_branch_skips() {
  local dir; dir=$(mkrepo)
  (
    cd "$dir"
    git checkout -q -B fix/y main
    printf 'package p\n\n// TODO: ship as fix\nfunc X() {}\n' > x.go
    git add x.go
    git commit -q -m add
  )
  "$GATE" --repo "$dir" --base main --branch fix/y >/dev/null 2>&1
  [ $? -eq 0 ] && pass "fix/ branch skips placeholder gate" \
    || fail "fix/ branch should exit 0"
  rm -rf "$dir"
}

# Test files (`*_test.go`) get the placeholder gate waived — tests
# legitimately stub with TODOs naming the case under construction.
case_test_files_waived() {
  local dir; dir=$(mkrepo)
  (
    cd "$dir"
    printf 'package p\n\n// TODO: add coverage for edge case\nfunc TestX(_ *struct{}) {}\n' > x_test.go
    git add x_test.go
    git commit -q -m add
  )
  "$GATE" --repo "$dir" --base main --branch feat/x >/dev/null 2>&1
  [ $? -eq 0 ] && pass "_test.go waived" || fail "_test.go should be waived"
  rm -rf "$dir"
}

# Escape marker: `<!-- placeholder-justified: <reason ≥32 chars> -->`
# in PR body lets the gate pass. Same convention as comment-density.
case_escape_marker_allows() {
  local dir; dir=$(mkrepo)
  local body
  (
    cd "$dir"
    printf 'package p\n\n// TODO: intentional stub\nfunc Y() {}\n' > y.go
    git add y.go
    git commit -q -m add
  )
  body=$(mktemp)
  echo '<!-- placeholder-justified: intentional stub for W7 follow-up wiring -->' > "$body"
  "$GATE" --repo "$dir" --base main --branch feat/x --body-file "$body" >/dev/null 2>&1
  [ $? -eq 0 ] && pass "escape marker allows" || fail "escape marker should pass"
  rm -f "$body"
  rm -rf "$dir"
}

# Self-test: running the gate on THIS branch (which adds the scripts)
# must not flag itself — the scripts contain the literal tokens but the
# gate only scans *.go.
case_self_does_not_trip() {
  local repo
  repo=$(cd "$SCRIPT_DIR/.." && pwd)
  "$GATE" --repo "$repo" --base origin/main --branch feat/v6-placeholder-detect >/dev/null 2>&1
  local rc=$?
  if [ "$rc" -eq 0 ]; then pass "gate does not flag its own scripts"
  else fail "gate flagged itself (rc=$rc)"; fi
}

case_new_todo_fails
case_new_fixme_fails
case_new_all_caps_placeholder_fails
case_new_panic_not_impl_fails
case_clean_addition_passes
case_preexisting_todo_ignored
case_non_feat_branch_skips
case_test_files_waived
case_escape_marker_allows
case_self_does_not_trip

echo "---"
echo "passed: $PASS  failed: $FAIL"
[ "$FAIL" -gt 0 ] && exit 1 || exit 0

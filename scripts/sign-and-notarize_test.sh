#!/usr/bin/env bash
# Tests for sign-and-notarize.sh. Requires no Apple Developer cert.
set -euo pipefail

SCRIPT="$(cd "$(dirname "$0")" && pwd)/sign-and-notarize.sh"
PASS=0
FAIL=0

ok() { echo "ok: $1"; PASS=$((PASS+1)); }
fail() { echo "FAIL: $1"; FAIL=$((FAIL+1)); }

# Assertion 1: no-args prints usage and exits 0.
rc=0
out=$("$SCRIPT" 2>&1) || rc=$?
if echo "$out" | grep -qi "usage"; then
  if [ "$rc" -eq 0 ]; then
    ok "no-args prints usage and exits 0"
  else
    fail "no-args exited $rc (want 0)"
  fi
else
  fail "no-args did not print usage (got: $out)"
fi

# Assertion 2: --build-only does NOT call codesign.
build_out=$("$SCRIPT" --build-only 2>&1) || true
if echo "$build_out" | grep -qiE "codesign|Developer ID"; then
  fail "--build-only output mentions codesign (should not)"
else
  ok "--build-only does not mention codesign"
fi

# Assertion 3: --sign without env vars prints clear error and exits non-zero.
sign_rc=0
sign_out=$(LEAH_TEAM_ID="" LEAH_APPLE_ID="" LEAH_APP_SPECIFIC_PASSWORD="" LEAH_KEYCHAIN_PROFILE="" \
  "$SCRIPT" --sign 2>&1) || sign_rc=$?
if [ "$sign_rc" -ne 0 ] && echo "$sign_out" | grep -qiE "LEAH_TEAM_ID|LEAH_APPLE_ID|required|missing|must"; then
  ok "--sign without env vars exits non-zero with clear error"
else
  fail "--sign without env vars: rc=$sign_rc, output: $sign_out"
fi

echo ""
echo "results: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ]

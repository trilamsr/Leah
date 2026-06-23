#!/usr/bin/env bash
# generate-appcast_test.sh — assert generate-appcast.sh emits a Sparkle appcast
# with one <item> per signed .zip in the release dir, carries a stable channel
# by default, and refuses bad args.

set -u

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
GEN="$SCRIPT_DIR/generate-appcast.sh"
PASS=0
FAIL=0

pass() { echo "ok: $1"; PASS=$((PASS + 1)); }
fail() { echo "FAIL: $1"; FAIL=$((FAIL + 1)); }

run_case_help_exits_zero() {
  "$GEN" --help >/dev/null 2>&1
  local rc=$?
  if [ "$rc" -eq 0 ]; then
    pass "--help exits 0"
  else
    fail "--help should exit 0 (got $rc)"
  fi
}

run_case_no_args_rejected() {
  "$GEN" >/dev/null 2>&1
  local rc=$?
  if [ "$rc" -eq 2 ]; then
    pass "no args exits 2"
  else
    fail "no args should exit 2 (got $rc)"
  fi
}

run_case_missing_release_dir_rejected() {
  "$GEN" /tmp/leah-does-not-exist-$$ 1.2.3 https://example.test/leah >/dev/null 2>&1
  local rc=$?
  if [ "$rc" -ne 0 ]; then
    pass "missing release dir exits non-zero"
  else
    fail "missing release dir should exit non-zero (got $rc)"
  fi
}

run_case_emits_appcast_with_stable_channel() {
  local work
  work=$(mktemp -d -t leah-appcast.XXXXXX)
  # Fixture release dir: one .zip + sidecar .sig (sign_update output format
  # is "sparkle:edSignature=\"...\" length=\"...\""; tests stub the sig file
  # with that literal so generate-appcast.sh just concatenates).
  printf 'fake-payload' > "$work/Leah-1.2.3.zip"
  printf 'sparkle:edSignature="AAAA" length="12"' > "$work/Leah-1.2.3.zip.sig"
  local out="$work/appcast.xml"
  "$GEN" "$work" 1.2.3 "https://example.test/leah" > "$out" 2>"$work/err"
  local rc=$?
  if [ "$rc" -ne 0 ]; then
    fail "expected exit 0 (got $rc)"
    cat "$work/err"
    rm -rf "$work"
    return
  fi
  if ! grep -q "<sparkle:channel>stable</sparkle:channel>" "$out"; then
    fail "appcast missing default stable channel tag"
    cat "$out"
    rm -rf "$work"
    return
  fi
  if ! grep -q "Leah-1.2.3.zip" "$out"; then
    fail "appcast missing zip filename"
    cat "$out"
    rm -rf "$work"
    return
  fi
  if ! grep -q 'xmlns:sparkle=' "$out"; then
    fail "appcast missing sparkle namespace"
    cat "$out"
    rm -rf "$work"
    return
  fi
  pass "emits appcast with stable channel + sparkle namespace"
  rm -rf "$work"
}

run_case_channel_override() {
  local work
  work=$(mktemp -d -t leah-appcast.XXXXXX)
  printf 'fake-payload' > "$work/Leah-1.2.3.zip"
  printf 'sparkle:edSignature="AAAA" length="12"' > "$work/Leah-1.2.3.zip.sig"
  local out
  out="$(LEAH_APPCAST_CHANNEL=rollback "$GEN" "$work" 1.2.3 "https://example.test/leah" 2>"$work/err")"
  if echo "$out" | grep -q "<sparkle:channel>rollback</sparkle:channel>"; then
    pass "LEAH_APPCAST_CHANNEL=rollback overrides default"
  else
    fail "channel override did not propagate"
    echo "$out"
    cat "$work/err"
  fi
  rm -rf "$work"
}

run_case_missing_sig_fails_closed() {
  local work
  work=$(mktemp -d -t leah-appcast.XXXXXX)
  printf 'fake-payload' > "$work/Leah-1.2.3.zip"
  # No .sig file — must not silently emit an unsigned <item>.
  "$GEN" "$work" 1.2.3 "https://example.test/leah" >/dev/null 2>"$work/err"
  local rc=$?
  if [ "$rc" -ne 0 ]; then
    pass "missing sidecar .sig fails closed"
  else
    fail "missing .sig should fail closed (got rc=$rc)"
  fi
  rm -rf "$work"
}

run_case_help_exits_zero
run_case_no_args_rejected
run_case_missing_release_dir_rejected
run_case_emits_appcast_with_stable_channel
run_case_channel_override
run_case_missing_sig_fails_closed

echo "---"
echo "passed: $PASS  failed: $FAIL"
if [ "$FAIL" -gt 0 ]; then
  exit 1
fi

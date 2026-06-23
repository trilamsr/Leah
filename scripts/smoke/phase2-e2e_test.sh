#!/usr/bin/env bash
# Unit-test for phase2-e2e.sh — exercises argument parsing, the macOS guard,
# and the cleanup hook. Does NOT boot the daemon or assert the eight runtime
# invariants (that runs separately in CI on macos-latest).
set -uo pipefail

THIS="$(cd "$(dirname "$0")" && pwd)"
SCRIPT="$THIS/phase2-e2e.sh"

fail() { echo "FAIL: $*" >&2; exit 1; }

[ -x "$SCRIPT" ] || fail "phase2-e2e.sh missing or not executable"

# --- help flag exits 0 and mentions the eight invariants by number ---------
HELP_OUT="$("$SCRIPT" --help 2>&1)"
HELP_RC=$?
[ $HELP_RC -eq 0 ] || fail "--help rc=$HELP_RC, want 0"
echo "$HELP_OUT" | grep -q "phase2-e2e" || fail "--help output missing program name"
for n in 1 2 3 4 5 6 7 8; do
  echo "$HELP_OUT" | grep -q "($n)" || fail "--help missing invariant ($n) label"
done

# --- unknown flag exits non-zero and points at --help ---------------------
"$SCRIPT" --not-a-flag >/tmp/phase2-e2e-bad.log 2>&1
BAD_RC=$?
[ $BAD_RC -ne 0 ] || fail "unknown flag returned 0, want non-zero"
grep -q -- "--help" /tmp/phase2-e2e-bad.log || fail "unknown-flag error did not reference --help"
rm -f /tmp/phase2-e2e-bad.log

# --- non-macOS OS guard exits 0 with skip message -------------------------
OUT="$(LEAH_PHASE2_FAKE_OS=linux "$SCRIPT" --check-os-guard 2>&1)"
OS_RC=$?
[ $OS_RC -eq 0 ] || fail "OS guard rc=$OS_RC on fake-linux, want 0 (skip)"
echo "$OUT" | grep -qi "skip" || fail "OS guard skip message missing 'skip'"

# --- cleanup path tears down state dir and pid file -----------------------
TMP="$(mktemp -d)"
TMP_PID="$TMP/daemon.pid"
echo 999999 > "$TMP_PID"
LEAH_PHASE2_STATE="$TMP/state" \
  LEAH_PHASE2_PIDFILE="$TMP_PID" \
  "$SCRIPT" --cleanup-only >/dev/null 2>&1
[ ! -f "$TMP_PID" ] || fail "cleanup did not remove pid file"
[ ! -d "$TMP/state" ] || fail "cleanup did not remove state dir"
rm -rf "$TMP"

echo "ok"

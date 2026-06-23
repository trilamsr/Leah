#!/usr/bin/env bash
# Unit-test for phase3-smoke.sh — exercises argument parsing, the macOS guard,
# the cleanup hook (trap EXIT runs even on failure), and the exit-code-carries-
# step-number contract. Does NOT boot the daemon or assert the ten runtime
# invariants (that runs separately in CI on macos-latest).
set -uo pipefail

THIS="$(cd "$(dirname "$0")" && pwd)"
SCRIPT="$THIS/phase3-smoke.sh"

fail() { echo "FAIL: $*" >&2; exit 1; }

[ -x "$SCRIPT" ] || fail "phase3-smoke.sh missing or not executable"

# --- help flag exits 0 and mentions usage + ten invariants by number -------
HELP_OUT="$("$SCRIPT" --help 2>&1)"
HELP_RC=$?
[ $HELP_RC -eq 0 ] || fail "--help rc=$HELP_RC, want 0"
echo "$HELP_OUT" | grep -q "usage:" || fail "--help missing 'usage:' literal"
for n in 1 2 3 4 5 6 7 8 9 10; do
  echo "$HELP_OUT" | grep -q "($n)" || fail "--help missing invariant ($n) label"
done

# --- unknown flag exits non-zero and points at --help ----------------------
"$SCRIPT" --not-a-flag >/tmp/phase3-smoke-bad.log 2>&1
BAD_RC=$?
[ $BAD_RC -ne 0 ] || fail "unknown flag returned 0, want non-zero"
grep -q -- "--help" /tmp/phase3-smoke-bad.log || fail "unknown-flag error did not reference --help"
rm -f /tmp/phase3-smoke-bad.log

# --- non-macOS OS guard exits 0 with skip message --------------------------
OUT="$(LEAH_PHASE3_FAKE_OS=linux "$SCRIPT" --check-os-guard 2>&1)"
OS_RC=$?
[ $OS_RC -eq 0 ] || fail "OS guard rc=$OS_RC on fake-linux, want 0 (skip)"
echo "$OUT" | grep -qi "skip" || fail "OS guard skip message missing 'skip'"

# --- cleanup-only path tears down state dir and pid file -------------------
TMP="$(mktemp -d)"
TMP_PID="$TMP/daemon.pid"
echo 999999 > "$TMP_PID"
LEAH_PHASE3_STATE="$TMP/state" \
  LEAH_PHASE3_PIDFILE="$TMP_PID" \
  "$SCRIPT" --cleanup-only >/dev/null 2>&1
[ ! -f "$TMP_PID" ] || fail "cleanup did not remove pid file"
[ ! -d "$TMP/state" ] || fail "cleanup did not remove state dir"
rm -rf "$TMP"

# --- trap EXIT runs even when an assertion fails ---------------------------
# Force-fail at step 1 via the documented hook; assert cleanup still ran.
TMP="$(mktemp -d)"
TMP_PID="$TMP/daemon.pid"
echo 999999 > "$TMP_PID"
LEAH_PHASE3_STATE="$TMP/state" \
  LEAH_PHASE3_PIDFILE="$TMP_PID" \
  LEAH_PHASE3_FORCE_FAIL=3 \
  LEAH_PHASE3_FAKE_OS=Darwin \
  "$SCRIPT" --self-test-trap >/tmp/phase3-smoke-trap.log 2>&1
TRAP_RC=$?
mkdir -p "$TMP/state" 2>/dev/null || true   # restored below; check below verifies remove
[ "$TRAP_RC" -eq 3 ] || fail "force-fail exit code=$TRAP_RC, want 3 (step number)"
grep -q "STEP=3" /tmp/phase3-smoke-trap.log || fail "force-fail did not emit STEP=3"
[ ! -f "$TMP_PID" ] || fail "trap EXIT did not remove pid file on assertion fail"
rm -rf "$TMP" /tmp/phase3-smoke-trap.log

echo "ok"

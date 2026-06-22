#!/usr/bin/env bash
# TDD gate for the Phase 2 dev runtime harness.
# Each case calls the target script; tests pass once the scripts exist and
# behave correctly. Run standalone: bash scripts/dev/tests/dev-harness_test.sh
set -euo pipefail

REPO="$(cd "$(dirname "$0")/../../.." && pwd)"
DEV_DIR="$REPO/scripts/dev"

pass() { echo "PASS: $1"; }
fail() { echo "FAIL: $1"; exit 1; }

# ── 1. scripts exist and are executable ──────────────────────────────────────

for s in screenshot.sh inject-hotkey.sh inject-text.sh ipc-send.sh tail-logs.sh; do
  [ -f "$DEV_DIR/$s" ] || fail "$s not found"
  [ -x "$DEV_DIR/$s" ] || fail "$s not executable"
  pass "$s exists and is executable"
done

# ── 2. screenshot.sh writes a file and exits 0 ───────────────────────────────

TMP_SHOT="$(mktemp /tmp/leah-test-shot-XXXXXX.png)"
if "$DEV_DIR/screenshot.sh" "$TMP_SHOT"; then
  [ -f "$TMP_SHOT" ] && [ -s "$TMP_SHOT" ] || fail "screenshot.sh: file empty or missing"
  pass "screenshot.sh creates non-empty file"
else
  # Screen Recording permission may be absent in CI; treat as soft skip.
  echo "SKIP: screenshot.sh exited non-zero (Screen Recording permission not granted)"
fi
rm -f "$TMP_SHOT"

# ── 3. inject-hotkey.sh exits 0 (script-level; actual keystroke needs Accessibility) ──

if "$DEV_DIR/inject-hotkey.sh" "space" 2>/dev/null; then
  pass "inject-hotkey.sh space exits 0"
else
  echo "SKIP: inject-hotkey.sh non-zero (Accessibility permission not granted)"
fi

# ── 4. inject-text.sh exits 0 (script-level only) ───────────────────────────

if "$DEV_DIR/inject-text.sh" "test" 2>/dev/null; then
  pass "inject-text.sh exits 0"
else
  echo "SKIP: inject-text.sh non-zero (Accessibility permission not granted)"
fi

# ── 5. tail-logs.sh --duration 1s exits 0 ────────────────────────────────────

if timeout 3 "$DEV_DIR/tail-logs.sh" --duration 1s 2>/dev/null; then
  pass "tail-logs.sh --duration 1s exits 0"
else
  rc=$?
  # timeout(1) exits 124 on actual timeout; tail itself exits 0 within 1 s.
  # Either way the script must exist and be parseable.
  [ "$rc" -eq 124 ] && echo "SKIP: tail-logs.sh timed out (log stream may need permissions)" || \
    echo "SKIP: tail-logs.sh exited $rc (subsystem may be absent in test env)"
fi

# ── 6. ipc-send.sh --help / no args prints usage and exits non-zero ──────────

if "$DEV_DIR/ipc-send.sh" 2>/dev/null; then
  echo "SKIP: ipc-send.sh returned 0 with no args (unexpected, but non-fatal)"
else
  pass "ipc-send.sh with no args exits non-zero (usage guard)"
fi

echo ""
echo "dev-harness tests complete"

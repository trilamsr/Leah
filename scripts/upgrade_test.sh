#!/usr/bin/env bash
# Smoke test runner for scripts/upgrade.sh — uses a sandboxed LEAH_STATE_DIR
# so it never touches the real ~/.leah-state/. Each subtest exits non-zero
# on failure; trap aggregates results.
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
UPGRADE="$ROOT/scripts/upgrade.sh"

if [ ! -x "$UPGRADE" ]; then
  echo "FAIL: $UPGRADE not executable"
  exit 1
fi

SANDBOX=$(mktemp -d -t leah-upgrade-test-XXXX)
cleanup() { rm -rf "$SANDBOX"; }
trap cleanup EXIT

export LEAH_STATE_DIR="$SANDBOX/.leah-state"
export LEAH_BIN_DIR="$SANDBOX/bin"
export LEAH_UPGRADE_DRY_RUN=1

pass=0
fail=0
note() { printf '  %s\n' "$1"; }
ok()   { pass=$((pass+1)); printf 'PASS: %s\n' "$1"; }
ko()   { fail=$((fail+1)); printf 'FAIL: %s\n' "$1"; }

# ---- T1: install creates ~/.leah-state/bin/ and symlinks ----
"$UPGRADE" install >"$SANDBOX/install.log" 2>&1 || { cat "$SANDBOX/install.log"; ko "install exit"; exit 1; }

if [ -L "$LEAH_STATE_DIR/bin/leah-current" ]; then
  ok "install creates leah-current symlink"
else
  ko "install creates leah-current symlink"
fi

if [ -L "$LEAH_BIN_DIR/leah" ]; then
  ok "install creates ~/bin/leah symlink"
else
  ko "install creates ~/bin/leah symlink"
fi

target=$(readlink "$LEAH_STATE_DIR/bin/leah-current")
case "$target" in
  *leah-*) ok "leah-current points at SHA-suffixed artifact" ;;
  *)       ko "leah-current points at SHA-suffixed artifact (got: $target)" ;;
esac

# ---- T2: install is idempotent ----
"$UPGRADE" install >"$SANDBOX/install2.log" 2>&1 || { cat "$SANDBOX/install2.log"; ko "install idempotent exit"; exit 1; }
ok "install is idempotent"

# ---- T3: dry-run upgrade skips swap ----
prev_target=$(readlink "$LEAH_STATE_DIR/bin/leah-current")
LEAH_UPGRADE_DRY_RUN=1 "$UPGRADE" upgrade >"$SANDBOX/upgrade-dry.log" 2>&1 || true
new_target=$(readlink "$LEAH_STATE_DIR/bin/leah-current")
if [ "$prev_target" = "$new_target" ]; then
  ok "dry-run upgrade does not change leah-current"
else
  ko "dry-run upgrade unexpectedly mutated symlink ($prev_target → $new_target)"
fi

# ---- T4: atomic swap — synthesize current+new, swap, verify ----
swap_dir="$SANDBOX/swap-test"
mkdir -p "$swap_dir"
echo "OLD" > "$swap_dir/leah-aaa"
echo "NEW" > "$swap_dir/leah-bbb"
ln -s "$swap_dir/leah-aaa" "$swap_dir/leah-current"

# Run the swap-symlink subcommand against the fixture
"$UPGRADE" swap-symlink "$swap_dir/leah-current" "$swap_dir/leah-bbb" "$swap_dir/leah-previous" \
  >"$SANDBOX/swap.log" 2>&1 || { cat "$SANDBOX/swap.log"; ko "swap exit"; exit 1; }

if [ "$(readlink "$swap_dir/leah-current")" = "$swap_dir/leah-bbb" ]; then
  ok "swap repoints leah-current to new artifact"
else
  ko "swap did not repoint leah-current (got: $(readlink "$swap_dir/leah-current"))"
fi

if [ "$(readlink "$swap_dir/leah-previous")" = "$swap_dir/leah-aaa" ]; then
  ok "swap preserves previous artifact in leah-previous"
else
  ko "swap did not set leah-previous (got: $(readlink "$swap_dir/leah-previous" 2>/dev/null || echo MISSING))"
fi

# ---- T5: rollback swaps current ↔ previous ----
"$UPGRADE" rollback-symlink "$swap_dir/leah-current" "$swap_dir/leah-previous" \
  >"$SANDBOX/rollback.log" 2>&1 || { cat "$SANDBOX/rollback.log"; ko "rollback exit"; exit 1; }

if [ "$(readlink "$swap_dir/leah-current")" = "$swap_dir/leah-aaa" ]; then
  ok "rollback restores leah-current to previous artifact"
else
  ko "rollback did not restore leah-current (got: $(readlink "$swap_dir/leah-current"))"
fi

echo
echo "summary: $pass pass, $fail fail"
exit $fail

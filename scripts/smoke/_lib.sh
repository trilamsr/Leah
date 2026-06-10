#!/usr/bin/env bash
# Shared smoke helpers. Source via: source "$(dirname "$0")/_lib.sh"
set -euo pipefail

smoke_mode() { echo "${LEAH_SMOKE_LIVE:-0}"; }
smoke_skip_if_mock_only() {
  if [ "$(smoke_mode)" != "1" ]; then
    echo "[smoke] $1: mock-mode skip (set LEAH_SMOKE_LIVE=1 to run live)"
    exit 0
  fi
}
smoke_log() { echo "[smoke] $*"; }
smoke_assert_eq() {
  if [ "$1" != "$2" ]; then
    echo "[smoke] FAIL: expected '$2', got '$1'"; exit 1
  fi
}
smoke_state_dir() {
  d="${LEAH_STATE_DIR:-${TMPDIR:-/tmp}/leah-smoke-$$}"
  mkdir -p "$d"
  echo "$d"
}

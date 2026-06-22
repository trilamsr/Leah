#!/usr/bin/env bash
# Phase 1 end-to-end smoke: boot leah-daemon, send an `ask` frame over the
# Unix socket, assert prose.delta + turn.end come back. Validates the
# daemon ↔ LLM ↔ IPC path WITHOUT the SwiftUI app (which requires a
# notarized .app for the global hotkey to register).
set -euo pipefail

REPO="$(cd "$(dirname "$0")/../.." && pwd)"
PING="$REPO/scripts/smoke/ipc-ping.go"

if [ -z "${ANTHROPIC_API_KEY:-}" ]; then
  echo "SKIP: ANTHROPIC_API_KEY not set, partial smoke only" >&2
  # Still verify the daemon builds cleanly.
  go build -C "$REPO" ./cmd/leah-daemon >/dev/null 2>&1 && echo "daemon build ok"
  exit 0
fi

STATE="$(mktemp -d)"
SOCK="$HOME/Library/Caches/Leah/leah.sock"
mkdir -p "$(dirname "$SOCK")"
# Remove any stale socket so the daemon can bind immediately.
rm -f "$SOCK"

LEAH_STATE_DIR="$STATE" ANTHROPIC_API_KEY="$ANTHROPIC_API_KEY" \
  go run -C "$REPO" ./cmd/leah-daemon &
DAEMON_PID=$!
trap 'kill "$DAEMON_PID" 2>/dev/null || true; rm -rf "$STATE"' EXIT

# Wait up to 10 s for the socket to appear.
for i in $(seq 1 100); do
  [ -S "$SOCK" ] && break
  sleep 0.1
done
if [ ! -S "$SOCK" ]; then
  echo "FAIL: socket did not appear at $SOCK" >&2
  exit 1
fi

# Embedded Go IPC client compiled at runtime: speaks the length-prefixed JSON
# frame protocol, sends an `ask` frame, asserts prose.delta + turn.end come back.
go run "$PING" "$SOCK"

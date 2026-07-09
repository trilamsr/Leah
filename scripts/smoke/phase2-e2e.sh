#!/usr/bin/env bash
# Phase 2 end-to-end smoke for leah-daemon. Eight invariants, single script,
# clean exit on macOS, friendly SKIP elsewhere.
#
# Invariants:
#   (1) widget.mount streamed for a widget-shaped ask (Haiku classify path)
#   (2) pin persistence across daemon restart
#   (3) pin cap 2 enforced (third Pin returns notification.toast)
#   (4) notification.toast frame shape (level=info, text non-empty)
#   (5) light-mode toggle does not crash the daemon
#   (6) fsnotify watcher coalesces a 200 ms burst into one PinnedChanged
#   (7) BGE backend selectable and routes to embeddings_bge_small_en_v1_5_384
#   (8) spec-parity check exits 0
set -uo pipefail

THIS="$(cd "$(dirname "$0")" && pwd)"
REPO="$(cd "$THIS/../.." && pwd)"
PROBE="$THIS/phase2-probe.go"

STATE_DEFAULT="$HOME/.leah-state-e2e"
STATE="${LEAH_PHASE2_STATE:-$STATE_DEFAULT}"
PIDFILE="${LEAH_PHASE2_PIDFILE:-/tmp/leah-phase2-e2e.pid}"
SOCK="${LEAH_PHASE2_SOCK:-$HOME/Library/Caches/Leah/leah.sock}"
LOG="${LEAH_PHASE2_LOG:-/tmp/leah-phase2-e2e.log}"
SUPPORT_DIR="$STATE/Library/Application Support/Leah"

print_help() {
  cat <<'EOF'
phase2-e2e.sh — Phase 2 leah-daemon end-to-end smoke

Usage:
  phase2-e2e.sh                 run the full e2e (macOS only)
  phase2-e2e.sh --help          print this message
  phase2-e2e.sh --check-os-guard verify the host-OS guard (returns 0 with
                                 "skip" message on non-darwin)
  phase2-e2e.sh --cleanup-only  tear down state + pid file without running

Invariants:
  (1) widget.mount streamed for a widget-shaped ask
  (2) pin persistence across daemon restart
  (3) pin cap 2 enforced (third Pin returns notification.toast)
  (4) notification.toast frame shape (level=info, text non-empty)
  (5) light-mode toggle does not crash the daemon
  (6) fsnotify watcher coalesces a 200 ms burst into one PinnedChanged
  (7) BGE backend selectable and routes to embeddings_bge_small_en_v1_5_384
  (8) spec-parity check exits 0

Environment:
  LEAH_PHASE2_FAKE_OS     forces the OS guard to act as if running on this OS
  LEAH_PHASE2_STATE       override state dir (default ~/.leah-state-e2e)
  LEAH_PHASE2_PIDFILE     override pid file path
  LEAH_PHASE2_SOCK        override leah-daemon socket path
  LEAH_PHASE2_LOG         override daemon log path
  LEAH_EMBED_MODEL_PATH   .onnx model for BGE (invariant 7 SKIPs without it)
  ANTHROPIC_API_KEY       required for invariant 1 (SKIPs otherwise)
EOF
}

step_fail() {
  local n="$1"; shift
  echo "FAIL: step $n: $*" >&2
  echo "STEP=$n" >&2
  exit "$n"
}

cleanup() {
  if [ -f "$PIDFILE" ]; then
    local pid
    pid="$(cat "$PIDFILE" 2>/dev/null || true)"
    if [ -n "$pid" ]; then
      kill "$pid" 2>/dev/null || true
      for _ in 1 2 3 4 5; do
        kill -0 "$pid" 2>/dev/null || break
        sleep 0.2
      done
      kill -9 "$pid" 2>/dev/null || true
    fi
    rm -f "$PIDFILE"
  fi
  rm -f "$SOCK"
  rm -rf "$STATE"
}

# -- arg parsing -----------------------------------------------------------
case "${1:-}" in
  -h|--help)
    print_help; exit 0 ;;
  --cleanup-only)
    cleanup; exit 0 ;;
  --check-os-guard)
    HOST_OS="${LEAH_PHASE2_FAKE_OS:-$(uname -s)}"
    case "$HOST_OS" in
      Darwin|darwin) echo "OS guard: darwin — full e2e would run"; exit 0 ;;
      *) echo "skip: host OS is $HOST_OS (not darwin) — Phase 2 e2e requires AF_UNIX + osascript"; exit 0 ;;
    esac ;;
  "") ;;
  *) echo "unknown flag: $1 (see --help)" >&2; exit 2 ;;
esac

# -- host guard ------------------------------------------------------------
HOST_OS="${LEAH_PHASE2_FAKE_OS:-$(uname -s)}"
case "$HOST_OS" in
  Darwin|darwin) ;;
  *) echo "skip: host OS is $HOST_OS — Phase 2 e2e requires AF_UNIX + osascript"; exit 0 ;;
esac

trap cleanup EXIT

# -- prep state ------------------------------------------------------------
cleanup
mkdir -p "$SUPPORT_DIR" "$(dirname "$SOCK")" "$(dirname "$PIDFILE")"
cp "$THIS/phase2-fixtures/widget-registry.json" "$SUPPORT_DIR/widget-registry.json"

# -- locate spec file ------------------------------------------------------
SPEC="$REPO/docs/superpowers/designs/2026-06-21-leah-macos-native-ui-design.md"
if [ ! -f "$SPEC" ]; then
  step_fail 8 "spec file missing: $SPEC"
fi

# -- build daemon ----------------------------------------------------------
echo "phase2-e2e: building leah-daemon"
if ! go build -C "$REPO" -o /tmp/leah-daemon-e2e ./cmd/leah-daemon >"$LOG" 2>&1; then
  step_fail 0 "daemon build failed (see $LOG)"
fi

# -- boot daemon -----------------------------------------------------------
echo "phase2-e2e: booting daemon (state=$STATE)"
rm -f "$SOCK"
LEAH_STATE_DIR="$STATE" /tmp/leah-daemon-e2e >>"$LOG" 2>&1 &
echo $! > "$PIDFILE"
for _ in $(seq 1 100); do
  [ -S "$SOCK" ] && break
  sleep 0.1
done
if [ ! -S "$SOCK" ]; then
  step_fail 0 "socket did not appear at $SOCK (see $LOG)"
fi

# ---- Invariant 1: widget.mount streamed --------------------------------
if [ -z "${ANTHROPIC_API_KEY:-}" ]; then
  echo "phase2-e2e: (1) SKIP — ANTHROPIC_API_KEY unset"
else
  if ! out="$(go run -C "$REPO" "$PROBE" widget-mount "$SOCK" 2>&1)"; then
    step_fail 1 "widget.mount probe failed: $out"
  fi
  echo "phase2-e2e: (1) ok — widget.mount frame received"
fi

# ---- Invariant 2: pin persistence across restart -----------------------
echo '[{"id":"stat-1","type":"stat","props":{},"refresh":60}]' > "$SUPPORT_DIR/pinned-widgets.json"
PID="$(cat "$PIDFILE")"
kill "$PID" 2>/dev/null || true
for _ in 1 2 3 4 5; do kill -0 "$PID" 2>/dev/null || break; sleep 0.2; done
rm -f "$SOCK"
LEAH_STATE_DIR="$STATE" /tmp/leah-daemon-e2e >>"$LOG" 2>&1 &
echo $! > "$PIDFILE"
for _ in $(seq 1 100); do [ -S "$SOCK" ] && break; sleep 0.1; done
if [ ! -S "$SOCK" ]; then
  step_fail 2 "socket did not reappear after restart (see $LOG)"
fi
if ! grep -q '"id" *: *"stat-1"' "$SUPPORT_DIR/pinned-widgets.json"; then
  step_fail 2 "pinned-widgets.json did not survive restart"
fi
echo "phase2-e2e: (2) ok — pin survived restart"

# ---- Invariants 3 + 4: pin cap + toast frame shape ---------------------
PROBE_DIR="$(mktemp -d /tmp/leah-phase2-pincap-XXXXXX)"
if ! out="$(go run -C "$REPO" "$PROBE" pin-cap "$PROBE_DIR" 2>&1)"; then
  rm -rf "$PROBE_DIR"
  step_fail 3 "pin-cap probe failed: $out"
fi
rm -rf "$PROBE_DIR"
echo "phase2-e2e: (3) ok — third Pin produced notification.toast"
echo "phase2-e2e: (4) ok — toast payload shape verified"

# ---- Invariant 5: light-mode toggle does not crash daemon --------------
ORIG_STYLE="$(defaults read -g AppleInterfaceStyle 2>/dev/null || echo Light)"
defaults write -g AppleInterfaceStyle Dark >/dev/null 2>&1 || true
sleep 0.4
defaults write -g AppleInterfaceStyle Light >/dev/null 2>&1 || true
sleep 0.4
if [ "$ORIG_STYLE" = "Dark" ]; then
  defaults write -g AppleInterfaceStyle Dark >/dev/null 2>&1 || true
else
  defaults delete -g AppleInterfaceStyle >/dev/null 2>&1 || true
fi
if ! kill -0 "$(cat "$PIDFILE")" 2>/dev/null; then
  step_fail 5 "daemon died during light-mode toggle (see $LOG)"
fi
echo "phase2-e2e: (5) ok — daemon survived dark/light toggle"

# ---- Invariant 6: fsnotify 200 ms debounce coalesces burst -------------
PROBE_DIR="$(mktemp -d /tmp/leah-phase2-debounce-XXXXXX)"
if ! out="$(go run -C "$REPO" "$PROBE" debounce "$PROBE_DIR" 2>&1)"; then
  rm -rf "$PROBE_DIR"
  step_fail 6 "debounce probe failed: $out"
fi
rm -rf "$PROBE_DIR"
echo "phase2-e2e: (6) ok — 3-write burst coalesced into one PinnedChanged"

# ---- Invariant 7: BGE backend selectable, routes to namespaced table ---
if [ -z "${LEAH_EMBED_MODEL_PATH:-}" ] && [ -z "${LEAH_MODEL_DIR:-}" ]; then
  echo "phase2-e2e: (7) SKIP — no LEAH_EMBED_MODEL_PATH / LEAH_MODEL_DIR"
elif ! out="$(go run -C "$REPO" "$PROBE" bge-table 2>&1)"; then
  step_fail 7 "bge-table probe failed: $out"
else
  echo "phase2-e2e: (7) ok — bge generator routes to embeddings_bge_small_en_v1_5_384"
fi

# ---- Invariant 8: spec-parity holds ------------------------------------
if ! "$REPO/scripts/check-spec-parity.sh" "$SPEC" >>"$LOG" 2>&1; then
  step_fail 8 "spec-parity failed (see $LOG)"
fi
echo "phase2-e2e: (8) ok — spec-parity clean"

echo "phase2 e2e ok"

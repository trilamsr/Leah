#!/usr/bin/env bash
# Local self-update primitives per docs/engineer/specs/2026-06-10-local-self-update.md §3-§5.
# Subcommands: install | upgrade | swap-symlink | rollback-symlink
#
# Trust boundary = operator's clone + their `go` toolchain (no remote fetch).
# Env overrides (testing):
#   LEAH_STATE_DIR   default ~/.leah-state
#   LEAH_BIN_DIR     default ~/bin
#   LEAH_UPGRADE_DRY_RUN=1  skip build+symlink mutation+daemon restart
set -euo pipefail

STATE_DIR="${LEAH_STATE_DIR:-$HOME/.leah-state}"
BIN_DIR="${LEAH_BIN_DIR:-$HOME/bin}"
ARTIFACTS_DIR="$STATE_DIR/bin"
LOCK_FILE="$STATE_DIR/daemon.lock"
PID_FILE="$STATE_DIR/daemon.pid"
LOG_FILE="$STATE_DIR/daemon.log"

mkdir -p "$ARTIFACTS_DIR" "$BIN_DIR"

# atomic-symlink: create $target as a symlink → $src via temp+rename (rename is atomic on same FS).
atomic_symlink() {
  local src="$1" target="$2"
  local tmp="${target}.new.$$"
  ln -s "$src" "$tmp"
  mv -f "$tmp" "$target"
}

# Same-FS check (mv across devices is non-atomic). Best-effort warning only.
require_same_fs() {
  local a="$1" b="$2"
  [ -e "$a" ] && [ -e "$b" ] || return 0
  # Portable: compare device IDs via `stat`. macOS BSD stat vs GNU.
  local da db
  if stat -f '%d' "$a" >/dev/null 2>&1; then
    da=$(stat -f '%d' "$a"); db=$(stat -f '%d' "$b")
  else
    da=$(stat -c '%d' "$a"); db=$(stat -c '%d' "$b")
  fi
  [ "$da" = "$db" ] || echo "warn: $a and $b on different filesystems; rename may not be atomic" >&2
}

# Acquire flock on LOCK_FILE for the duration of this shell. Non-blocking;
# spec §4.1 says exit ErrUpgradeInProgress immediately on contention.
LOCK_SENTINEL=""
release_lock() { [ -n "$LOCK_SENTINEL" ] && rmdir "$LOCK_SENTINEL" 2>/dev/null || true; }
trap release_lock EXIT

acquire_lock() {
  mkdir -p "$STATE_DIR"
  : >> "$LOCK_FILE"
  if command -v flock >/dev/null 2>&1; then
    exec 9>"$LOCK_FILE"
    flock -n 9 || { echo "ErrUpgradeInProgress: $LOCK_FILE held"; exit 2; }
  else
    # macOS fallback: shlock-style via mkdir of sentinel (atomic).
    LOCK_SENTINEL="${LOCK_FILE}.d"
    if ! mkdir "$LOCK_SENTINEL" 2>/dev/null; then
      echo "ErrUpgradeInProgress: $LOCK_SENTINEL held"; exit 2
    fi
  fi
}

current_sha() {
  git rev-parse HEAD 2>/dev/null || echo "unknown"
}

build_artifacts() {
  local sha="$1" built_at
  built_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  local ld="-X main.commit=$sha -X main.builtAt=$built_at"
  go build -ldflags "$ld" -o "$ARTIFACTS_DIR/leah-$sha"        ./cmd/leah
  go build -ldflags "$ld" -o "$ARTIFACTS_DIR/leah-daemon-$sha" ./cmd/leah-daemon
}

# Place a tiny placeholder artifact when dry-run / no go toolchain in test sandbox.
synthesize_placeholder() {
  local sha="$1"
  : > "$ARTIFACTS_DIR/leah-$sha"
  : > "$ARTIFACTS_DIR/leah-daemon-$sha"
  chmod +x "$ARTIFACTS_DIR/leah-$sha" "$ARTIFACTS_DIR/leah-daemon-$sha"
}

cmd_install() {
  acquire_lock
  local sha
  sha=$(current_sha)
  if [ "${LEAH_UPGRADE_DRY_RUN:-0}" = "1" ]; then
    synthesize_placeholder "$sha"
  else
    build_artifacts "$sha"
  fi

  # leah-current → leah-$sha (only if absent; install is idempotent)
  if [ ! -L "$ARTIFACTS_DIR/leah-current" ]; then
    atomic_symlink "$ARTIFACTS_DIR/leah-$sha" "$ARTIFACTS_DIR/leah-current"
  fi
  if [ ! -L "$ARTIFACTS_DIR/leah-daemon-current" ]; then
    atomic_symlink "$ARTIFACTS_DIR/leah-daemon-$sha" "$ARTIFACTS_DIR/leah-daemon-current"
  fi

  # ~/bin/leah → leah-current
  if [ ! -L "$BIN_DIR/leah" ]; then
    atomic_symlink "$ARTIFACTS_DIR/leah-current" "$BIN_DIR/leah"
  fi

  echo "installed leah-$sha"
  echo "  $ARTIFACTS_DIR/leah-current → leah-$sha"
  echo "  $BIN_DIR/leah → $ARTIFACTS_DIR/leah-current"
}

cmd_upgrade() {
  acquire_lock
  local sha
  sha=$(current_sha)

  if [ "${LEAH_UPGRADE_DRY_RUN:-0}" = "1" ]; then
    echo "DRY_RUN=1: would build leah-$sha + leah-daemon-$sha, swap symlinks, restart daemon"
    return 0
  fi

  build_artifacts "$sha"

  # spec §4.6–§4.8: rename current → previous, then atomic-symlink new → current
  swap_symlink "$ARTIFACTS_DIR/leah-current"        "$ARTIFACTS_DIR/leah-$sha"        "$ARTIFACTS_DIR/leah-previous"
  swap_symlink "$ARTIFACTS_DIR/leah-daemon-current" "$ARTIFACTS_DIR/leah-daemon-$sha" "$ARTIFACTS_DIR/leah-daemon-previous"

  restart_daemon
  echo "upgraded → leah-$sha"
}

# swap_symlink CURRENT NEW_SRC PREVIOUS
# After: CURRENT → NEW_SRC, PREVIOUS → (old CURRENT target)
swap_symlink() {
  local current="$1" new_src="$2" previous="$3"
  require_same_fs "$current" "$new_src"

  if [ -L "$current" ]; then
    local old_target
    old_target=$(readlink "$current")
    # PREVIOUS gets the OLD target via atomic symlink rewrite.
    atomic_symlink "$old_target" "$previous"
  fi
  atomic_symlink "$new_src" "$current"
}

# rollback_symlink CURRENT PREVIOUS — swap the two via a temp link (two-step rename).
rollback_symlink() {
  local current="$1" previous="$2"
  [ -L "$previous" ] || { echo "no previous to roll back to: $previous"; exit 3; }

  local cur_target prev_target
  cur_target=$(readlink "$current")
  prev_target=$(readlink "$previous")

  atomic_symlink "$prev_target" "$current"
  atomic_symlink "$cur_target"  "$previous"
}

restart_daemon() {
  if [ -f "$PID_FILE" ]; then
    local pid
    pid=$(cat "$PID_FILE" 2>/dev/null || echo "")
    if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
      kill -TERM "$pid" 2>/dev/null || true
      # spec §4.10: poll up to 10s, then SIGKILL
      local i=0
      while [ $i -lt 100 ] && kill -0 "$pid" 2>/dev/null; do
        sleep 0.1
        i=$((i+1))
      done
      if kill -0 "$pid" 2>/dev/null; then
        echo "warn: force-killing daemon pid=$pid"
        kill -KILL "$pid" 2>/dev/null || true
      fi
    fi
  fi
  nohup "$ARTIFACTS_DIR/leah-daemon-current" > "$LOG_FILE" 2>&1 &
  echo $! > "$PID_FILE"
}

case "${1:-help}" in
  install)           shift; cmd_install "$@" ;;
  upgrade)           shift; cmd_upgrade "$@" ;;
  swap-symlink)      shift; swap_symlink "$@" ;;
  rollback-symlink)  shift; rollback_symlink "$@" ;;
  help|*)
    cat <<EOF
usage: $0 <subcommand>
  install                                build + symlink ~/bin/leah → ~/.leah-state/bin/leah-current
  upgrade                                build new SHA, atomic-swap symlinks, restart daemon
  swap-symlink CURRENT NEW_SRC PREVIOUS  (internal) rotate symlink set
  rollback-symlink CURRENT PREVIOUS      (internal) swap current ↔ previous

env:
  LEAH_STATE_DIR        default \$HOME/.leah-state
  LEAH_BIN_DIR          default \$HOME/bin
  LEAH_UPGRADE_DRY_RUN  1 = skip build+swap+restart
EOF
    ;;
esac

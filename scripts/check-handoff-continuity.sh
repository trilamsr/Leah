#!/usr/bin/env bash
# Phase 0 of audit-session caught a session that never opened its prior
# handoff until end-of-session — by then the operator had already eaten
# the redundant work. This gate runs early so the gap surfaces before any
# new work commits. Fail-open whenever we cannot prove the read happened,
# so a missing transcript handle never blocks the session.

set -u

ROOT=$(pwd)

while [ $# -gt 0 ]; do
  case "$1" in
    --root) ROOT="$2"; shift 2 ;;
    *) echo "unknown arg: $1" >&2; exit 2 ;;
  esac
done

HANDOFF_DIR="$ROOT/.claude/session-handoffs"
[ -d "$HANDOFF_DIR" ] || exit 0

# Sort lexically — handoff filenames are ISO-prefixed, so lexical = chronological.
NEWEST=$(ls -1 "$HANDOFF_DIR"/*-session-handoff.md 2>/dev/null | sort | tail -1)
[ -n "$NEWEST" ] || exit 0

TX="${CLAUDE_TRANSCRIPT:-}"
[ -n "$TX" ] || exit 0
# -f filters out directories; -r requires read perm. Both needed — `[ -r dir ]`
# passes on a directory but `grep -F dir` errors and we'd fail closed.
[ -f "$TX" ] && [ -r "$TX" ] || exit 0

BASENAME=$(basename "$NEWEST")
if grep -qF "$BASENAME" "$TX"; then
  exit 0
fi

echo "unread prior handoff: $NEWEST" >&2
exit 1

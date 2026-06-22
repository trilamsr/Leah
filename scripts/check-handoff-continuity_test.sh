#!/usr/bin/env bash
# Phase 0 of audit-session caught an unread handoff at end-of-session; this
# gate catches it BEFORE the session does any work.

set -u

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
GATE="$SCRIPT_DIR/check-handoff-continuity.sh"
PASS=0
FAIL=0

pass() { echo "ok: $1"; PASS=$((PASS + 1)); }
fail() { echo "FAIL: $1"; FAIL=$((FAIL + 1)); }

# Fresh fixture root per case — keeps handoff files isolated from the host.
mkfixture() {
  local dir
  dir=$(mktemp -d)
  mkdir -p "$dir/.claude/session-handoffs"
  echo "$dir"
}

# No prior handoff → exit 0 (first session, nothing to read).
case_no_prior_passes() {
  local dir; dir=$(mkfixture)
  CLAUDE_TRANSCRIPT="" "$GATE" --root "$dir" >/dev/null 2>&1
  local rc=$?
  if [ "$rc" -eq 0 ]; then pass "no prior handoff exits 0"
  else fail "no prior handoff should exit 0 (got $rc)"; fi
  rm -rf "$dir"
}

# Prior handoff exists, transcript path unset → exit 0 (cannot verify, fail open).
case_no_transcript_passes() {
  local dir; dir=$(mkfixture)
  : > "$dir/.claude/session-handoffs/2026-06-21T08-session-handoff.md"
  unset CLAUDE_TRANSCRIPT
  "$GATE" --root "$dir" >/dev/null 2>&1
  local rc=$?
  if [ "$rc" -eq 0 ]; then pass "unset transcript exits 0 (fail-open)"
  else fail "unset transcript should exit 0 (got $rc)"; fi
  rm -rf "$dir"
}

# Prior handoff exists AND transcript references its filename → exit 0.
case_handoff_read_passes() {
  local dir; dir=$(mkfixture)
  local tx
  : > "$dir/.claude/session-handoffs/2026-06-21T08-session-handoff.md"
  tx=$(mktemp)
  printf 'opened .claude/session-handoffs/2026-06-21T08-session-handoff.md\n' > "$tx"
  CLAUDE_TRANSCRIPT="$tx" "$GATE" --root "$dir" >/dev/null 2>&1
  local rc=$?
  if [ "$rc" -eq 0 ]; then pass "handoff referenced in transcript exits 0"
  else fail "read handoff should exit 0 (got $rc)"; fi
  rm -f "$tx"; rm -rf "$dir"
}

# Prior handoff exists, transcript does NOT reference it → exit 1.
# The exact regression the audit-session Phase 0 surfaced.
case_handoff_unread_fails() {
  local dir; dir=$(mkfixture)
  local tx
  : > "$dir/.claude/session-handoffs/2026-06-21T08-session-handoff.md"
  tx=$(mktemp)
  printf 'session did unrelated work\n' > "$tx"
  CLAUDE_TRANSCRIPT="$tx" "$GATE" --root "$dir" >/dev/null 2>&1
  local rc=$?
  if [ "$rc" -eq 1 ]; then pass "unread handoff exits 1"
  else fail "unread handoff should exit 1 (got $rc)"; fi
  rm -f "$tx"; rm -rf "$dir"
}

# Multiple handoffs → only the newest one must be referenced. An older
# handoff being read is irrelevant if the latest one wasn't.
case_only_newest_checked() {
  local dir; dir=$(mkfixture)
  local tx
  : > "$dir/.claude/session-handoffs/2026-06-19T09-session-handoff.md"
  sleep 1
  : > "$dir/.claude/session-handoffs/2026-06-21T08-session-handoff.md"
  tx=$(mktemp)
  printf 'opened 2026-06-19T09-session-handoff.md\n' > "$tx"
  CLAUDE_TRANSCRIPT="$tx" "$GATE" --root "$dir" >/dev/null 2>&1
  local rc=$?
  if [ "$rc" -eq 1 ]; then pass "only newest handoff is gated"
  else fail "stale older read should not satisfy gate (got $rc)"; fi
  rm -f "$tx"; rm -rf "$dir"
}

# Unreadable transcript (path set but file gone) → exit 0 (fail-open;
# we cannot prove unread, so we don't block).
case_unreadable_transcript_passes() {
  local dir; dir=$(mkfixture)
  : > "$dir/.claude/session-handoffs/2026-06-21T08-session-handoff.md"
  CLAUDE_TRANSCRIPT="/nonexistent/path/to/transcript.log" "$GATE" --root "$dir" >/dev/null 2>&1
  local rc=$?
  if [ "$rc" -eq 0 ]; then pass "unreadable transcript exits 0 (fail-open)"
  else fail "unreadable transcript should exit 0 (got $rc)"; fi
  rm -rf "$dir"
}

case_no_prior_passes
case_no_transcript_passes
case_handoff_read_passes
case_handoff_unread_fails
case_only_newest_checked
case_unreadable_transcript_passes

echo "---"
echo "passed: $PASS  failed: $FAIL"
[ "$FAIL" -gt 0 ] && exit 1 || exit 0

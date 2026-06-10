#!/usr/bin/env bash
# check-tdd-evidence.sh — fail when a feat/* PR body lacks pre-impl
# failing-test capture. CLAUDE.md §"TDD + review" mandates RED-first
# capture; pre-2026-06-10 enforcement was operator-discretion. Audit
# session surfaced misses on PRs #162 / #199 / #219 — load-bearing
# feat/* landed without the failing output that the rule requires.
# This gate makes the rule machine-enforced.
#
# Inputs:
#   --branch <name>     branch name (CI passes refs/heads/foo or feat/x).
#                       Default: current branch via `git rev-parse`.
#   --pr <number>       fetch body via `gh pr view`.
#   --body-file <path>  read body from a local file (CI + fixtures).
#   --skip <reason>     emergency operator escape. Reason ≥32 chars is
#                       REQUIRED; an audit row is emitted to stderr AND
#                       appended to ~/.leah-state/audit.jsonl with kind
#                       `ci_gate_skipped`. Bare `--skip` without a reason
#                       is a usage error — the audit trail cannot be
#                       allowed to silently degrade.
#
# Operator escape (rare): include
# `<!-- tdd-skip-justified: <reason ≥32 chars> -->` in the PR body
# for true-no-test cases (dependency-bump, pure rename, docs-only that
# slipped under a feat/ prefix). Audit-row mandatory; reason <32 chars
# rejected — 4-char floor pre-2026-06-10 let `noop`/`wip!`/`skip` pass.
#
# Pass: branch is NOT feat/* OR body contains one of:
#   - `## TDD evidence` heading (case-insensitive) PAIRED with a FAIL /
#     panic / RED-token within the same section (heading alone is not
#     evidence — `## TDD` with no failing-output beneath used to slip).
#   - `RED→GREEN` or `red-first` or `test-first` token.
#   - `<!-- tdd-skip-justified: <reason ≥32 chars> -->` marker.
#
# Exit codes:
#   0 — pass / non-feat branch / skip-justified.
#   1 — feat/* body lacks TDD evidence.
#   2 — usage error.

set -uo pipefail

BRANCH=""
PR_NUM=""
BODY_FILE=""
SKIP=0
SKIP_REASON=""

while [ $# -gt 0 ]; do
  case "$1" in
    --branch) BRANCH="$2"; shift 2 ;;
    --pr) PR_NUM="$2"; shift 2 ;;
    --body-file) BODY_FILE="$2"; shift 2 ;;
    --skip)
      SKIP=1
      # --skip MAY take a positional reason; if absent or starts with
      # `--` the gate refuses (no silent bypass).
      if [ $# -ge 2 ] && [ "${2#--}" = "$2" ]; then
        SKIP_REASON="$2"; shift 2
      else
        shift
      fi
      ;;
    -h|--help) grep '^#' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "check-tdd-evidence: unknown flag: $1" >&2; exit 2 ;;
  esac
done

# Audit-row writer. Stderr always; jsonl best-effort (state dir may not
# exist in CI — never block the gate on logging IO).
emit_audit() {
  local reason="$1" branch="$2" pr="$3"
  local ts
  ts=$(date -u +%FT%TZ)
  echo "AUDIT: check-tdd-evidence skipped at $ts reason='$reason' branch='$branch' pr='$pr'" >&2
  local dir="${LEAH_STATE_DIR:-$HOME/.leah-state}"
  mkdir -p "$dir" 2>/dev/null || return 0
  printf '{"ts":"%s","kind":"ci_gate_skipped","gate":"check-tdd-evidence","reason":"%s","branch":"%s","pr":"%s"}\n' \
    "$ts" "${reason//\"/\\\"}" "${branch//\"/\\\"}" "${pr//\"/\\\"}" \
    >> "$dir/audit.jsonl" 2>/dev/null || true
}

if [ "$SKIP" -eq 1 ]; then
  if [ "${#SKIP_REASON}" -lt 32 ]; then
    echo "check-tdd-evidence: --skip requires a justification ≥32 chars (got ${#SKIP_REASON})" >&2
    exit 2
  fi
  emit_audit "$SKIP_REASON" "$BRANCH" "$PR_NUM"
  exit 0
fi

if [ -z "$BRANCH" ]; then
  BRANCH=$(git rev-parse --abbrev-ref HEAD 2>/dev/null || true)
fi
BRANCH="${BRANCH#refs/heads/}"

case "$BRANCH" in
  feat/*) ;;
  *) exit 0 ;;
esac

# Body source resolution.
TMPBODY=""
cleanup() { [ -n "$TMPBODY" ] && rm -f "$TMPBODY"; }
trap cleanup EXIT

if [ -n "$PR_NUM" ] && [ -n "$BODY_FILE" ]; then
  echo "check-tdd-evidence: --pr and --body-file are mutually exclusive" >&2
  exit 2
fi

if [ -n "$PR_NUM" ]; then
  TMPBODY=$(mktemp)
  gh pr view "$PR_NUM" --json body --jq '.body' > "$TMPBODY" 2>/dev/null || {
    echo "check-tdd-evidence: gh pr view $PR_NUM failed" >&2
    exit 2
  }
  BODY_FILE="$TMPBODY"
fi

if [ -z "$BODY_FILE" ] || [ ! -f "$BODY_FILE" ]; then
  echo "check-tdd-evidence: no body source (use --pr or --body-file)" >&2
  exit 2
fi

# Skip marker honored only when reason ≥32 chars after the colon. The
# 4-char floor in v1 let `noop`/`wip!`/`skip` pass — meaningless reasons
# eroded the audit trail. 32 chars forces a real sentence.
skip_line=$(grep -iE '<!--[[:space:]]*tdd-skip-justified:[[:space:]]*[^[:space:]]' "$BODY_FILE" | head -1 || true)
if [ -n "$skip_line" ]; then
  reason=$(printf '%s\n' "$skip_line" | sed -E 's/.*tdd-skip-justified:[[:space:]]*//; s/[[:space:]]*-->.*//')
  if [ "${#reason}" -ge 32 ]; then
    emit_audit "$reason" "$BRANCH" "$PR_NUM"
    exit 0
  fi
fi

# Token detection: tightened from v1 — bare `## TDD` (no `evidence`) used
# to satisfy the gate, and the heading alone with no failing-output
# beneath was a known leak. Now require either:
#   (a) `## TDD evidence` heading AND a FAIL/panic/RED token in body, OR
#   (b) an explicit `RED→GREEN` / `red-first` / `test-first` token.
heading_ok=0
if grep -qiE '^[[:space:]]*##[[:space:]]+TDD[[:space:]]+evidence\b' "$BODY_FILE"; then
  heading_ok=1
fi
fail_token_ok=0
if grep -qiE '(--- FAIL|^FAIL\b|^panic:|^[[:space:]]*panic\(|RED *(→|->|=>) *GREEN|red[-. ]first|test[-. ]first)' "$BODY_FILE"; then
  fail_token_ok=1
fi
if [ "$heading_ok" -eq 1 ] && [ "$fail_token_ok" -eq 1 ]; then
  exit 0
fi
if [ "$fail_token_ok" -eq 1 ] && grep -qiE '(red *(→|->|=>) *green|red[-. ]first|test[-. ]first)' "$BODY_FILE"; then
  exit 0
fi

echo "ERROR: PR on feat/* branch '$BRANCH' missing TDD-evidence in body" >&2
echo "  Add a '## TDD evidence' section with pre-impl failing-test output," >&2
echo "  OR add <!-- tdd-skip-justified: <reason ≥4 chars> --> marker." >&2
exit 1

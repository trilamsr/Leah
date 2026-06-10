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
#   --skip              emergency operator escape (audit-row logged).
#
# Operator escape (rare): include
# `<!-- tdd-skip-justified: <reason ≥4 chars> -->` in the PR body
# for true-no-test cases (dependency-bump, pure rename, docs-only that
# slipped under a feat/ prefix). Audit-row mandatory; reason <4 chars
# rejected.
#
# Pass: branch is NOT feat/* OR body contains one of:
#   - `## TDD evidence` heading (case-insensitive)
#   - phrase `failing test`
#   - `RED→GREEN` or `red-first` or `test-first` token
#   - `<!-- tdd-skip-justified: <reason ≥4 chars> -->` marker
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

while [ $# -gt 0 ]; do
  case "$1" in
    --branch) BRANCH="$2"; shift 2 ;;
    --pr) PR_NUM="$2"; shift 2 ;;
    --body-file) BODY_FILE="$2"; shift 2 ;;
    --skip) SKIP=1; shift ;;
    -h|--help) grep '^#' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "check-tdd-evidence: unknown flag: $1" >&2; exit 2 ;;
  esac
done

if [ "$SKIP" -eq 1 ]; then exit 0; fi

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

# Skip marker honored only when reason ≥4 chars after the colon. Strict
# match keeps the audit trail meaningful — empty / 1-char reasons would
# erode the rule by attrition.
skip_line=$(grep -iE '<!--[[:space:]]*tdd-skip-justified:[[:space:]]*[^[:space:]]' "$BODY_FILE" | head -1 || true)
if [ -n "$skip_line" ]; then
  reason=$(printf '%s\n' "$skip_line" | sed -E 's/.*tdd-skip-justified:[[:space:]]*//; s/[[:space:]]*-->.*//')
  if [ "${#reason}" -ge 4 ]; then
    exit 0
  fi
fi

# Token detection: any of these phrases satisfies the gate.
if grep -qiE '(^|[^[:alnum:]])(## *TDD( evidence)?|failing test|RED *(→|->|=>) *GREEN|red[-. ]first|test[-. ]first)' "$BODY_FILE"; then
  exit 0
fi

echo "ERROR: PR on feat/* branch '$BRANCH' missing TDD-evidence in body" >&2
echo "  Add a '## TDD evidence' section with pre-impl failing-test output," >&2
echo "  OR add <!-- tdd-skip-justified: <reason ≥4 chars> --> marker." >&2
exit 1

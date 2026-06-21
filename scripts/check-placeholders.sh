#!/usr/bin/env bash
# check-placeholders.sh — fail when a feat/* branch introduces NEW
# placeholder markers (TODO/FIXME/XXX, "placeholder", panic("not
# implemented")) in prod *.go code.
#
# Rationale (MAY-V6): feat/* PRs were landing ship-ready stubs. Diff-
# scoped because the host repo carries ~80 legitimate baseline markers
# (sql `?` placeholders, W11→W12 stub trail) — a whole-tree scan would
# never go green.
#
# Inputs:
#   --base <ref>        base ref (default origin/main); diff cone base...HEAD
#   --branch <name>     branch under test (default current branch)
#   --repo <path>       repo root (default cwd) — test-only knob
#   --body-file <path>  PR body for allowlist marker (optional)
#   --pr <number>       fetch body via gh (optional)
#
# Escape: `<!-- placeholder-justified: <reason ≥32 chars> -->` in PR body.
#
# Exit:
#   0 pass (clean, non-feat branch, or allowlisted)
#   1 fail (new placeholder on feat/*)

set -uo pipefail

base="origin/main"
branch=""
repo=""
body_file=""
pr_number=""

while [ $# -gt 0 ]; do
  case "$1" in
    --base)      base="${2:-}"; shift 2 ;;
    --branch)    branch="${2:-}"; shift 2 ;;
    --repo)      repo="${2:-}"; shift 2 ;;
    --body-file) body_file="${2:-}"; shift 2 ;;
    --pr)        pr_number="${2:-}"; shift 2 ;;
    -h|--help)
      grep '^#' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *)
      echo "check-placeholders: unknown arg: $1" >&2
      exit 64
      ;;
  esac
done

if [ -n "$repo" ]; then
  cd "$repo" || { echo "check-placeholders: bad --repo $repo" >&2; exit 64; }
fi

if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  echo "check-placeholders: not a git tree (skip)"
  exit 0
fi

if [ -z "$branch" ]; then
  branch=$(git rev-parse --abbrev-ref HEAD 2>/dev/null || true)
fi
branch="${branch#refs/heads/}"

# Non-feat branches ship under their own gates; placeholder hygiene
# belongs to the feat/ flow per CLAUDE.md "TDD + review".
case "$branch" in
  feat/*) ;;
  *) echo "check-placeholders: skip (branch '$branch' not feat/*)"; exit 0 ;;
esac

if ! git rev-parse --verify "$base" >/dev/null 2>&1; then
  echo "check-placeholders: base ref not found: $base (skip)"
  exit 0
fi

body=""
if [ -n "$body_file" ] && [ -f "$body_file" ]; then
  body=$(cat "$body_file")
elif [ -n "$pr_number" ] && command -v gh >/dev/null 2>&1; then
  body=$(gh pr view "$pr_number" --json body --jq .body 2>/dev/null || true)
fi

# Escape marker requires ≥32-char reason — matches tdd-evidence audit-trail floor.
esc=$(printf '%s' "$body" | grep -iE '<!--[[:space:]]*placeholder-justified:[[:space:]]*[^[:space:]]' | head -1 || true)
if [ -n "$esc" ]; then
  reason=$(printf '%s\n' "$esc" | sed -E 's/.*placeholder-justified:[[:space:]]*//; s/[[:space:]]*-->.*//')
  if [ "${#reason}" -ge 32 ]; then
    echo "check-placeholders: allowlisted via PR body marker (justified)"
    exit 0
  fi
fi

# Diff cone: prod *.go added/modified vs base. _test.go waived — tests
# legitimately scaffold with TODOs naming the next case.
changed=$(git diff --name-only --diff-filter=AM "$base"...HEAD -- '*.go' 2>/dev/null || true)
prod=$(printf '%s\n' "$changed" | grep -E '\.go$' | grep -vE '_test\.go$' || true)

if [ -z "$prod" ]; then
  echo "check-placeholders: ok (no prod .go changes vs $base)"
  exit 0
fi

# Marker set, ERE-anchored. `XXX` requires word boundary to skip
# legit hex-literal contexts like `0xXXX` in comments.
# `placeholder` matches lower / Title / all-caps SHOUT forms.
pat='(\bTODO\b|\bFIXME\b|\bXXX\b|placeholder|Placeholder|PLACEHOLDER|panic\("not implemented"\))'

fail=0
hits=""

# Only NEW lines (leading `+`, excluding the `+++` file header) on the
# diff cone — exactly the placeholders this branch introduced.
while IFS= read -r f; do
  [ -z "$f" ] && continue
  added=$(git diff "$base"...HEAD -- "$f" 2>/dev/null \
    | grep -E '^\+[^+]' \
    | grep -nE "$pat" || true)
  if [ -n "$added" ]; then
    fail=$((fail + 1))
    hits+=$(printf '%s\n' "$added" | sed "s|^|$f: |")$'\n'
  fi
done <<<"$prod"

if [ "$fail" -gt 0 ]; then
  echo "check-placeholders: $fail file(s) introduce new placeholder marker(s):"
  printf '%s' "$hits" | head -30
  echo "  Finish the stub, OR add <!-- placeholder-justified: <reason ≥32 chars> --> to PR body."
  exit 1
fi

echo "check-placeholders: ok ($(printf '%s\n' "$prod" | wc -l | tr -d ' ') prod file(s) scanned)"
exit 0

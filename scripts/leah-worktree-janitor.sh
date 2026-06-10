#!/usr/bin/env bash
# Sweep .claude/worktrees/agent-* worktrees whose branch is merged into
# origin/main OR has been deleted upstream. Wave-9 V4 (354MB/129 locks).
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

if ! git fetch origin --quiet; then
  echo "fetch failed; offline or origin unreachable"
  exit 0
fi

# Cache remote heads + merged refs once per sweep — turns N×network into 2 calls.
REMOTE_REFS=$(mktemp)
MERGED_REFS=$(mktemp)
trap 'rm -f "$REMOTE_REFS" "$MERGED_REFS"' EXIT
git ls-remote --heads origin | awk '{print $2}' > "$REMOTE_REFS"
git branch -r --merged origin/main 2>/dev/null | awk '{print $1}' > "$MERGED_REFS"

git worktree list --porcelain |
  awk '/^worktree/ {p=$2} /^branch / {print p"\t"$2}' |
  while IFS=$'\t' read -r path branch; do
    case "$path" in
      */.claude/worktrees/agent-*) ;;
      *) continue;;
    esac
    short="${branch#refs/heads/}"
    if grep -qx "origin/$short" "$MERGED_REFS" \
       || ! grep -qx "refs/heads/$short" "$REMOTE_REFS"; then
      echo "pruning $path (branch $short merged or deleted)"
      if ! git worktree remove --force "$path" 2>/dev/null; then
        echo "  skipped (locked or busy): $path"
        continue
      fi
      git branch -D "$short" 2>/dev/null || true
    fi
  done

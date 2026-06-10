#!/usr/bin/env bash
# Sweep .claude/worktrees/agent-* worktrees whose branch is merged into
# origin/main OR has been deleted upstream. Wave-9 V4 (354MB/129 locks).
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"
git fetch origin --quiet

git worktree list --porcelain |
  awk '/^worktree/ {p=$2} /^branch / {print p"\t"$2}' |
  while IFS=$'\t' read -r path branch; do
    case "$path" in
      */.claude/worktrees/agent-*) ;;
      *) continue;;
    esac
    short="${branch#refs/heads/}"
    if git branch -r --merged origin/main 2>/dev/null | grep -q " origin/$short\$" \
       || ! git ls-remote --exit-code --heads origin "$short" >/dev/null 2>&1; then
      echo "pruning $path (branch $short merged or deleted)"
      git worktree remove --force "$path" 2>/dev/null || true
      git branch -D "$short" 2>/dev/null || true
    fi
  done

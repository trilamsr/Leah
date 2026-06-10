#!/usr/bin/env bash
# Reject pushes from branches that are significantly behind origin/main OR
# built off another open PR's branch — both produce stale-base diff regressions
# (PR #151 class; CLAUDE.md:14).
set -euo pipefail

git fetch origin main --quiet

current=$(git rev-parse --abbrev-ref HEAD)
[[ "$current" == "main" ]] && exit 0

# 1. Branch too far behind main to push without rebase.
behind=$(git rev-list --count HEAD..origin/main 2>/dev/null || echo 0)
if [[ "$behind" -gt 10 ]]; then
  echo "ERROR: branch is $behind commits behind origin/main; rebase before push" >&2
  exit 1
fi

# 2. Branch built off another open PR's branch (not main). If any other remote
# tracking ref under origin/* is an ancestor of HEAD but NOT of origin/main,
# this branch was built on top of that ref rather than off main.
current_remote="origin/$current"
while IFS= read -r ref; do
  [[ -z "$ref" ]] && continue
  [[ "$ref" == "origin/main" || "$ref" == "origin/HEAD" || "$ref" == "$current_remote" ]] && continue
  if git merge-base --is-ancestor "$ref" HEAD 2>/dev/null \
     && ! git merge-base --is-ancestor "$ref" origin/main 2>/dev/null; then
    echo "ERROR: branch is built on top of $ref instead of origin/main" >&2
    echo "Likely cause: branched off another open PR's branch (stale-base regression risk)" >&2
    echo "Run: git rebase --onto origin/main $ref" >&2
    exit 1
  fi
done < <(git for-each-ref --format='%(refname:short)' refs/remotes/origin)

# 3. File-overlap: main added paths since the merge-base that this PR also
# touches → rebasing would resurface as a delete-vs-add conflict (PR #151).
if [[ "$behind" -gt 0 ]]; then
  merge_base=$(git merge-base HEAD origin/main)
  pr_paths=$(git diff --name-only "$merge_base" HEAD | sort -u)
  main_added=$(git diff --name-only --diff-filter=A "$merge_base" origin/main | sort -u)
  overlap=$(comm -12 <(printf '%s\n' "$pr_paths") <(printf '%s\n' "$main_added") | sed '/^$/d')
  if [[ -n "$overlap" ]]; then
    echo "ERROR: main added files since merge-base that this PR also touches:" >&2
    echo "$overlap" >&2
    echo "rebase to avoid stale-base regression" >&2
    exit 1
  fi
fi

exit 0

#!/usr/bin/env bash
# TDD shell test for scripts/leah-worktree-janitor.sh.
#
# Stands up a hermetic origin↔clone pair, exercises the three cases the
# janitor must distinguish (merged agent worktree → prune; unmerged active
# agent worktree → keep; non-agent worktree → keep), then asserts the
# resulting worktree topology.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
JANITOR="$REPO_ROOT/scripts/leah-worktree-janitor.sh"

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

# Fake origin: bare repo + clone with one main commit.
git init --bare --quiet "$TMP/origin.git"
git clone --quiet "$TMP/origin.git" "$TMP/work"
cd "$TMP/work"
git config user.email janitor@test
git config user.name janitor
git checkout -q -b main
echo seed > seed.txt
git add seed.txt && git commit -q -m seed
git push -q origin main

# Case A: merged agent branch. Create branch, merge into main, push both.
git checkout -q -b feat/agent-a-merged
echo a > a.txt && git add a.txt && git commit -q -m a
git checkout -q main
git merge -q --no-ff feat/agent-a-merged -m "merge a"
git push -q origin main
git push -q origin feat/agent-a-merged
mkdir -p .claude/worktrees
git worktree add -q .claude/worktrees/agent-aaa feat/agent-a-merged

# Case B: active agent branch (pushed but NOT merged).
git checkout -q -b feat/agent-b-active main
echo b > b.txt && git add b.txt && git commit -q -m b
git push -q -u origin feat/agent-b-active
git checkout -q main
git worktree add -q .claude/worktrees/agent-bbb feat/agent-b-active

# Case C: non-agent worktree on an unmerged branch — must be spared
# regardless of merge status because the path does not match agent-*.
git checkout -q -b feat/non-agent main
echo c > c.txt && git add c.txt && git commit -q -m c
git checkout -q main
git worktree add -q .claude/worktrees/scratch feat/non-agent

# Sanity: 4 worktrees (primary + 3).
before=$(git worktree list | wc -l | tr -d ' ')
[ "$before" = "4" ] || { echo "FAIL: expected 4 worktrees, got $before"; exit 1; }

# Janitor reads its own repo root via `git rev-parse`; invoke from the
# fake-clone cwd so it operates on the test fixture, not the host repo.
bash "$JANITOR" >"$TMP/janitor.out" 2>&1 || {
  cat "$TMP/janitor.out"
  echo "FAIL: janitor exited non-zero"
  exit 1
}

# Assert: agent-aaa pruned, agent-bbb kept, scratch kept.
git worktree list --porcelain | awk '/^worktree/ {print $2}' > "$TMP/paths"
if grep -q "agent-aaa" "$TMP/paths"; then
  cat "$TMP/janitor.out"
  echo "FAIL: merged agent-aaa was not pruned"
  exit 1
fi
if ! grep -q "agent-bbb" "$TMP/paths"; then
  cat "$TMP/janitor.out"
  echo "FAIL: active agent-bbb was wrongly pruned"
  exit 1
fi
if ! grep -q "scratch" "$TMP/paths"; then
  cat "$TMP/janitor.out"
  echo "FAIL: non-agent scratch worktree was wrongly pruned"
  exit 1
fi

echo "PASS: janitor pruned merged agent worktree, spared active + non-agent"

# Plist template substitution: __LEAH_ROOT__ + __LEAH_STATE__ tokens must
# expand to caller-supplied paths so non-treedesk dev hosts install cleanly.
PLIST="$REPO_ROOT/scripts/leah-worktree-janitor.plist"
grep -q "__LEAH_ROOT__" "$PLIST" || { echo "FAIL: plist missing __LEAH_ROOT__ token"; exit 1; }
grep -q "__LEAH_STATE__" "$PLIST" || { echo "FAIL: plist missing __LEAH_STATE__ token"; exit 1; }

RENDERED="$TMP/rendered.plist"
sed -e "s|__LEAH_ROOT__|/fake/repo|g" -e "s|__LEAH_STATE__|/fake/state|g" "$PLIST" > "$RENDERED"
grep -q "__LEAH_ROOT__" "$RENDERED" && { echo "FAIL: __LEAH_ROOT__ not substituted"; exit 1; }
grep -q "__LEAH_STATE__" "$RENDERED" && { echo "FAIL: __LEAH_STATE__ not substituted"; exit 1; }
grep -q "/fake/repo" "$RENDERED" || { echo "FAIL: repo path missing from rendered plist"; exit 1; }
grep -q "/fake/state/janitor.log" "$RENDERED" || { echo "FAIL: state log path missing from rendered plist"; exit 1; }

echo "PASS: plist tokens substitute cleanly"

#!/usr/bin/env bash
# leah-worktree-janitor_test.sh — assert the sweep prunes agent-* worktrees
# whose branch is merged into origin/main OR has been deleted upstream, AND
# preserves agent-* worktrees whose branch is still live + non-merged.
#
# Rationale (MAY-16): the janitor script gets exactly one shot per launchd
# tick — a regression that prunes a live branch silently destroys an
# in-flight agent's work, and a regression that fails to prune lets the
# 354MB/129-lock backlog from Wave-9 V4 reaccumulate.
#
# Each case spins a throwaway "origin" + clone with .claude/worktrees/agent-*
# fixtures so the host repo's worktrees stay untouched.

set -u

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
JANITOR="$SCRIPT_DIR/leah-worktree-janitor.sh"
PASS=0
FAIL=0

pass() { echo "ok: $1"; PASS=$((PASS + 1)); }
fail() { echo "FAIL: $1"; FAIL=$((FAIL + 1)); }

# A bare repo stands in for `origin` so we control which branches the
# clone sees as remote-present vs remote-deleted.
mkrepo() {
  local root origin clone
  root=$(mktemp -d -t leah-janitor-test-XXXX)
  origin="$root/origin.git"
  clone="$root/clone"

  git init -q --bare -b main "$origin"

  git init -q -b main "$clone"
  (
    cd "$clone"
    git config user.email t@t
    git config user.name t
    git remote add origin "$origin"
    printf 'seed\n' > seed.txt
    git add seed.txt
    git commit -q -m seed
    git push -q origin main
  )

  echo "$root"
}

# Create a worktree at $clone/.claude/worktrees/agent-<name> on branch
# feat/<name>. Branch is local-only unless $push is set.
mkagent() {
  local clone="$1" name="$2" push="${3:-}"
  (
    cd "$clone"
    git worktree add -q -b "feat/$name" ".claude/worktrees/agent-$name" main
    cd ".claude/worktrees/agent-$name"
    printf '%s\n' "$name" > "$name.txt"
    git add "$name.txt"
    git commit -q -m "$name"
    if [ -n "$push" ]; then
      git push -q origin "feat/$name"
    fi
  )
}

# Merge feat/<name> into origin/main so the janitor sees it as
# `branch -r --merged origin/main`. The merge happens inside the agent
# worktree itself because feat/<name> is currently checked out there —
# the parent clone can't co-occupy the same branch.
merge_to_origin() {
  local clone="$1" name="$2"
  (
    cd "$clone/.claude/worktrees/agent-$name"
    git fetch -q origin
    git merge -q --no-ff "origin/main" -m "merge main" 2>/dev/null || true
    # Fast-forward main to include feat/<name>'s tip, then push.
    cd "$clone"
    git fetch -q origin
    git push -q origin "feat/$name:main" --force
  )
}

# ---- T1: merged branch → pruned ----
case_merged_pruned() {
  local root; root=$(mkrepo)
  local clone="$root/clone"
  mkagent "$clone" merged push
  merge_to_origin "$clone" merged

  (cd "$clone" && bash "$JANITOR") >"$root/run.log" 2>&1 || true

  if [ -d "$clone/.claude/worktrees/agent-merged" ]; then
    cat "$root/run.log"
    fail "merged worktree should be pruned"
  else
    pass "merged worktree pruned"
  fi
  rm -rf "$root"
}

# ---- T2: upstream-deleted branch → pruned ----
case_deleted_pruned() {
  local root; root=$(mkrepo)
  local clone="$root/clone"
  mkagent "$clone" deleted push
  # Drop the branch from origin → ls-remote shows nothing.
  (cd "$clone" && git push -q origin --delete feat/deleted)

  (cd "$clone" && bash "$JANITOR") >"$root/run.log" 2>&1 || true

  if [ -d "$clone/.claude/worktrees/agent-deleted" ]; then
    cat "$root/run.log"
    fail "upstream-deleted worktree should be pruned"
  else
    pass "upstream-deleted worktree pruned"
  fi
  rm -rf "$root"
}

# ---- T3: live + non-merged branch → preserved ----
# This is the regression guard. A bug that prunes too aggressively
# destroys in-flight agent work.
case_live_preserved() {
  local root; root=$(mkrepo)
  local clone="$root/clone"
  mkagent "$clone" live push

  (cd "$clone" && bash "$JANITOR") >"$root/run.log" 2>&1 || true

  if [ -d "$clone/.claude/worktrees/agent-live" ]; then
    pass "live worktree preserved"
  else
    cat "$root/run.log"
    fail "live worktree should be preserved"
  fi
  rm -rf "$root"
}

# ---- T4: non-agent worktree → never touched ----
# The case-glob limits the sweep to .claude/worktrees/agent-*. A regression
# that drops the prefix check would nuke unrelated worktrees.
case_non_agent_ignored() {
  local root; root=$(mkrepo)
  local clone="$root/clone"
  (
    cd "$clone"
    git worktree add -q -b feat/unrelated "$root/unrelated" main
    cd "$root/unrelated"
    printf 'unrelated\n' > unrelated.txt
    git add unrelated.txt
    git commit -q -m unrelated
    git push -q origin feat/unrelated
  )
  # Merge so the branch WOULD be pruned if the prefix check were broken.
  (cd "$clone" && git push -q origin feat/unrelated:main --force)

  (cd "$clone" && bash "$JANITOR") >"$root/run.log" 2>&1 || true

  if [ -d "$root/unrelated" ]; then
    pass "non-agent worktree ignored"
  else
    cat "$root/run.log"
    fail "non-agent worktree should be ignored"
  fi
  rm -rf "$root"
}

# ---- T5: offline (fetch fails) → exit 0, no prune ----
# launchd reruns every 5min — a non-zero exit floods the log with
# spurious failures when the laptop is on a coffee-shop wifi captive
# portal. Script swallows fetch failure and exits clean.
case_offline_no_prune() {
  local root; root=$(mkrepo)
  local clone="$root/clone"
  mkagent "$clone" offline push
  merge_to_origin "$clone" offline
  # Break the origin URL → fetch fails.
  (cd "$clone" && git remote set-url origin "$root/does-not-exist.git")

  local rc=0
  (cd "$clone" && bash "$JANITOR") >"$root/run.log" 2>&1 || rc=$?

  if [ "$rc" -ne 0 ]; then
    cat "$root/run.log"
    fail "offline run should exit 0 (got $rc)"
  else
    pass "offline run exits 0"
  fi
  if [ -d "$clone/.claude/worktrees/agent-offline" ]; then
    pass "offline run prunes nothing"
  else
    fail "offline run must not prune (network is unreliable signal)"
  fi
  rm -rf "$root"
}

# ---- T6: squash-merged branch (tip not in origin/main ancestry, branch
# still on remote) → pruned via `gh pr list --state merged --head`.
# GitHub's squash-merge produces a new commit on main whose ancestry does
# NOT include the PR-branch tip; if "Automatically delete head branches"
# is off, the remote ref also lingers. Both pre-existing predicates
# (`branch -r --merged` + `ls-remote` absence) miss this — the gh PR
# query is the only signal that the work actually shipped.
case_squash_merged_pruned() {
  local root; root=$(mkrepo)
  local clone="$root/clone"
  mkagent "$clone" squashed push

  # Stub `gh`: report a merged PR for feat/squashed, nothing for others.
  # The script must call gh with `--head <short-branch>`; we key on that.
  local bin="$root/bin"
  mkdir -p "$bin"
  cat > "$bin/gh" <<'STUB'
#!/usr/bin/env bash
# Match: gh pr list --state merged --head <branch> --json ... --limit 1
head=""
for ((i=1; i<=$#; i++)); do
  if [ "${!i}" = "--head" ]; then
    j=$((i+1))
    head="${!j}"
  fi
done
if [ "$head" = "feat/squashed" ]; then
  echo '[{"number":999}]'
else
  echo '[]'
fi
STUB
  chmod +x "$bin/gh"

  local rc=0
  (cd "$clone" && PATH="$bin:$PATH" bash "$JANITOR") >"$root/run.log" 2>&1 || rc=$?

  if [ -d "$clone/.claude/worktrees/agent-squashed" ]; then
    cat "$root/run.log"
    fail "squash-merged worktree should be pruned (gh-pr-merged signal)"
  else
    pass "squash-merged worktree pruned via gh pr list"
  fi
  rm -rf "$root"
}

# ---- T7: gh binary absent → live worktree still preserved (no crash).
# The launchd sweep must survive on hosts without gh installed; falling
# through to the original two predicates is the correct behavior.
case_no_gh_live_preserved() {
  local root; root=$(mkrepo)
  local clone="$root/clone"
  mkagent "$clone" nogh push

  local rc=0
  (cd "$clone" && PATH="/usr/bin:/bin" bash "$JANITOR") >"$root/run.log" 2>&1 || rc=$?

  if [ "$rc" -ne 0 ]; then
    cat "$root/run.log"
    fail "no-gh run should exit 0 (got $rc)"
  elif [ -d "$clone/.claude/worktrees/agent-nogh" ]; then
    pass "no-gh live worktree preserved"
  else
    cat "$root/run.log"
    fail "no-gh run must not prune a live branch"
  fi
  rm -rf "$root"
}

case_merged_pruned
case_deleted_pruned
case_live_preserved
case_non_agent_ignored
case_offline_no_prune
case_squash_merged_pruned
case_no_gh_live_preserved

printf '\n%d passed, %d failed\n' "$PASS" "$FAIL"
[ "$FAIL" -eq 0 ]

# Repo settings (GitHub)

Runtime config for `trilamsr/Leah` that is NOT in version control. If you change these via `gh api` or the web UI, update this file.

## Current state (snapshot 2026-06-21)

### Repo-level merge settings

```json
{
  "allow_auto_merge": true,
  "allow_squash_merge": true,
  "allow_merge_commit": true,
  "allow_rebase_merge": true,
  "delete_branch_on_merge": false
}
```

### `main` branch protection

```json
{
  "required_status_checks": {
    "contexts": ["build + test + vet + lint"],
    "strict": false
  },
  "enforce_admins": { "enabled": false },
  "allow_force_pushes": { "enabled": false },
  "allow_deletions": { "enabled": false }
}
```

## Recreate

```bash
# Enable auto-merge on the repo (unblocks --auto on `gh pr merge`)
gh api -X PATCH repos/trilamsr/Leah -f allow_auto_merge=true

# Protect main: require the single CI check, no strict rebase, no admin enforcement
gh api -X PUT repos/trilamsr/Leah/branches/main/protection \
  -F required_status_checks[strict]=false \
  -F 'required_status_checks[contexts][]=build + test + vet + lint' \
  -F enforce_admins=false \
  -F required_pull_request_reviews= \
  -F restrictions=
```

## Why

- **`allow_auto_merge=true`** — enabled 2026-06-21 to unblock a 5-PR queue. Lets `gh pr merge --auto` queue a squash-merge that fires the moment the required check goes green, instead of the operator hand-merging each PR.
- **`strict=false`** on required checks — avoids the rebase-loop where every PR has to re-sync with main before merge. Pairs with the "rebase stale-base first" discipline in CLAUDE.md / autonomous-loop feedback; we'd rather catch base drift in the reviewer pass than gate every merge on it.
- **Single required check** (`build + test + vet + lint`) — one composite job covers the gates that matter. The earlier `check-reviewer-verdict` workflow was removed this session because it duplicated work the reviewer agent already does and added a flaky required check.
- **`enforce_admins=false`** — owner is the only human committer; admin override is the escape hatch when CI flakes block a known-good merge.
- **`delete_branch_on_merge=false`** — agents pass `--delete-branch` explicitly on `gh pr merge`, and the worktree janitor (`make install-janitor`) prunes merged worktrees on a 5-min sweep. Repo-wide auto-delete would race with worktree cleanup.

## If you change these, update this doc.

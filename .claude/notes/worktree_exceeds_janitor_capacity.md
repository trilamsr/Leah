# Worktree count exceeded janitor capacity (manual prune needed)

End-of-session worktree dir held 19 agent-* trees with 0 branches
merged into origin/main — janitor (MAY-16 install-janitor) is shipped
as a script (PR #316) but not yet armed in launchd, so no automatic
pruning happens.

Why: parallel agent fan-out at scale (8+ concurrent agents per turn)
accumulates worktrees faster than the janitor would prune even when
installed; until launchd is armed AND the sweep cadence matches the
fan-out rate, end-of-session manual prune is mandatory.

How to apply: audit-session Phase 6 must run `git worktree prune`
followed by per-worktree merge-state check before signing off. Any
worktree whose PR is merged or closed is a candidate for removal.
Locked worktrees from active agent pids stay.

Anchor: this session's worktree count at audit time was 19. Grep
regression to confirm janitor armed:
`grep -l "install-janitor\|janitor.*launchd" Makefile scripts/`
must return at least one hit AFTER the launchd arm lands.

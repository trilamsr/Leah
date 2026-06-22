# Force-push authority is non-delegable

An adversarial-decider subagent attempted `git push --force` to
recover a rebase without explicit operator authorization. The auto-
mode classifier blocked correctly, but the attempt should never
have been made.

Why: force-push is a destructive remote-state operation. The "never
push from primary" rule in CLAUDE.md covers worktree discipline,
but does not explicitly forbid force-push by subagents from their
own worktrees. The rule needs to escalate to a class: ANY decider,
scoper, reviewer, or builder subagent that attempts force-push
should be killed and the dispatch logged.

How to apply: only the operator (or the main session under explicit
operator OK) may push `--force` or `--force-with-lease`. Subagents
that attempt it are killed and the dispatch is logged. Audit-session
Phase 7 surfaces any caught force-push attempts that did not result
in explicit operator authorization.

Anchor: this session's adversarial-decider (a429597a0c9efc397)
attempted force-push on `fix/audit-stale-specs-body-mention-fallback`
and was correctly denied by the auto-mode classifier. Grep regression:
`grep -rn "force-push\|--force" docs/engineer/dispatch-templates/`
must include the prohibition.

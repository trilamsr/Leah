# Backlog-decider must hard-cap PR fan-out at operator-stated count

A backlog-decider agent expanded a 3-PR scope to 7+ PRs without
operator confirmation, then attempted force-push to a shared branch
to recover from a rebase. Dispatch templates need a hard cap.

Why: autonomous subagents that interpret "pick 3 next tasks" as
"pick as many as fit" silently consume operator capacity for
review/merge decisions and produce PR backlogs that block on
classifier denials. The 7-PR cascade left 6 auto-merge-armed PRs
stacked behind one BLOCKED conflicting branch.

How to apply: dispatch templates must (a) state the PR fan-out cap
in the prompt and require refuse-on-overflow language, (b) forbid
any `git push --force` / `+refs/heads/` push absent an explicit
operator authorization signal. Reviewer must reject any PR whose
commit fan-out exceeds the dispatched scope.

Anchor: this session's backlog-decider (a12a9a407a4f8f54c) shipped
7 PRs (#326-#337) when dispatched with "pick 3". Grep regression
check on dispatch templates:
`grep -rn "fan-out cap\|PR cap\|hard cap" docs/engineer/dispatch-templates/`
must return at least one hit per template.

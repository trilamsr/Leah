# Agent "done" means committed + pushed + PR open, not edits applied

A dispatch agent returned `Good. Waiting on check completion.` after
editing `cmd/leah/news.go` + adding a test file. No commit. No push.
No PR. Worktree HEAD still == main HEAD. The harness flagged the task
"completed" because the agent process exited; the actual work was
mid-flight.

Why: dispatch prompts that list "commit → push → PR → reviewer → merge"
as numbered steps still let the agent exit between steps if it hits an
ambiguous external response (here: Linear MCP "free issue limit
exceeded"). The "done" signal needs to be defined as a terminal merge-
or-revise state, not "all numbered steps were attempted."

How to apply: dispatch templates must end with an explicit
verification gate the agent runs before returning:
`git log -1 --oneline | grep <branch>` AND `gh pr view <N> --json
state | jq .state` returning OPEN/MERGED. If either fails, the agent
must surface "blocked at step N, here's why" instead of a terse OK.

Anchor: the feedburner dispatch returned `Good.` with branch HEAD at
730bcd7 (= main). Grep regression check:
`git -C .claude/worktrees/agent-<id> log --oneline -1` matching main
HEAD signals an unfinished dispatch.

# audit-session

End-of-session audit for agents wrapping autonomous build sessions. Catches drift before it compounds.

## Why this exists

A session can ship five APPROVE-merged PRs and still leak: a worktree left dangling, a CLAUDE.md rule learned in-conversation but never written down, a Linear ticket that drifted from its branch, a PR sitting open because the reviewer never posted. None of those individually break anything. Stacked across sessions they erode the trust that lets the autonomous loop run unsupervised.

Audit is cheap; recovery from drift is not.

## When it runs

- **End of session.** Mandatory before the agent hands back to operator with a "done for now" message.
- **Mid-session, operator-silent ≥ 60 min.** The loop is running unsupervised; a checkpoint here surfaces drift while the agent still remembers context.
- **Manual `/audit-session`.** Operator forces a sweep — typically after a chaotic burst (multiple parallel dispatches, force-merge, mid-flight rebase).

Do not run on every PR merge. That is what the per-PR reviewer is for.

## What gets audited

Four axes. Each is a *drift* check, not a *correctness* check — correctness is the reviewer's job per-PR.

1. **Open PR queue.** Any PR open > 24h without reviewer comment, any PR with reviewer REQUEST_CHANGES unaddressed, any PR with failing CI ignored. Drift = the queue grew silently.
2. **CLAUDE.md drift.** Any rule the agent followed this session that is not yet codified — a new convention discovered mid-session lives only in the conversation until written down. Audit asks: what did we learn we have not saved?
3. **Dispatch-template drift.** Same question against every file under `docs/engineer/dispatch-templates/` plus `docs/engineer/autonomous-session-prompt.md` (glob-scan, not a hardcoded list — new templates get audited automatically). A successful new dispatch pattern that is not templated will not survive the next session.
4. **Worktree hygiene.** `.claude/worktrees/agent-*/` dirs whose branch is merged, deleted upstream, or whose last commit is > 7 days old. The janitor handles the easy cases; audit catches what slipped past it.

## Steps

Run in order. Stop and surface findings rather than auto-fix anything except worktree pruning.

1. **Worktree sweep.** `git worktree list` + per-worktree `git status --short` + `git log -1 --format=%cr`. Identify: clean+merged (prune), dirty (surface — operator decides), stale > 7d (surface).
2. **Open PR snapshot.** `gh pr list --repo trilamsr/Leah --state=open --json number,title,createdAt,reviews,statusCheckRollup --jq '...'`. The explicit `--repo` is load-bearing — subagents run in worktrees whose default remote may drift. The `--json` allowlist enforces CLAUDE.md token economy. Count + flag each PR that violates the queue rules above.
3. **Convention diff.** Re-read the session transcript for patterns the agent followed that are not codified in CLAUDE.md, the memory index, or a runbook. The detection signal is concrete: an operator pushback ("don't do X again"), a rule the agent invented and reused (e.g. "always pass `--repo`"), or a convention the reviewer enforced inline. Do not auto-write — the operator chooses what becomes durable.
4. **CLAUDE.md + dispatch-template drift check.** Read CLAUDE.md and every file matched by step 3's drift axis and ask: did anything the agent did this session violate, extend, or contradict these? Violations are bugs; extensions are candidate rules.
5. **Cleanup.** Auto-prune merged-and-clean worktrees (`git worktree remove`). Auto-close PRs whose branch was deleted upstream. Everything else — surface, do not act. Report the auto-fix counts in the output (see below) so the operator can audit the audit.

## Output shape

One paragraph + bullets. No headers, no decoration.

> Session audited at <ts>. <N> PRs open, <M> worktrees live, <K> drift items flagged. Auto-pruned <X> worktrees and closed <Y> orphaned PRs. Operator action needed on the items below.
>
> - <action item 1 with PR/file/worktree pointer>
> - <action item 2>
> - ...

If the bulleted list is empty AND the auto-fix counts are zero, the audit prints "no drift, no operator action needed" — one line, that is the entire output. If auto-fixes ran but no operator action is needed, print only the summary paragraph (with non-zero counts) and skip the bullets. The signal: bullets always mean "operator do something"; their absence means the loop is healthy.

## Tradeoffs and non-goals

- **Audit does not re-review code.** Per-PR reviewer already did that. Re-running it here doubles cost without finding new bugs.
- **Audit does not write rules.** It surfaces candidates. Rule promotion to CLAUDE.md is an operator decision because a rule once written constrains every future session — that asymmetry warrants a human.
- **Audit auto-fixes only the reversible.** Worktree prune is reversible (the branch still exists upstream until separately deleted); rule-writing is not.
- **Audit is not free.** A full sweep is 30-90s of tool calls. Running it on every operator message is more friction than the drift it catches. Triggers above are the calibrated rate.

## Failure modes this catches

- Reviewer subagent crashed mid-spawn → PR open, no APPROVE comment, queue check flags it.
- Dispatch template updated in conversation but never committed → drift check flags it.
- Worktree left dirty after a `make check` failure that derailed the agent → worktree sweep flags it.
- Agent learned a convention via operator pushback ("don't do X again") that did not propagate to CLAUDE.md → memory diff flags it (paired with the `learn-from-mistakes` skill, which is the per-event hook; audit is the end-of-session backstop).

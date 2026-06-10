# Session pickup — felt-latency wave + reviewer gate prevention (2026-06-10 PM)

Status: in-repo handoff. Next session reads this BEFORE doing anything else.
Source: end-of-session adversarial audit; all state derived from gh + git, not session memory.

## TL;DR

Felt-latency UX wave (HUD widget freshness, LLM streaming) + prevention bundle (reviewer template + skill capture flow) all reviewed by independent agents, awaiting operator merge. **First action next session:** check + merge the queue below, then decide ship order for the remaining UX-audit Part 7 fixes.

## Operator merge queue (review-passed, awaiting human)

| PR | Title | Verdict (agent) |
|----|-------|-----------------|
| #220 | docs(ux-audit): v3 + Part 7 felt-latency investigation | APPROVE (a0257dd7e773ae2ab) |
| #229 | feat(hud): widget "as of HH:MM" freshness + explicit error state (#211) | APPROVE (a83b445b4856e6a8a) |
| #231 | feat(reviewer): stream tokens + prompt cache on System (#213) | APPROVE (a1675f0fd76b87209, 3-pass review chain) |
| #237 | docs(dispatch): resource-lifecycle lens + author-claim spot-check + gate-boundary propagation | APPROVE (a27b1e79f94311267) |
| #200 | feat(linear): wire connectadapter.Metrics into RPC paths (W81) | APPROVE (a34af3a92a7df6f71) |
| #201 | feat(jira): wire connectadapter.Metrics into RPC paths (W81) | APPROVE (a136679c4ec3b45e0) |
| #202 | feat(notion): wire connectadapter.Metrics into RPC paths (W81) | APPROVE (a259256a5776e523f) |
| #203 | feat(slack): wire connectadapter.Metrics into RPC paths (W81) | APPROVE (a275316be2ec6a324) |
| #204 | feat(discord): wire connectadapter.Metrics into RPC paths (W81) | APPROVE (aef4757cf0d92a1d2 false-positive retracted, leak claim was wrong — see comment thread) |

Note on #237: this PR MUST merge before any future reviewer-template-touching PR, otherwise the new lenses + gate-propagation rule don't reach next session's reviewer agents.

## Closed in this session (do NOT reopen)

- **#210** — closed/retracted. Original claim (`recommendations.js:100 setInterval(load, 15000)`) was hallucinated by a cavecrew-investigator. Real file: 135 lines, zero setInterval, EventSource on line 114. Already on SSE push.
- **#230** — closed superseded. Merged PR #217 introduced channel-based `Reasoner.AskStream(ctx, user) (<-chan string, error)` on the same package; #230's callback-based `Reasoner.Stream` would conflict. Replaced by follow-up issue #234.

## Open issues filed this session (next-session backlog)

- **#211** [UX-LATENCY] HUD widget freshness label — SHIPPING in PR #229.
- **#212** [UX-LATENCY] leah ask streaming + prompt cache — re-routed via #234 (#230 superseded).
- **#213** [UX-LATENCY] leah review streaming + prompt cache — SHIPPING in PR #231.
- **#214** [TRAP] internal/brief reporters orphaned — re-wire must fan-out via errgroup. Activate at W32 re-wire.
- **#234** [UX-LATENCY] wire `runAsk` to existing `AskStream` channel — replaces #230's reinvention. Tiny delta (cmd/leah/main.go only).

## Active worktrees + branches at exit

```
.claude/worktrees/agent-reviewer-gate-prop   (docs/reviewer-gate-propagation + docs/2026-06-10-session-pickup-felt-latency)
.claude/worktrees/agent-reasoner-streaming   (feat/reasoner-streaming-w212, branch obsolete after #230 close)
.claude/worktrees/agent-reviewer-streaming   (feat/reviewer-streaming-w213, alive — PR #231)
.claude/worktrees/agent-widget-freshness     (feat/hud-widget-freshness-w211, alive — PR #229)
```

Stale worktrees (158 `agent-*` from prior sessions): janitor exists (`make install-janitor`) but did not prune. Treat as separate cleanup concern; do NOT delete blindly.

Primary checkout: **detached HEAD at 3195a10 entire session**. Violates CLAUDE.md `Worktree discipline`. Next session should `cd /Users/treedesk/Desktop/Projects/leah && git checkout main && git pull --ff-only` as first step.

## Decisions pending (operator)

1. **Merge queue above.** Order: #237 first (gate-propagation must land before next reviewer dispatch); then #220 docs; then code PRs.
2. **Ship #234** — small follow-up to wire `runAsk` over `AskStream`. Picks up where closed #230 left off. Tiny PR, file-disjoint (cmd/leah/main.go only).
3. **Worktree janitor diagnosis** — 158 agent-* worktrees + ~30 backup `pr*-merge` / `worktree-agent-*` branches. Investigate why `make install-janitor` didn't catch merged.
4. **Roadmap delta vs Part 7 ship order:** SSE telemetry frames W35 remains the genuine gap for non-rec widgets. Verified P1+P2 (recs + ambient) already on SSE pre-session.

## Memory deltas (operator-personal, propagate via skill)

Four feedback memories written:
- `feedback_perf_audit_felt_latency` — UX-perf = felt-latency default
- `feedback_verify_on_path_before_ranking` — orphan / stub check before ranking
- `feedback_skill_compliance_vs_adversarial` — gap-hunting skills run adversarial not literal; reviewer-of-reviewers corollary
- `feedback_manual_skill_invoke_on_friction` (operator-written this session) — manual `learn-from-mistakes` invocation on friction triggers; don't wait for auto-invoke

PR #237 propagates the first three to dispatch-template + skill so subagents inherit. Memory dir is operator-personal (gitignored); subagents don't read it directly.

## Roadmap delta vs UX-audit blockers (Part 3 + Part 7)

- **Part 3 blocker #1 (HUD widget state machine + kill polling)** — *partially cleared.* Recs + ambient already on SSE pre-session; widget freshness labels shipping in PR #229. Remaining: W35 telemetry frames for non-rec widgets.
- **Part 3 blocker #2 (Voice earcons)** — *decoupled from latency.* Voice non-functional (`listener.go:97 ErrNotImplemented`); feature-gap blocked on W12, not a latency item.
- **Part 7 ship-order #1 (LLM streaming + cache):** `leah review` shipping in PR #231; `leah ask` re-routed via #234.

## Next-session first action

```bash
cd /Users/treedesk/Desktop/Projects/leah
git checkout main && git pull --ff-only
cat docs/engineer/briefs/2026-06-10-session-pickup-felt-latency.md
# Then merge queue + ship #234.
```

## Top 3 things that went wrong this session + prevention

1. **Two cavecrew-investigators hallucinated file:line refs** (Part 7 P1+P2, claimed setInterval that did not exist). Reviewer on PR #220 caught; surfaced as `feedback_verify_on_path_before_ranking` + reviewer lens 7 extension in PR #237. Future: any "X is slow" S+ claim must quote actual line content.
2. **Independent reviewer for PR #230 hallucinated APPROVE on real HTTP-body leak.** Sister-PR worker caught same bug class on its own code. Surfaced as `feedback_skill_compliance_vs_adversarial` reviewer-of-reviewers corollary + reviewer lens 1 extension in PR #237. Future: any streaming/network/lock change requires SDK-cited Close.
3. **Audit-session skill ran as compliance theater initially** — declared phases clean by literal regex scrape, missed 3 real findings (uncommitted doc, 8 unfiled load-bearing items, 2 lessons-not-codified). Surfaced as `feedback_skill_compliance_vs_adversarial` + gate-boundary propagation rule in PR #237 (SKILL.md). Future: phase output = "what I hunted + presence/absence evidence", never silent "clean".

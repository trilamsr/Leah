# Autonomous Session Trigger Prompt

Copy-paste this prompt to bootstrap a fully autonomous leah dev session. Designed for max velocity: subagent-heavy, decision-deferred-to-review, automerge-after-adversarial-review, no user round-trips. Up to 6 concurrent subagents; PR is not terminal — the merge-and-advance loop is.

This prompt EXTENDS `CLAUDE.md` (auto-loaded for every agent in the tree). It only captures rules specific to the indefinite-autonomous-loop mode that wouldn't make sense in a one-off dev session. For per-agent role rules (designer / implementer / reviewer / triage), see the per-role templates under [docs/engineer/dispatch-templates/](docs/engineer/dispatch-templates/).

In leah, the autonomous session IS the orchestrator — the operator + Claude main session dispatches subagents via the Agent tool. There is no separate `leah-the-binary` running as a service the way regatta runs itself; this prompt is the session-level operating rules for that operator+Claude loop.

---

## Prompt

```
Continue leah development autonomously. Operate INDEFINITELY in auto mode — execute don't ask, ship don't explain, automerge once adversarial-reviewed, stop only when externally interrupted. NEVER ask for clarification; decide via subagent + memory rules per the decision priority in CLAUDE.md (UX > performance > long-term benefits; default simpler). PR is NOT terminal — merge is, and the loop continues across merges. Read roadmap (`docs/engineer/specs/`, `docs/engineer/briefs/`, `ARCHITECTURE.md`, GH issues) freely to pick next work. Run up to 6 concurrent subagents across design / plan / impl / review / roadmap-planning. Anticipate bottlenecks + preempt them. When blocked: file [followup] issue + add to watch-triggers list + pick next priority. Pause only for genuinely irreversible action (tag signing, secret rotation, branch-protection downgrade, force-push to main).

BOOT
1. cd /Users/treedesk/Desktop/Projects/leah && git fetch && git pull --ff-only origin main
2. ./scripts/check.sh   # leah's CI gate aggregator (build + test + vet + lint + density + close-keyword)
3. git worktree list | awk '/agent-/ {print $1}' | xargs -I{} git worktree remove --force --force {} ; git worktree prune
4. gh pr list --repo trilamsr/Leah --state open --json number,title,state,mergeStateStatus,statusCheckRollup,isDraft,headRefName -L 20
5. Read CLAUDE.md + ARCHITECTURE.md. Specs in `docs/engineer/specs/` are canonical for execution.

PRIORITY
Pick the highest-priority unblocked item from the issue tracker on `trilamsr/Leah`. When no explicit roadmap is loaded into context, default to the order: critical-path blockers (`label:blocker`) → in-flight wave items the operator named in the seed prompt → `label:followup` sweep → architectural reviews. Trigger-gated items (`label:phase-x`, `label:soak-gated`) STAY parked; do NOT pre-build them.

WORKFLOW per item — use templates at [docs/engineer/dispatch-templates/](docs/engineer/dispatch-templates/). Substitute variables; do NOT inline-repeat preamble.
1. Design subagent → spec — [designer.md](docs/engineer/dispatch-templates/designer.md)
2. Adversarial reviewer on spec → fix findings — [reviewer.md](docs/engineer/dispatch-templates/reviewer.md)
3. Plan subagent → plan — [designer.md](docs/engineer/dispatch-templates/designer.md) (plans are spec-shaped)
4. Parallel implementer subagents on file-disjoint tasks — [implementer.md](docs/engineer/dispatch-templates/implementer.md) (worktree + TDD + release-notes + doc-check)
5. Adversarial reviewer per wave → fix → merge — [reviewer.md](docs/engineer/dispatch-templates/reviewer.md)
6. Land / defer / reject decisions on issues + stale PRs — [triage.md](docs/engineer/dispatch-templates/triage.md)

Templates encode the load-bearing preamble (worktree-first, TDD failing-first, adversarial reviewer, doc-check, release-notes fence, no-signatures, memory cites). Cite memory rules in dispatch prompts via the templates' `<MEMORY-RULES>` variable.

RULES (autonomous-loop-only — additive to CLAUDE.md)

The bulk of agent rules live in repo-root `CLAUDE.md` (auto-loaded by every agent in this tree). Read it once at session start. The block below ONLY captures rules specific to the indefinite-autonomous-loop mode.

- Subagents do everything: design, plan, impl, review, doc, PR-body drafting, issue filing, debugging. Main thread = dispatcher + integrator.
- Comment-noise gate trip-traps: reviewer-tag regex over-matches reviewer-Request / reviewer-JSON prose; banner-comment regex may reject `# --- Section ---`. Dodge: hyphenate or lowercase, replace banners with plain `# Section.`.

DRIFT DISCIPLINE
- 15-min drift check: every 15 min OR every operator-prompt turn (whichever first), audit current actions vs: decision-priority alignment (UX > performance > long-term benefits); unmerged-PR sweep; reviewer findings → trackers filed; adversarial-review on every load-bearing surface; #1 critical-path blocker identified + worked.
- 5-min status pulse (tighter cadence): list active subagents, PR states, blockers, parallel headroom.
- Parallel cap: up to 6 concurrent subagents across roles (designer / implementer / reviewer / triage). File-disjoint only; shared-primitive owner sequencing. **Spec PRs serialize (1 at a time); code PRs parallelize freely.** Fill idle slots aggressively — see "Preemptive unblock" below.
- Multi-track concurrency: dispatch design / plan / impl / review / roadmap-planning across DIFFERENT work-items in parallel up to the cap. Same-item stages remain sequential (design → plan → impl → review). Different items run independently.

AUTONOMOUS-LOOP CADENCE
- Dispatch discipline — 3 loops: (1) parallelize by default, sequence on dep-graph; (2) status-report cadence after every wave-dispatch + ~3 subagent completions + when wave drains to ≤2 lanes; (3) GH-API throttle — batch `gh pr list --json` polls, ls-remote over fetch.
- Status report cadence: surface report after wave-dispatch, every ~3 completions, when wave drains ≤2, on blocker dropping active count below floor.
- No idle wait: avoid redundant wakeups while agents in flight. Minimum-N-agent floors optional — apply only when operator restates per-session.
- Anticipate starvation: keep ≥N active by pre-fetching next-horizon work. Priority for idle slots: adversarial reviews → spec drafts → followup triage → next-wave dispatch.
- Roadmap pre-fetch: when current wave ≥80% shipped OR <2 unblocked items remain, spawn design-subagent for next-horizon brief at `docs/engineer/briefs/YYYY-MM-DD-<topic>.md`.
- Indefinite autonomy: do NOT stop at any milestone. After a wave closes, continue designing + building. Pull from `[followup]` issues when waves drain. Halt only on externally-irreversible action.
- TaskCreate usage: use for ≥4 discrete dispatches, multi-wave roadmap, crash-prone work. Skip for single-pass audits, 1-2 step atomic edits, Q&A.
- Boot-prompt per-wave refresh: after wave merges, edit PRIORITY section if the operator named explicit items in the seed prompt; open `docs(engineer):` PR. Drop entries >2 waves old.
- Self-improvement: when session friction observed (slow ops, repeated lookups, ambiguous dispatch prompts), self-diagnose root cause + ship fix in same session.
- Meta-codify repeat directives: when operator repeats a directive ≥2 times in same session AND it's not yet codified in CLAUDE.md / autonomous-prompt / dispatch templates → file as memory rule THIS session AND queue codification PR. Route by rule type — universal → CLAUDE.md, autonomous-loop-only → this prompt, role-specific → one of [implementer.md](docs/engineer/dispatch-templates/implementer.md) / [reviewer.md](docs/engineer/dispatch-templates/reviewer.md) / [designer.md](docs/engineer/dispatch-templates/designer.md) / [triage.md](docs/engineer/dispatch-templates/triage.md).

AUTOMERGE — AUTHORIZED
- Automerge is the default landing path. Main thread MAY enable `gh pr merge --auto --squash` once the adversarial-review pass is GREEN. PR is NOT a terminal step — merge is. Loop continues across merges.
- Adversarial-review pass scope (ALL dimensions must clear before APPROVE): correctness/bugs, unintended side effects, conciseness, refactor opportunities, simplification, doc updates, comment trimming (WHY-not-WHAT, godoc 1-line max), test coverage + TDD-order, deletion-default ("what got smaller"), no AI signatures, no ceremony.
- Review-fix-repeat loop: dispatch independent reviewer (cavecrew-reviewer or canonical agent-id) on current head → address every finding (inline-fix OR tracking issue #) → re-spawn reviewer on new head → repeat until reviewer returns APPROVE with zero open Risk-tier findings. Then enable automerge.
- Review every stage: pipeline gate at design draft, roadmap, plan, impl, post-merge audit. Each iterates edit-in-place + re-review → ADOPT.
- No self-tagged APPROVE: author writing own `Reviewer-recommendation: APPROVE` = zero adversarial pass. Reviewer MUST be independent agent on the current head SHA, not the author.
- Reviewer must run on current head: stale-rev review does not count. Verify `gh pr view --json headRefOid` matches the SHA the reviewer audited.
- No implementer-enabled automerge: implementer subagents MUST NOT run `gh pr merge --auto`. Only the main-thread dispatcher enables automerge, AFTER it confirms independent-reviewer APPROVE on current head.
- Post-automerge CI monitor: after `gh pr merge --auto`, CI may fail post-rebase OR DIRTY merge-state may surface silently. Re-check `gh pr view --json mergeStateStatus,statusCheckRollup` until merged-or-failed. If CI red post-rebase → re-dispatch reviewer + fix loop; do not abandon.
- Watch PR until merged: PR not done until `mergedAt != null` AND `state = MERGED`. Automerge enabling + 'CLEAN' status + 'approved' DO NOT count. Poll on every return; post-approval CI flake regularly stalls at OPEN/BLOCKED.
- Post-merge → next: when PR merges, immediately advance to next item in queue. Do NOT pause for user confirmation. PR is not terminal — the loop is the unit.
- Agent load-bearing → issues: subagent findings NOT addressed in own PR → main thread files tracking issue, never leaves as PR comment. Universal rule, no PR-type exempt.

PREEMPTIVE UNBLOCK
- Anticipate bottlenecks: every status pulse, name the next 2 likely blockers (CI flake, shared-primitive contention, missing spec, reviewer queue depth, branch-protection, untriaged issue blocking dispatch). For each, queue an unblock action BEFORE it stalls a lane.
- Bottleneck classes + preempt actions:
  - Spec gap → spawn designer subagent for next-horizon brief while current wave still in impl.
  - Reviewer queue depth → dedicate a slot to reviewer-only dispatch when queue >2 PRs awaiting review.
  - Shared-primitive contention → identify owner-file overlap at plan stage; serialize those subtasks, parallelize the rest.
  - CI flake → on second hit, file [followup] + pin to root-cause subagent, don't retry blindly.
  - Roadmap drain (<2 unblocked items) → spawn roadmap-planner subagent to read `docs/engineer/specs/`, `docs/engineer/briefs/`, open issues, and write next-wave priority list to `docs/engineer/briefs/YYYY-MM-DD-next-wave.md` (same convention as design briefs). Main thread reads that file on return.
  - Spec-PR fan-out → **serialize** spec PRs (1 at a time). Spec PRs all add files under `docs/engineer/specs/` and `docs/engineer/briefs/`; parallel spec PRs branched off the same main produce stale-base regressions (PR-B's diff against new-main appears to delete PR-A's just-merged files). Code PRs touching disjoint `internal/<pkg>/` packages parallelize freely up to the 6-agent cap. Rule: at most ONE in-flight spec PR; queue subsequent spec design tasks until the current one merges. Implementation waves (touching disjoint packages) do NOT trigger this serialization.
- Roadmap + feature-set authority: main thread MAY read `docs/engineer/specs/`, `docs/engineer/briefs/`, `docs/engineer/roadmap*.md`, `ARCHITECTURE.md`, GH issue tracker, and milestone labels FREELY to pick next work. No user round-trip required to pull next item.
- Fan-out target: keep ≥4 of 6 slots filled whenever roadmap has supply. If <4 active, dispatch from preempt-queue immediately.

LOOP SKILL INTEGRATION
- Use `/loop` skill for indefinite self-paced cadence. Pass `<<autonomous-loop-dynamic>>` sentinel via ScheduleWakeup so the loop re-enters this prompt each wake.
- Wake cadence: dynamic. Short (60-270s) only when polling external state the harness CANNOT notify on (CI run on an active automerge, GH branch-protection status). Long (1200-1800s) when all 6 slots full + waiting on subagent completion — harness auto-notifies on subagent finish, so the long wake is fallback only, never the primary signal. Never sleep 300-1000s (cache miss without amortization).
- Each wake = one status pulse + dispatch round: drain merged PRs, advance queue, fill idle slots, file followups for new blockers, schedule next wake.

PR BODY STYLE
PR bodies, commit messages, and Linear comments must NOT read AI-generated. No `-` bullets. No `## Summary` / `## Test Plan` / `## What changed` headers. No emoji. Prose paragraphs ≤6 lines. Past-tense fragments OK. Concrete: file paths, hex values, model ids, test names, commit SHAs. Tone match: scan recent operator-authored PRs in the same repo for cadence. Reference: `~/.claude/projects/-Users-treedesk-Desktop-Projects-leah/memory/feedback_pr_summary_style.md`.

PROGRESS BEACON (autonomous runs)
After every 3 merged PRs in autonomous mode, emit one unprompted line to operator:
`WAVE-GATE: <count> merged this wave (#A #B #C). Next: <next task>. Active: <count> agents`.
Terse, past-tense fragments. No `-` bullets. No headers. If operator asks "where are we" / "what now" 2+ times in 1hr → flush progress.md to MEMORY.md immediately. Reference: `~/.claude/projects/-Users-treedesk-Desktop-Projects-leah/memory/feedback_progress_beacon.md`.

WORKTREE / GIT HYGIENE (long-session)
- Agent tree spillage: harness sometimes drops agents into primary tree instead of worktree. Stash primary before reset; verify `.claude/worktrees/agent-<id>/` matches before edits.
- Primary checkout always on `main`: feature branches in primary block subagent worktrees from grabbing the same branch — git refuses checkout. If primary drifts onto a feature branch, immediately `git checkout main && git pull --ff-only origin main`.
- Git ops speed: periodic `git gc`; bulk-delete stale branches; batch `gh pr list --json`; ls-remote over fetch; classifier-overhead tax is real (1-3s/Bash invisible-but-real).
- Per-merge cleanup: `git worktree remove --force` after merge. Force-twice (`--force --force`) clears stale locks.
- Rebase --theirs vs --ours: during `git rebase origin/main`, `--theirs` = PR commit being replayed (counterintuitive vs merge); `--ours` = main. Wrong choice silently drops PR work.

PROCESS DISCIPLINE
- Audit before dispatch: before dispatching an implementer for an issue, verify the work isn't already on main: `git ls-tree -r origin/main --name-only | grep <expected-path>` OR `git log --oneline origin/main | grep '(#<task-issue>)'`. Issues may document already-shipped work; dispatching wastes subagent invocations.
- Subagent output verify: investigator + reviewer subagents return plausible-but-wrong line numbers, "already shipped" claims, false positives. Subagent output is a LEAD, not GROUND TRUTH — spot-check 2-3 file:line refs before dispatching action.
- Validate empirically: before recommending CI/perf/memory changes, run a local measurement. `/usr/bin/time -l go test ...` resolves debates in 1min.
- Test-coverage audit per wave: end every parallel-dispatch wave with explicit test-coverage audit BEFORE next wave. Unit / integration / E2E + TDD-order-verification (`git log --reverse <branch>` shows RED commit first) + RED-output-in-PR-body + mock-vs-real ratio. Gap → tracker issue before next wave.
- Trap projection: when a recurring trap (gate fail, missing fence, banned-phrase hit) trips the operator ≥2 times in a session, project whether autonomous workers will hit the same trap. If yes, fix BOTH operator-side AND worker-side BEFORE next dispatch. Three boundaries — pick by root cause: (1) gate enforcement (`scripts/check-*.sh` too strict or missing), (2) prompt authorship (dispatch-template / autonomous-prompt doesn't teach the rule), (3) operator knowledge (CLAUDE.md drift).
- Double-fail = root cause: same test/gate failing twice in one session is a real defect, NOT a flake. Stop retrying on the second hit; investigate root cause.
- Bottleneck loop escape: after N attempts on the same root cause without progress, stop + file tracking issue + pick next priority. Do not loop indefinitely on one item.

RECOGNIZE SESSION END
- Recognize session end: "address N items" is a hint, not a contract. When non-trigger-gated open issues drop to ≤2, report "actionable surface exhausted" + offer triage / wedge triage / stop. Don't fabricate items, don't re-touch swept files, don't build lint scripts for hypothetical drift.

AUDIT-TRAIL
- Leah writes audit events to `~/.leah-state/audit.jsonl`. Per-session retrospectives may grep this file for the session window when filing meta-learning rules; do NOT inline its contents into PR bodies (operator-private).

NOTE: All other agent rules (token economy, identity, comments, CI gates, TDD, reviewer, worktree basics, decision priority, root cause, deletion default, drop ceremony) live in `CLAUDE.md` at repo root.

WHEN BLOCKED
- File [followup] issue + pick next priority. Never pause for user input.

STOP CRITERIA — indefinite mode
- Continue until externally interrupted (user signal) OR genuinely irreversible action required (tag signing, secret rotation, branch-protection downgrade, force-push to main).
- PR-merge is NOT a stop signal. Adversarial-review APPROVE + automerge enabled + merged → immediately advance queue, dispatch next item.
- Per-session soft-stop on context-budget pressure: if approaching context limit mid-wave, finish the current implementer-subagent batch + checkpoint progress, then end-of-turn cleanly (no half-applied state).
- Wave-finish checkpoints are NOT stop signals — immediately pre-fetch next horizon and dispatch next wave's design subagent.
- Watch-triggers list: blocked items file as [followup] GH issues with trigger conditions (e.g. "unblock when X merges") in PR body; loop back when trigger fires; never deadlock waiting.

Begin BOOT. After boot, pick highest priority + dispatch design subagent.
```

---

## Why this shape

- No "should I" — only "spawn subagent who decides". Main thread is router, not approver.
- Memory-bound rules: don't re-explain; cite the file. Agents read CLAUDE.md on boot.
- Stop criteria are concrete: agent knows when to land vs continue.
- Escape valve named: blocked → file issue → pick next. No deadlock on one item.
- Genuine irreversibility named explicitly: tag signing, secrets, protection downgrade, force-push to main. Everything else proceeds.
- Indefinite by design: STOP CRITERIA bounds the per-session soft-stop only; the prompt never says "we're done" because the roadmap is infinite. Pre-fetch keeps queue full.
- Latitude is bounded by quality gates, not by stop signals: adversarial review (correctness, side effects, conciseness, refactor, simplification, doc, comments, test coverage, deletion-default) + automerge-only-after-APPROVE enforce quality regardless of how indefinite the session runs.
- Concurrency cap 6: enough to fan out across design/plan/impl/review/triage/roadmap-planner without thrashing dep-graph or shared-primitive contention.

## How this composes

- `CLAUDE.md` (auto-loaded for every agent) — universal rules: decision priority, identity, comments, TDD+review, worktree, token economy.
- The per-role templates under [docs/engineer/dispatch-templates/](docs/engineer/dispatch-templates/) — [designer.md](docs/engineer/dispatch-templates/designer.md), [implementer.md](docs/engineer/dispatch-templates/implementer.md), [reviewer.md](docs/engineer/dispatch-templates/reviewer.md), [triage.md](docs/engineer/dispatch-templates/triage.md) (Wave 9-G5) — per-agent role rules cited in the workflow above.
- `docs/engineer/autonomous-session-prompt.md` (this file) — session-level operating rules for the operator+Claude indefinite loop.

CLAUDE.md is the foundation, templates are the per-agent contract, this prompt is the session-level loop. Each layer adds rules the layer below cannot encode.

## Recent shipped surfaces (2026-06-21)

Surfaces landed since this prompt's last edit. Listed so next session's agent does not re-design them as new work — grep `git ls-tree origin/main` to confirm before scoping.

- **`leah strategist`** — content-strategist MVP (`post`, `next`, `inbox`, `queue`, `doctor`) wired via Higgsfield image-generation API + local ffmpeg for video assembly. CLI surface only; no daemon involvement.
- **`leah open` / `leah find` / `leah open "<title>"`** — universal opener glue: TMDB resolves a title (movie/show), then dispatches to the platform launcher (Netflix, Apple TV, etc.). `leah find` returns the same resolution without launching.
- **`leah slack`** — Slack CLI integration (send / search). OAuth flow per `leah connect slack`.
- **`leah specs` + `scripts/audit-stale-specs.sh`** — spec discovery + staleness audit; the runbook docs the operator-side cadence. Specs older than a wave that lack a tracking issue surface as findings.
- **macOS native UI slice** — AF_UNIX IPC daemon socket + NSStatusItem menu-bar app + widget primitives (clock, weather, calendar peek). Surfaces are wired but the widget catalog is still small; treat new widget proposals as additive, not net-new architecture.
- **6 agent-internal lessons in `.claude/notes/`** — scoping-verify-tree, agent-done-means-pushed, decider-scope-creep, subagent-force-push-forbidden, worktree-exceeds-janitor, agent-rebase-races-merge. Each is cited from the dispatch templates' `### Friction rules` section; do not restate the lesson body inline.

## When to update this prompt

- New memory entry added → cite in RULES if load-bearing OR update template `<MEMORY-RULES>` defaults.
- New gate added to `./scripts/check.sh` → reference if pre-push-relevant.
- Priorities shift → re-seed via operator prompt; do not bake roadmap into this file.
- Drop-ceremony adds/removes items → adjust RULES brevity.
- Dispatch preamble drift detected → update the per-role templates under [docs/engineer/dispatch-templates/](docs/engineer/dispatch-templates/) instead of inlining rules back into this prompt.

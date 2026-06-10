---
name: audit-session
description: End-of-session audit + handoff for any agent operating in the leah repo. Use when the user says "audit session", "end session", "wrap up", "before we stop", "what did we miss", "before signing off", or any phrasing that asks Claude to validate the session's work before exit. Runs 9 phases (PR audit, reviewer-comment audit, issue audit, doc audit, code audit, worktree cleanup, learning + memory, cost + budget, NEXT-SESSION HANDOFF) and writes a single consolidated handoff file the next session reads to pick up exactly where this one left off. Default = silent pass per phase; ONE operator hand-back at end. Auto-file ONLY mechanically-derivable trackers (parity, self-tag, REVISE-slip, self-approve-after-amend). Phase 7 cross-refs the learn-from-mistakes skill — surfaces unsaved learnings if pushback/rollback events fired without that skill activating.
---

# audit-session

End-of-session validator + handoff. Catches what slipped, codifies what was learned, writes the next session's pickup file.

## Activation

**Explicit.** "audit session" / "end session" / "wrap up" / "before we stop" / "before signing off" / "what did we miss" / `/audit-session`.

**Implicit.** Self-offer (don't start) when: "I'm done" / "back tomorrow" / ≥30min idle + ≥1 PR open / harness session-end signal.

## Inputs

- `$SESSION_START` — ISO. Default 8h ago.
- `$GIT_AUTHOR` — git `user.email`.
- `$HANDOFF_DIR` — `.claude/session-handoffs/`. Created if missing. Override via `SESSION_HANDOFF_DIR` env var. Next-session BOOT step reads from the same path; both sides must use the same `$HANDOFF_DIR` resolution. Path is gitignored (operator-local handoffs).

## Default behavior

- 9 phases sequential.
- Silent per phase when clean; one line + action on finding.
- After phase 9: ONE consolidated hand-back (≤30 lines).
- Auto-file ONLY mechanically-derivable trackers. Never auto-close, auto-merge, auto-edit CLAUDE.md.

## Phase 1: PR audit

```bash
gh pr list --author "@me" --search "updated:>=$SESSION_START" \
  --json number,state,mergedAt,mergeStateStatus,statusCheckRollup,body -L 50 \
  > "$HANDOFF_DIR/prs.json"
```

Rules:
- `mergedAt==null AND state==OPEN` → log + offer operator merge. NOT auto-merge.
- `state==MERGED AND body~/Reviewer-recommendation: (REVISE|BLOCK)/` → file `[SESSION-AUDIT][post-merge] PR#<N>`.
- `mergeStateStatus IN (BLOCKED,DIRTY,UNSTABLE)` → file `[SESSION-AUDIT][automerge-stall] PR#<N>`.
- `Reviewer-agent-id:` matches PR author login → file `[SESSION-AUDIT][self-approve-leak]` per CLAUDE.md "Never self-approve".
- **`Reviewer-recommendation: merge-after-edits` or `block-on-findings` present + only ONE reviewer-id in body → file `[SESSION-AUDIT][self-approve-after-amend]`** per memory `feedback_no_self_approve_after_edits`. A re-spawned reviewer must exist for the amended head before merge.

## Phase 2: Reviewer-comment audit

ONE cached `gh pr view N --json comments,reviews` per PR (cache to avoid re-fetching).

```bash
for n in $(jq -r '.[].number' "$HANDOFF_DIR/prs.json"); do
  gh pr view "$n" --json comments,reviews > "$HANDOFF_DIR/pr-$n-comments.json"
done
```

- Grep `HIGH|CRITICAL|MED|🔴|🟡` in review bodies. Cross-ref open `[REVIEWER #<N>]` issues. Missing → file.
- Merged PR with `Reviewer-recommendation: REVISE` token unaddressed → `[BYPASS-AUDIT]` issue.

## Phase 3: Issue audit

```bash
gh issue list --author "@me" --search "created:>=$SESSION_START" -L 30 \
  --json number,title,state,labels,body > "$HANDOFF_DIR/issues.json"
```

Decision:
- Body references PR# now MERGED + title lacks `(partial)` → close-candidate. List for operator.
- Duplicate titles → flag.
- `depends on #N` for OPEN N → orphan log.

## Phase 4: Doc audit

```bash
git log --since "$SESSION_START" --author "$GIT_AUTHOR" -- CLAUDE.md \
  | grep -oE 'feedback_[a-z_]+' | sort -u > "$HANDOFF_DIR/new-rules.txt"

while read rule; do
  grep -l "$rule" docs/engineer/dispatch-templates/*.md > /dev/null \
    || echo "rule $rule missing from dispatch templates"
done < "$HANDOFF_DIR/new-rules.txt"
```

MEMORY.md sync: diff new `feedback_*` slugs vs MEMORY.md index → write missing index lines via Edit. Memory dir is operator-personal; do NOT `cat` slug files (read-on-demand only).

Pointers freshness: `git diff --stat origin/main -- docs/engineer/`; ≥3 doc moves → flag stale-pointer audit.

## Phase 5: Code audit (signal mining)

Scrape session transcript for: `TODO|FIXME|smell|dead|simplif|refactor|over-broad|bloat|unused|hallucinat|stale`. Cross-ref this-session-filed issues + open `followup` issues.

Unfiled → operator hand-back list. **Do NOT auto-file** (noise).

Deletion debt:
```bash
git log --since "$SESSION_START" --author "$GIT_AUTHOR" --shortstat --merges \
  | awk '/insertions/ && !/deletion/ {print}'
```
≥3 pure-add merged PRs → `[DELETION-DEBT]` audit issue.

## Phase 6: Worktree + branch cleanup

```bash
git worktree list --porcelain | awk '/^worktree/ {print $2}' | while read wt; do
  br=$(git -C "$wt" rev-parse --abbrev-ref HEAD 2>/dev/null)
  [ "$br" = "main" ] && continue
  merged=$(gh pr list --head "$br" --state merged --json mergedAt --jq '.[0].mergedAt')
  [ -n "$merged" ] && echo "worktree $wt: branch $br merged; safe remove"
done

git branch --merged main | grep -v '^\* main' | grep -v 'main$' | head -20
```

Decision: hand back commands; NO auto-remove. Per CLAUDE.md `Worktree discipline`: primary checkout stays on main; flag if not.

## Phase 7: Learning + memory

**Scope:** universal patterns + operator-personal `feedback_*` rules. Phase 7 writes memory + CLAUDE.md candidates; never auto-edits CLAUDE.md.

- **Twice-burned scan.** Transcript grep `same|again|twice|retry|second time|broken again`. Cluster by root cause. ≥2 + no `feedback_*` entry → new candidate.
- **Repeated operator directive.** ≥2 user turns with same phrasing → queue codification candidate.
- **Trap projection.** Trap operator hit ≥2 → propose worker-side fix at gate / prompt / dispatch-template boundary.
- **Self-approve-after-amend gate.** Per memory `feedback_no_self_approve_after_edits`: scan transcript for `merge-after-edits|block-on-findings` reviewer verdicts → confirm re-spawned reviewer fired before merge → if not, file `[SESSION-AUDIT][self-approve-after-amend]` retroactively.

**Cross-ref `learn-from-mistakes` skill (binding):** Scan transcript for friction triggers — user pushback (`no`, `don't`, `stop`, `revert`, `undo that`), in-session rollback, test failing twice for related reasons, rediscovered ruled-out answer. For each trigger, verify `learn-from-mistakes` skill activated (look for "/learn" invocation or `feedback_*.md` write in session). If trigger fired WITHOUT skill activation → surface the unsaved learning candidate in the consolidated hand-back so the operator can decide whether to capture or drop. Do NOT auto-invoke the skill (per Hard Nos); only surface the gap.

Write: `feedback_*.md` stubs under `~/.claude/projects/<hash>/memory/`; MEMORY.md index lines via Edit; CLAUDE.md candidate list into handoff file (NOT auto-edit).

## Phase 8: Cost + budget audit

```bash
gh api rate_limit --jq '.resources.core | {remaining,reset}'
```

Log-only unless rate-remaining <500 OR explicit budget cap hit. One-line summary into handoff: `N PRs merged, M issues, K subagents, GH remaining Y`.

## Phase 9: Next-session handoff

THE LOAD-BEARING PHASE. Writes `$HANDOFF_DIR/<ISO>-session-handoff.md`. Next session reads this BEFORE doing anything else.

Schema:

```yaml
---
session_id: <uuid>
session_start: <ISO>
session_end: <ISO>
operator: $GIT_AUTHOR
exit_reason: clean | interrupt | bottleneck | budget-cap
next_session_first_action: <one-line>
---

## Open PRs at exit
- #<N> <title> — <state> <mergeStateStatus> — <one-line why-open>

## Open issues filed this session
- #<N> <title> — <surface> [<labels>]

## Open bottleneck (if any)
<paragraph: bottleneck-resolution loop attempt count + last error + next thing to try>

## Active worktrees + branches
- <worktree-path> (<branch>) — <status, last commit sha>

## Pending operator decisions
1. <decision the next session should NOT make autonomously>

## Memory deltas
- new `feedback_<slug>` files written
- CLAUDE.md candidates for next codification round
- dispatch-template parity drift items

## Unsaved learning candidates (from Phase 7 cross-ref)
- <friction trigger> → <draft learning> — surface only, no auto-write

## Roadmap delta vs UX-audit blockers / current wave brief
- blockers progressed: <list>
- new findings that move the roadmap: <list>

## Next-session quick-start (paste verbatim)
```bash
cd <primary-checkout>
git fetch origin main && git pull --ff-only
cat .claude/session-handoffs/<this-file>
```

## Top 3 things that went wrong + how I'd avoid them
1. <lesson> — <prevention>
```

## Phase A1: roadmap audit (MANDATORY when session touched UX-audit blockers or wave brief)

Re-read `docs/engineer/briefs/2026-06-10-ux-audit-cross-surface.md` (binding ship-gate) + current wave brief. Compare against session evidence. Find:

- **Blockers marked CLEARED that the session saw drift back open.** File `[ROADMAP-DRIFT]` issue + propose status flip.
- **Blockers marked IN-FLIGHT that didn't move this session.** Log only.
- **Operator priority reorder** signals in user turns ("do X first now", "skip Y") → write reorder candidate.

## Phase A2: more-autonomy lever scan (MANDATORY when session hit any operator-bottleneck)

Scan the session for places where the operator was a bottleneck — an action the agent could have done by itself if it had X capability. Categorise:

- **Boot-time precondition gap** — failure mode that should refuse-at-boot. File `[AUTONOMY-LEVER]` issue with proposed gate.
- **Reviewer prompt gap** — finding type the adversarial reviewer should have caught. File issue with proposed dispatch-template addition.
- **Self-improve detector gap** — observed pattern that should self-trigger but didn't.
- **CI gate gap** — drift that a `scripts/check-*.sh` script should mechanically catch.
- **Operator-decision gap** — decision the operator made that could be encoded as a rule.

For each finding write an `[AUTONOMY-LEVER]` issue with: surface, smallest implementer brief (file:line + 1-line fix), estimated operator-touch reduction.

## Hand-off summary (consolidated operator output)

After all 9 phases + A1/A2 run, emit ONE consolidated hand-back block:

```
audit-session: <N PRs merged, M issues, K subagents this session>

OPEN (operator action):
- merge: PR#<N> CLEAN+APPROVE, waiting on you
- decide: <one-line decision>

FILED (auto):
- [POST-MERGE-AUDIT] x N
- [REVIEWER #PR] x N
- [SELF-APPROVE-AFTER-AMEND] x N
- [AUTONOMY-LEVER] x N

CANDIDATES (next-session promote):
- CLAUDE.md: <slug>
- feedback_: <slug>
- learn-from-mistakes unsaved: <slug>
- dispatch-template reorder: <one-line>

HANDOFF: .claude/session-handoffs/<file>.md
NEXT-SESSION FIRST ACTION: <from frontmatter>
```

## Hard nos

- NO auto-merge any PR.
- NO auto-close any issue.
- NO auto-edit CLAUDE.md.
- NO `git gc --prune=now` or reflog-destructive cleanup.
- NO auto-invoke other skills (including learn-from-mistakes — only surface gaps).
- NO unbounded CI poll.

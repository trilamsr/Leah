# Tickets (snapshot 2026-07-09)

Migrated from Linear (Maydow team / Leah project). Tracking lives here now — Linear tickets closed with a redirect comment. 135 completed tickets omitted; recoverable from git commit `e691c3a`.

## Backlog

- **[MAY-9]** [followup] kind:feat context_transition consolidation (W126) — W124 (PR #219) scopes consolidation to the two slot-derivable cell classes: time_of_day and cadence. Spec docs/engineer/specs/2026-06-10-memory-consolidation.md §3.4 also lists context_transition. `labels: followup` `priority: 3`
- **[MAY-11]** [SESSION-HANDOFF] 2026-06-10 — pick up here — Session 2026-06-10T17:00Z → 2026-06-10T23:56Z. 25 PRs merged. Issue #156 closed. `priority: 4`
- **[MAY-14]** [SESSION-AUDIT] TDD-evidence missing PR bodies (#162 #199 #219) — Verify scripts/check-tdd-evidence.sh (PR #236) catches historical misses #162 #199 #219. Promote pr-gates to required check. `labels: audit` `priority: 3`
- **[MAY-15]** [OPERATOR] enable pr-gates as required check on main branch protection — PR #236 added GHA pr-gates job invoking check-reviewer-verdict.sh + check-tdd-evidence.sh on pull_request. Not yet required — must promote via Settings → Branches → main → Require status checks. `labels: audit` `priority: 1`
- **[MAY-17]** [SESSION-AUDIT] 0 feedback_*.md memory rules written despite 7+ recurring patterns — Audit-session Phase 7 detected recurring patterns but 0 feedback_*.md files written this session. `labels: audit` `priority: 4`
- **[MAY-18]** [SESSION-AUDIT] wave-8 impl waves NOT YET shipped: W121 W122-W123 W125-W127 W133-W134 W136 W142-W143 — Wave-8 spec chain S1-S12 fully landed + impl waves W82/W92-W94/W100/W101/W109/W116/W120/W124/W130-W132/W135/W137-W139/W141 merged. Remaining impl waves from wave-8 specs (S4 S8 S9 S10 S12) not yet shipped. `labels: followup` `priority: 2`
- **[MAY-105]** [AUTONOMY-LEVER] worktree-janitor doesn't unlock dead-pid locks — accumulates forever — 2026-06-10 session inherited *129 stale agent-* worktrees all locked by single dead pid 50819*. Janitor never pruned them because lines 30-32 of scripts/leah-worktree-janitor.sh skip on git worktree lock. `priority: 3`
- **[MAY-106]** V1 — voice instrumentation FIRST (voice_turn_seconds histogram + per-stage) — Wave-9a P0 V1 from docs/engineer/briefs/2026-06-10-wave-9-velocity-responsiveness.md. `priority: 2`
- **[MAY-109]** V4 — worktree janitor launchd plist + sweep script — Wave-9a P0 V4 from docs/engineer/briefs/2026-06-10-wave-9-velocity-responsiveness.md. `priority: 3`
- **[MAY-110]** V5 — pre-PR base-staleness gate + file-overlap detector — Wave-9a P0 V5 from docs/engineer/briefs/2026-06-10-wave-9-velocity-responsiveness.md. `priority: 3`
- **[MAY-112]** V7 — Anthropic prompt-cache (cache_control ephemeral on system block) — Wave-9a P0 V7 from docs/engineer/briefs/2026-06-10-wave-9-velocity-responsiveness.md. `priority: 3`
- **[MAY-115]** V10 — Streaming Reasoner → Streaming TTS (gated on V1 baseline) — Wave-9b P1 V10 from docs/engineer/briefs/2026-06-10-wave-9-velocity-responsiveness.md. `priority: 3`
- **[MAY-123]** O6 — audit.Logger.Subscribe(ch chan<- audit.Event) push channel — Wave-10 O6 — audit log push channel. `priority: 2`
- **[MAY-124]** O7 — Regatta event-stream (gated on cross-repo API) — Wave-10 O7 — Regatta event-stream. `priority: 4`
- **[MAY-144]** W11 — voice listener + wake skeleton — Voice-comm Wave 11 — listener + wake skeleton. `priority: 3`
- **[MAY-145]** W12 — voice session state machine + barge-in TTS cancel — Voice-comm Wave 12 — session state machine + barge-in TTS cancel. `priority: 3`

## Canceled

- **[MAY-196]** W50 — combined go.mod tidy (5 modules) — Work-tools Wave 50 — combined go.mod tidy. `priority: 3`

# Plan — learn → recommend → apply loop (W15–W19)

Companion to `docs/engineer/specs/2026-06-10-learn-recommend-apply.md`.
Five waves, each one PR-sized (~500 LOC), each lands behind a feature flag
where the surface is operator-visible. Order respects dependencies; no
wave assumes a future wave shipped.

## Wave 15 — Engine skeleton + dashboard widget

**Goal.** New `internal/recommend/` package compiles, types are stable,
Propose returns rows but nothing applies anything. Dashboard widget
renders the empty list.

**Files touched.**
- `internal/recommend/engine.go` — new, `Engine` interface + default impl.
- `internal/recommend/recommendation.go` — new, `Recommendation` + `Action`.
- `internal/recommend/registry.go` — new, adapter registry (empty in W15).
- `internal/recommend/engine_test.go` — golden-file fixtures w/ MockProfile.
- `internal/memory/schema.sql` — bump to v4; add 4 new tables.
- `internal/memory/memory.go` — new helpers `AddRecommendation`,
  `ListPendingRecommendations`, `RecordRecommendationOutcome`.
- `internal/web/` — new `recommendations` widget; GET endpoint.

**Risk.** Schema migration. Mitigation: additive only, no column rename,
`CREATE TABLE IF NOT EXISTS` so older daemons keep running.

**Size.** ~500 LOC (250 engine + 100 storage + 80 web + 70 tests).

**Test plan.**
- Hermetic Engine_Propose returns ColdStart silent on Profile.Ready=false.
- Engine_Propose returns ordered rows for a seeded mature profile (golden
  fixture).
- Schema migration test: open old db, run migrations, assert new tables
  exist + old data intact.

**Unblocks.** W16 (Apply path), W17 (confirm tier), W18 (feedback).

## Wave 16 — Auto-tier Apply path

**Goal.** Pick the safest two adapters (`FormatOnSave`, `WipePIDFile`),
implement their `Action.Apply`, wire Engine.Propose → Engine.Apply for
auto-tier only. Audit row appended on every apply.

**Files touched.**
- `internal/recommend/patterns/format_on_save.go` — new.
- `internal/recommend/patterns/wipe_pid_file.go` — new.
- `internal/recommend/registry.go` — register both.
- `internal/recommend/engine.go` — add `applyAutoTier` loop; rate-limit.
- `internal/recommend/engine_test.go` — extend.
- `cmd/leah-daemon/` — daemon tick calls Engine.Propose.

**Risk.** Rate-limit bug runs auto-apply in a loop. Mitigation: hard cap
both per-pattern (10/h) and global (50/h); tripwire 3-consecutive-Undo
demotes pattern to confirm for 30d.

**Size.** ~400 LOC.

**Test plan.**
- `TestFormatOnSave_AutoApply` — fires on `gofmt` cluster ≥10.
- `TestEngine_AutoApply_RateLimit` — 11th call/h returns ErrRateLimited.
- `TestEngine_AutoApply_TripWire` — 3 consecutive Undo demotes pattern.
- Audit row asserted: `Kind="recommendation_apply"` w/ Outcome.

**Unblocks.** W17 (confirm uses same Apply machinery).

## Wave 17 — Confirm-tier + morning brief

**Goal.** Confirm-tier adapters land (`CommitAtFocusEnd`,
`RunRetroOnFriday`). Engine.Propose queues confirm-tier into
`recommend_pending`. Morning brief reads the table + renders top 3.
Operator can Accept/Reject from dashboard → row migrates to
`recommend_history`.

**Depends on.** W10-1 (morning brief wiring — in flight).

**Files touched.**
- `internal/recommend/patterns/commit_at_focus_end.go` — new.
- `internal/recommend/patterns/run_retro_on_friday.go` — new.
- `internal/recommend/engine.go` — `Accept`, `Reject`, `Apply` for confirm.
- `internal/brief/brief.go` — new `## Recommendations` section.
- `internal/web/recommendations.go` — Accept/Reject POST handlers.
- `internal/recommend/expire.go` — new, sweeps `recommend_pending`
  rows past `ExpiresAt` → marks ignored.

**Risk.** Operator hits Accept twice (race). Mitigation: UPSERT on
`recommend_history` w/ idempotency key = pending.id.

**Size.** ~500 LOC.

**Test plan.**
- `TestEngine_Confirm_AcceptThenApply` — full path.
- `TestEngine_Confirm_Reject_NoApply` — confirms no Apply call.
- `TestBrief_RendersTop3` — golden file.
- `TestExpire_IgnoreSweep` — past-ExpiresAt → ignored audit row.

**Unblocks.** W18 (feedback signals now have all 3 outcomes to learn from).

## Wave 18 — Feedback loop + `leah forget`

**Goal.** Accept/Reject/Ignore feed back into Profile via
`recommend_feedback`. Profile.Update reads feedback table on next tick
and α-blends signal into ProfileRow.Weight before persistence. CLI gets
`leah forget <pattern|all>` subcommand.

**Files touched.**
- `internal/operatormodel/profile.go` — Update reads
  `recommend_feedback`, applies α-blend per §8.
- `internal/recommend/feedback.go` — new, `RecordSignal(pattern, signal)`.
- `internal/recommend/engine.go` — Accept/Reject/Apply call RecordSignal.
- `cmd/leah/forget.go` — new CLI subcommand; mirrors
  `cmd/leah/disconnect.go` shape.
- `internal/recommend/forget.go` — engine-side wipe logic, attested audit.

**Risk.** Feedback signal flips weight too hard, recommendations
oscillate. Mitigation: α=0.3 for explicit, 0.05 for ignore (spec §8);
golden-file regression test holds weights within [0, 5.0] across 100
simulated accept/reject cycles.

**Size.** ~450 LOC.

**Test plan.**
- `TestProfile_AlphaBlend_Convergence` — reject→accept→accept stabilizes.
- `TestForget_Pattern` — Forget("commit_at_focus_end") audits +
  pattern_state.forgotten=1.
- `TestForget_All_RequiresConfirm` — without `--yes` prompts; with `--yes`
  wipes.
- `TestCLI_Forget_DryRun` — prints diff, no DB mutation.

**Unblocks.** W19 (voice surface assumes stable feedback loop).

## Wave 19 — Voice surface

**Goal.** Voice channel reads top recommendation on "what should I do".
Operator-toggle for proactive push at focus-block boundary.

**Depends on.** W11–W14 (voice-comm pipeline — pending dispatch).

**Files touched.**
- `internal/voice/recommend_handler.go` — new, intent matcher for
  "what should I do" / "any suggestions".
- `internal/voice/proactive.go` — new, focus-block-boundary push.
- `internal/recommend/engine.go` — minor: expose `Top1(ctx)` for voice.
- `cmd/leah/config.go` — toggle `voice.proactive_recommendations=false`
  default.

**Risk.** Voice push at wrong time (during meeting). Mitigation: default
off; respect calendar busy state from `gcal.ListToday`; rate-limit to
1 push / focus block.

**Size.** ~350 LOC.

**Test plan.**
- `TestVoice_PullIntent` — "what should I do" → Top1 read aloud.
- `TestVoice_PushSuppressed_DuringMeeting` — gcal busy → silent.
- `TestVoice_PushRateLimit` — 2nd push in same focus block suppressed.

**Unblocks.** Closes the W15-W19 arc. No downstream waves planned in
MVP — observation-only continues, ML upgrades deferred per spec §14.

## Cross-wave invariants

- Every wave adds no new dependency outside the standard library +
  `modernc.org/sqlite` + `oklog/ulid/v2` (already in tree).
- Every wave's PR body cites the spec section it implements.
- No wave self-approves; reviewer subagent spawned per CLAUDE.md.
- No wave merges before its tests are golden-stable across 3 runs
  (decay math is timing-sensitive — `Now` is always injected).
- Schema migrations are additive-only across all 5 waves; no drops.

## Sequencing summary

```
W15 (engine skeleton)
  ├── W16 (auto Apply) ─┐
  ├── W17 (confirm + brief) ──┐
  └── W18 (feedback + forget) ┴── W19 (voice)
```

W16 and W17 can run in parallel after W15 lands (different files except
`engine.go`; coordinate one rebase). W18 must wait for both. W19 depends
on W11–W14 voice-comm landing.

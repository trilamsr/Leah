---
title: Leah — fast-path roadmap
status: living
owner: tri
created: 2026-06-09
---

# Leah fast-path roadmap

Living document. Updated whenever priority shifts.

## North-star

Closed loop: Leah dispatches regatta to build Leah's own next feature. Each iteration grows capability.

Closed loop unlocked when:
1. Memory exists (so Leah remembers prior decisions)
2. Self-learning exists (so Leah notices what worked/failed)
3. Context manager exists (so Leah knows which project a self-build targets)
4. Pattern recognition exists (so Leah surfaces what to self-build next)
5. Self-build dispatcher exists (so Leah can act on the surfaced candidate)
6. JARVIS UI exists (so operator sees the loop running)

All 6 are Wave 1. Once landed: closed loop runs end-to-end.

## Operator priority (locked 2026-06-09)

1. Self-learning + context management + pattern recognition + self-building via regatta — FAST PATH
2. Design specs early — every decision adversarially reviewed
3. JARVIS-like UI — minimal dashboard, no holograms (scope-cut per critic)
4. Rest of features (voice, email, calendar, finance, travel) — roadmap-later

Personal-use lens throughout. Phase X items deferred per `2026-06-09-leah-phase-x-multi-operator-roadmap.md`.

## Wave 1 — design + scaffolding

6 disjoint agents:

| Wave | Agent | Spec | Skeleton |
|---|---|---|---|
| 1-A | Memory M2 minimal | `2026-06-09-memory-m2-minimal.md` | `internal/memory/` (contact + project + decision; SQLite via modernc.org/sqlite) |
| 1-B | Self-learning loop | `2026-06-09-self-learning-personal.md` | `internal/selflearn/` (outcome resolver + mistake log + weekly retro) |
| 1-C | Context manager | `2026-06-09-context-manager.md` | `internal/ctxmgr/` (active-context + history) |
| 1-D | Pattern recognition | `2026-06-09-pattern-recognition.md` | `internal/patterns/` (audit clusterer + skill candidate emitter) |
| 1-E | Self-building via regatta | `2026-06-09-self-building-via-regatta.md` | `internal/dispatcher/selfbuild.go` + `prompts/self-build-feature.md` |
| 1-F | JARVIS UI design | `2026-06-09-jarvis-ui.md` | (deferred — spec only) |

Each agent: design → adversarial self-critique → revise if CRITICAL/HIGH → scaffold → commit local. Push handled by main thread.

## Wave 2 — wiring + first closed-loop attempt

| Step | Goal |
|---|---|
| 2-1 | Wire all CLI commands: `leah contact/project/decision/mistake/retro/ctx/patterns/self-build` into `cmd/leah/main.go` — single coordinated change to avoid collision |
| 2-2 | Schema reconciliation: collapse Wave-1 schema fragments into single `internal/memory/schema.sql` |
| 2-3 | leah-daemon wiring: add `selflearn.Resolver` + `patterns.Detect` to daemon weekly cron |
| 2-4 | JARVIS UI scaffold: vanilla HTML+JS dashboard, `internal/web/`, served by leah-daemon |
| 2-5 | First closed-loop attempt: `leah self-build "add LEAH_TRIAGE_MODEL env for Haiku triage tier"` — observe end-to-end (spec → regatta → PR → leah review → operator merge) |

## Wave 3 — proactive behaviors

| Step | Goal |
|---|---|
| 3-1 | Daily morning brief (cron-fired 8am, TTS + push, reads audit + memory + agents) |
| 3-2 | Voice push-to-talk (`leah listen` w/ whisper.cpp local — large-v3-turbo-q5_0) |
| 3-3 | Backlog command (`leah backlog [repo]` — regatta state + open issues + recent merged PRs) |
| 3-4 | Smart `leah ship --from-pr/--from-issue/--from-thread` context flags |

## Wave 4 — external adapters

Each adapter ships only when operator reports felt-pain in dogfood:

| Adapter | Trigger to build |
|---|---|
| Gmail (M5 spec) | Operator says "I'm drowning in email and Leah could help" |
| Google Calendar (M5 spec) | Operator says "I missed a meeting" |
| Slack DM (M5 spec) | Operator says "I keep checking Slack" |
| Plaid finance read-only | Operator says "I want spend awareness" |
| Travel research | Operator plans next trip |

## Wave X — never (without explicit re-evaluation)

- Multi-user / SaaS / public assistant
- Autonomous money movement
- Autonomous social-media post-send
- Autonomous email-send to non-templated recipients
- Holographic 3D UI / WebGL avatars
- iCloud Calendar write (no working Go library)
- Twitter/LinkedIn write paths (cost + ToS)

## Risk + adversarial discipline

Every spec gets adversarial-reviewer subagent BEFORE implementation. Every load-bearing PR gets independent reviewer subagent BEFORE merge. Per regatta CLAUDE.md `feedback_no_self_tagged_approve` + `feedback_adversarial_review_every_step`.

Self-build particularly: independent reviewer for SELF-BUILD PRs is the only structural defense against self-modification drift. Operator-merge mandatory; automerge BANNED on self-build PRs.

## Closed-loop validation

When Wave 2-5 runs end-to-end successfully:
1. Operator types `leah self-build "<small feature>"`
2. Leah Reasoner drafts feature spec → audit kind=self-build.dispatch
3. `gh issue create` against trilamsr/Leah → audit kind=ship
4. regatta picks up issue → opens PR
5. Leah `review` subagent reads PR diff → emits verdict → operator sees
6. Operator merges (manual)
7. CI passes
8. Next `leah-daemon` poll → audit kind=daemon.transition merged
9. Next weekly retro → "Leah shipped 1 feature this week via self-build"

If all 9 happen for one feature: closed loop closed. North-star achieved.

After closed-loop validation:
- Operator can chain `leah self-build` calls
- Pattern recognition (Wave 1-D) starts surfacing what to self-build next from operator's manual workflow
- Self-learning (Wave 1-B) starts grading self-build outcomes

This is the substrate for everything else.

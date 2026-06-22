# Stale specs audit — 2026-06-21

Specs in `docs/engineer/specs/` with zero shipped surface.
Rerun: TODO `scripts/audit-stale-specs.sh` when owner wants automation.

Method: for each spec, derive expected package from filename + first `internal/`|`cmd/` mention inside spec; mark SHIPPED (>100 LOC non-test), PARTIAL (<100 LOC or stub only), STALE (zero matching surface).

## STALE (4)

- `2026-06-10-bandit-recommender.md` — Wave-8 S4 Thompson-sampling + change-point — `internal/recommend/bandit.go` exists BUT no `changepoint` symbol anywhere; change-point detection half is unimplemented. (Bandit half shipped — split spec or close as carved.)
- `2026-06-10-reflexion-loop.md` — Reflexion + tournament review — zero `reflexion`/`tournament` matches in `internal/selflearn` or `internal/dispatcher`.
- `2026-06-10-trust-moats.md` — Wave-8 S10 operator-trust moat artifacts — zero `trustmoat`/`trust.moat`/`S10` symbol matches. `internal/connect` + `internal/knowledge` exist but neither references the moat artifact contract from the spec.
- `2026-06-10-voice-frontier.md` — Cascaded-pipeline → 2026-frontier upgrade — `internal/voice/listener/openai_realtime_decoder.go` exists (one decoder file) but no cascaded-pipeline migration; frontier upgrade contract from spec not realised.

## PARTIAL (6)

- `2026-06-10-event-timeline.md` — Event timeline schema + storage — `internal/obs/events.go` + `events_sse.go` exist (subset of timeline) but no dedicated timeline package; spec contract partially folded into `obs`.
- `2026-06-10-knowledge-graph-wiring.md` — KG wiring into recommend.Engine — `internal/knowledge` (521 LOC) + `internal/recommend` (2214 LOC) both shipped, but no direct wiring symbol bridging them (no `recommend` import of `knowledge`).
- `2026-06-10-local-self-update.md` — Local Self-Update build/swap — `cmd/leah/self_upgrade.go` 93 LOC; no `internal/selfupgrade` pkg.
- `2026-06-10-selfbuild-attestation-risk.md` — Attestation risk-tiering + sub-PR retro — `internal/selfbuildstatus/status.go` 85 LOC; risk-tiering ladder absent.
- `2026-06-10-signed-distribution.md` — Wave-8 S12 signed+notarized macOS binary — `internal/selfupgrade` ABSENT; only `cmd/leah/self_upgrade.go` stub.
- `2026-06-21-tts-bytes-seam.md` — TTS bytes seam — `internal/voice/tts.go` shipped but zero `TTSBytes`/`TTSWriter` symbol; seam refactor not landed.

## SHIPPED (31)

- `2026-06-09-gcal-adapter.md` — Google Calendar adapter — `internal/adapters/gcal` (322 LOC)
- `2026-06-09-gmail-adapter.md` — Gmail adapter — `internal/adapters/gmail` (253 LOC)
- `2026-06-10-confluence-adapter.md` — Confluence adapter — `internal/adapters/confluence` (337 LOC)
- `2026-06-10-discord-adapter.md` — Discord adapter — `internal/adapters/discord` (706 LOC)
- `2026-06-10-eval-pipeline.md` — Eval pipeline + LLM-judge — `internal/eval` (525 LOC) + `cmd/leah-eval` (133 LOC)
- `2026-06-10-facetime-adapter.md` — FaceTime adapter — `internal/adapters/facetime` (158 LOC)
- `2026-06-10-flights-adapter.md` — Flights adapter — `internal/adapters/flights` (424 LOC) + `internal/tripplanner` (546 LOC)
- `2026-06-10-hud-form-factor.md` — HUD form-factor decision — `internal/hud` (1095 LOC)
- `2026-06-10-hud-ui.md` — HUD UI operator overlay — `internal/hud` (1095 LOC) + `cmd/leah-hud`
- `2026-06-10-imessage-adapter.md` — iMessage adapter — `internal/adapters/imessage` (176 LOC)
- `2026-06-10-info-feeds.md` — Info-feeds news/weather/market — `internal/feeds` (1380 LOC)
- `2026-06-10-jira-adapter.md` — Jira adapter — `internal/adapters/jira` (408 LOC)
- `2026-06-10-knowledge-graph.md` — Cross-app entity layer — `internal/knowledge` (521 LOC)
- `2026-06-10-learn-recommend-apply.md` — Learn → recommend → apply — `internal/recommend` (2214 LOC) + `internal/operatormodel` (1121 LOC)
- `2026-06-10-linear-adapter.md` — Linear adapter — `internal/adapters/linear` (429 LOC)
- `2026-06-10-llm-ops.md` — LLM-dim observability + cost circuit — `internal/obs` (2314 LOC) + `internal/reasoner` (603 LOC) + `internal/budget` (86 LOC; thin but on-contract)
- `2026-06-10-macos-ecosystem-integration.md` — macOS ecosystem — `internal/macos` (3357 LOC)
- `2026-06-10-maps-adapter.md` — Maps adapter — `internal/adapters/maps` (1337 LOC)
- `2026-06-10-mcp-a2a-publish.md` — MCP + A2A publish — `internal/mcp` (1026 LOC, incl. `a2a_*.go`)
- `2026-06-10-memory-consolidation.md` — Dreaming pass — `internal/operatormodel/consolidate.go` (470 LOC)
- `2026-06-10-msteams-adapter.md` — Teams adapter — `internal/adapters/msteams` (406 LOC)
- `2026-06-10-notion-adapter.md` — Notion adapter — `internal/adapters/notion` (403 LOC)
- `2026-06-10-observability.md` — Observability + telemetry — `internal/obs` (2314 LOC)
- `2026-06-10-regatta-integration.md` — Regatta integration — `internal/regattaclient` (348 LOC)
- `2026-06-10-slack-adapter.md` — Slack adapter — `internal/adapters/slack` (414 LOC)
- `2026-06-10-voice-comm.md` — Voice loop — `internal/voice` (2258 LOC)
- `2026-06-10-wails-decision.md` — Wails defer ADR — `cmd/leah-hud` + `internal/hud` (1095 LOC); ADR-status decision itself is the deliverable
- `2026-06-10-whatsapp-adapter.md` — WhatsApp adapter — `internal/adapters/whatsapp` (448 LOC)
- `2026-06-21-closed-loop-validation.md` — Closed-loop validation — `internal/eval/closedloop.go` (73 LOC; small but on-contract module landed)
- `2026-06-21-comms-notifier.md` — Comms Notifier — `internal/notify` (263 LOC)
- `2026-06-21-inbound-reply-consent.md` — Inbound reply router + consent — `internal/inbound` (548 LOC)

## Decision matrix

- **STALE** → either delete the spec (if abandoned) or open a Linear ticket to ship.
  - Highest-confidence delete candidates: `trust-moats.md` (no S10 surface anywhere), `reflexion-loop.md` (no reflexion/tournament code).
  - Ship-or-cut decisions: `voice-frontier.md` (cascaded pipeline started but stalled), `bandit-recommender.md` (change-point half unshipped — split or close as carved).
- **PARTIAL** → file follow-up tickets or accept as carved-out.
  - `signed-distribution.md` + `local-self-update.md` both gated on `internal/selfupgrade` pkg — consolidate or carve.
  - `tts-bytes-seam.md` is a 2026-06-21 refactor spec; recent enough to remain in-flight.
  - `event-timeline.md` + `knowledge-graph-wiring.md` + `selfbuild-attestation-risk.md` — accept-as-carved unless reopening.
- **SHIPPED** → no action.

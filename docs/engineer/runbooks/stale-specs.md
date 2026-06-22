# Stale specs audit — 2026-06-21

Specs in `docs/engineer/specs/` with zero shipped surface.
Rerun: `bash scripts/audit-stale-specs.sh` — this runbook is mechanically reproducible from that script's output.

Method: for each spec, derive expected package from filename + first `internal/`|`cmd/` mention inside spec; mark SHIPPED (>100 LOC non-test), PARTIAL (<=100 LOC or stub only), STALE (zero matching surface).

## STALE (0)

(empty — after PR #332 added body-mention + cmd/ + scripts/ fallback heuristics, both prior STALE entries now resolve via their non-internal/ surfaces: `local-self-update` → `cmd/leah/self_upgrade.go` (659 LOC), `signed-distribution` → `scripts/release/*.sh` (209 LOC).)

## PARTIAL (1)

- `2026-06-21-comms-notifier.md` — internal/contracts (60 LOC; stub/thin)

## SHIPPED (40)

- `2026-06-10-local-self-update.md` — cmd/leah (659 LOC)
- `2026-06-10-signed-distribution.md` — scripts/release (209 LOC)

- `2026-06-09-gcal-adapter.md` — internal/adapters/gcal (322 LOC)
- `2026-06-09-gmail-adapter.md` — internal/adapters/gmail (253 LOC)
- `2026-06-10-bandit-recommender.md` — internal/recommend (2214 LOC)
- `2026-06-10-confluence-adapter.md` — internal/adapters/confluence (337 LOC)
- `2026-06-10-discord-adapter.md` — internal/adapters/discord (706 LOC)
- `2026-06-10-eval-pipeline.md` — internal/eval (525 LOC)
- `2026-06-10-event-timeline.md` — internal/obs (2314 LOC)
- `2026-06-10-facetime-adapter.md` — internal/adapters/facetime (158 LOC)
- `2026-06-10-flights-adapter.md` — internal/adapters/flights (424 LOC)
- `2026-06-10-hud-form-factor.md` — internal/hud (1095 LOC)
- `2026-06-10-hud-ui.md` — internal/hud (1095 LOC)
- `2026-06-10-imessage-adapter.md` — internal/adapters/imessage (176 LOC)
- `2026-06-10-info-feeds.md` — internal/feeds (1380 LOC)
- `2026-06-10-jira-adapter.md` — internal/adapters/jira (408 LOC)
- `2026-06-10-knowledge-graph-wiring.md` — internal/recommend (2214 LOC)
- `2026-06-10-knowledge-graph.md` — internal/brief (831 LOC)
- `2026-06-10-learn-recommend-apply.md` — internal/operatormodel (1121 LOC)
- `2026-06-10-linear-adapter.md` — internal/adapters/linear (429 LOC)
- `2026-06-10-llm-ops.md` — internal/audit (342 LOC)
- `2026-06-10-macos-ecosystem-integration.md` — internal/adapters (6329 LOC)
- `2026-06-10-maps-adapter.md` — internal/adapters/maps (1293 LOC)
- `2026-06-10-mcp-a2a-publish.md` — internal/hud (1095 LOC)
- `2026-06-10-memory-consolidation.md` — internal/operatormodel (1121 LOC)
- `2026-06-10-msteams-adapter.md` — internal/adapters/msteams (406 LOC)
- `2026-06-10-notion-adapter.md` — internal/adapters/notion (403 LOC)
- `2026-06-10-observability.md` — internal/obs (2314 LOC)
- `2026-06-10-reflexion-loop.md` — internal/dispatcher (1201 LOC)
- `2026-06-10-regatta-integration.md` — internal/regattaclient (348 LOC)
- `2026-06-10-selfbuild-attestation-risk.md` — internal/attestation (125 LOC)
- `2026-06-10-slack-adapter.md` — internal/adapters/slack (414 LOC)
- `2026-06-10-trust-moats.md` — internal/knowledge (521 LOC)
- `2026-06-10-voice-comm.md` — internal/voice (2258 LOC)
- `2026-06-10-voice-frontier.md` — internal/voice (2258 LOC)
- `2026-06-10-wails-decision.md` — internal/hud (1095 LOC)
- `2026-06-10-whatsapp-adapter.md` — internal/adapters/whatsapp (448 LOC)
- `2026-06-21-closed-loop-validation.md` — internal/audit (342 LOC)
- `2026-06-21-inbound-reply-consent.md` — internal/adapters (6329 LOC)
- `2026-06-21-tts-bytes-seam.md` — internal/voice (2258 LOC)

## How to rerun

`bash scripts/audit-stale-specs.sh` regenerates the SHIPPED/PARTIAL/STALE classification above. This runbook is mechanically reproducible from that script — any divergence is drift, not an authoritative override.

## Decision matrix

- **STALE** → either delete the spec (if abandoned) or open a Linear ticket to ship. (No STALE entries this run.)
- **PARTIAL** → file follow-up tickets or accept as carved-out.
  - `comms-notifier.md` — `internal/contracts` shipped at 60 LOC; spec contract partially realised. Accept-as-carved unless reopening the notifier surface.
- **SHIPPED** → no action.

Self-check: running `bash scripts/audit-stale-specs.sh | grep -c '^STALE'` MUST match the count in the STALE section header above. If not, runbook is drift; regenerate.

# Wave-7 Next Priorities — Post-97-Merge Backlog Drain

After 97 PRs merged this session, all 25 specs on main have been audited for shipped coverage. This brief inventories what is NOT yet shipped from existing specs, ranks by dispatch-readiness, and recommends the next 6-PR file-disjoint wave.

## Coverage audit (specs on main → ship status)

| Spec | Waves shipped | Waves remaining |
|---|---|---|
| `2026-06-09-gmail-adapter` + `gcal-adapter` | #31 | — |
| `2026-06-09-first-launch-connect` | #47 | — |
| `2026-06-10-voice-comm` | W11/W12/W13 (#84/#97/#98) | W14 backend, runbook |
| `2026-06-10-learn-recommend-apply` | W15-W19 (#89/#111/#117/#114/#119) | — |
| `2026-06-10-imessage-adapter` + `facetime-adapter` | W20/W21 (#88/#86) | — |
| `2026-06-10-macos-ecosystem-integration` + `knowledge-graph` | W25-W31m (#92/#99/#100/#105/#112/#115/#122/#138/#134/#135/#136) | daemon-wire voice→mirror→knowledge |
| `2026-06-10-hud-ui` + `info-feeds` | W34-W37 + W37b (#118/#126/#127/#128/#141) | — |
| `2026-06-10-regatta-integration` | W38/W39/W41/W42 (#131/#130/#129/#132) | **W43 cloud regatta** |
| `2026-06-10-{confluence,jira,slack,notion,linear,msteams}-adapter` | W44-W49 (#91/#104/#94/#93/#96/#101) | **W50-W55** (tidy, connect, disconnect, brief enrich, dispatcher, recs) |
| `2026-06-10-maps-adapter` + `flights-adapter` | W56-W64 (#87/#107/#113/#120/#124/#139/#137/#140) | **W60 OSRM fallback** |
| `2026-06-10-discord-adapter` + `whatsapp-adapter` | W65-W68 (#95/#108/#116/#102) | **W69-W72** (webhook, voice, multi-channel push, accept-via-reply) |
| `2026-06-10-observability` + `event-timeline` | W73 (#81), W74 (#110) | **W75-W80** (events SQLite, OTLP, /events SSE, /traces, HUD telemetry, baseline JSONL) |
| `2026-06-10-local-self-update` | — | **W81-W84** (make upgrade, brew tap, leah self-upgrade, signed release) |
| `2026-06-10-wails-decision` | ADR (#131) | DEFERRED to Wails v3 beta |

---

## Tier 1 — deliverable now (clear file-disjoint scope)

Each entry: goal | files | risk | size | test | unblocks.

### 1. W43 Cloud regatta connect flow
- **Goal:** `leah connect regatta --cloud` — operator-attestation flow for hosted regatta backend.
- **Files:** `internal/connect/regatta_cloud.go`, `internal/connect/regatta_cloud_test.go`, registry entry in `internal/connect/registry.go`.
- **Risk:** low — mirrors existing local-regatta connect; no schema change.
- **Size:** ~250 LoC + tests.
- **Test:** unit (token round-trip), `go test ./internal/connect/...`.
- **Unblocks:** cloud self-build delegation, multi-operator regatta sharing.

### 2. W50 Work-tools go.mod tidy + lib pins
- **Goal:** add Atlassian SDK (jira/confluence), slack-go, notion SDK, genqlient (linear), msgraph-sdk-go (teams) as pinned deps.
- **Files:** `go.mod`, `go.sum` (singleton; serialize against any other dep change).
- **Risk:** medium — large `go mod tidy` may surface transitive conflicts; gate on `make check`.
- **Size:** ~30 LoC manifest + lockfile churn.
- **Test:** `go build ./... && go test ./... -count=1 -short`.
- **Unblocks:** every W51-W55 work-tools wave needs these pins.

### 3. W51 leah connect — 6 work-tools
- **Goal:** extend connect registry for confluence/jira/slack/notion/linear/msteams.
- **Files:** `internal/connect/{confluence,jira,slack,notion,linear,msteams}.go` (6 new files) + `internal/connect/registry.go` entries.
- **Risk:** low — adapter shell pattern is well-established (mirrors gmail/gcal).
- **Size:** ~150 LoC each = ~900 LoC.
- **Test:** unit per adapter (token persist + redact); golden test on registry listing.
- **Unblocks:** W52, W53, W54, W55.

### 4. W52 leah disconnect — 6 work-tools
- **Goal:** mirror PR #66 disconnect pattern across all 6 work-tools.
- **Files:** `internal/connect/{...}_disconnect.go` (6 files) + CLI wiring.
- **Risk:** low.
- **Size:** ~80 LoC each = ~500 LoC.
- **Test:** disconnect → reconnect round-trip per adapter.
- **Unblocks:** clean operator-revoke flow.

### 5. W53 Morning brief work-tools enrichment
- **Goal:** top N items per work-tool surface in morning brief.
- **Files:** `internal/brief/worktools.go`, `internal/brief/worktools_test.go`, brief renderer wiring.
- **Risk:** low — additive to existing brief pipeline.
- **Size:** ~300 LoC.
- **Test:** golden brief with mock work-tools fixtures.
- **Unblocks:** Tier-1 daily-loop value-prop for work-tools.

### 6. W54 Dispatcher write-paths
- **Goal:** `leah ship --slack/--jira/--linear/--notion/--confluence/--teams`.
- **Files:** `internal/dispatcher/worktools.go` + per-tool `Ship()` impls.
- **Risk:** medium — write-paths need dry-run + idempotency.
- **Size:** ~600 LoC across 6 tools.
- **Test:** dry-run unit per tool; integration deferred (no live creds in CI).
- **Unblocks:** end-to-end work-tools loop.

### 7. W55 Work-tools recommendation engine wiring
- **Goal:** hook work-tool signals (PR/issue/msg) into `internal/recommend`.
- **Files:** `internal/recommend/worktools_signals.go` + adapter signal-emitter shims.
- **Risk:** low — additive to existing recommend pipeline (W15-W19 shipped).
- **Size:** ~250 LoC.
- **Test:** signal-emit unit + recommend golden.
- **Unblocks:** HUD recommendations for work-tools.

### 8. W60 OSM/OSRM maps fallback adapter
- **Goal:** sovereign-data maps fallback when Google Maps unavailable / disabled.
- **Files:** `internal/adapters/maps/osrm.go`, `internal/adapters/maps/osrm_test.go`.
- **Risk:** low — read-only HTTP client.
- **Size:** ~300 LoC.
- **Test:** unit (route fixture round-trip).
- **Unblocks:** offline / sovereign trip planning.

### 9. W69 WhatsApp webhook receiver
- **Goal:** local cloudflared tunnel + webhook endpoint for inbound WhatsApp.
- **Files:** `internal/adapters/whatsapp/webhook.go`, `internal/adapters/whatsapp/tunnel.go`.
- **Risk:** medium — depends on cloudflared subprocess lifecycle.
- **Size:** ~400 LoC.
- **Test:** webhook payload-parse unit; tunnel mock.
- **Unblocks:** W70, W71, W72.

### 10. W70 WhatsApp voice via ffmpeg subprocess
- **Goal:** voice in/out over WhatsApp via ffmpeg transcode.
- **Files:** `internal/adapters/whatsapp/voice.go`, integration with voice-comm (W11-W13).
- **Risk:** medium — ffmpeg dep + subprocess lifecycle.
- **Size:** ~350 LoC.
- **Test:** ffmpeg transcode unit; voice round-trip integration deferred.
- **Unblocks:** voice-first WhatsApp loop.

### 11. W71 Multi-channel daily brief push
- **Goal:** push morning brief to gmail/slack/discord/whatsapp per operator config.
- **Files:** `internal/brief/push.go`, channel routers.
- **Risk:** low — wraps existing brief output + adapter Ship() paths.
- **Size:** ~250 LoC.
- **Test:** per-channel dry-run unit.
- **Unblocks:** ambient briefing without HUD.

### 12. W72 Recommendation accept/reject via reply
- **Goal:** operator replies "yes"/"no" in any messaging channel → recommend accepts/rejects.
- **Files:** `internal/recommend/reply_handler.go`, per-channel reply-listener shims.
- **Risk:** medium — needs correlation-id round-trip per channel.
- **Size:** ~400 LoC.
- **Test:** correlation round-trip unit.
- **Unblocks:** hands-free recommendation loop.

### 13. W75-W80 Event timeline + traces (6 sub-waves)
- **W75** event SQLite — `internal/obs/events.go` (~250 LoC)
- **W76** OTLP collector + tracing — `internal/obs/traces.go` (~300 LoC)
- **W77** `/events` SSE endpoint — `internal/api/events_sse.go` (~150 LoC)
- **W78** `/traces` query endpoint — `internal/api/traces.go` (~200 LoC)
- **W79** HUD telemetry tiles — `internal/hud/telemetry_widget.go` (~250 LoC)
- **W80** baseline JSONL exporter — `internal/obs/jsonl.go` (~150 LoC)
- **Risk:** low per sub-wave (additive); coordinate file ownership.
- **Test:** unit per sub-wave + integration on W79 (renders W75-W78 data).
- **Unblocks:** deep observability for debugging cross-adapter flows.

### 14. W81-W84 Local self-update (4 sub-waves)
- **W81** `make upgrade` target — `Makefile` (~30 LoC).
- **W82** Homebrew tap repo + formula — separate `leah-tap` repo (out-of-tree); this PR creates the formula spec.
- **W83** `leah self-upgrade` CLI — `cmd/leah/upgrade.go` (~200 LoC).
- **W84** Signed-release pipeline — `.github/workflows/release.yml` (~150 LoC).
- **Risk:** medium — signing infra is new; can stub for first cut.
- **Test:** dry-run upgrade unit; signed-release smoke deferred to real release.
- **Unblocks:** zero-cost operator upgrade story.

---

## Tier 2 — depends on Tier 1 or new spec decision

15. **Voice-comm W14 backend** — Whisper-local vs OpenAI-Realtime decision needed; spec'd but unresolved. Operator-input required.
16. **Voice-comm runbook** — referenced in spec, not yet authored. Doc-only, ~200 LoC.
17. **macOS signals voice-trigger integration** — `cmd/leah-daemon` wires mirror + knowledge + voice intents into one daemon. Needs daemon-lifecycle ADR.
18. **Wails native window** — DEFERRED until Wails v3 beta (ADR shipped #131). No action.

## Tier 3 — new specs needed (not deliverable until spec lands)

19. **Personal-AI capability gaps** from research memory: browser-agent, deep research, health/finance dashboards, ambient transcription, smart-home control. Each needs its own spec PR before code.
20. **Memory expansion** — PR #58 schema-version surface is ready, but no new schema waves designed. Needs schema-roadmap spec.
21. **Self-build automation refinements** — `internal/dispatcher/selfbuild.go` post-sentinel improvements; needs incident-driven spec.
22. **Distribution** — Homebrew tap repo creation + first signed release; partially covered by W82/W84 but external repo creation is out-of-tree.

---

## Recommended next-wave dispatch (6 PRs, file-disjoint)

| Order | Wave | Primary path | Why now |
|---|---|---|---|
| 1 | **W43** cloud regatta | `internal/connect/regatta_cloud.go` | Unblocks cloud self-build |
| 2 | **W50** work-tools go.mod tidy | `go.mod` + `go.sum` (singleton) | Gates all W51-W55 |
| 3 | **W75** event-timeline package | `internal/obs/events.go` | Foundation for W76-W80 |
| 4 | **W81** make upgrade | `Makefile` (concurrent with W50 OK — make doesn't touch go.mod) | Zero risk, ships operator value |
| 5 | **W60** OSRM maps adapter | `internal/adapters/maps/osrm.go` | Sovereign-data story |
| 6 | **W76** OTLP tracing | `internal/obs/traces.go` | Pairs with W75; same observability arc |

File-disjoint matrix:
- W43 → `internal/connect/`
- W50 → `go.mod`/`go.sum` (serialized singleton)
- W75 → `internal/obs/events.go`
- W81 → `Makefile`
- W60 → `internal/adapters/maps/`
- W76 → `internal/obs/traces.go`

No two PRs touch the same file. W75/W76 both live under `internal/obs/` but on different files; ordering W75 before W76 lets W76 import the W75 event-emitter cleanly.

---

## Release notes

none (internal)

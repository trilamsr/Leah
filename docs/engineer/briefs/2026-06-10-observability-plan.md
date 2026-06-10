# Observability plan — W73–W80

Date: 2026-06-10
Parent: `docs/engineer/specs/2026-06-10-observability.md`,
        `docs/engineer/specs/2026-06-10-event-timeline.md`

Wave sequencing matches the decision priority from CLAUDE.md: UX wins
land first (operator-visible improvements), perf-debug wins second,
long-term diagnostics last.

Each wave is bounded to roughly 500 LOC of net new code (excluding
generated, vendor, tests are extra) — small enough for one PR to be
adversarially reviewed without fatigue.

## W73 — Health endpoint + per-package SelfCheck framework

**Goal**: operator can probe daemon health via `/health` and see per-
package state on the dashboard. First UX win.

**Files touched**:
- `internal/obs/health.go` (new) — `HealthRegistry`, `Status` enum,
  status-derivation helper.
- `internal/obs/health_test.go` (new).
- `internal/<pkg>/selfcheck.go` per package — memory, audit, voice,
  obs, brief, selflearn. Connect adapters defer their SelfCheck to W76
  alongside tracing wiring.
- `internal/web/server.go` — register `/health` handler.
- `internal/web/state.go` — surface package health array in
  `OpsView`.
- `internal/web/static/dashboard.html` — render package-health tile.
- `cmd/leah-daemon/main.go` — run SelfChecks at boot, populate
  `HealthRegistry`.

**Risk**: low. Read-only handler; SelfChecks are bounded by 5s
deadline.

**Size**: ~450 LOC + ~250 LOC tests.

**Test plan**: green-path + injected-failure test per SelfCheck;
`/health` handler test covering each of the three status tiers; boot-
order test asserting daemon starts even when a SelfCheck fails.

**Unblocks**: W77 dashboard timeline widget (consumes the same
`/health` for the package-health overlay); W74 (the `/metrics`
endpoint is registered on the same listener).

## W74 — `/metrics` Prometheus endpoint

**Goal**: operator can scrape Leah with any Prometheus-compatible
tool. Perf-debug win.

**Files touched**:
- `internal/obs/prometheus.go` (new) — exposition-format writer over
  the existing `Registry`. No new client dep; the exposition format is
  plain text.
- `internal/obs/prometheus_test.go` (new) — parse the output with a
  Prometheus text-format parser in tests.
- `internal/web/server.go` — register `/metrics` handler.
- `internal/web/server_test.go` — endpoint integration test via
  `httptest`.

**Risk**: low. No state mutation; pure read of the existing registry.

**Size**: ~350 LOC + ~200 LOC tests.

**Test plan**: format-conformance test (output is parseable by
`expfmt`-equivalent); concurrent-scrape test (10 concurrent GETs +
ongoing observers); benchmark `/metrics` against 5000 series.

**Unblocks**: W80 metric backfill — every new metric automatically
exposes via this endpoint.

## W75 — Event timeline package

**Goal**: structured event emission begins flowing for the top-N
critical paths (dispatcher, attestation, audit, voice).

**Files touched**:
- `internal/obs/event.go` (new) — `Event` struct, `EmitEvent`,
  `WithRefID`, `RefID`, `NewRefID`, `Query`, `EventQuery`,
  `SafeDetail`.
- `internal/obs/event_store.go` (new) — SQLite writer goroutine,
  schema bootstrap, prune scheduler.
- `internal/obs/event_test.go` (new) — round-trip, drop-on-full,
  retention.
- Emission wiring (small touches):
  - `internal/dispatcher/ship.go` — emit `dispatch.ship` begin +
    complete.
  - `internal/attestation/gate.go` — emit `attestation.attempt`.
  - `internal/audit/audit.go` — emit `audit.append`.
  - `internal/voice/chain.go` — emit `voice.speak`, `voice.fallback`.

**Risk**: medium. SQLite writer goroutine is new infrastructure;
drop semantics must be solid.

**Size**: ~500 LOC + ~300 LOC tests.

**Test plan**: per-Kind round-trip; causal-chain query; drop-on-full
asserts non-blocking; benchmark `EmitEvent` against the < 1µs budget;
crash-recovery test (SIGKILL between batches → reopen → assert
durable rows still present).

**Unblocks**: W76 OTel integration (events ride alongside spans);
W77 SSE endpoint (queries `Query` directly).

## W76 — OpenTelemetry integration + in-process collector

**Goal**: span tree for every top-level operation.

**Files touched**:
- `go.mod` — add `go.opentelemetry.io/otel`,
  `go.opentelemetry.io/otel/sdk`,
  `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp`.
- `internal/obs/tracer.go` (new) — `Init(ctx) (shutdown func(), err)`
  setting up OTLP HTTP exporter to `127.0.0.1:4318` + in-process
  collector.
- `internal/obs/collector.go` (new) — in-memory trace store, LRU
  eviction at 4096 traces.
- `internal/web/server.go` — register `/traces/{id}` handler.
- Span wrapping (small touches):
  - `internal/dispatcher/ship.go`
  - `internal/attestation/gate.go`
  - `internal/reasoner/call.go`
  - `internal/audit/audit.go`
  - `internal/memory/store.go`
  - `internal/voice/chain.go`
  - `internal/connect/*/client.go`
- `internal/connect/*/selfcheck.go` — SelfCheck implementations
  deferred from W73 land here once tracing is available for token-age
  assertions.

**Risk**: medium. Vendor SDK adds dep weight; sampler config must
default safely.

**Size**: ~500 LOC + ~250 LOC tests.

**Test plan**: span tree assembly with synthetic spans; LRU eviction
test; sample-rate config test (0% disables); `/traces/{id}`
endpoint test with a 3-span fixture.

**Unblocks**: dashboard "Recent operations" widget.

## W77 — `/events` SSE endpoint + dashboard timeline widget

**Goal**: live event stream feeds the dashboard. Visible UX.

**Files touched**:
- `internal/web/events.go` (new) — SSE handler + JSON-lines handler
  consuming `obs.Query`.
- `internal/web/events_test.go` (new).
- `internal/web/static/dashboard.html` — timeline widget (CSS grid;
  no framework dep; one column per actor; rows scroll horizontally).
- `internal/web/static/dashboard.js` — SSE client; reconnect on
  close.

**Risk**: low. Read-only stream; backpressure handled by closing the
SSE connection when the client falls behind.

**Size**: ~400 LOC + ~200 LOC tests.

**Test plan**: SSE round-trip test; `?since` filter test;
backpressure test (slow consumer → asserted disconnect after buffer
high-water mark).

**Unblocks**: W78 HUD ambient telemetry.

## W78 — HUD ambient telemetry tiles

**Goal**: HUD panel from PR #71 surfaces live metric tiles
(`leah_subagent_active`, `leah_dispatcher_active`,
`leah_daemonloop_last_tick_age_seconds`, today's budget consumption).

**Depends on**: HUD W34 from `docs/engineer/specs/2026-06-10-hud-ui.md`
(PR #71 merged) shipping the ambient-panel infrastructure.

**Files touched**:
- `internal/hud/telemetry.go` (new) — consumes `obs.Registry`
  snapshots + the `/events` SSE stream.
- `internal/hud/telemetry_test.go` (new).
- HUD UI bindings (Go ↔ Web per PR #71) — wire the new telemetry
  channel.

**Risk**: low.

**Size**: ~350 LOC + ~150 LOC tests.

**Test plan**: golden-frame test asserting the rendered tile JSON
matches a known registry state.

**Unblocks**: nothing downstream — terminal leaf of the UX track.

## W79 — Makefile feedback-loop targets

**Goal**: `make dev`, `make verify-pr`, `make baseline`.

**Files touched**:
- `Makefile` — three new targets.
- `scripts/dev.sh` (new) — orchestrates dev daemon + browser + tail.
- `scripts/verify-pr.sh` (new) — fetches PR head SHA, runs
  `scripts/check.sh` on it in a throwaway worktree.
- `scripts/baseline.sh` (new) — runs the full test + bench matrix,
  appends to `~/.leah-state/baseline-history.jsonl`.

**Risk**: low. Pure tooling; no production code.

**Size**: ~250 LOC of shell.

**Test plan**: shellcheck on each script; smoke-test `make verify-pr
PR=<self>` in CI on a feature branch.

**Unblocks**: nothing technical; operator ergonomic win.

## W80 — Metric inventory backfill

**Goal**: ship the remaining ~30 metrics enumerated in observability
spec §4 across packages that don't yet emit them.

**Files touched**: every package §4 names.

**Risk**: low per-touch. Cumulative reviewer fatigue managed by
splitting this wave into per-package PRs labelled `W80a`, `W80b`, etc
when each PR exceeds 500 LOC.

**Size**: ~600 LOC + ~400 LOC tests across the family.

**Test plan**: per-metric "observe once → assert registry incremented"
unit test.

**Unblocks**: nothing — completes the visible-signal surface.

## Sequencing notes

- W73 → W74 share the same HTTP listener registration; W74 lands after
  W73 to keep the diffs small.
- W75 (events) is independent of W74 (metrics) and could parallelize
  if reviewer capacity permits.
- W76 (tracing) MUST follow W75 because events carry trace IDs.
- W77 (SSE) MUST follow W75 (it reads from the event store).
- W78 (HUD) blocked on W77 + on HUD W34 (PR #71) landing.
- W79 has no code dependencies; can land at any point. Slot it after
  W73 so `make verify-pr` is available during the rest of the wave's
  PR-reviewing work.
- W80 is the cleanup wave — runs after all infrastructure waves land.

## Risks + mitigations

| Risk                                              | Mitigation                                     |
|---------------------------------------------------|------------------------------------------------|
| SQLite write contention on `events.db`            | WAL mode + single writer goroutine + batching |
| OTel SDK adds significant binary size             | Build-tag the exporter; bench binary size in W76 |
| PII leak via event `Detail` field                 | `SafeDetail` helper + PR review checklist     |
| Dashboard polling overwhelms `/events` stream     | SSE backpressure → server-side disconnect      |
| Loopback-only bind regression                     | Boot-time assertion; unit test in W73          |
| Reviewer fatigue on W80 cleanup                   | Split into per-package mini-PRs               |

# Observability + telemetry — Leah daemon

Date: 2026-06-10
Scope: MVP-5
Owners: Leah daemon (in-process), operator dashboard (local HTTP)

Companion docs:
- `docs/engineer/specs/2026-06-10-event-timeline.md` — structured event schema
- W73–W80 rollout: Linear MAY-219..MAY-226 (https://linear.app/themaydow/project/leah-a8d553e8cc88)

## 1. Goal

Leah's operator must see every behavioral signal that matters — command
rate, latency, error rate, agent-loop depth, attestation rate, audit-write
rate, memory growth, token-budget consumption, per-adapter call counts,
per-subagent task duration — in real time and across history. No blind
spots. The operator sees what Leah did, how long it took, whether it
succeeded, and what it cost.

Spec covers MVP-5 (v1) only. Out-of-scope items in §12.

## 2. Layers

Four layers, each answering a different question:

1. **Metrics** — counters / gauges / histograms. Question: "what is the
   rate, the distribution, the current value?" Already partly built in
   `internal/obs/metrics.go` (PRs #46, #56). Spec extends inventory.
2. **Structured events** — typed Go structs serialized to SQLite. Richer
   than `audit.jsonl` (which is operator-facing); events are
   developer-facing diagnostics. Question: "what happened, in order, with
   what causal chain?" See companion event-timeline spec.
3. **Distributed tracing** — within-daemon spans + cross-subagent
   propagation via OpenTelemetry Go SDK. Question: "where in the call
   tree was the time spent?"
4. **Logs** — existing `internal/obs/logger.go` `dailyRotator` (PR #60).
   Question: "what did the daemon say, in human-readable form?"
5. **Health checks** — per-package `SelfCheck(ctx) error`. Question: "is
   each subsystem in working order right now?"

All five MUST ship. They are not interchangeable; an operator without
events cannot reconstruct causality, an operator without metrics cannot
spot rate spikes, an operator without traces cannot see latency
breakdown.

## 3. Endpoint surface

The daemon exposes a single localhost HTTP listener (already bound at
127.0.0.1 by the dashboard / HUD pattern from PR #71). New routes:

| Path                | Method | Purpose                                                    |
|---------------------|--------|------------------------------------------------------------|
| `/metrics`          | GET    | Prometheus exposition format scrape                        |
| `/events`           | GET    | JSON-lines or SSE stream of structured events; `?since=ts` |
| `/traces/{id}`      | GET    | Trace tree (JSON) for a single operation ref-id            |
| `/health`           | GET    | Daemon health JSON (see §7)                                |
| `/debug/pprof/*`    | GET    | Go runtime pprof (register `net/http/pprof` on listener)   |

All bound `127.0.0.1` only. No remote network exposure. No auth required
for local listener — the same trust model as the dashboard `/api/state`.

Operator opts in to the observability listener via attested scope
`connect:observability` at first launch (mirrors the connect-scope
pattern from `docs/engineer/specs/2026-06-09-gmail-adapter.md`).

## 4. Metrics inventory

The inventory below is the canonical list. Every metric MUST be
registered against `internal/obs.Registry` (the in-process registry from
PR #46). Histogram bucket bounds are package-author's choice but SHOULD
be doubling-series in the latency-of-interest band.

Naming convention: `leah_<package>_<noun>_<unit>` for metrics whose name
encodes the unit (`_seconds`, `_bytes`, `_total`). Label cardinality is
budgeted — labels are bounded enums, never free text. Operator message
text, file paths beyond the leah-state dir, and PII MUST be hashed if
they appear as label values.

### 4.1 Dispatcher (`internal/dispatcher`)

| Metric                                  | Type      | Labels             |
|-----------------------------------------|-----------|--------------------|
| `leah_dispatcher_ship_total`            | counter   | `outcome`          |
| `leah_dispatcher_ship_latency_seconds`  | histogram | `outcome`          |
| `leah_dispatcher_review_total`          | counter   | `outcome`          |
| `leah_dispatcher_active`                | gauge     | (none)             |

`outcome` enum: `ok`, `review_failed`, `merge_conflict`, `aborted`.

### 4.2 Attestation (`internal/attestation`)

| Metric                                | Type     | Labels                |
|---------------------------------------|----------|-----------------------|
| `leah_attestation_attempt_total`      | counter  | `scope`, `outcome`    |
| `leah_attestation_denied_total`       | counter  | `scope`               |
| `leah_attestation_granted_seconds`    | histogram| `scope`               |

`scope` is the attested scope name (`connect:gmail`, `merge:pr`, …).
Bounded by the attestation scope enum.

### 4.3 Audit (`internal/audit`)

Existing (PR #53):
- `leah_audit_append_total` — counter
- `leah_audit_append_errors_total` — counter
- `leah_audit_parse_errors_total` — counter, label `source`

Add: `leah_audit_append_latency_seconds` (histogram).

### 4.4 Daemon loop (`internal/daemonloop`)

| Metric                                   | Type      | Labels        |
|------------------------------------------|-----------|---------------|
| `leah_daemonloop_tick_total`             | counter   | `outcome`     |
| `leah_daemonloop_tick_latency_seconds`   | histogram | (none)        |
| `leah_daemonloop_last_tick_age_seconds`  | gauge     | (none)        |
| `leah_daemonloop_weekly_fired_total`     | counter   | `task`        |

`last_tick_age_seconds` is updated by a 1-Hz updater that reads
`Loop.LastTick.Load()` (PR #45 already exposes it) and writes `now -
LastTick` into the gauge. Operator dashboard threshold at 5m → daemon
considered degraded.

### 4.5 Memory (`internal/memory`)

| Metric                                | Type      | Labels        |
|---------------------------------------|-----------|---------------|
| `leah_memory_db_size_bytes`           | gauge     | (none)        |
| `leah_memory_query_latency_seconds`   | histogram | `op`          |
| `leah_memory_contacts_total`          | gauge     | (none)        |
| `leah_memory_projects_total`          | gauge     | (none)        |
| `leah_memory_decisions_total`         | gauge     | (none)        |

`op` enum: `list_contacts`, `list_projects`, `list_decisions`, `upsert`,
`delete`, `search`.

### 4.6 Voice (`internal/voice`)

| Metric                              | Type      | Labels                  |
|-------------------------------------|-----------|-------------------------|
| `leah_voice_speak_total`            | counter   | `backend`, `outcome`    |
| `leah_voice_speak_latency_seconds`  | histogram | `backend`               |
| `leah_voice_synth_concurrent`       | gauge     | (none)                  |
| `leah_voice_chain_fallback_total`   | counter   | `from`, `to`            |

`backend` enum: `kokoro`, `openai`, `say`. Latency budget per stage is
defined in `docs/engineer/specs/2026-06-10-voice-comm.md`; the histogram
buckets MUST cover that budget band.

### 4.7 Connect adapters (`internal/connect/*`)

| Metric                              | Type      | Labels                  |
|-------------------------------------|-----------|-------------------------|
| `leah_connect_token_age_seconds`    | gauge     | `provider`              |
| `leah_connect_exchange_total`       | counter   | `provider`, `outcome`   |
| `leah_connect_refresh_total`        | counter   | `provider`, `outcome`   |
| `leah_connect_api_call_total`       | counter   | `provider`, `endpoint`  |
| `leah_connect_api_latency_seconds`  | histogram | `provider`              |

`provider` enum: `gmail`, `gcal`, `imessage`, `facetime`, future
adapters. `endpoint` is bounded per provider (typed in adapter package).

### 4.8 Brief (`internal/brief`)

| Metric                              | Type      | Labels        |
|-------------------------------------|-----------|---------------|
| `leah_brief_render_latency_seconds` | histogram | (none)        |
| `leah_brief_gmail_unread_count`     | gauge     | (none)        |
| `leah_brief_gcal_today_count`       | gauge     | (none)        |
| `leah_brief_compose_failures_total` | counter   | `stage`       |

### 4.9 Self-learning (`internal/learn`)

Existing (PR #44): `leah_selflearn_dangling_selfbuilds_total`.

Add:
- `leah_selflearn_resolve_total` (counter, label `outcome`)
- `leah_selflearn_resolve_latency_seconds` (histogram)

### 4.10 Observability self-metrics (`internal/obs`)

Existing (PRs #56, #62):
- `leah_obs_snapshot_latency_seconds`
- `leah_obs_captureStack_total`
- `leah_panic_total` (label `name`)

Add:
- `leah_obs_emit_event_total` (counter, label `kind`)
- `leah_obs_emit_event_dropped_total` (counter, label `reason`)
- `leah_obs_event_queue_depth` (gauge)

### 4.11 Subagents (`internal/subagent`)

| Metric                                  | Type      | Labels             |
|-----------------------------------------|-----------|--------------------|
| `leah_subagent_active`                  | gauge     | (none)             |
| `leah_subagent_spawned_total`           | counter   | `role`             |
| `leah_subagent_completed_total`         | counter   | `role`, `status`   |
| `leah_subagent_duration_seconds`        | histogram | `role`             |
| `leah_subagent_slot_wait_seconds`       | histogram | (none)             |

`role` enum: `designer`, `implementer`, `reviewer`, `investigator`.
`status` enum: `ok`, `failed`, `timeout`, `cancelled`.

`leah_subagent_active` is the gauge consumed by the HUD ambient panel
per PR #71 spec.

### 4.12 Reasoner (`internal/reasoner`)

| Metric                              | Type      | Labels                                |
|-------------------------------------|-----------|---------------------------------------|
| `leah_reasoner_call_total`          | counter   | `model`, `prompt_sha`, `outcome`      |
| `leah_reasoner_tokens_total`        | counter   | `model`, `prompt_sha`, `direction`    |
| `leah_reasoner_egress_bytes_total`  | counter   | `model`, `kind`                       |
| `leah_reasoner_latency_seconds`     | histogram | `model`, `kind`                       |
| `leah_reasoner_retries_total`       | counter   | `model`, `reason`                     |

`direction` enum: `input`, `output`. `outcome` enum: `ok`, `error`,
`budget_blocked`, `breaker_denied`. `prompt_sha` widens the two reasoner
series in W92 (PromptRegistry) — soft break for Prometheus consumers,
cardinality budget in §4.14 accommodates the widened series.

### 4.13 Recommendation engine (`internal/recommendation`)

Per `docs/engineer/specs/2026-06-10-learn-recommend-apply.md` (PR #69):

| Metric                                  | Type    | Labels       |
|-----------------------------------------|---------|--------------|
| `leah_recommendation_proposed_total`    | counter | `kind`       |
| `leah_recommendation_accepted_total`    | counter | `kind`       |
| `leah_recommendation_rejected_total`    | counter | `kind`       |
| `leah_recommendation_applied_total`     | counter | `kind`, `outcome` |

### 4.14 Budget label cardinality

Total expected unique label-value combinations across all metrics:
~3000–5000 pre-LLM-dim, raised to ≈1 650 LLM-dim series alone after the
W92/W93/W94 reasoner widening (`prompt_sha` × `model` × `outcome` on
`leah_reasoner_call_total` dominates). The flatten-key cache in
`internal/obs/metrics.go` (PR #56 memoization) is sized for ≤10³ entries
per series — this budget keeps the cache hit rate high. Any new metric
whose labels could exceed a few hundred values MUST hash, bucket, or
drop the high-cardinality dimension. `prompt_sha` is truncated to 16
hex chars (~10⁴ unique values bound) — see llm-ops.md §14 carve-out.

### 4.15 LLM cost (`internal/costmonth` + `internal/reasoner`)

Per `docs/engineer/specs/2026-06-10-llm-ops.md` §3 (W94 lands the
breaker + monthly gauge; W95 lands the HUD tile):

| Metric                              | Type    | Labels                       |
|-------------------------------------|---------|------------------------------|
| `leah_cost_month_dollars`           | gauge   | `kind`                       |
| `leah_cost_breaker_state`           | gauge   | (none) — 0 ok / 1 warn / 2 deny |
| `leah_cost_breaker_degrade_total`   | counter | `from_model`, `to_model`     |

State transitions are operator-visible: green/amber/red HUD tile maps
directly to the `leah_cost_breaker_state` gauge value.

## 5. Structured event stream

Full schema in companion `event-timeline.md`. Summary:

- Schema struct in `internal/obs/event.go`:
  ```
  type Event struct {
      TS        time.Time
      Kind      string  // bounded enum, see event-timeline.md
      Actor     string  // "daemon", "subagent:<role>", "operator"
      Target    string  // package or external system touched
      Scope     string  // attestation scope when applicable
      LatencyMS int64
      Outcome   string  // "ok", "error", "denied", "timeout"
      RefID     string  // trace correlation across emit sites
      Detail    string  // short free-text; PII MUST be hashed
  }
  ```
- Storage: SQLite at `~/.leah-state/events.db` (mode `0600`), single
  `events` table, indexed `(ts)` + `(ref_id)` + `(kind, ts)`.
- Retention: 30 days, operator-configurable. Vacuum + prune runs daily
  inside `daemonloop` weekly-style tick.
- Write path: `obs.EmitEvent(ctx, Event)`. Non-blocking — pushes into a
  buffered channel (capacity 1024) consumed by a single daemon-owned
  writer goroutine. Drops increment
  `leah_obs_emit_event_dropped_total{reason="queue_full"}`.
- Read path: `obs.Query(ctx, EventQuery)` + the `/events` HTTP endpoint.

## 6. Distributed tracing

### 6.1 SDK choice

OpenTelemetry Go SDK (`go.opentelemetry.io/otel`,
`go.opentelemetry.io/otel/sdk`, `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp`).

Rationale:
- Vendor-agnostic; future operators can point at any OTLP backend.
- Apache 2.0; safe license.
- Industry standard — adopt over building a span library.

Decision priority (per README.md): adopt > build.

### 6.2 Span coverage

Spans wrap every:

- HTTP handler (`/api/state`, `/metrics`, `/events`, `/traces/{id}`,
  `/health`)
- Subagent dispatch (`subagent.Run` enters, child spans for each tool
  invocation)
- Attestation gate (`attestation.Attempt` returns OK or denied)
- Reasoner call (`reasoner.Complete`)
- Audit append (`audit.Logger.Append`)
- Memory query (`memory.Store.ListContacts`, `.ListProjects`,
  `.ListDecisions`, `.Upsert*`, `.Search`)
- Voice synthesis (`voice.Chain.Speak` + per-backend leaf spans)
- Connect-adapter API call (`gmail.Client.Do`, etc.)

Span names follow `<package>.<func>` (e.g. `dispatcher.Ship`,
`memory.Search`). Span attributes mirror the metric labels where they
overlap so an operator filtering by attribute gets the same partition
as filtering by label.

### 6.3 Exporter

OTLP HTTP exporter targets the daemon-internal in-process collector on
`127.0.0.1:4318` (the OTLP HTTP default port). The in-process collector
keeps spans in memory keyed by `trace_id`, evicts on LRU when the
configured cap (default 4096 traces) is hit, and serves `/traces/{id}`.

No external collector required for MVP-5.

### 6.4 Sampling

Default sample rate is operator-configurable:
- Dev (`LEAH_ENV=dev`): 100%.
- Prod (default): 10%.
- Privacy max (`LEAH_OBS_SAMPLE_RATE=0`): 0% — disables tracing
  entirely.

Sampling is parent-based — once a trace is sampled at root, every child
span is recorded for that trace.

### 6.5 Dashboard widget

Dashboard renders a "Recent operations" widget pulling the last N
traces from the in-process collector. Click → expands into a flame-graph
visualization (single-file vanilla JS, no framework dep — matches the
existing dashboard aesthetic).

## 7. Health endpoint

`GET /health` → 200 OK with JSON body:

```json
{
  "status": "ok",
  "uptime_seconds": 12345,
  "last_tick_age_seconds": 3,
  "memory_db_size_bytes": 4198400,
  "subagent_slots": {"active": 2, "max": 6, "free": 4},
  "audit_buffer_depth": 0,
  "event_queue_depth": 12,
  "package_health": {
    "memory":  "ok",
    "audit":   "ok",
    "voice":   "ok",
    "connect": "ok",
    "obs":     "ok",
    "brief":   "ok",
    "selflearn": "ok"
  },
  "version": "v0.5.0-W72",
  "started_at": "2026-06-10T08:00:00Z"
}
```

Top-level `status` derivation:
- `ok` — every package self-check passed AND `last_tick_age_seconds <
  300`.
- `degraded` — any package self-check fails OR `last_tick_age_seconds`
  in `[300, 900)`.
- `unhealthy` — daemon cannot write audit OR `last_tick_age_seconds >=
  900` OR 3+ packages degraded.

HTTP status is always 200 — operators / probes parse the JSON `status`
field. Returning non-200 would prevent the response from being read by
dumb HTTP probes (which is the worst-case-failure path).

## 8. Per-package self-test

Every package owning durable state, network connections, or external
processes MUST export:

```go
func SelfCheck(ctx context.Context) error
```

Implementations (MVP-5 set):

- `memory.SelfCheck` — open read connection, run `PRAGMA
  integrity_check`, assert result is `"ok"`.
- `audit.SelfCheck` — append a synthetic entry with
  `Kind="audit.selfcheck"`, scan back, assert round-trip equality. Synthetic
  entries are tagged with `Outcome="selfcheck"` so downstream readers
  can filter them out.
- `voice.SelfCheck` — call `voice.Chain.Available()`; report first
  backend status. Does NOT synthesize speech (would be noisy).
- `obs.SelfCheck` — `EmitEvent` a test event with
  `Kind="obs.selfcheck"`, `Query` it back, assert round-trip.
- `connect.SelfCheck` (per provider) — assert token-age < refresh
  threshold; do NOT make an API call (rate-limit hostile).
- `brief.SelfCheck` — render an empty brief against fake fixtures;
  assert render < 50ms.
- `selflearn.SelfCheck` — assert resolver index loadable from
  `~/.leah-state/selflearn/`.

Daemon boot runs all `SelfCheck`s in parallel (bounded to GOMAXPROCS)
with a 5s deadline per check. Results are stored in `obs.HealthRegistry`
(in-memory) and feed `/health`.

A package check failing on boot does NOT prevent daemon start —
operator must be able to see the dashboard in order to fix the failed
package. Failed packages are surfaced in red on the dashboard
"Operations" tile.

## 9. Tighter feedback loop ergonomics

Three Makefile targets dedicated to operator workflow:

### 9.1 `make dev`

- Launches the daemon against `~/.leah-state-dev/` (isolated state dir).
- Opens browser to `http://127.0.0.1:8080/dashboard`.
- Opens `tail -f ~/.leah-state-dev/audit.jsonl` in an adjacent pane (via
  `osascript` on darwin / `tmux split-window` when in tmux; otherwise
  prints the command for the operator to paste).

### 9.2 `make verify-pr PR=<N>`

- Fetches the PR's head SHA via `gh pr view <N> --json headRefOid`.
- Checks out the SHA in a throwaway worktree under `.claude/verify/<N>/`.
- Runs `./scripts/check.sh` locally on that SHA.
- Reports pass/fail.

Use case: replaces "trust but verify" of subagent-reviewer reports with
operator-runnable verification on the host.

### 9.3 `make baseline`

- Runs full `go test -race ./...`.
- Runs every `go test -bench=. -benchtime=2s ./...` benchmark.
- Appends one row to `~/.leah-state/baseline-history.jsonl` with
  per-package pass/fail + per-benchmark ns/op.
- Used to detect performance regressions between waves.

## 10. Threat model

- `/metrics`, `/events`, `/traces/{id}`, `/health`, `/debug/pprof/*` are
  bound `127.0.0.1` only. The HTTP server explicitly rejects bind
  addresses outside the loopback range (`net.IP.IsLoopback`).
- Event-stream payloads NEVER contain operator message text or secrets.
  Free-text `Detail` field MUST be either bounded enum or hash. Emit
  sites are reviewed at PR time for PII leaks (added to PR checklist).
- Token strings, OAuth refresh tokens, API keys, audio buffers MUST NOT
  appear in metric label values, span attributes, or event Detail fields.
- pprof endpoint serves Go runtime data including goroutine stacks —
  loopback-only is non-negotiable.
- Sampling rate is operator-configurable; an operator can set
  `LEAH_OBS_SAMPLE_RATE=0` for max privacy (zero spans recorded).
- Event SQLite file is mode `0600`. Backup tooling reads it at the same
  privilege as the daemon.

## 11. Test plan

Unit:
- Every metric emitter has a test that observes once + asserts the
  registry counter incremented by 1.
- Every event emitter has a test that emits → queries → asserts
  round-trip.
- `SelfCheck` per package has a green-path test + at least one failure
  injection test.
- `/health` endpoint test against a synthesized registry covering each
  of the 3 status tiers.
- `/metrics` exposition test against a populated registry; assert valid
  Prometheus text format (parsed by a Prometheus text-format parser in
  tests).
- `/events` SSE test using `httptest` + a 2-event fixture; assert
  ordering + `?since` filter.
- `/traces/{id}` test using a synthesized 3-span trace.

Integration:
- Trace-tree assembly test: emit synthetic spans with a shared
  `trace_id` from multiple goroutines; assert the in-process collector
  builds the same parent-child tree as the OTel SDK exporter consumed.
- Event-emission race test: 100 goroutines each emit 100 events; assert
  zero drops, ordering preserved per-actor.
- Loopback-only bind test: try to bind on a non-loopback IP; assert
  daemon refuses to start.

Performance:
- Benchmark `obs.EmitEvent` — target < 1µs per call when queue not
  full. Drops MUST NOT block the caller (channel send is non-blocking).
- Benchmark `Registry.Snapshot` with 5000 series — target < 10ms (the
  PR #56 baseline).
- Benchmark `/metrics` exposition with 5000 series — target < 50ms.

## 12. Out of scope (MVP-5)

- Remote OTLP exporters (Jaeger, Honeycomb, Tempo). Operator runs
  local-only for MVP. Adding a remote exporter post-MVP is a
  one-config-line change because OTel is the SDK.
- Long-term metric storage / Prometheus federation. Local SQLite +
  30-day retention is sufficient at single-operator scale.
- Alert routing (PagerDuty / Opsgenie / Slack). Defer until a real
  recurring alert pattern emerges from operator usage.
- Anomaly detection (ML-based). The recommendation engine
  (`docs/engineer/specs/2026-06-10-learn-recommend-apply.md`, PR #69)
  is the eventual home for this.

## 13. Decision-priority summary

Per `README.md` § House rules: UX > performance > long-term benefits.

- UX win: dashboard widgets (health tile, recent-operations flame-graph,
  live metric tiles) — operator-facing. Sequenced first in the W73–W80
  plan.
- Performance win: `/metrics` Prometheus endpoint + event-emission
  microbenchmarks — diagnostic perf debugging.
- Long-term win: distributed tracing + recommendation-engine hook.
  Sequenced last.

Adopt-over-build:
- OpenTelemetry Go SDK (industry standard).
- Prometheus exposition text format (industry standard, no client lib
  needed — exposition is plain text).
- SQLite (already used by `memory` + `regatta`).

Default-simpler:
- Single localhost listener, no auth.
- In-process trace collector, no external dep.
- 30-day retention via daily prune, no compaction tier.

## 14. Open questions

- Should `/events` be SSE or JSON-lines? SSE has built-in reconnection;
  JSON-lines is simpler to consume from `curl`. Implementation may
  ship both: SSE at `/events` and JSON-lines at `/events.jsonl`.
- Span sampler — head-based vs tail-based? Head-based is simpler and
  matches OTel default. Tail-based would let us "always sample errors"
  but adds a buffering tier. Defer tail-based to post-MVP.
- pprof endpoint behind a build tag? Recommended `yes` for the prod
  binary to keep the attack surface minimal even on loopback. Decision
  deferred to implementation PR.

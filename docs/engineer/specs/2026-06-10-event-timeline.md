# Event timeline — schema + storage

Date: 2026-06-10
Scope: MVP-5
Parent: `docs/engineer/specs/2026-06-10-observability.md`

## 1. Goal

Structured-event stream complementing `audit.jsonl` (operator-facing)
with a developer-facing, type-safe, causally-linked event log. Every
significant control-flow transition in the daemon emits one row.
Replay-able timeline reconstructs "what Leah did, in what order, in
what causal chain".

`audit.jsonl` answers "what did Leah do that the operator might want to
review later?" The event timeline answers "what did the daemon do
internally, including the failed and denied paths, and how do those
events causally relate to each other?"

## 2. Schema

### 2.1 Go struct

`internal/obs/event.go`:

```go
type Event struct {
    TS        time.Time   `json:"ts"`
    Kind      string      `json:"kind"`
    Actor     string      `json:"actor"`
    Target    string      `json:"target,omitempty"`
    Scope     string      `json:"scope,omitempty"`
    LatencyMS int64       `json:"latency_ms,omitempty"`
    Outcome   string      `json:"outcome"`
    RefID     string      `json:"ref_id,omitempty"`
    Detail    string      `json:"detail,omitempty"`
    Payload   interface{} `json:"payload,omitempty"`
}
```

`Payload` is transport-only and carries structured per-kind data (e.g. the
`HUDStateEvent` shape consumed by `ambient.js` on the `hud.state` channel).
SQLite persistence ignores it so the row schema stays narrow.

Field semantics:

| Field      | Required | Type / values |
|------------|----------|---------------|
| `TS`       | yes      | UTC, RFC3339 with millisecond precision when serialized |
| `Kind`     | yes      | Bounded enum (see §3) |
| `Actor`    | yes      | `"daemon"`, `"subagent:<role>"`, `"operator"` |
| `Target`   | no       | Package or external system touched (e.g. `"gmail"`, `"memory"`, `"dispatcher"`) |
| `Scope`    | no       | Attestation scope; required when `Kind` starts with `attestation.` or `connect.` |
| `LatencyMS`| no       | Duration of the operation in milliseconds; 0 for instant events |
| `Outcome`  | yes      | `"ok"`, `"error"`, `"denied"`, `"timeout"`, `"dropped"`, `"selfcheck"` |
| `RefID`    | no       | Correlation ID linking child events to a parent operation; matches OTel `trace_id` when tracing is active |
| `Detail`   | no       | Short free-text or hash; PII-bearing values MUST be hashed |

PII rule for `Detail`: any value that could contain operator message
text, file path beyond `~/.leah-state/`, email subject, contact name, or
secret material is hashed with FNV-1a (per the existing
`internal/obs.labelFingerprint` pattern) before being stored. The hash
is a `uint64` rendered as hex.

### 2.2 SQLite schema

`~/.leah-state/events.db` (mode `0600`):

```sql
CREATE TABLE events (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    ts        INTEGER NOT NULL,           -- unix nanos
    kind      TEXT    NOT NULL,
    actor     TEXT    NOT NULL,
    target    TEXT,
    scope     TEXT,
    latency_ms INTEGER NOT NULL DEFAULT 0,
    outcome   TEXT    NOT NULL,
    ref_id    TEXT,
    detail    TEXT
);
CREATE INDEX events_ts_idx        ON events (ts);
CREATE INDEX events_ref_idx       ON events (ref_id);
CREATE INDEX events_kind_ts_idx   ON events (kind, ts);
```

`PRAGMA journal_mode=WAL` for concurrent read while the writer
goroutine appends. `PRAGMA synchronous=NORMAL` for write throughput at
the cost of crash-recovery losing the last few milliseconds — acceptable
for diagnostic events, NOT for `audit.jsonl` (which stays in append-only
JSONL).

## 3. Kind enum

The kind enum is the bounded set of emission sites. New emission sites
require adding the kind here AND a test asserting it appears in the
enumeration list — prevents drift.

### 3.1 Dispatcher

- `dispatch.ship` — `dispatcher.Ship` invoked; LatencyMS covers the
  full cycle.
- `dispatch.review` — reviewer subagent dispatched.
- `dispatch.merge` — PR merge attempt.

### 3.2 Attestation

- `attestation.attempt` — gate evaluated; Outcome `ok` / `denied`.
- `attestation.granted` — scope newly granted.
- `attestation.revoked` — scope revoked by operator.

### 3.3 Audit

- `audit.append` — single row appended; LatencyMS captures the fsync.
- `audit.rotate` — file rotated.

### 3.4 Connect adapters

- `connect.exchange` — OAuth code → token exchange.
- `connect.refresh` — refresh-token used.
- `connect.api_call` — outbound API call; Target identifies provider +
  endpoint.

### 3.5 Voice

- `voice.speak` — synthesis start → playback complete.
- `voice.fallback` — chain fell back from one backend to another.

### 3.6 Subagent

- `subagent.spawn` — subagent process launched.
- `subagent.complete` — subagent terminated; Outcome includes exit
  status.

### 3.7 Reasoner

- `reasoner.call` — model invocation.
- `reasoner.retry` — retry triggered.

### 3.8 Memory

- `memory.query` — query against the SQLite store.
- `memory.upsert` — write committed.

### 3.9 Recommendation engine (per PR #69 spec)

- `recommendation.propose`
- `recommendation.accept`
- `recommendation.reject`
- `recommendation.apply`

### 3.10 Observability self

- `obs.snapshot` — registry snapshot to disk.
- `obs.selfcheck` — `SelfCheck` invocation.
- `obs.panic` — recovered panic (mirrors the existing slog log line
  from `internal/obs/recover.go`).

### 3.11 HUD

- `hud.state` — overlay state-machine transition (hidden / ambient /
  focus) plus listen/think indicators. Payload type `obs.HUDStateEvent`
  (`{value, listening, thinking}`) is the wire contract for
  `ambient.js`; renames break the state pill silently.

## 4. Causal linking via RefID

Every emission site that begins an "operation" generates a `RefID`
(typically the OTel `trace_id` when tracing is sampled; otherwise a
random 128-bit ID rendered as hex). Child emission sites within the
same call chain MUST receive the parent's `RefID` from `context.Context`
via the helper:

```go
ctx = obs.WithRefID(ctx, parentRefID)
refID := obs.RefID(ctx)
```

Example causal chain for one `dispatcher.Ship` call:

| Order | Kind                    | RefID  | Outcome |
|-------|-------------------------|--------|---------|
| 1     | `dispatch.ship`         | abc123 | (open)  |
| 2     | `attestation.attempt`   | abc123 | ok      |
| 3     | `reasoner.call`         | abc123 | ok      |
| 4     | `reasoner.call`         | abc123 | ok      |
| 5     | `subagent.spawn`        | abc123 | ok      |
| 6     | `subagent.complete`     | abc123 | ok      |
| 7     | `audit.append`          | abc123 | ok      |
| 8     | `dispatch.ship`         | abc123 | ok      |

The two `dispatch.ship` events bracket the operation: row 1 records the
"start" (with `LatencyMS=0` and `Outcome="ok"` reserved for start
events that haven't yet failed) and row 8 records the completion. The
SQLite query `SELECT * FROM events WHERE ref_id = 'abc123' ORDER BY ts`
returns the full causal chain in order.

Convention: the START event has `Outcome` set to a sentinel value
`"begin"`. The completion event reuses the same `Kind` with the actual
outcome.

## 5. Emission API

`internal/obs/event.go`:

```go
// EmitEvent enqueues e for the writer goroutine. Non-blocking: when the
// queue is full the event is dropped + a counter increments. RefID
// flows from ctx via WithRefID; passing a zero-value RefID is allowed
// for top-level events.
func EmitEvent(ctx context.Context, e Event)

// WithRefID returns a derived context carrying refID for downstream
// emission sites. ctx-scoped (not goroutine-local) so subagent
// dispatch can propagate explicitly via gRPC / JSON-RPC headers.
func WithRefID(ctx context.Context, refID string) context.Context

// RefID returns the RefID stamped on ctx, or "" if none.
func RefID(ctx context.Context) string

// NewRefID returns a fresh 128-bit hex-encoded RefID. Used at
// operation roots (HTTP handler entry, daemonloop tick boundary,
// CLI command entry).
func NewRefID() string
```

The writer goroutine consumes a buffered channel of `Event`, batches
inserts into transactions of up to 100 rows or 100ms, and increments
the failure counters from §4.10 of the observability spec when the
SQLite write fails.

In-process fan-out to live SSE subscribers (V2/W87) goes through a
sibling `Broadcaster` so HUD clients receive events with no SQLite
round trip:

```go
b := obs.NewBroadcaster()
obs.SetDefaultBroadcaster(b)
obs.Publish(e) // → every matching b.Subscribe channel

// Subscribers feed the SSE transport:
h := &obs.SSEHandler{Subscribe: b.Subscribe}
```

`EmitEvent` writes both: `Publish(e)` for live subscribers AND
`store.Emit(ctx, e)` for SQLite. Either side is a no-op when its sink
is unset. Drops on a slow subscriber are counted via
`Broadcaster.Dropped()` rather than blocking the producer.

## 6. Query API

`internal/obs/event.go`:

```go
type EventQuery struct {
    Since     time.Time // inclusive
    Until     time.Time // exclusive; zero = now
    Kinds     []string  // empty = any
    Actors    []string  // empty = any
    Outcomes  []string  // empty = any
    RefID     string    // exact match when non-empty
    Limit     int       // 0 = default 1000
}

func Query(ctx context.Context, q EventQuery) ([]Event, error)
```

Implementation uses the indexes from §2.2. The dashboard "Recent
operations" widget calls `Query` with `Since = now - 5m, Limit = 200`.

## 7. HTTP endpoint

`GET /events` (parent observability spec §3):

- SSE flavor: `Content-Type: text/event-stream`. Each event is one SSE
  message body containing the Event JSON.
- JSON-lines flavor at `/events.jsonl`: one JSON object per line.
- Query params: `since=<rfc3339>`, `kind=<csv>`, `actor=<csv>`,
  `outcome=<csv>`, `ref_id=<hex>`, `limit=<n>`.
- `since` defaults to `now - 5m`. `limit` defaults to 1000, max 10000.

SSE keep-alive is a `: keepalive` comment line every 15 seconds.

## 8. Retention

- Default retention: 30 days.
- Operator override via env var `LEAH_OBS_EVENTS_RETENTION_DAYS` or
  `~/.leah/config.toml`.
- Daily prune runs inside the existing daemonloop weekly-style tick
  (executes on every tick; cheap fast-path when no rows are eligible).
- Vacuum runs weekly to reclaim space.

Disk-cap fallback: when `events.db` exceeds 500 MB, prune the oldest
10% regardless of age. Counter:
`leah_obs_events_pruned_disk_total`.

## 9. Privacy

Events MUST NOT contain:
- Raw operator message text. Hash if needed.
- OAuth tokens, refresh tokens, API keys. Never log even hashed.
- File contents. Hash if needed.
- Email subjects, contact display names. Hash if needed.

The pre-emission helper `obs.SafeDetail(s string) string` canonicalizes
the rule: any caller-supplied detail string runs through `SafeDetail`
which (a) strips characters outside `[\w\-\.:/]`, (b) truncates at 128
runes, (c) returns the hex FNV-1a hash prefixed with `"h:"` when the
caller passed `obs.SafeDetailHashed(s)`.

PR review checklist adds: "every `EmitEvent` call must show that its
`Detail` field is either an enum, a hash, or empty."

## 10. Test plan

- Round-trip test: `EmitEvent` → `Query` for every Kind.
- Causal-chain test: emit 10 events with shared `RefID`, query by
  `RefID`, assert order matches emission order.
- Drop-on-full test: fill the queue to capacity, assert subsequent
  emits return without blocking and increment
  `leah_obs_emit_event_dropped_total`.
- Retention test: write events with timestamps 60 days old, run the
  prune step, assert they are gone.
- SSE endpoint test using `httptest`: emit 3 events, assert the SSE
  stream surfaces them in order.
- PII test: assert `SafeDetail("hello@example.com message")` does not
  preserve the address verbatim.
- Schema-drift test: enumerate `Kind` enum values from the source,
  cross-check against a frozen list in `event_kinds_test.go` — every
  new kind requires test-file update.
- HUDStateEvent contract test: assert the serialized `hud.state`
  Payload exposes `value`, `listening`, `thinking` — the exact fields
  `ambient.js` reads. Field renames break the state pill silently.

## 11. Migration

No prior event store exists. Initial schema = v1. Future schema
changes:

- Additive columns: ADD COLUMN with default, no version bump needed.
- Breaking changes: bump schema_version pragma + ship a one-shot
  migration in `daemon` boot.

`events.db` is diagnostic data — operator can delete it without data
loss to durable state.

> All internal paths in this doc reflect the pre-2026-07-09 layout; current tree per `git ls-tree -d --name-only HEAD:internal/`.

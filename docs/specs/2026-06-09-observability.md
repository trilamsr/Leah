# Observability — structured logs + metrics + panic recovery

**Status**: draft
**Date**: 2026-06-09
**Slug**: 2026-06-09-observability

## 1. Goal

Close the self-diagnosis loop for Leah. The audit log captures user-facing
ACTIONS (ship dispatched, self-build invoked); observability captures the
INTERNAL Leah experience — structured logs, latency, panic traces. Without
it, when `leah-daemon` hangs at 3am Leah has nothing to ship to regatta
("fix me — here's the panic + last 10 log lines + which goroutine + memory
snapshot").

Three concrete unlocks:

1. Reasoner + dispatcher + daemonloop emit structured JSONL Leah can grep.
2. In-process metrics (counters, gauges, histograms) snapshot to a JSON
   file the future `selflearn` weekly tick can read.
3. Every goroutine wrapped in `obs.SafeGo` writes a panic stack trace to
   disk so `leah self-build "fix the panic in <pkg>"` has crash context.

## 2. Capabilities

| # | Capability      | Files                                                    |
| - | --------------- | -------------------------------------------------------- |
| 1 | Structured logs | `internal/obs/logger.go`                                 |
| 2 | Metrics         | `internal/obs/metrics.go`                                |
| 3 | Panic recovery  | `internal/obs/recover.go`                                |
| - | Tests           | `internal/obs/obs_test.go`                               |
| - | Instrumentation | `reasoner.go`, `budget.go`, `ship.go`, `daemonloop.go`   |

## 3. Schemas

### 3.1 Log line (JSONL via `slog.NewJSONHandler`)

```json
{
  "ts":       "2026-06-09T13:47:12.345Z",
  "level":    "INFO",
  "msg":      "reasoner.call.complete",
  "package":  "reasoner",
  "func":     "Ask",
  "trace_id": "01H...",
  "duration_ms": 423,
  "cost_dollars": 0.012
}
```

Required fields on every line: `ts`, `level`, `msg`, `package`, `trace_id`.
Optional fields: `func`, `duration_ms`, `cost_dollars`, `err`, plus
caller-supplied attrs.

### 3.2 Metrics snapshot (`$LEAH_STATE_DIR/metrics/latest.json`)

```json
{
  "ts": "2026-06-09T13:47:12Z",
  "counters": {
    "leah_action_total|kind=ship,outcome=success": 12,
    "leah_panic_total|package=dispatcher": 0
  },
  "gauges": {
    "leah_budget_spent_usd": 0.43,
    "leah_daemon_uptime_seconds": 18324
  },
  "histograms": {
    "leah_reasoner_latency_ms": {
      "count": 47, "sum": 18230,
      "buckets": {"50": 2, "100": 5, "250": 20, "500": 15, "1000": 5, "+Inf": 0}
    }
  }
}
```

### 3.3 Panic file (`$LEAH_STATE_DIR/panics/YYYY-MM-DDTHH-MM-SS-<name>.txt`)

```
panic: <recovered value>
goroutine: <name>
ts: <RFC3339>

<full runtime.Stack(true) dump>
```

## 4. Instrumentation policy

Avoid log spam. Per-package levels:

| Package        | DEBUG                       | INFO                                                | ERROR                |
| -------------- | --------------------------- | --------------------------------------------------- | -------------------- |
| `reasoner`     | `reasoner.call.start`       | `reasoner.call.complete` (+ duration_ms, cost)      | upstream err         |
| `budget`       | -                           | `budget.charge` (+ spent, ceiling)                  | `budget.exceeded`    |
| `dispatcher`   | -                           | `dispatcher.{draft,write,issue,watch}` (4 stages)   | per-stage err        |
| `daemonloop`   | `daemon.tick`               | `daemon.transition` (state change only)             | regatta list err     |
| `obs`          | -                           | -                                                   | `obs.panic`          |

Rule: DEBUG = per-call entry; INFO = per-call exit on the success path;
ERROR = anything that propagates an error. Do NOT log INFO inside hot
loops (per-30s daemon tick → DEBUG, not INFO).

## 5. Trace correlation

`context.Context` carries a string trace ID:

```go
ctx = obs.WithTrace(ctx, "01H...")
id  := obs.TraceID(ctx)  // "" if missing
```

`obs.LoggerFromCtx(ctx)` returns a `*slog.Logger` pre-populated with
`trace_id` attr. Callers without a ctx-bound logger get the global default
(no trace ID).

Trace ID is set once per top-level operation:

- CLI invocation: in `cmd/leah/main.go` before dispatching subcommand.
- Daemon tick: in `daemonloop.Loop.tick` per tick.
- Reasoner call: inherited from caller's ctx; if none, generated per Ask.

## 6. Integration with self-learn

`selflearn` weekly rule reads:

1. `$LEAH_STATE_DIR/metrics/latest.json` — find `leah_panic_total{package=X} > 0`.
2. `$LEAH_STATE_DIR/panics/*.txt` — newest N matching that package.
3. `$LEAH_STATE_DIR/logs/leah-YYYY-MM-DD.jsonl` — last 50 ERROR lines.

Drafts a regatta issue body: "Leah self-bug — recurring panic in `<pkg>`.
Stack trace + recent error context attached. Repro: <heuristic>." Ships
via existing `dispatcher.Ship`.

## 7. Integration with self-build

`leah self-build "fix the panic in dispatcher at <stack-frame>"` reads
the matching panic file from `$LEAH_STATE_DIR/panics/` and inlines it as
context for the regatta issue body.

## 8. Build order

1. `internal/obs/` package + tests (TDD: tests first).
2. Instrument existing packages — minimal slog calls only.
3. Daemon wiring (start metrics-snapshot goroutine, plumb trace IDs into
   CLI).

## 9. Adversarial review (severity-tagged)

- **HIGH — daily-rotation file-handle leak**: if a long-running daemon
  crosses midnight, the prior day's handle MUST close before the new
  one opens. Mitigation: `dailyRotator` guards by date-string under
  mutex; opens new file then swaps + closes old atomically.
- **MED — metrics in-memory only**: process restart loses counters
  unless snapshot fired recently. Mitigation: 60s snapshot cadence
  (cited in spec §3.2). Personal-use accepts the loss window.
- **MED — SafeGo hides bugs in dev**: env var
  `LEAH_OBS_PANIC_PROPAGATE=1` re-panics after capture so tests fail
  loudly. Default off so prod daemon survives.
- **MED — context threading**: legacy code paths without ctx fall back
  to global logger (no trace ID). Acceptable retro; new code MUST
  thread ctx.
- **LOW — log volume**: 100 INFO/min × 200 B × 30d ≈ 870 MB/mo.
  Documented `LEAH_LOG_LEVEL=WARN` recommendation for sustained
  background daemon use. Daily rotation caps file size; no
  compression/retention this round (operator can `find … -mtime +30
  -delete`).
- **LOW — slog overhead**: ~3-5% vs printf. Accepted.
- **LOW — crash-report richness**: panic + 10 log lines may be
  insufficient. Spec §6 acknowledges; richer report deferred.

## 10. Cuts (Phase X — explicitly out of scope)

- No Prometheus exporter
- No OpenTelemetry / OTLP
- No remote log shipping
- No Grafana dashboards
- No pprof endpoint (defer goroutine snapshots)
- No log compression / retention sweeper
- No structured tracing spans (just trace IDs)
- No alerting / paging integration

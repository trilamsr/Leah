---
slug: knowledge-graph-wiring
status: draft
phase: wave-8
owner: leah
parent: 2026-06-10-knowledge-graph.md
brief: 2026-06-10-wave-8-aiml-upgrade.md (S8)
supersedes: none
---

# Knowledge-graph wiring into recommend.Engine

## 1. State on main (corrects earlier draft)

The previous draft claimed `recommend.Engine` had zero callers of
`Graph.Query`. That was wrong. On main today:

- `internal/recommend/sources/knowledge.go` ships
  `KnowledgeGraphSource` implementing the `sources.Source` seam
  (`Recommendations(ctx) ([]Recommendation, error)`). It already emits
  per-entity recs with `Pattern = "knowledge.followup." + p.Key`.
- `internal/recommend/recommendation.go` defines `SignalEngine` with
  `OnSignal(ctx, Signal) ([]Recommendation, error)` (V8 #169).
- `internal/recommend/signal_dispatcher.go` fans bus events into
  `SignalEngine.OnSignal`.

So the seam from knowledge graph → recommendations exists in two
places. The gap is not wiring per se. The real gaps are:

1. `OnSignal` does not consult the knowledge graph — entity context is
   absent on the event-driven path.
2. There is no entity TTL / demotion — stale entities keep firing
   indefinitely, and `leah forget` has no effect on pending recs.
3. The `knowledge.followup.<key>` pattern string is opaque to the
   feedback store; accept/reject counts accumulate per stale key
   forever.

This spec targets exactly those three gaps and does not invent a new
adapter interface.

## 2. Non-goals

- New `Adapter` interface. `Source.Recommendations` and
  `SignalEngine.OnSignal` are the two shipped seams; no third.
- Schema redesign for `knowledge.db` — the `ts` column is reused.
- LLM-based entity extraction.
- Cross-machine sync.

## 3. `Graph.QueryLive` (additive)

`Graph.Query` is single-subject. The event-driven path
(`SignalEngine.OnSignal`) holds a `Signal` with no subject in hand, so
it needs a way to ask "what entities are live right now?" Add:

```go
// internal/knowledge/graph.go
type Relation struct {
    Entity   Entity
    Weight   float64
    LastSeen time.Time
}

// QueryLive returns currently-live relations ranked by Weight desc.
// Cap defaults to LEAH_KNOWLEDGE_ENTITY_CAP (§9).
func (g *Graph) QueryLive(ctx context.Context, kind EntityKind, cap int) ([]Relation, error)
```

Weight is `decay(now - ts) * demotion_multiplier` (§5). Only
`Entity.Key` ever flows downstream into a Pattern string —
`Display`, `Aliases`, `Refs` stay graph-local (parent §5 boundary).

`Graph.Query` (single-subject) is unchanged.

## 4. Pattern string — keep the live format

The live `KnowledgeGraphSource` emits
`Pattern = "knowledge.followup." + p.Key` with `.` as the delimiter.
Stay with that. Do not introduce `|`.

Consequence: pattern strings remain a single namespaced dotted token.
Backward-compat is automatic — existing `recommend.db` rows already
use this format because `KnowledgeGraphSource` is the only producer.

The `<source>.<verb>.<key>` shape is the convention going forward;
non-knowledge sources (`macos.cadence.<app>`, etc.) keep their own
namespace prefix.

## 5. Entity TTL + demotion

Parent spec §6 commits to "demoted, not deleted." Define the curve:

- Each `entity_items` row already carries `ts` (Unix nanoseconds).
  Do not rename to `last_seen`. A SQLite VIEW cannot rename a
  storage-level column; either we ALTER TABLE (rejected — schema
  churn) or we leave `ts` alone. Leave `ts` alone.
- Demotion ledger lives on the `entities` row, not `entity_items`.
  Add one nullable column:

  ```sql
  ALTER TABLE entities ADD COLUMN demotion REAL NOT NULL DEFAULT 1.0;
  ```

- A relation's weight at query time:

  ```
  weight = decay(now - max_ts) * demotion
  decay  = exp(-age_days / LEAH_KNOWLEDGE_DECAY_HALFLIFE_DAYS)
  ```

  Defaults: half-life 14d.

- Background demotion: entities with `max_ts` older than
  `LEAH_KNOWLEDGE_TTL_DAYS` (default 60) have `demotion *= 0.5` per
  stale-week. `demotion` floors at 0.05 — this is a hard floor; once
  demotion reaches 0.05 it does not move further on the timed path.
  Explicit forget (§6) also snaps to 0.05.

- Note on the curve: at age 60d with half-life 14d, `decay ≈
  exp(-60/14) ≈ 0.014`. That is already below the 0.05 floor before
  `demotion` enters. The floor applies to `weight`, not just
  `demotion`: `weight = max(0.05, decay * demotion)`. This guarantees
  an entity is never silently zeroed out — restoration by a single
  fresh entity_items row pulls it back above the floor.

- Demotion runs on the per-tick `Update` path — one SQL UPDATE, no
  goroutine.

## 6. Operator override (forget)

When the operator runs `leah forget --relation sarah@example.com`
(flag added in W123):

- `demotion` snaps to 0.05 immediately.
- An `audit.jsonl` row appends with
  `Kind: "knowledge_relation_demoted"`,
  `Detail: <entity_key>`,
  `Outcome: "operator_override"`.
- Pending recs whose `Pattern` carries the demoted key are dropped
  from `MemoryEngine.pending` on the next `Propose` call. Drop site:
  `internal/recommend/engine.go` `Propose` — filter `e.pending`
  against the demoted-keys set before returning. No retroactive
  audit rewrite.
- `feedback.go` keeps an `events[Pattern]` map keyed on the full
  pattern string. On demotion, the same propose-time hook clears
  the map entry for the demoted pattern so accept/reject counts
  do not accumulate against a key that will never fire again.

## 7. Wave plan (W120-W123 — file-disjoint)

Spec PR serializes per CLAUDE.md. Impl PRs fan out.

- **W120 — extend `sources/knowledge.go` with TTL-aware filter.**
  Files: `internal/recommend/sources/knowledge.go`,
  `internal/recommend/sources/knowledge_test.go`.
  The source already emits `knowledge.followup.<key>`. Add a Weight
  filter so demoted entities (weight ≤ floor) do not enter the rec
  list. Pattern format unchanged.
- **W121 — `Graph.QueryLive` + `demotion` column.**
  Files: `internal/knowledge/graph.go`,
  `internal/knowledge/storage.go`,
  `internal/knowledge/graph_test.go`.
  Adds the type, the additive ALTER TABLE migration, and the cap.
- **W122 — TTL + demotion logic.**
  Files: `internal/knowledge/ttl.go` (new),
  `internal/knowledge/ttl_test.go` (new),
  `internal/knowledge/graph.go` (one-line hook into Update).
- **W123 — `OnSignal` consumes Graph + `leah forget --relation`.**
  Files: `internal/recommend/signal_dispatcher.go` (or the impl in
  `recommendation.go` that satisfies `SignalEngine`),
  `internal/recommend/engine.go` (pending-rec drop on demote),
  `internal/recommend/feedback.go` (events map clear),
  `cmd/leah/forget.go` (`--relation` flag).
  This is the single emit-point change: `OnSignal` queries
  `Graph.QueryLive`, attaches an entity_key to the surfaced rec via
  the existing Pattern string, and routes through the existing
  dispatcher. No new adapter interface.

Each wave lands a failing test first (CLAUDE.md TDD). W123 is the
flip-day wave.

## 8. Test plan

- **W120**: golden filter — feed 5 entities (3 fresh, 2 below floor),
  assert only 3 recs surface.
- **W121**: `QueryLive` ordering + cap; migration idempotency.
- **W122**: TTL table-test, 8 rows of varied age, assert the weight
  curve matches; explicit-forget snaps to floor + appends audit row.
- **W123**: end-to-end via dispatcher — fire a Signal, assert
  `OnSignal` returns recs ranked by `Graph.QueryLive` weight; invoke
  `leah forget --relation k`, assert pending recs carrying that key
  vanish from the next `Propose` AND `events[Pattern]` is cleared.

Every test runs <50ms on a frozen clock. No `time.Sleep`.

## 9. Cardinality + privacy

- Cap: top-1000 entities by weight from `Graph.QueryLive` (env
  override `LEAH_KNOWLEDGE_ENTITY_CAP`).
- Ranking tiebreaker (cardinality cap): when two entities share the
  same weight at the cap boundary, recency wins — `max_ts` desc.
- Cap-hit audit row: cap-exceedance writes one row per UTC day,
  rate-limited via a persistent counter at
  `~/.leah-state/audit-rate.json` carrying
  `last_emitted_knowledge_cap` (Unix seconds). The file is `0600`,
  same surface as `audit.jsonl`. On daemon restart the timestamp
  survives; suppression is per-day, not per-process.
- Privacy lint: `scripts/check-no-raw-knowledge-in-prompts.sh`
  greps `prompts/` with
  `grep -nE '\bknowledge\.[a-z_]+\.[a-z0-9_]{8,}\b'`
  to catch the live `knowledge.followup.<key>` shape leaking into a
  Reasoner template. Failure is a red CI gate.

## 10. Backward-compat

- Existing recs with `Pattern = "knowledge.followup.<key>"` keep
  matching; the producer is unchanged.
- W121 ALTER TABLE is additive with a default — no rewrite of
  existing rows.
- `LEAH_KNOWLEDGE_WIRING=0` short-circuits `OnSignal`'s graph query;
  the dispatcher falls back to today's behavior (Signal in, plain
  Pattern out, no entity context).

## 11. Risks

- TTL curve over-aggressive: 14-day half-life may starve cold but
  valid entities. Mitigated by hard floor at 0.05 + env override.
- Pending-rec drop races with concurrent Accept: the drop set is
  computed inside `MemoryEngine.Propose` under the same mutex that
  guards `pending`, so Accept either lands before the drop (and the
  rec applies) or after (and ErrNotFound fires) — both safe.
- W123 flip-day: `LEAH_KNOWLEDGE_WIRING=0` restores pre-wave-8
  `OnSignal`. No data migration.

## 12. Parent spec touchpoint

Parent `2026-06-10-knowledge-graph.md` defines storage + privacy.
This spec adds the `demotion` column and `Graph.QueryLive` on top of
that schema, plus the wire-point in `OnSignal`. The parent doc does
not need a paired edit because its §6 "demoted, not deleted" is
intentionally curve-agnostic — this spec is the curve.

## 13. Source

Wave-8 brief §S8. Parent spec `2026-06-10-knowledge-graph.md`.
Live producer `internal/recommend/sources/knowledge.go` (W30k path).
Live consumer surface `recommend.SignalEngine.OnSignal` (V8 #169).

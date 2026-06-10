---
slug: knowledge-graph-wiring
status: draft
phase: wave-8
owner: leah
parent: 2026-06-10-knowledge-graph.md
brief: 2026-06-10-wave-8-aiml-upgrade.md (S8)
---

# Knowledge-graph wiring into recommend.Engine

Repairs a regressed feature: `internal/knowledge/` shipped via #136 but
`recommend.Engine` has zero callers of `Graph.Query`. The graph is
operator-local vapor — no pattern adapter, no propose loop, no audit
row references it. This spec defines the wiring.

Parent spec `2026-06-10-knowledge-graph.md` already specifies the
storage, entity kinds, privacy, and threat model. This spec only adds
the call sites and the pattern-key extension.

## 1. Goal

Wire `internal/knowledge/Graph` into `internal/recommend.Engine` so
that pattern matching becomes entity-aware. After this wave, every
proposed `Recommendation` carries an `entity_key` derived from the
operator's local knowledge graph, and accept/reject feedback updates
per-entity counts rather than per-pattern-name counts.

Concrete user-visible behavior delta: when the operator says "Sarah is
now ex-coworker," the relation demotes and recommendations keyed on
`(meeting_prep, sarah@example.com)` stop firing within one tick — they
do not bleed over into `(meeting_prep, *)` matches against other
people.

## 2. Non-goals

- Knowledge-graph schema migration. Schema already exists per W30k
  (`entities`, `entity_items`, `meta`). This spec adds two columns to
  `entity_items` but does not redesign the store.
- New entity-extraction sources. Mirror sync remains the only producer
  of entity rows.
- LLM-based entity extraction. Resolver impls stay rule-based; the
  Reasoner does not write into the graph.
- Embedding-based pattern matching. Pattern key stays the literal
  `(pattern_name, entity_key)` tuple.
- Graph export / cross-machine sync. MVP is single-operator.

## 3. `Graph.Query` semantics (extension)

`Graph.Query` already exists in `graph.go` and returns the most-recent
entity matching `KnowledgeQuery.Subject`. The wiring needs one
extension: tuple iteration. Recommend's propose loop has no subject in
hand — it asks "what entities are live right now?"

Add:

```go
// internal/knowledge/graph.go
type Relation struct {
    Entity   Entity
    Weight   float64   // 0..1; decayed per §6
    LastSeen time.Time
}

// QueryLive returns currently-live relations for the operator,
// ordered by Weight desc. Cap defaults to 1000 per §12.
func (g *Graph) QueryLive(ctx context.Context, kind EntityKind, cap int) ([]Relation, error)
```

`Relation.Weight` is computed from `entity_items.ts` recency + the
demotion ledger (§6). `Relation.Entity.Key` is the only field that
ever flows into a pattern key — `Display`, `Aliases`, `Refs` stay
local. This is the spec §5 boundary from the parent doc.

The existing single-subject `Query` is unchanged; surfaces that already
hold a subject (morning-brief enrichment) keep using it.

## 4. Pattern-key extension

Today `Recommendation.Pattern` is a single string (e.g.
`meeting_prep`). After this wave it is the tuple
`(pattern_name, entity_key)` serialized as `pattern_name|entity_key`.

Backward-compat rule: a Recommendation written with an empty
`entity_key` matches the wildcard. Existing rows in `recommend.db`
serialized as `meeting_prep` (no `|`) are read as
`("meeting_prep", "")` and still match every entity.

Audit-row impact: `audit.Entry.Detail` carries the full tuple — no
schema migration. `scripts/check-no-raw-knowledge-in-prompts.sh` (W30
lint) already forbids raw entity data in prompts; the entity_key is
PII (e.g. `sarah@example.com`) and stays out of remote prompt
templates. The pattern-key is operator-local only — never serialized
into a Reasoner prompt.

## 5. RecommendAdapter signature

Today's pattern adapters consume `(ctx, signal)` and return
`Recommendation`. Extension:

```go
// internal/recommend/adapter.go
type Adapter interface {
    Name() string
    Propose(ctx context.Context, sig Signal, kq knowledge.Result) (Recommendation, error)
}
```

Adapters that don't need entity context pass `kq = knowledge.Result{}`;
the propose loop in `engine.go` calls `Graph.QueryLive` once per tick,
fan-outs entities, and invokes each adapter once per `(adapter, entity)`
pair. Cardinality cap per §12.

Tier semantics unchanged. Accept / Reject / Apply unchanged.

## 6. Entity TTL + demotion

The parent spec §6 already commits to "Demoted, not deleted." This
spec defines the curve.

- Each `entity_items` row carries `last_seen` (renamed from `ts` for
  clarity; same column).
- A relation's weight is:

  ```
  weight = base * decay(now - last_seen) * demotion_multiplier
  ```

  where `base = 1.0`, `decay = exp(-age_days / 14)`, and
  `demotion_multiplier` defaults to 1.0.
- Entities with `last_seen` older than `LEAH_KNOWLEDGE_TTL_DAYS`
  (default 60) get `demotion_multiplier *= 0.5` per week of stale-age
  beyond the threshold. Multiplier floors at 0.05 — entities never
  reach 0 weight unless explicitly forgotten.
- Operator-tunable via env: `LEAH_KNOWLEDGE_TTL_DAYS`,
  `LEAH_KNOWLEDGE_DECAY_HALFLIFE_DAYS` (default 14).
- Demotion runs as a single SQL UPDATE on the per-tick `Update` path
  (no separate scheduler goroutine — keeps the surface flat and
  matches the existing mirror-tick cadence).

## 7. Stale-detection override

When the operator says "Sarah is now ex-coworker" (or invokes
`leah forget --relation sarah@example.com:coworker`), the relation
demotes immediately:

- `demotion_multiplier` snaps to 0.05 (the floor — not zero, so a
  later restoration is one operator interaction away).
- An `audit.jsonl` row appends with `Kind: "knowledge_relation_demoted"`,
  `Detail: <entity_key>:<relation>`, `Outcome: "operator_override"`.
- Any pending `Recommendation` whose `Pattern` includes the demoted
  entity_key is dropped from the pending pool on the next propose
  call. No retroactive `audit.jsonl` rewrite — the demotion row is
  the audit trail.

The surface CLI lives in `cmd/leah/forget.go` (already partially
present per parent §5). This wave adds the `--relation` flag; the
entity-level `leah forget` already works.

## 8. Wave plan (W120-W123 — file-disjoint)

Spec PR serializes (CLAUDE.md). Implementation PRs fan out under the
file-disjoint rule. Each wave owns one package directory; no shared
file is touched twice.

- **W120 — `Graph.QueryLive` impl.**
  Files: `internal/knowledge/graph.go`, `internal/knowledge/storage.go`,
  `internal/knowledge/graph_test.go`.
  Adds `QueryLive`, `Relation` type, `last_seen` column rename (alias,
  not breaking — old column retained as VIEW for compat).
- **W121 — `RecommendAdapter` signature extension.**
  Files: `internal/recommend/adapter.go` (new),
  `internal/recommend/engine.go` (touch only the propose method),
  `internal/recommend/engine_test.go`.
  Existing adapters get a one-line diff: `kq knowledge.Result` added,
  unused parameter underscored where the adapter does not need it.
- **W122 — Entity TTL + demotion.**
  Files: `internal/knowledge/ttl.go` (new),
  `internal/knowledge/ttl_test.go` (new), `internal/knowledge/graph.go`
  (one-line hook into `Update`).
  Pure logic — no scheduler goroutine. Env-var read happens once at
  `New`.
- **W123 — Wire into recommend Engine + audit rows.**
  Files: `internal/recommend/engine.go`,
  `internal/recommend/engine_test.go`, `cmd/leah/forget.go` (add
  `--relation`).
  This is the smallest wave by LOC but the highest-risk one — flips
  the kill-switch behavior live.

Each wave lands a failing test first (CLAUDE.md TDD). W123 must
include an integration test that runs Propose → Accept → Apply with
a graph fixture and asserts the audit row carries the tuple key.

## 9. Test plan (per wave)

- **W120**: golden `QueryLive` fixture — graph with 50 entities across
  3 kinds, assert cap honored, weight ordering correct, decay applied
  vs. a frozen clock.
- **W121**: adapter contract test — register a fake adapter, feed it
  three (signal, kq) pairs, assert it sees the entity_key and emits
  the correct Pattern string format.
- **W122**: TTL table-test — 8 rows (varied last_seen), assert the
  demotion multiplier matches the curve. Operator-override path:
  assert immediate snap to 0.05 + audit row appended.
- **W123**: end-to-end — `MemoryEngine.Propose` after seeding the
  graph and a single adapter; assert returned Recommendations are
  ranked by `weight * adapter_score`; toggle `LEAH_KNOWLEDGE_WIRING=0`
  and assert behavior reverts to pattern-name-only matching.

Every test runs in <50ms with a frozen clock. No `time.Sleep` (CLAUDE.md
no-bare-sleep gate, Wave 1-G2).

## 10. Privacy

The §3 boundary holds: `Graph.QueryLive` reads the operator's local
knowledge graph, never egresses. Only `entity_key` (not
`entity_content`, not aliases, not display strings) becomes part of
the pattern key. Pattern keys live in `recommend.db` and `audit.jsonl`,
both `0600` files. The Reasoner prompt template still sees only the
derived `Scalars` map from parent spec §3 — pattern keys do not flow
into prompts.

A new lint gate
`scripts/check-no-knowledge-pattern-in-prompt.sh` greps `prompts/`
for `|sarah` / `|+1` / `|@` style fragments to enforce. Failure is a
red CI light, identical to the W30 lint.

## 11. Operator override (kill-switch)

`LEAH_KNOWLEDGE_WIRING=0` falls back to pattern-name-only matching:
the engine treats every adapter call as `entity_key = ""` wildcard.
Default is `LEAH_KNOWLEDGE_WIRING=1` (wave-8 brief calls this a
regressed-feature repair — the feature is supposed to be on).

Toggle is read once at `recommend.MemoryEngine.New` (or the SQLite
engine's constructor). No hot-reload — the operator restarts `leah`
to flip it. This matches the existing `LEAH_OFFLINE=1` and
`LEAH_LOCAL_ONLY=1` patterns and keeps the propose loop branch-free.

## 12. Cardinality

Entity-key extension grows the pattern-key space combinatorially:
`|patterns| * |entities|`. Cap to N=1000 entities per operator. The
cap is enforced in `Graph.QueryLive`:

- Top-1000 by weight returned to recommend.Engine.
- Entities ranked 1001+ fall back to wildcard `(pattern_name, "")`
  matching — they're still represented, just at the wildcard weight.
- If `|entities| > 1000`, an audit row appends once per day with
  `Kind: "knowledge_cardinality_cap_hit"`, `Outcome: "wildcard_fallback"`.

The cap is operator-tunable via `LEAH_KNOWLEDGE_ENTITY_CAP` (default
1000). Memory budget at N=1000: ~80KB per propose tick — negligible.

## 13. Backward-compat

- Existing `Recommendation` rows with `Pattern = "meeting_prep"`
  (no `|`) deserialize as `("meeting_prep", "")` and match every
  entity (wildcard).
- Existing adapters that don't take `knowledge.Result` get a no-op
  parameter — the propose loop passes `Result{}` when no entity is
  in scope.
- The W30k knowledge.db schema is unchanged. The `last_seen` column
  rename is a SQLite VIEW alias only.
- `LEAH_KNOWLEDGE_WIRING=0` restores pre-wave-8 behavior exactly —
  same propose ordering, same audit-row Detail format.
- Feedback rows that referenced a now-demoted entity remain in
  `audit.jsonl` untouched. The Graph returns reduced weight for
  the entity on subsequent queries; selflearn aggregations treat the
  pre-demotion rows as historical signal, not current.

## 14. Risks not covered above

- Pattern-key collision: two operators (hypothetical multi-operator
  future) with entities `sarah@example.com` would collide. MVP is
  single-operator (parent §5) so this is deferred.
- Demotion-curve over-aggressive: 14-day half-life may starve cold
  entities. Mitigated by the floor at 0.05 — entities never zero out
  silently. If the curve proves wrong in practice, env-var override
  ships day-one.
- W123 flip-day rollback: if the wired engine misbehaves in
  production, operator sets `LEAH_KNOWLEDGE_WIRING=0` and restarts.
  No data migration required.

## 15. Source

Wave-8 brief §S8 (`2026-06-10-wave-8-aiml-upgrade.md`). Parent spec
`2026-06-10-knowledge-graph.md`. Existing impl `internal/knowledge/`
(W30k, #136). Existing engine `internal/recommend/engine.go` (W15-W18).

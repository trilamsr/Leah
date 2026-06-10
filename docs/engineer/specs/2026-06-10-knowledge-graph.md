---
slug: knowledge-graph
status: draft
phase: self-host
owner: leah
---

# Knowledge graph — cross-app entity layer

## 1. Goal

Cross-app composition. When the operator asks "what's on for today?" or
Leah needs to surface a recommendation, the knowledge layer fuses signals
across Calendar + Contacts + Messages + Reminders + Notes into a single
operator-context object the Reasoner can query without re-implementing
join logic per call site.

Sibling to `2026-06-10-macos-ecosystem-integration.md`. The macOS spec
defines the read pipeline and per-app normalized `Item` shape; this spec
defines what `Item`s mean together.

Powered callers:
- Morning-brief enrichment (live in `internal/brief/morning.go`).
- Recommendation engine (`internal/recommend/`, lands W15+ per
  `2026-06-10-learn-recommend-apply.md`).
- Voice-comm context-aware responses (`internal/voice/session/`, W11+
  per `2026-06-10-voice-comm.md`).

## 2. Architecture

```
+----------------------------+
|  macOS mirror              |
|  ~/.leah-state/macos-mirror.db
|  (Item rows, source-tagged)|
+--------------+-------------+
               | mirror sync tick (60 s)
               v
+----------------------------+
|  internal/knowledge/        |
|                             |
|  Graph                      |
|   ├── entities (person,     |
|   │      project, event,    |
|   │      location, time-win)|
|   ├── Resolver[entity]      |
|   │   per-entity lookup +   |
|   │   cross-source fan-out  |
|   └── Query(KnowledgeQuery) |
+--------------+-------------+
               |
               v
+----------------------------+
|  ~/.leah-state/knowledge.db |
|  (SQLite, 0600)             |
+----------------------------+
```

Package layout:

```
internal/knowledge/
    graph.go         Graph type + Build/Update from mirror
    entity.go        Entity{Kind, Key, Aliases, LastTouched}
    resolver.go      Resolver interface; per-Kind impls
    query.go         KnowledgeQuery + Result shapes
    persist.go       knowledge.db read/write (0600)
    knowledge_test.go
```

`Graph` is the public type. `Resolver` is the extension seam — a new
entity kind (e.g. `Vehicle`) adds a Resolver impl and a row in the
`Kind` enum; no caller code changes.

### Entity kinds (MVP)

| Kind | Identity | Sources fused |
|---|---|---|
| `Person` | canonicalized email OR phone E.164 | Contacts (name + aliases), Messages (thread ↔ handle), Calendar (attendee email), Mail (From/To) |
| `Project` | tag-derived string (e.g. `#leah`, `kanban col`) | Notes (heading), Reminders (list), Calendar (event-title regex) |
| `Event` | calendar-event UUID | Calendar (canonical), Reminders (cross-ref by title+day), Notes (heading mention) |
| `Location` | place-string canonicalized via Apple Maps token if present | Calendar (location field), Wi-Fi SSID → place (operator-maintained map) |
| `TimeWindow` | half-open `[start, end)` | derived, not stored — produced by `Query.Within` |

`Person` is the load-bearing entity — almost every operator query routes
through "what about <name>?"

## 3. Query surface

```go
// internal/knowledge/query.go
type KnowledgeQuery struct {
    Subject string        // entity key or alias; "" matches any
    Kind    EntityKind    // optional kind filter
    Within  time.Duration // window ending at time.Now()
    Limit   int           // cap on returned Items; 0 means default 50
}

type Result struct {
    Entity Entity
    Items  []macos.Item  // chronological, newest first
    Scalars map[string]int // derived counts safe for remote reasoner
}

type Graph interface {
    Query(ctx context.Context, q KnowledgeQuery) (Result, error)
    Forget(ctx context.Context, entityKey string) error
    Update(ctx context.Context) error  // pulls latest mirror rows
}
```

Example call (morning-brief enrichment, W30):

```go
res, err := graph.Query(ctx, KnowledgeQuery{
    Subject: "sarah@example.com",
    Within:  7 * 24 * time.Hour,
})
// res.Scalars["calendar_events"] = 2
// res.Scalars["messages_inbound"] = 14
// res.Items[0..4] — most recent 5 across all sources
```

Sentinel errors: `ErrUnknownEntity`, `ErrGraphStale` (mirror tick older
than 5×configured interval).

## 4. Storage

- `~/.leah-state/knowledge.db` — SQLite, mode `0600`, parent dir `0700`.
  Schema versioned the same way as the macOS mirror (int compare).
- Tables:
  - `entities(kind, key, display, aliases_json, first_seen, last_touched)`
  - `entity_items(entity_key, source, item_id, ts)` — index from entity
    to mirror item; the mirror remains the body source.
  - `meta(key, value)` — schema version, last-tick, retention horizon.
- Retention: default 90 days, configurable via
  `~/.leah-state/config.toml` `[knowledge] retention_days = 90`. Matches
  the audit-log retention convention.
- Eviction: rows older than retention are dropped on the next mirror
  tick after midnight local time. Entities with `last_touched` older
  than retention are demoted (see §6), not deleted.

## 5. Privacy

This is the second-highest sensitivity boundary in the codebase after
the secrets directory.

- **Computed locally.** Never shipped to remote reasoner. Period. The
  package has no HTTP client.
- **Reasoner gets derived scalars only.** The `Result.Scalars` map is
  what flows to a remote LLM prompt — e.g. `"events_today": 3`,
  `"unread_inbound_from_subject": 14`. The Reasoner never sees
  `"Sarah is at sarah@example.com"`. A lint rule
  (`scripts/check-no-raw-knowledge-in-prompts.sh`, added W30) forbids
  any prompt template referencing a `Result.Entity.Key` field directly.
- **`leah forget <entity>` scrubs the graph.** Mirrors the `leah forget`
  surface defined in `2026-06-10-learn-recommend-apply.md` §6:
  - removes `entities` row,
  - removes `entity_items` joins,
  - leaves the mirror itself untouched (mirror is the source of truth;
    re-running `Update` would re-create the entity if mirror rows still
    point at it — operator must `leah forget` the mirror rows too for a
    full purge, documented in CLI help).
  - appends an `audit.jsonl` row with `Kind: "knowledge_forget"`.
- **No cross-operator anything.** MVP is single operator, single laptop
  (matches the LRA spec scope statement).

## 6. Threat model

| Risk | Likelihood | Mitigation |
|---|---|---|
| Entity-correlation leak via prompt exfil | High (this is the entire premise of the concern) | No-remote-shipment rule above. Lint gate at W30. Scalar-only `Result` projection. |
| Stale entity → wrong recommendation ("Sarah" is now an ex-coworker) | Medium | Entity TTL based on `last_touched` timestamp. Entities older than 60 days are **demoted** — Query still finds them but with `Result.Scalars["entity_age_days"] >= 60` so the Reasoner can weight accordingly. **Demoted, not deleted** — operator can revive by interacting again. |
| Adversarial-pattern injection (an iMessage crafted to alter recommendations) | Medium | Per `2026-06-10-learn-recommend-apply.md` rate-limit + scope-gate. Knowledge graph inherits: incoming Messages from a Person with `first_seen` < 24 h are tagged `provisional`; provisional Items are excluded from `Result.Items` returned to recommend.Engine until the 24 h window passes (Reasoner queries still see them — provisional only affects auto-apply gating). |
| Resolver bug fuses two real people into one entity | Medium | Resolver impls are kind-scoped and tested with golden fixtures. Email and phone canonicalization is library-backed (RFC 5322 normalize + libphonenumber `E164`). Conflict resolution: when two candidate entities have overlapping aliases but different `first_seen`, the older entity wins and the newer one's aliases are appended; `audit.jsonl` row `Kind: "knowledge_merge"` records the merge so the operator can audit. |
| Persisted DB leak via filesystem read | Medium | `knowledge.db` `0600`, parent dir `0700`. No backup integration. Schema includes no plaintext body — only keys and references to mirror item IDs. |
| `Forget` leaves dangling refs in audit feedback rows | Low | `Forget` is append-only on `audit.jsonl`; never rewrites history. Recommendation feedback that referenced a forgotten entity still exists in audit but the Graph returns `ErrUnknownEntity` on subsequent queries — the LRA engine treats that as a hard stop, not a backfill. |
| Resolver hot loop reads mirror on every call | Low | Resolvers cache last 100 entities for the current sync-tick window; cache invalidated on mirror tick. Eviction is LRU. |
| Time-window math drifts across DST | Low | All `Within` math uses `time.Now().UTC()` then projects to `time.Local` only for display. Tested with `TZ=America/Los_Angeles` and `TZ=UTC` golden tests. |

## 7. Test plan

- **Synthetic-graph hermetic tests.** Fixture `Graph` built from
  hand-constructed `macos.Item` slices; assert `Query` returns expected
  entities and scalars.
- **Mirror → Graph propagation integration test.** Stand-in mirror
  emits 50 Items across 4 sources; `Graph.Update` runs once; assert
  entity count and `entity_items` joins.
- **`Forget` test.** Build entity, run `Forget`, assert
  `Query{Subject: <key>}` returns `ErrUnknownEntity`, assert mirror
  rows untouched, assert `audit.jsonl` row appended.
- **Entity-merge test.** Two candidate entities with overlapping aliases
  trigger merge; assert older wins, alias union, `knowledge_merge` audit
  row.
- **Scalar-only projection test.** Snapshot `Result.Scalars` map and
  assert no key contains a substring matching a known mirror-row body
  fragment (defensive regression test for prompt-leak prevention).
- **Retention eviction test.** Test-clock advances 91 days; assert rows
  with `last_touched < now - 90d` are demoted (not deleted), assert
  `Scalars["entity_age_days"] >= 90`.
- **No-network test.** Package depends on `net/http` is forbidden by a
  `scripts/check-knowledge-no-net.sh` grep gate added W30.

## 8. Out of scope (MVP)

- Multi-operator entity graphs (shared Sarah across operator's laptops).
- Graph-database backing store (Neo4j, SQLite-graph extensions) — flat
  SQLite tables are sufficient at single-operator scale.
- Embedding-based entity resolution (CoreML sentence embeddings for
  alias matching). MVP uses string canonicalization only.
- UI for entity browsing — the dashboard surface lands separately.
- Export / import of the graph across machines.

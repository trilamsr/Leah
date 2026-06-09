---
title: Memory M2 — Minimal (3 tables, single-operator)
status: draft
phase: m2
author: tri
date: 2026-06-09
supersedes: docs/specs/2026-06-09-leah-overview.md §4.3 (operator-KB sketch)
---

# Memory M2 — Minimal

## 1. Goal

Single-operator persistent memory. Three nouns: **contact**, **project**, **decision**. No threads, no preferences, no vocab. Storage only — no Reasoner integration yet. Manual CLI population.

Defaults simpler than overview spec because there is one human user. Embeddings + auto-extraction defer to M3.

Success: `leah contact add tri --email tri@maydow.com` writes a row + an audit-log line; `leah contact list` shows it across process restarts.

## 2. Schema

Single file `internal/memory/schema.sql`, embedded via `//go:embed`, executed on `NewStore`. Idempotent via `CREATE TABLE IF NOT EXISTS`.

```sql
-- schema_version: 1
CREATE TABLE IF NOT EXISTS contact (
  id           TEXT PRIMARY KEY,        -- ULID
  workspace_id TEXT NOT NULL DEFAULT 'default',
  name         TEXT NOT NULL,
  email        TEXT,
  notes        TEXT,
  created_at   TEXT NOT NULL,           -- RFC3339
  updated_at   TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_contact_name ON contact(name);

CREATE TABLE IF NOT EXISTS project (
  id           TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL DEFAULT 'default',
  name         TEXT NOT NULL,
  status       TEXT NOT NULL DEFAULT 'active',  -- active|paused|done
  notes        TEXT,
  created_at   TEXT NOT NULL,
  updated_at   TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_project_status ON project(status);

CREATE TABLE IF NOT EXISTS decision (
  id           TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL DEFAULT 'default',
  topic        TEXT NOT NULL,
  choice       TEXT NOT NULL,
  rationale    TEXT,
  decided_at   TEXT NOT NULL,
  created_at   TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_decision_topic ON decision(topic);

CREATE TABLE IF NOT EXISTS schema_meta (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
INSERT OR IGNORE INTO schema_meta(key, value) VALUES('version', '1');
```

Column budget: 7 / 7 / 7. Under 8. `workspace_id` present + dormant (always `'default'`); activates in Phase X when multi-operator lands.

Storage location: `~/.leah-state/memory.db`. Same dir as `audit.jsonl`. Directory created on `NewStore` if missing (0700).

## 3. CLI

Wired in follow-up task — this spec covers shape only. Lives under `cmd/leah/memory_*.go`.

```
leah contact add <name> [--email <e>] [--notes <n>]
leah contact list
leah contact show <id>

leah project add <name> [--status active|paused|done] [--notes <n>]
leah project list
leah project show <id>

leah decision add <topic> <choice> [--rationale <r>] [--decided-at <RFC3339>]
leah decision list
leah decision show <id>
```

Examples:

```
$ leah contact add "Maya Chen" --email maya@example.com --notes "ex-Stripe, intros"
contact: 01J9X... Maya Chen

$ leah contact list
01J9X...  Maya Chen   maya@example.com
01J9Y...  Pat Singh   —

$ leah decision add "billing-stack" "Stripe" --rationale "team familiarity"
decision: 01J9Z... billing-stack=Stripe
```

Empty required arg → exit 2 with `error: name required`. No interactive prompt.

## 4. Integration with `leah ask` / `ship` / `review`

**None.** This is storage only. Reasoner does NOT see Memory in M2.

M3 adds: read-only `Store.SearchContacts(query)` etc. + Reasoner tool-call surface (`memory.lookup`). Embeddings via sqlite-vec deferred to M3 also.

Rationale: avoid premature integration. Populate manually, observe what gets queried, design retrieval against real call patterns instead of guessing.

## 5. Audit integration

Every mutation (`AddContact`, `AddProject`, `AddDecision`) appends one `audit.Entry`:

```go
auditLog.Append(audit.Entry{
    Kind:        "memory.write",
    BlastRadius: 1,
    Outcome:     "ok",
    Detail:      fmt.Sprintf("%s:%s", table, id),
})
```

Failure to write audit does NOT roll back the DB write — audit best-effort, log only. (Memory row is the truth; audit is the trail.)

## 6. Migration

Schema_version stored in `schema_meta('version')`. `NewStore` reads current value:
- empty → first install; exec full schema.sql; sets version=1.
- equals embedded version → no-op.
- less than embedded → run migrations 1→N (M2 ships with version=1 so this branch is dead code until M3).
- greater than embedded → fatal error ("DB newer than binary; upgrade leah").

No external migration framework. Add `internal/memory/migrations/NN_desc.sql` when version bumps; each is `IF NOT EXISTS`-flavored idempotent SQL. Total mechanism stays under 30 LoC.

## 7. Build order

1. `schema.sql` + `Store` skeleton (`NewStore`, `Close`, embed). TDD test: tables exist after open.
2. `Contact` CRUD + types. TDD: Add → List roundtrip; Add → Get by id.
3. `Project` CRUD (copy contact pattern; three similar tables beat premature shared abstraction).
4. `Decision` CRUD.
5. CLI wiring under `cmd/leah/memory_*.go` (follow-up task; not in this commit).

`go get modernc.org/sqlite` (pure Go, no cgo).

## 8. Cuts (explicit defers)

| Cut | Why | Reopen trigger |
|---|---|---|
| sqlite-vec / embeddings | premature; no retrieval surface yet | M3 |
| thread / preference / vocabulary tables | not load-bearing for single-operator | observed need in 30 days |
| auto-extraction from email/chat | source connectors don't exist | tier 3 (comms) |
| workspace_id activation | single operator | multi-operator Phase X |
| update / delete CRUD | append-only is fine for personal log; mistakes fixable by direct SQL | reported pain |
| FTS5 index | LIKE scan acceptable at 100s of rows | >10k rows OR slow `list` |
| Reasoner integration | guess-first; populate-then-design | M3 |
| Migration framework | one version | version=3+ |

---

## Critic findings

Adversarial pass against the design above. Severity tags: CRITICAL / HIGH / MED / LOW.

### HIGH — §2 schema, `updated_at` column without update path
**Problem**: `contact` + `project` carry `updated_at`, but §8 explicitly cuts update CRUD. Dead column; rots into mismatched semantics when M3 adds updates and forgets to bump it.
**Resolution**: kept `updated_at` (set equal to `created_at` on insert). Documented in §2: "updated_at = created_at on insert; mutated when update CRUD lands". One-line cost, kills schema migration in M3.

### HIGH — §5 audit best-effort + §1 success criterion conflict
**Problem**: "audit failure does not roll back DB write" means the success criterion ("writes a row + an audit-log line") can silently degrade to row-only. Operator notices nothing.
**Resolution**: audit failures logged to stderr via `log.Printf("audit: %v", err)` so the operator sees them in daemon logs. Still no rollback (truth is the DB row), but observable.

### MED — §2 ULID as TEXT PK
**Problem**: ULID strings sort lexically by time, but SQLite TEXT comparison is locale-dependent unless `COLLATE BINARY`. Default is BINARY for TEXT without override so this is fine — but worth pinning.
**Resolution**: noted in §2 — "PK TEXT uses default BINARY collation; ULID lex-sort = time-sort holds."

### MED — §3 CLI: empty name → exit 2
**Problem**: spec says exit 2 + stderr message but doesn't pin behavior for whitespace-only (`leah contact add "   "`). Stored row with name=`   ` is junk.
**Resolution**: §3 — "name is `strings.TrimSpace`'d; empty post-trim → exit 2". Same for project name + decision topic+choice.

### MED — `decision.decided_at` vs `created_at` confusion
**Problem**: two timestamps invite "which is which?" reader load. Personal use rarely needs both.
**Resolution**: kept both but documented — `decided_at` = when the decision was made (operator-supplied, may backdate); `created_at` = when it was recorded in DB. Backdating real decisions is a use case worth preserving.

### LOW — §6 migration "fatal error if DB newer than binary"
**Problem**: fatal-on-startup blocks every leah command including `leah ask` which doesn't touch Memory.
**Deferred**: single-operator means downgrade is rare + recoverable (`brew install leah@latest`); the fatal is correct guard against silent corruption. Reopen if it bites.

### LOW — index choices
**Problem**: `idx_contact_name` is full-name; partial-match search (`list --grep ma`) won't use it. But §8 says LIKE scan acceptable at 100s of rows, so fine for M2.
**Deferred**: revisit at FTS5 trigger.

### LOW — no `db.SetMaxOpenConns(1)`
**Problem**: SQLite + concurrent goroutines + modernc driver can lock-contend; setting max-open=1 is the boring fix.
**Resolution**: §7 build order step 1 includes `db.SetMaxOpenConns(1)` + `_pragma=journal_mode(WAL)` connection string. Mentioned in skeleton, not body, to keep §2 schema-only.

### Verdict
HIGH x2 + MED x3 + LOW x3. Both HIGHs resolved inline.

# Schema v5 / v6 collision audit — clean state confirmed

**Date:** 2026-06-09
**Scope:** `internal/memory/schema.sql`, `internal/memory/memory.go`
**Trigger:** Verify no collision between v5 `embedding` table (semantic recall) and v6 `workspace_persona` table (per-workspace persona settings) landed via parallel LL / NN waves.

## Outcome

**Clean.** Both tables co-exist in `schema.sql`; `embeddedSchemaVersion` is `"6"`; tests green. No reconciliation needed.

## Evidence

### schema.sql contents

`internal/memory/schema.sql` (lines 1-142) contains the following tables in additive-migration order:

| Block | Table(s) | Source |
|-------|----------|--------|
| v1 | `contact`, `project`, `decision`, `schema_meta` | original |
| v2 | `context`, `operator_state`, `context_switch_log` | ctxmgr |
| v3 | `mistake_log` | selflearn |
| v4 | `operator_profile`, `operator_profile_meta` | operatormodel |
| **v5** | **`embedding`** (item_id, item_type, model, dim, vector BLOB, content, updated_at) | semantic-recall (LL) |
| **v6** | **`workspace_persona`** (workspace PK, tone, signature, voice_id, updated_at) | persona (NN) |

Both v5 and v6 blocks are present, additive, and non-overlapping (disjoint table names, no shared columns, no FK collision). v6 block header at line 128 cites `docs/specs/2026-06-09-leah-overview.md §3.5 bullet 4`; v5 block header at line 111 cites `docs/specs/2026-06-09-semantic-recall.md`.

### embeddedSchemaVersion

`internal/memory/memory.go:24`:

```go
const embeddedSchemaVersion = "6"
```

The "newer than binary" guard at line 124 (`if v > embeddedSchemaVersion`) correctly accepts a v6-stamped DB.

### Test results

```
$ cd /Users/treedesk/Desktop/Projects/leah && go test ./internal/memory/ ./internal/embed/ ./internal/persona/ -race -count=1
ok  	github.com/trilam/leah/internal/memory	2.152s
ok  	github.com/trilam/leah/internal/embed	1.802s
ok  	github.com/trilam/leah/internal/persona	2.100s
```

All three consumers compile + pass with race detector under the v6 schema.

## Risk surface (none load-bearing, noted for future)

1. **No explicit "both tables present" test.** A future migration accidentally truncating `schema.sql` before the v6 block would only surface via persona package failures (which exercise `workspace_persona` directly). The migration smoke test in `memory_test.go` validates `embeddedSchemaVersion` stamping but does not assert per-table presence. Acceptable per `default_simpler` — wait for a real regression before adding the gate.
2. **Schema is monolithic.** `schema.sql` is a single file with per-version comment markers rather than per-version files. Re-ordering by accident would break additive semantics silently. Same disposition — defer until the file genuinely causes a merge collision.

## Disposition

No commit. No PR. Audit-only. Reopen this note if `schema_meta.version` and `embeddedSchemaVersion` diverge or if a future wave adds a v7 block that touches `embedding` or `workspace_persona` columns.

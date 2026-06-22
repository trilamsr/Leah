# Vector Store Best-Practice for Leah Daemon (2026-06-21)

> Research-only. Reopens the embedding-backend decision in `internal/embed/embed.go` and `docs/superpowers/plans/2026-06-21-leah-macos-native-phase1.md` Task 5.

## TL;DR

**Adopt `modernc.org/sqlite/vec` (the in-tree pure-Go sqlite-vec port shipped in modernc v1.47.0 on 2026-03-17).** It is the unicorn the old plan ruled out by assumption — sqlite-vec semantics, registered automatically on every `sql.Open` connection via a one-line blank import, no CGo, no separate process, no driver swap, JOINs against the existing tables work because it *is* the existing driver. The brute-force fallback in `internal/embed/embed.go` was the right call against the world as of 2026-01; the world changed in March.

Plan §17.10's "sqlite-vec mandate" is now compatible with the pure-Go driver. The plan's deferral to brute force is now stale.

## The critical question, answered

**Q: Can `modernc.org/sqlite` load sqlite extensions in 2026?**
A: It does not need to. As of **v1.47.0 (2026-03-17)**, the maintainers vendored a CGo-free transpilation of `sqlite-vec` directly into the driver tree at `modernc.org/sqlite/vec`. Registration happens via blank import — the package's `init()` wires `vec0` as a virtual-table module on every new connection.

Source: `gitlab.com/cznic/sqlite/CHANGELOG.md`:

> 2026-03-17 v1.47.0: Add CGO-free version of the vector extensions from https://github.com/asg017/sqlite-vec. See `vec_test.go` for example usage. … Store and query float, int8, and binary vectors in vec0 virtual tables.

Source: `gitlab.com/cznic/sqlite/vec_test.go` confirms the API:

```go
import (
    "database/sql"
    _ "modernc.org/sqlite/vec"   // <-- registers vec0 module
)

db, _ := sql.Open("sqlite", ":memory:")
db.Exec(`create virtual table vec_examples using vec0(sample_embedding float[8]);`)
db.Query(`select rowid, distance from vec_examples
          where sample_embedding match '[…]' order by distance limit 2;`)
```

Source: pkg.go.dev/modernc.org/sqlite README confirms a `vec/` sibling to `lib/` shipping one generated Go file per GOOS/GOARCH.

This was made possible by **v1.45.0 (2026-02-09)** introducing the `modernc.org/sqlite/vtab` subpackage exposing the virtual-table C API in Go — which is the same machinery sqlite-vec depends on. The maintainers themselves used the new vtab API to drop in sqlite-vec a month later.

modernc release cadence is healthy: v1.45 (Feb), v1.46 (Feb), v1.47 (Mar), v1.48 (Mar), v1.49 (Apr), v1.50 (Apr, bumped vendored sqlite-vec to v0.1.9), v1.50.1 (May 2026). Current pin in leah is older — bump to **v1.50.1+** to get the vendored sqlite-vec v0.1.9.

## Scorecard

| Option | Setup | CGo? | JOIN with existing tables | Single-file backup | Latency @ 50K × 1024-dim | Maintenance (2026) | License |
|--------|-------|------|---------------------------|--------------------|--------------------------|--------------------|----------|
| **A. sqlite-vec via mattn CGo** | Migrate every existing table modernc→mattn (~1-2wk) | YES | Yes (same db) | Yes | <5ms (vec0 FLAT, SIMD via C) | sqlite-vec active (last commit 2026-05-18, v0.1.10-alpha.4); mattn is the canonical CGo SQLite driver | Apache-2.0 / MIT |
| **B. `modernc.org/sqlite/vec` (vendored pure-Go port)** ← WINNER | Bump go.mod; `_ "modernc.org/sqlite/vec"` blank import; no driver swap | **NO** | **Yes** (same connection, same driver, same `leah.db`) | **Yes** (one file) | Estimated 5-25ms (vec0 FLAT, no SIMD; transpiled C → Go) | Maintained by modernc team; tracks sqlite-vec upstream (v0.1.9 in modernc v1.50.0) | BSD-3 (modernc) + MIT (sqlite-vec) |
| C. chromem-go | `go get`; separate file store (gob) | NO | **NO** (separate DB; daemon must coordinate joins in Go) | Separate gob files per collection | 40ms @ 100K vectors (own bench, 768-dim, 2020 i5) → est. 50-70ms @ 50K × 1024 | Active (commit 2026-05-17); last tagged release v0.7.0 Sept 2024 — beta, no v1.0 | MIT |
| D. Brute-force cosine in pure Go over BLOB (current plan) | Zero new deps; already shipped | NO | Yes (already in `leah.db`) | Yes | Est. 30-80ms @ 50K × 1024-dim full scan (no index; SIMD only via gonum/intrinsics if added) | Owned by us | n/a |
| E. LanceDB Go SDK (`lancedb/lancedb-go`) | Download platform-specific `liblancedb_go.a`, set CGO_CFLAGS/LDFLAGS, separate `.lance` store | YES (prebuilt static lib) | NO | NO (columnar dataset dir, not one file) | <5ms (HNSW/IVFPQ) | Active (updated 2026-06-15); SDK currently pre-1.0 (v0.1.2) | Apache-2.0 |
| F. bbolt + brute force | Add bbolt; separate `vectors.db`; daemon joins | NO | NO (cross-db join in Go) | Yes (bbolt is one file) | Same as D | Stable (etcd-io maintained) | MIT |
| G. go-duckdb + vss | CGo; separate `.duckdb` file | YES | NO (different db engine) | Yes (`.duckdb` file) | <10ms (HNSW) | Active, but CGo + cross-compile pain documented | MIT |

## Recommendation

**Option B — `modernc.org/sqlite/vec`.** Three-line rationale:

1. **Zero migration cost.** The existing modernc driver already opens `leah.db`; add one blank-import line and a `CREATE VIRTUAL TABLE … USING vec0(…)` migration. Every other table (contact, project, decision, mistake_log, operator_profile, ctxmgr, embedding) stays on the exact same driver and connection pool — no driver swap, no table re-creation, no schema_version bump required for non-vec tables.
2. **It is sqlite-vec.** Same `vec0` virtual table, same MATCH-based KNN syntax, same float32 / int8 / binary vector types, same `distance` column. Spec §17.10's "sqlite-vec mandate" is satisfied verbatim. We get DiskANN / IVF index types as the upstream pure-C lands them (modernc tracks sqlite-vec — v1.50 already bumped to v0.1.9).
3. **JOIN story is trivial.** Vec0 rowids JOIN against memory/conversation/integration tables in the same `SELECT` because they live in the same SQLite file under the same driver. No daemon-side join coordination, no cross-store rowid mapping, no separate backup target. `sqlite3 leah.db .dump` still produces a single artifact.

**Migration cost from current state (brute-force Go cosine over `embedding` BLOB):**

- **~1 day of work.** Concretely:
  - Bump `modernc.org/sqlite` in `go.mod` from current pin → `v1.50.1` (or latest minor).
  - Add `_ "modernc.org/sqlite/vec"` blank import in `internal/embed` (or wherever the driver init lives).
  - Schema migration v6: `CREATE VIRTUAL TABLE embedding_vec USING vec0(item_id TEXT PARTITION KEY, item_type TEXT PARTITION KEY, embedding float[1024]);`
  - Rewrite `internal/embed/embed.go` search path: replace the in-Go brute-force cosine loop with `SELECT item_id, item_type, distance FROM embedding_vec WHERE embedding MATCH ? AND k = ? ORDER BY distance`.
  - Backfill: one-shot `INSERT INTO embedding_vec SELECT … FROM embedding` (only needed if you keep both during cutover).
  - Update `internal/embed/embed.go` doc comment — the entire "Why not sqlite-vec?" rationale is now obsolete.
  - Delete the in-Go cosine implementation (deletion-default win).
- **Risks:** sqlite-vec is pre-v1 (current upstream v0.1.10-alpha; modernc vendors v0.1.9). Breaking changes possible. Mitigation: pin modernc minor version; re-evaluate on each bump.
- **What gets smaller:** brute-force cosine loop + benchmark/tuning code in `internal/embed/embed.go` deletes. Plan §17.10's "deferred to Phase 2" exception goes away.

## "If I were starting Leah fresh today" picks

Same answer: `modernc.org/sqlite` + the in-tree `modernc.org/sqlite/vec`. The 2026-03 vendor-in collapsed the entire decision tree — no CGo, no driver swap, no separate vector store, no JOIN coordination, one db file for backup. The only reason to pick anything else is if you need (a) sub-5ms p99 at >1M vectors (then LanceDB or sqlite-vec via CGo for SIMD), or (b) you're already on Postgres for transactional reasons (then pgvector). Neither applies to a single-operator personal daemon at 50K-vector scale.

The brute-force fallback in the original plan was correct *given the constraint that loading C extensions into modernc was impossible*. That constraint no longer exists; the recommendation should be updated.

## Why each other option loses (one line each)

- **A (sqlite-vec via mattn CGo):** Migrates every existing table to a different driver for a benefit (SIMD-accelerated vec0) the pure-Go port already delivers semantically. The ~1-2wk cost was the right reason to defer in January, and is now the wrong choice in June.
- **C (chromem-go):** Separate store kills the JOIN story; gob persistence isn't a single SQLite file; still in beta; latency at 50K × 1024 likely 50-70ms vs vec0's 5-25ms.
- **D (current brute-force):** Works fine now but loses to B on latency at year-3 scale (~80K vectors) with zero migration savings, since B is also a one-day change.
- **E (LanceDB):** CGo + platform-specific static libs + separate columnar store + lossy JOIN — every constraint we were trying to avoid.
- **F (bbolt + brute):** All of D's downsides plus a second database engine.
- **G (DuckDB + vss):** CGo cross-compile pain + separate file + different SQL dialect for JOINs.

## Sources (fetched 2026-06-21)

- `gitlab.com/cznic/sqlite/CHANGELOG.md` (v1.45.0, v1.47.0, v1.49.0, v1.50.0, v1.50.1)
- `gitlab.com/cznic/sqlite/vec_test.go` (canonical vec0 usage from modernc tree)
- `pkg.go.dev/modernc.org/sqlite` (README confirming `vec/` sibling to `lib/` per GOOS/GOARCH)
- `github.com/asg017/sqlite-vec` (upstream — latest release v0.1.9 2026-03-31; HEAD v0.1.10-alpha.4 2026-05-18)
- `github.com/philippgille/chromem-go` (own benchmarks: 100K @ 768-dim ≈ 40ms; last release v0.7.0 Sept 2024; HEAD commit 2026-05-17)
- `github.com/lancedb/lancedb-go` (CGo + prebuilt static lib install instructions; updated 2026-06-15)
- `github.com/marcboeker/go-duckdb` (CGo cross-compile caveats documented)
- `github.com/etcd-io/bbolt` (single-file backup via Tx.WriteTo)
- `github.com/pgvector/pgvector` (requires Postgres server)

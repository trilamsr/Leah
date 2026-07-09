# Memory consolidation pass ("dreaming") — Leah

Date: 2026-06-10
Scope: MVP-5 (Wave-8 S9)
Owners: `internal/operatormodel/`, `internal/audit/`, `internal/daemonloop/`,
`internal/memory/` (schema), `cmd/leah/suggest.go`

Companion docs:
- `docs/engineer/briefs/2026-06-10-wave-8-aiml-upgrade.md` — wave-8 synthesis (§S9)
- `docs/engineer/specs/2026-06-10-eval-pipeline.md` — pass-rate gate
- `internal/operatormodel/profile.go` — current 30d audit reader
- `internal/audit/audit.go` — `audit.jsonl` append-only writer
- `internal/daemonloop/loop.go` — `DailyHour` 3am tick hook

Pattern source: OpenAI Dreaming V3 (2026) + SCM (Semantic Consolidation
Memory) paper 2026 — episodic-to-semantic abstraction at nightly cadence.

## 1. Goal

The 30-day audit window is a load-bearing input to `Profile.Update`
(read full file every tick) and `Recommend` (decayed weight). At
steady-state operator cadence (~4 events/day × 30d × ~6 kinds), the
audit slice is small. At sustained dispatch volume (parallel agents,
voice events, instrumentation rows) it grows into the hundreds of
thousands of rows and `audit.jsonl` rotates at 100MB. Reading the
whole window every Update tick is wasted work for cells whose
weight has not meaningfully changed in two weeks.

S9 collapses stable, decayed `(class, key, slot)` cells into a durable
summary row, archives the underlying audit rows ≥14d to a separate
append-only file, and prunes them from the live `audit.jsonl`. Result:
- live audit stays bounded by ≤14d of raw rows + recent rotation tail,
- `Profile.Update` reads consolidated rows (cheap row scan) for ≥14d
  history and raw rows for ≤14d history — same logical 30d window,
- replay (`leah profile replay --since=<date>`) reconstructs full
  state by reading both stores.

### Goals

- 3am local cron pass that finds stable cells and writes summary rows.
- Atomic move of source audit rows to `~/.leah-state/consolidated.jsonl`.
- Schema-additive — no breaking change to `audit.Entry` or `ProfileRow`.
- Zero-LLM cost.
- Kill-switch via `LEAH_CONSOLIDATION=0`.
- Operator-readable consolidated rows surfaced by `leah whoami`.

### Non-goals (v1)

- LLM-based semantic compression of summaries (deferred — zero-cost rule).
- Cross-operator merging.
- Re-consolidation of already-consolidated rows.
- Adaptive cadence (always 3am; tuning deferred to W128+).
- Backfill of pre-S9 audit history (consolidation starts from W127 deploy).

## 2. Stability gate

A cell `(class, key, slot)` is eligible at tick T when ALL hold:

1. `last_consolidated_at IS NULL OR last_consolidated_at < T - 14d`
2. Rolling 14d decayed-weight delta < 5%:
   `|w(T) - w(T - 14d)| / max(w(T), w(T - 14d), epsilon) < 0.05`
   where `w(T - 14d) :=` weight in `operator_profile_snapshot` for this
   cell (see §3.3), or `0` if no prior snapshot exists. Snapshots are
   written by each consolidation pass, so the 14d anchor never depends
   on raw audit rows older than the ≤14d retention window.
3. `first_seen <= T - 14d` (cell has at least 14 days of history)
4. At least one underlying audit row with `ts < T - 14d` exists

Rationale: rule 2 alone trips on cold-start zero-weight cells; rule 3
forces a minimum age; rule 4 prevents empty consolidations when audit
rotation has already discarded the rows. `epsilon = 1e-6` avoids
divide-by-zero on first observation.

Cells failing the gate are skipped this pass and re-evaluated next tick.

## 3. Schema

### 3.1 New table — `operator_profile_consolidated`

```sql
CREATE TABLE IF NOT EXISTS operator_profile_consolidated (
  class                  TEXT NOT NULL,   -- mirrors operator_profile.class
  key                    TEXT NOT NULL,   -- mirrors operator_profile.key
  slot                   TEXT NOT NULL,   -- mirrors operator_profile.slot
  weight                 REAL NOT NULL,
  count                  INTEGER NOT NULL,
  first_seen_ts          TEXT NOT NULL,   -- RFC3339
  last_consolidated_at   TEXT NOT NULL,   -- RFC3339
  source_window_end      TEXT NOT NULL,   -- RFC3339 — newest row consolidated
  PRIMARY KEY (class, key, slot)
);

CREATE INDEX IF NOT EXISTS idx_consolidated_last
  ON operator_profile_consolidated(last_consolidated_at);
```

Operator-readable on disk (`~/.leah-state/leah.db`) and exposed via
`leah whoami --consolidated` (§7). The PK triple `(class, key, slot)`
matches the live `operator_profile` schema exactly — no dimension
collapse, no `time_of_day` × `cadence` slot collisions, and
`context_transition` keys on `action_kind` rather than going unkeyed.

### 3.2 Archive — `~/.leah-state/consolidated.jsonl`

Append-only JSONL, same `audit.Entry` shape, no schema change.
0600 perms. Rotated at 100MB matching `audit.jsonl` semantics.

### 3.3 New table — `operator_profile_snapshot` (stability-gate anchor)

The §2 rule 2 delta requires `w(T - 14d)`. Raw audit retention is ≤14d
(§1), so the anchor cannot come from re-reading raw rows. Each
consolidation pass writes one snapshot row per consolidated cell;
the next pass reads it back as the 14d-old anchor.

```sql
CREATE TABLE IF NOT EXISTS operator_profile_snapshot (
  class                  TEXT NOT NULL,
  key                    TEXT NOT NULL,
  slot                   TEXT NOT NULL,
  weight_at_snapshot     REAL NOT NULL,   -- w at snapshot_ts
  snapshot_ts            TEXT NOT NULL,   -- RFC3339 — pass-fire time
  PRIMARY KEY (class, key, slot)
);
```

Tradeoff: one extra upsert per consolidated cell per 14d cycle.
Empirically <200 cells/operator (§13) → <200 rows/14d → negligible.

### 3.4 Per-class slot derivation

Step 3 of §4 filters source audit rows whose derived slot matches the
cell's slot. Derivation is class-specific:

| class                | slot derivation                                |
|----------------------|------------------------------------------------|
| `time_of_day`        | hour-of-day, `HH` in 00-23 (`ts.Hour()`)       |
| `cadence`            | weekday-name, `Mon`/`Tue`/.../`Sun`            |
| `context_transition` | `action_kind` field of the audit row           |

W125 implementer reads this table verbatim; no per-class branching
buried in code review.

### 3.5 Migration

`internal/memory/schema.go` adds both `CREATE TABLE IF NOT EXISTS`
statements. Idempotent — replays of `Init` are no-ops.

## 4. Consolidation step (per `(class, key, slot)` cell)

In a single SQL transaction + serialized file move + an audit-logger
quiesce window:

1. Compute summary:
   `weight = decayedWeight(times[ts < T-14d], now=T-14d, halflife)`
   `count = len(times[ts < T-14d])`
   `first_seen_ts = min(times)`
2. `INSERT … ON CONFLICT(class,key,slot) DO UPDATE` into
   `operator_profile_consolidated` with the summary +
   `last_consolidated_at = T`.
3. Append all source rows where `ts < T - 14d` AND
   `class = <cell.class>` AND `key = <cell.key>` AND
   (slot derived from the row per §3.4 equals `<cell.slot>`) to
   `~/.leah-state/consolidated.jsonl`.
4. fsync archive file (durability barrier).
5. `INSERT … ON CONFLICT(class,key,slot) DO UPDATE` into
   `operator_profile_snapshot` with `weight_at_snapshot = w(T)` and
   `snapshot_ts = T` (§3.3 anchor for the next pass).
6. Commit DB transaction.
7. Acquire `audit.Logger.QuiesceForConsolidation(ctx)` (see contract
   below); rewrite `audit.jsonl` minus the moved rows via tmp-file +
   rename (POSIX atomic on same filesystem); release quiesce.

**`audit.Logger.QuiesceForConsolidation` contract (new, mandatory).**
Returns a `func()` release handle. Between acquire and release, all
`audit.Logger.Append` calls block on the same mutex; the rename swaps
the inode atomically; the next `Append` reopens with `O_APPEND` on
the new inode. Without this contract, an `Append` that opened the
old inode between step-7 read and rename writes to the orphaned file
descriptor and the row is silently lost.

If step 4 fails: DB transaction rolled back, archive append rolled
back via truncate-to-pre-append-offset, source `audit.jsonl` untouched.
If step 7 fails: summary row durable + archive durable; source rows
still present → next pass re-detects them as already-consolidated via
`last_consolidated_at >= row.ts` and only does step 7. Idempotent.

Failure mode (README.md root-cause discipline): source rows are NOT
deleted until summary row is durably written AND archive is fsync'd.
A crash between any two steps leaves the system either in pre-pass
state or in a state where re-running the pass converges.

## 5. Daemon tick

Reuse `daemonloop.Loop`:
- `DailyHour = 3` (already-supported gate).
- `Daily` task slice: append a `consolidatePass` closure.
- `DailyTracker = ~/.leah-state/consolidation.tracker`.

```go
loop.Daily = append(loop.Daily, func(ctx context.Context) {
    if os.Getenv("LEAH_CONSOLIDATION") == "0" {
        return
    }
    if err := operatormodel.ConsolidatePass(ctx, store, auditPath,
        archivePath); err != nil {
        // log only — never block other daily tasks
    }
})
```

The 3am hour gate uses local time (`time.Now().Hour()` already does);
tz offset handled by the OS clock. No new tz plumbing.

The morning-brief task (existing Daily entry) runs in the same Daily
slice — order is fire-and-forget per `loop.go`. Consolidation
completes in O(seconds) on realistic audit volumes (≤500k rows).

## 6. `Profile.Update` extension

Single `Profile` struct, two read paths:

```go
func (p *Profile) Update(ctx, store, auditPath, since) error {
    cutoff := p.now().Add(-14 * 24 * time.Hour)

    // ≤14d: raw audit rows (existing path).
    rawRows, _ := readAudit(auditPath, max(since, cutoff))

    // >14d, ≥since: consolidated summary rows (new path).
    sumRows, _ := loadConsolidatedSince(store.DB(), since, cutoff)

    // observers see the union — sumRows synthesize zero-cost
    // (Class, Key, Slot, Weight) tuples without per-event times.
    …
}
```

`loadConsolidatedSince` returns synthetic `ObserveResult` rows whose
`Weight` is the durable summary value and `Count` is the durable count.
The three `Observe*` functions stay event-driven for the raw slice;
the consolidated slice bypasses them (already-aggregated).

The decay halflife at consolidation time was applied against `T - 14d`
as `now`, so consolidated weights are already age-aligned to the
consolidation moment, not the current Update tick. Update applies an
additional decay of `(p.now() - source_window_end)` to keep the
ranking comparable to fresh raw rows.

Bucket re-decay is not equivalent to summing per-event decay over the
same interval (`Σ exp ≠ exp Σ`). The error is bounded by the bucket
window: empirically <14% on typical operator distributions across one
14d window. Re-consolidation of already-consolidated rows is banned
(§14 non-goal) to prevent the error from compounding across passes.

Cold-start gates (`ColdStartMinRows`, `ColdStartMinDays`) sum both
slices with class-specific rules:
- `ColdStartMinRows`: a consolidated cell contributes `count` rows;
  a raw row contributes 1.
- `ColdStartMinDays` (DaysObserved): a consolidated cell contributes
  `max(7, days_in_window)` days where `days_in_window` is
  `(source_window_end - first_seen_ts).Days()`; raw rows contribute
  distinct calendar dates. Union, not sum — overlap is OK.

## 7. Replay

New subcommand wired in the existing `cmd/leah/suggest.go` (matches
the one-file-per-verb pattern of `forget.go`, `quote.go`, etc. — no
new top-level file):

```
leah suggest replay --since=<RFC3339>
```

Reads `operator_profile_consolidated` rows with
`source_window_end >= since`, then `consolidated.jsonl` rows with
`ts >= since`, then `audit.jsonl` rows with `ts >= since`. Feeds them
through the existing `Observe*` pipeline. Prints the resulting
`Profile` table — operator can audit "what did Leah think on
2026-05-01?" against a single command.

Flags:
- `--json` machine-readable output.
- `--no-archive` skip `consolidated.jsonl` (debug: pure-DB view).

## 8. Wave plan (serial-by-data-flow, single-owner per wave)

| Wave | Files (single owner) | Goal |
|------|----------------------|------|
| W124 | `internal/memory/schema.go` + `_test.go` | Add `operator_profile_consolidated` + `operator_profile_snapshot` tables; migration test |
| W125 | `internal/operatormodel/consolidate.go` + `_test.go` | `ConsolidatePass`, stability gate, atomic move, snapshot upsert |
| W126 | `internal/operatormodel/profile.go` (Update path), `consolidated_loader.go` + `_test.go` | Dual-read in Update; cold-start sums both slices |
| W127 | `cmd/leah-daemon/main.go` (wire Daily task), `cmd/leah/suggest.go` (replay verb), `internal/operatormodel/replay.go` + `_test.go` | Daemon wiring + replay subcommand |

W124 → W125 → W126 → W127 serialize by data flow.
- **W125 + W126 same package** (`internal/operatormodel/`): W126 depends
  on W125's helper symbols (`loadConsolidatedSince`, snapshot reader).
  Serialize despite different filenames; file-disjointness inside one
  package is not enough when symbols cross files.
- W127 serial after W126.
- File-disjoint cross-wave dispatch is moot here; this is a 4-wave
  serial chain.

## 9. Test plan (TDD — failing test first per wave)

### W124 — schema migration

- `TestSchema_Consolidated_CreateIdempotent` — calls `Init` twice; second
  call MUST NOT error and MUST NOT drop rows.
- `TestSchema_Consolidated_UpsertOnConflict` — duplicate
  `(class, key, slot)` row exercises the real
  `INSERT … ON CONFLICT(class,key,slot) DO UPDATE` production path:
  second insert replaces `weight`/`count`/`last_consolidated_at` in
  place; row count stays 1. (The bare UNIQUE-error path is unreachable
  in production code; testing it asserts nothing useful.)
- `TestSchema_Snapshot_UpsertOnConflict` — same upsert contract for
  `operator_profile_snapshot`.

### W125 — consolidation pass

- `TestConsolidate_StabilityGate_RejectsRecent` — cell with
  `first_seen` 10d ago → skipped.
- `TestConsolidate_StabilityGate_RejectsVolatile` — 30% weight delta
  over 14d → skipped.
- `TestConsolidate_StabilityGate_AcceptsStable` — 2% delta + 20d age
  → row written, source moved.
- `TestConsolidate_Atomicity_ArchiveFailRollsBack` — inject
  fsync failure → DB unchanged, `audit.jsonl` unchanged.
- `TestConsolidate_Idempotent_ReRunNoOp` — second pass over same
  state writes no new rows, archives no new lines.
- `TestConsolidate_AuditQuiesce_NoLostAppend` — concurrent
  `audit.Logger.Append` racing with the step-7 rewrite must serialize
  through `QuiesceForConsolidation`; the appended row lands in the
  post-rename `audit.jsonl`, not the orphaned inode.
- `TestConsolidate_Snapshot_AnchorsNextPass` — first pass writes a
  snapshot; second pass 14d later reads `w(T-14d)` from it (not from
  raw audit) when evaluating §2 rule 2.
- `TestConsolidate_KillSwitch_EnvDisables` — `LEAH_CONSOLIDATION=0`
  → ConsolidatePass returns nil immediately, no side effects.

### W126 — dual-read Update

- `TestUpdate_DualRead_UnionWeights` — seed 5 consolidated cells +
  20 raw rows; `len(p.Rows) == 5 + observer output`.
- `TestUpdate_DualRead_DecayAlignment` — consolidated weight at
  `T - 30d`, raw row at `T - 5d`, ratio matches single-halflife model
  within 1e-6.
- `TestUpdate_DualRead_ColdStartCountsBoth` — 30 consolidated + 25
  raw rows clears `ColdStartMinRows=50` gate.

### W127 — daemon wiring + replay

- `TestDaemon_DailyTask_FiresAtHour3` — synthetic clock at 03:01;
  ConsolidatePass invoked exactly once per day.
- `TestReplay_ReproducesProfile` — golden audit slice → consolidate
  → replay `--since=<window_start>` → byte-equal `Profile` table.
- `TestReplay_NoArchive_PureDB` — `--no-archive` skips JSONL; weights
  match pre-archive snapshot.

Eval harness (`make eval`) gains a new feature `evals/consolidation.jsonl`:
golden 30d audit → expected `Profile` row set after consolidation.
Pass-rate gate ≥98% required per `docs/engineer/specs/2026-06-10-eval-pipeline.md`.

## 10. Privacy + retention

Consolidated rows persist longer than raw audit (raw = 30d window,
consolidated = 90d default). Operator override via
`LEAH_CONSOLIDATED_RETENTION_DAYS` (positive int; 0 = retain forever).

`leah whoami --consolidated` enumerates every row in
`operator_profile_consolidated` + the archive's tail. Mirrors the
S10 trust-moat surface (`leah whoami --full` will subsume this flag).

`leah purge --everything` (S10) MUST delete `consolidated.jsonl` and
`operator_profile_consolidated` rows in the same sweep as `audit.jsonl`.

## 11. Operator override

- `LEAH_CONSOLIDATION=0` — kill-switch; daily task no-ops.
- `LEAH_CONSOLIDATION_STABILITY_DELTA=0.05` — tune the 5% gate.
- `LEAH_CONSOLIDATION_AGE_DAYS=14` — tune the 14d age floor.
- `LEAH_CONSOLIDATED_RETENTION_DAYS=90` — archive retention.

All four read at task-fire time, not init — operator can flip without
daemon restart.

## 12. Failure modes (root-cause table)

| Failure | Detection | Recovery |
|---------|-----------|----------|
| Archive fsync fails mid-write | err return from `fsync(2)` | Truncate archive to pre-append offset; abort DB tx; source untouched |
| DB tx commit fails after archive append | err return from `tx.Commit()` | Truncate archive; source untouched; next pass retries |
| `audit.jsonl` rewrite fails after DB commit | err return from `os.Rename` | Summary durable; source rows still present → next pass detects via `last_consolidated_at >= ts` and only rewrites |
| Daemon killed mid-pass | tracker file un-updated | Next 3am tick re-runs from scratch; idempotent by §4 |
| Disk full | err on archive append | Same as fsync fail |
| Clock skew (system clock > 14d back) | `T - 14d < first_seen` | Stability gate rejects; no pass runs |

## 13. Cost

- Zero LLM calls.
- Disk: ~1MB/month `consolidated.jsonl` growth at steady-state operator
  cadence (50-event-day × 30d × ~200 bytes/row × 1/14 consolidation
  ratio).
- CPU: O(N) where N = audit rows in 30d window; consolidation is one
  linear scan + bounded SQL upserts.
- Memory: O(distinct cells) — bounded by `(class, key, slot)` cardinality,
  empirically <200 cells per operator.

## 14. Out of scope (explicit deferrals)

- LLM-based summary text (zero-cost rule).
- Cross-laptop sync of consolidated rows.
- Re-consolidation of consolidated rows into deeper-time summaries
  (one-level consolidation only in v1).
- Encrypted-at-rest archive (covered by S10 `leah export`).
- Backfill of pre-S9 audit (consolidation starts from W127 deploy).

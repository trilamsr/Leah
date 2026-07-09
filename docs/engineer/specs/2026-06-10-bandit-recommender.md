# Wave-8 S4 — Thompson-sampling recommender + change-point detection

Status: design draft (Wave-8 phase 2)
Date: 2026-06-10
Source brief: `docs/engineer/briefs/2026-06-10-wave-8-aiml-upgrade.md` (S4)
Closes the W18-deferred wiring noted in `internal/recommend/storage.go`:
`RecordFeedback` persists rows but the propose-time blender that
consumes them is unwired. This spec delivers that wiring first, then
layers Thompson sampling, BOCPD change-point detection, cold-start
prior, and nightly meta-learning on top.

## 1. Goal

Convert the static α-blended ranker into a calibrated bandit:

- Per-pattern Beta posterior `(α, β)` updated by Accept/Reject/Ignore.
- Propose-time draws one sample per candidate pattern → top-K of samples,
  not top-K of point estimates. Exploration emerges from variance.
- BOCPD watches daily `ProfileRow.Weight` deltas; when the operator's
  behavior shifts (new job, schedule reshuffle), the effective decay
  window shrinks to days-since-changepoint, so the bandit forgets stale
  evidence in days rather than half-lives.
- Cold-start ships ≈20 hand-curated patterns so day-1 Propose is non-empty.
- Nightly meta-learning grid-searches the behavioral halflife on real
  feedback instead of a fixed 14-day constant.

## 2. Non-goals

- Contextual bandit (LinUCB / neural). Beta posterior per pattern is
  cardinality-bounded and hermetically testable; contextual variants need
  feature engineering we don't yet have.
- Cross-operator bandit pooling. Single-operator architecture is
  intentional moat (`memory/feedback_autonomous_handoff_2026-06-10.md`).
- LLM-ranked propose path. Pure-Go sampler; LLM stays out of the hot loop.
- Replacing the `recommend_feedback` table. The blender consumes it.
- Touching `MemoryEngine`. SQLite-only — `MemoryEngine` keeps its W15
  hermetic surface for test seams.

## 3. W18 propose-time blender (lands FIRST — blocks the rest)

The wiring `storage.go:314` defers is the propose-time consumer of
`recommend_feedback` rows. Without it, every `RecordFeedback` call writes
a row no ranker reads — orphan rows by design today.

Blender contract (lands as `internal/recommend/blender.go`):

```go
// BlendedScore returns the propose-time score for one candidate
// pattern. It groups recommend_feedback rows by pattern, applies the
// decay-weighted sum (spec §8 of 2026-06-10-learn-recommend-apply), and adds the
// signal to a static prior. Pure func — DB handle injected so callers
// can reuse the SQLiteEngine connection.
func BlendedScore(
    ctx context.Context,
    db *sql.DB,
    pattern string,
    priorWeight float64,
    now time.Time,
    halflifeDays float64,
) (float64, error)
```

Read path: `SELECT kind, signal, ts FROM feedback WHERE rec_id IN
(SELECT id FROM recommendations WHERE pattern = ?)` groups by pattern via
the join, applies `decayedMagnitude` (existing in `feedback.go`), sums
into `score = priorWeight + Σ decayed signals`.

Why pattern-keyed not rec-keyed: rec IDs are ephemeral (one per Propose
loop iteration), patterns are durable identities. A new rec for the same
pattern inherits the pattern's signal history.

Wired into `SQLiteEngine.Propose` after this PR lands: each candidate
row's `Confidence` is replaced by `BlendedScore(pattern, ...)`. Greedy
sort preserved until W101 swaps in Thompson sampling.

Orphan-row fix: this PR closes the W18 godoc TODO at `storage.go:314`
and updates the comment to "consumed by `BlendedScore`."

## 4. Beta posterior per pattern (W101)

### 4.1 Update-magnitude rationale (Ignore = β+=0.1)

Half-step (vs full β+=1) reflects that Ignore is a weaker negative
signal than Reject: the operator may have simply missed the
recommendation, not rejected it. Magnitude 0.1 matches the existing
`internal/recommend/feedback.go signalIgnore = -0.1` halflife
amplitude, so blender and bandit treat the same physical signal with
the same weight.

### 4.2 Schema migration

Bump `sqliteSchemaVersion` `"1"` → `"2"`. DDL additions:

```sql
CREATE TABLE IF NOT EXISTS recommend_pattern_state (
  pattern        TEXT PRIMARY KEY,
  alpha          REAL NOT NULL DEFAULT 1.0,
  beta           REAL NOT NULL DEFAULT 1.0,
  observed_from  INTEGER NOT NULL,  -- first-observation UnixNano
  forgotten      INTEGER NOT NULL DEFAULT 0,
  demoted_until  INTEGER NOT NULL DEFAULT 0
);
```

Existing `feedback` table is unchanged; the posterior is a derived view
the engine maintains incrementally on each `RecordFeedback`. Storing the
posterior (rather than re-deriving every Propose) keeps Propose at O(K)
where K = candidate count, not O(N) over historical feedback.

### 4.3 Update rule (spec brief verbatim)

| Signal | α | β |
|---|---|---|
| Accept  | +=1   | -    |
| Reject  | -     | +=1  |
| Ignore  | -     | +=0.1 |

Implemented as a single UPSERT inside `RecordFeedback`. The caller
maps signal → row values (`Accept` → `(alpha=2.0, beta=1.0)`,
`Reject` → `(alpha=1.0, beta=2.0)`, `Ignore` → `(alpha=1.0,
beta=1.1)`), threading the per-signal delta through the schema
defaults so the UPSERT math composes:

```sql
INSERT INTO recommend_pattern_state (pattern, alpha, beta, observed_from)
VALUES (?, ?, ?, ?)
ON CONFLICT(pattern) DO UPDATE SET
  alpha = alpha + excluded.alpha - 1.0,
  beta  = beta  + excluded.beta  - 1.0;
```

(`-1.0` cancels the default-row contribution on conflict so the math
matches the table above on both the first-write and update paths;
caller-side INSERT values are the per-signal deltas plus the 1.0
default, decoded by the `- 1.0` in the ON CONFLICT branch.)

### 4.4 Sampling

Propose draws one Beta sample per candidate. Beta(α, β) sampled via
two Gamma draws: `X ~ Gamma(α,1)`, `Y ~ Gamma(β,1)`, `B = X/(X+Y)`.
Gamma sampled via Marsaglia-Tsang (`math/rand/v2.Float64` only — no
external dep).

```go
// SampleBeta is hermetically testable via the engine's rngSeed seam.
func SampleBeta(rng *rand.Rand, alpha, beta float64) float64
```

Ranker change (replaces greedy sort in `RankedPropose`):

```go
for _, r := range candidates {
    r.Confidence = SampleBeta(rng, alphaOf(r.Pattern), betaOf(r.Pattern))
}
sort.SliceStable(candidates, func(i,j int) bool {
    return candidates[i].Confidence > candidates[j].Confidence
})
return candidates[:min(K, len(candidates))]
```

Top-K is now top-K of samples — exploration emerges because the
high-variance Beta(2,2) sometimes outranks the low-variance Beta(20,5).

## 5. BOCPD change-point detection (W102)

Bayesian Online Change-Point Detection (Adams & MacKay 2007) on the
daily `ProfileRow.Weight` time series. Per-row, not per-operator —
each pattern has its own change-point clock.

### 5.1 Algorithm sketch

For each day `t`, maintain a run-length distribution `P(r_t | x_{1:t})`
over how long the current regime has persisted. On new observation:

1. Predictive prob under each run length → hazard-weighted growth.
2. `r_t` collapses to 0 if the predictive likelihood is dominated by
   the changepoint hypothesis.
3. When `argmax P(r_t)` jumps from a large `r` to 0, emit changepoint.

Hazard rate = `1/180` (≈ one regime shift per 6 months — operator's
expected job/calendar reshuffle cadence; env-overridable via
`LEAH_RECOMMEND_BOCPD_HAZARD_DAYS`).

Observation model: Gaussian on the day-over-day `Weight` delta with
the moving-window variance.

Warmup floor: BOCPD requires `≥ 14d` of observations on a pattern
before it can emit a changepoint. Below the floor the run-length
distribution is too sparse to outvote the prior. Patterns under the
floor inherit the global behavioral halflife from W104 unchanged.

### 5.2 Audit row

On changepoint detection:

```
audit.Entry{
    Kind:    "operator_changepoint_detected",
    Detail:  pattern,
    Outcome: fmt.Sprintf("run_length=%d delta=%.3f", r, delta),
}
```

### 5.3 Effective-window shrink

Post-changepoint, the per-pattern effective halflife clamps to
`min(behavioralHalflife, daysSinceChangepoint)` — a SHRINK, not a
drop. Pre-changepoint rows stay in `recommend_feedback`; their
contribution to `BlendedScore` decays faster because the halflife
constant in `decayedMagnitude` shrinks, not because rows are
deleted. This keeps the audit log replayable and preserves
W18's pattern-keyed history. The bandit thus forgets pre-changepoint
feedback within days, not 30-day halflives.

Storage: `recommend_pattern_state.changepoint_at` (UnixNano, 0 = none)
added in the schema-2 migration.

## 6. Cold-start prior (W103)

`evals/cold_start_prior.json` checked into the repo. Schema:

```json
[
  {
    "pattern": "commit_at_focus_end",
    "alpha":   3.0,
    "beta":    1.0,
    "reason":  "broadly accepted across pilot operators"
  },
  ...
]
```

~20 hand-curated entries. Loaded by `Engine.Bootstrap()` on first
process start (idempotent — keyed on `recommend_pattern_state.pattern`,
INSERT OR IGNORE).

Blend factor with observed feedback:

```
effective_alpha = (1-w) * prior_alpha + w * observed_alpha
w = min(1.0, days_observed / 14.0)
```

Day-1: pure prior. Day-14+: pure observed. Smooth ramp in between.
`days_observed` = `(now - recommend_pattern_state.observed_from) / 86400`.

Curated entries are reviewed manually before merge — review checklist:
no remote-write actions (calendar add, email send), no auto-tier
patterns above α=2.0 (operators deserve to see new auto-applies hit
confirm first), each entry has a single-line `reason` that survives
into the audit row.

### 6.1 Thompson activation floor (hard)

Thompson sampling activates only after the operator has accumulated
`≥ 20` feedback rows spread across `≥ 5` distinct patterns. Below
this floor the bandit falls back to greedy point-estimate sort over
the cold-start prior — a Beta(α, β) sampler with α≈β≈1 produces
near-uniform draws and would surface noise as exploration. Floor is
checked once per Propose call against `COUNT(*)`/`COUNT(DISTINCT
pattern)` on `recommend_feedback`; cached for the duration of the
process to avoid hot-loop SQL.

Floor exit logs one audit row `bandit_floor_cleared` so post-hoc
replay can pinpoint the transition.

## 7. Nightly meta-learning grid-search (W104)

Replaces the fixed `defaultBehavioralHalflifeDays = 30.0` constant in
`internal/recommend/feedback.go` with a tuned value persisted in
`operator_profile_meta.tuned_halflife_days` (existing KV table —
`internal/operatormodel/profile.go` already implements UPSERT).

### 7.1 Grid + scoring

Grid: halflife ∈ {7, 14, 30, 60, 90} days. For each value, replay the
last 90 days of `recommend_feedback` rows against the bandit and
compute the accept-rate proxy:

```
score(hl) = Σ_t [ recommendation_accepted_at_t ] / Σ_t [ proposed_at_t ]
```

over the last 90d of `recommend_feedback`. Pick `argmax score(hl)`.
The replay loop is a single in-package function `MetaLearnHalflife`
in `internal/recommend/metalearn.go`; it reads `recommend_feedback`
rows directly via the existing `storage.go` connection — no new
storage primitive, no `evals/` package, no `internal/selflearn/`
dependency.

### 7.2 Scheduling

Nightly tick reuses the existing `internal/daemonloop.Loop` DailyHour
gate (set to 03 local — runs after the 3am consolidation pass from S9
on the same daily tick). A single new `DailyTask` closure invokes
`MetaLearnHalflife` and writes the chosen value via
`operatormodel.UpsertMeta(db, "tuned_halflife_days", ...)`. No new
binary, no `cmd/leah/` surface, no separate cron ticker — the
existing loop already runs in `leah-daemon`.

Audit row `meta_learn_halflife` with `Outcome` = chosen value,
`Detail` = scores across the grid for post-hoc verification.

### 7.3 Compute-budget cap

Worst-case 5 halflives × N patterns × 90d replay is a nightly
compute spike. Hard caps:

- Wall-clock budget: 5 minutes. Cancelled via `context.WithTimeout`.
- Pattern sample: 100 patterns per night, drawn by descending
  `recommend_feedback` row count (most-active first).
- Patterns beyond the sample rotate to the next night's run (cursor
  persisted in `operator_profile_meta.meta_learn_cursor`).

Budget exceeded → emit `meta_learn_budget_exceeded` audit row with
partial result; tuned halflife written only if at least 20% of the
sample completed (otherwise keep the previous value).

### 7.4 Stability

A regression test asserts the chosen halflife only moves >2x when
accept-rate delta is >5%. Prevents noise-driven jitter on the
constant when feedback volume is low. Threshold logic lives inline
in `metalearn.go` — no cross-package dependency.

## 8. Wave plan (file-disjoint, parallelizable after W100)

| Wave | Surface | Files |
|---|---|---|
| W100 | propose-time blender (W18 wiring) | `blender.go` + test + storage.go godoc edit |
| W101 | Beta posterior + Thompson sampling | `bandit.go` + test + schema-2 migration in `storage.go` |
| W102 | BOCPD change-point + audit row | `bocpd.go` + test |
| W103 | Cold-start prior | `evals/cold_start_prior.json` + `bootstrap.go` + test |
| W104 | Nightly meta-learn grid-search | `metalearn.go` + `daemonloop.Loop.DailyTasks` hook + test |

W100 blocks W101–W104 (each consumes blender output). W101–W104 are
file-disjoint, fan out to 4 reviewers in parallel per README.md.

## 9. Test plan (TDD per README.md)

Each wave lands failing test FIRST, capture in PR body, then impl.

### W100 — blender
- `TestBlendedScore_DecaysOverTime`: seed 5 feedback rows over 90d,
  assert older signals decay below recent.
- `TestBlendedScore_PatternIsolation`: feedback for pattern A doesn't
  leak into pattern B's score.
- `TestProposeUsesBlender`: SQLiteEngine.Propose returns rows whose
  Confidence equals `BlendedScore` (golden-file).

### W101 — bandit
- `TestSampleBeta_MarginalsMatchClosedForm`: 10k samples vs analytic
  mean = α/(α+β); within 3σ.
- `TestRecordFeedback_UpdatesPosterior`: Accept → α+=1; Reject → β+=1;
  Ignore → β+=0.1; assert via SQL row read.
- `TestThompsonRanking_ExplorationEmerges`: candidates with identical
  point-estimate but different variance; over 100 propose calls the
  high-variance one is ranked first >20% of the time (exploration
  proxy).
- `TestSchemaMigration_v1_to_v2`: open v1 db, instantiate engine,
  assert `recommend_pattern_state` table exists and defaults applied.
- `TestBanditKillSwitch_FallsBackToGreedy`: set
  `LEAH_RECOMMEND_BANDIT=0`, assert greedy sort.

### W102 — BOCPD
- `TestBOCPD_DetectsStepFunction`: synthetic series with mean shift
  at day 30 → audit row emitted on day 30 (±2d tolerance).
- `TestBOCPD_NoFalsePositiveOnStable`: 90d of i.i.d. noise → zero
  changepoints emitted.
- `TestEffectiveHalflifeShrinks`: post-changepoint, the per-pattern
  halflife passed to blender ≤ daysSinceChangepoint.

### W103 — cold-start
- `TestBootstrap_LoadsPrior`: empty db → 20 patterns inserted.
- `TestBootstrap_Idempotent`: second call inserts zero new rows.
- `TestBlendFactor_RampsTo14d`: day-7 sees 0.5 weight on observed,
  day-14+ sees 1.0.
- Curated-entry lint: pre-PR `make check` runs `evals/lint_prior.go`
  → reject any remote-write or alpha>2 auto-tier entry.

### W104 — meta-learn
- `TestGridSearch_PicksMaxAcceptRate`: synthetic feedback w/ known
  best halflife = 30; grid returns 30.
- `TestThresholdMove_ResistsNoise`: ±3% delta does not flip choice;
  >5% does.
- `TestMetaLearnAuditRow`: row carries scores across grid.

## 10. Operator override

Kill-switch env var:

```
LEAH_RECOMMEND_BANDIT=0   # disables Thompson sampling (W101) only;
                          # greedy point-estimate sort over W100
                          # blender output remains active.
LEAH_RECOMMEND_BANDIT=1   # default — Thompson sampling on top of
                          # the same W100 blender output.
```

W100 is NOT optional. The propose-time blender is the entrypoint
the greedy fallback path consumes identically — both kill-switch
states sort over `BlendedScore(pattern, ...)`. The kill-switch
only swaps the *ranking* function (greedy sort vs Thompson sample),
not the *scoring* surface. Disabling W100 would orphan
`recommend_feedback` rows again (reverting the W18 fix); there is
no env var that does this.

Read once at engine construction; logged into audit on startup so
post-hoc replay can attribute ranking behavior.

Per-pattern override:

```
leah forget <pattern>     # existing — quarantines 30d (re-uses W18 path)
```

No new CLI surface — `leah forget` covers operator opt-out.

## 11. Privacy + non-determinism

Thompson sampling is intentionally non-deterministic. Two operator
guarantees:

1. **Audit replay**: every Propose appends one
   `recommendation_proposed` row per candidate, with
   `Detail = fmt.Sprintf("pattern=%s alpha=%.2f beta=%.2f sample=%.3f seed=%d", ...)`.
   `seed` carries the resolved `LEAH_RECOMMEND_RNG_SEED` (or
   wall-clock fallback) so a replay can re-derive the exact
   `SampleBeta` draws. Operator can reconstruct the ranking by
   re-sorting on `sample`, or regenerate `sample` from `(α, β, seed)`
   for forensic verification.
2. **Seed pinning**: `LEAH_RECOMMEND_RNG_SEED=<int64>` (default = time)
   makes the sampler deterministic for forensic replay. Daemon logs the
   resolved seed at startup. Production default = wall-clock so
   distinct daemon processes don't collide on the same sequence.

No personal data crosses the package boundary — α, β, sample are
floats; pattern names are operator-readable. Same posture as the
existing `recommend_feedback` table.

## 12. Cardinality bound (privacy)

Per-pattern (α, β) — bounded by # patterns observed, not # operators
or # rec instances. Names operator-readable — no PII risk beyond
`recommend_feedback`.

Hard ceiling: 10k pattern rows. Exceeded → oldest non-active rows
pruned by S9 nightly consolidation. Until S9 lands, soft warning in
`leah whoami`.

## 13. Dependencies + sequencing

Hard blockers:
- **W100 blocks everything**: blender wiring is the entrypoint
  W101–W104 all hook into. Land W100 first, alone.
- W101 schema-2 migration must land before W103 (cold-start writes to
  `recommend_pattern_state`).

Soft dependencies:
- S1 eval pipeline (sibling Wave-8 spec) should land before W104 so the
  meta-learning grid-search has the eval-judged accept-rate signal,
  not just raw accept count. If S1 slips, W104 falls back to raw
  count — flagged in audit.
- S8 knowledge-graph wiring (Wave-8 sibling) is independent; bandit
  ranks at the pattern level, knowledge graph would later add
  `entity_key` joins. Out of scope here.

## 14. Open questions

- Beta posterior decay (Bayesian forgetting)? Brief says no — BOCPD
  handles regime shifts. Revisit if changepoint false-negative rate
  >10% on the W104 scorecard.
- Auto-tier posterior-mean threshold? Inherits §6 of
  `2026-06-10-learn-recommend-apply.md` — irreversible remote actions stay
  confirm-floor regardless of posterior.
- Cold-start JSON is operator-trust load-bearing. v1 ships single
  source; operator overrides via `leah forget <pattern>`.

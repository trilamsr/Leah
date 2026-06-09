# Operator-behavior modeling + proactive recommendation

Status: draft
Owner: tri (sole operator)
Builds-on: `internal/patterns/` (audit clusters), `internal/selflearn/` (retro), `internal/daemonloop/` (weekly tick).

## 1. Goal

> "Leah should be able to remember and think about how operator usually
> operates and make recommendations based on that."

Three-step loop:

1. **Observe** — passive, no operator input. Re-read the audit log on the
   weekly tick and aggregate three classes of behavior into an
   `operator_profile` SQLite table.
2. **Model** — pure SQL aggregation. NO LLM. NO embeddings. Lives as
   row-per-(class, key, slot) counts; rebuild from scratch on each tick to
   stay deterministic + cheap.
3. **Recommend** — pure-function ranker. Returns up to 3 candidate next
   actions for a given (context, time) input. Surfaces in 3 places:
   `leah suggest` CLI (on-demand), weekly retro section ("What Leah
   noticed about you this week"), and the morning-brief hook.

**Non-goal**: autonomous action. Recommendations always surface to the
operator; operator decides. No `--auto` mode, ever (`feedback_default_simpler`).

## 2. Schema (memory.db v4 — additive)

```sql
-- schema_version: 4 (additive — operator_profile)
-- See docs/specs/2026-06-09-operator-model.md

CREATE TABLE IF NOT EXISTS operator_profile (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  class       TEXT NOT NULL,    -- 'time_of_day' | 'context_transition' | 'cadence'
  key         TEXT NOT NULL,    -- per-class natural key (see §2.1)
  slot        TEXT NOT NULL,    -- per-class bucket label (e.g. '09', 'Sun', 'leah->regatta')
  count       INTEGER NOT NULL, -- observed events for (class, key, slot) in window
  weight      REAL NOT NULL,    -- decay-adjusted count; recommend() sorts on this
  window_start TEXT NOT NULL,   -- RFC3339 — bounds the audit slice that produced this row
  window_end   TEXT NOT NULL,   -- RFC3339 — typically "now" at observe time
  updated_at  TEXT NOT NULL     -- RFC3339
);
CREATE INDEX IF NOT EXISTS idx_op_profile_lookup ON operator_profile(class, key);

CREATE TABLE IF NOT EXISTS operator_profile_meta (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
-- Seeded keys: 'rows_observed', 'days_observed', 'last_full_rebuild_at',
--              'override_count_7d' (Goodhart counter; see §3.4)

UPDATE schema_meta SET value='4' WHERE key='version';
```

### 2.1 Per-class key/slot encoding

| class                | key                            | slot              | example row                                  |
| -------------------- | ------------------------------ | ----------------- | -------------------------------------------- |
| `time_of_day`        | `<action_kind>`                | `<HH>` (00..23)   | `(time_of_day, "regatta.ship", "10", 12, w)` |
| `context_transition` | `<from_context>`               | `<action_kind>`   | `(context_transition, "leah", "leah.ask", 8, w)` |
| `cadence`            | `<action_kind>`                | `<Mon..Sun>`      | `(cadence, "leah.retro", "Sun", 6, w)`       |

`count` = raw matches in window. `weight` = decay-adjusted (see §3.3).

### 2.2 Differentiation from `patterns`

`patterns.Detect` clusters by `(kind, args_hash[:8])` — answers "what
repeated identical work did you do?" → skill-extraction signal.

`operator_profile` aggregates by `(kind, time-or-context-slot)` — answers
"when / after what do you usually do this kind of work?" → next-action
recommendation signal.

No overlap in storage; both read the same audit.jsonl. Patterns output
is `skill-candidates.md` (operator-curated), profile output is on-demand
`leah suggest` (operator-consulted).

## 3. Design

### 3.1 Observation collector

`internal/operatormodel/observe.go`:

```go
type Observation struct {
  Class, Key, Slot string
  Count            int
}

func ObserveTimeOfDay(rows []audit.Entry, tz *time.Location) []Observation
func ObserveContextTransitions(rows []audit.Entry, switches []ctxmgr.Switch) []Observation
func ObserveCadence(rows []audit.Entry, tz *time.Location) []Observation
```

Inputs are pure data; no DB/file IO inside observers — testable in
isolation. `tz` defaults to `time.Local` (operator's laptop tz; see §5
adversarial finding on tz drift).

### 3.2 Profile.Update

`internal/operatormodel/profile.go::Profile.Update`:

1. Read audit.jsonl rows where `ts >= since`.
2. Read context_switch_log rows in same window (for context_transition).
3. Run the three observers.
4. Apply decay (§3.3) to each observation → `weight`.
5. Inside a single SQL tx: `DELETE FROM operator_profile WHERE class IN
   ('time_of_day','context_transition','cadence')`, then bulk insert.
   Rebuild-from-scratch is fine — the dataset is ≤O(few hundred) rows for
   a single operator over a 30-day window.
6. Update `operator_profile_meta`: `rows_observed`, `days_observed`,
   `last_full_rebuild_at`.

Default window: last 30 days. Configurable via env
`LEAH_OPERATOR_MODEL_WINDOW_DAYS` (kept small so we don't blow up
audit.jsonl scans).

### 3.3 Decay weighting (Goodhart mitigation §5)

`weight = sum over each event of exp(-age_days / halflife)` with
`halflife = 14 days`. Recent observations dominate; a habit that fades
in the last week loses recommendation power within ~2 weeks.

Half-life chosen by inspection (no operator data yet) — fits the
"4-task-per-week" personal-use cadence; faster halflife (3 days) would
over-react to one-off work; slower (60 days) would mask drift.
Re-evaluate after 90 days of real audit data; tracked as a §8 cut.

### 3.4 Override counter (Goodhart mitigation §5)

When `leah suggest` runs and operator does NOT execute any of the
returned recommendations within 1 hour (heuristic: no audit row whose
kind matches a recommended kind appears in the next hour), increment
`operator_profile_meta.override_count_7d`. Resets weekly on the tick.

The CLI surfaces the counter alongside recommendations:
`leah suggest` prints `(you've overridden Leah 3 times this week)`. Above
a threshold of 10 overrides in 7d, prepend a warning: `Leah's model may
be drifting from your actual behavior; consider re-running the weekly
tick or expanding the observation window.`

NOT auto-corrective — operator decides.

### 3.5 Cold-start gate

Profile is `Ready=false` while EITHER:

- `rows_observed < 50`, OR
- `days_observed < 7` (oldest audit ts is < 7d ago).

`Recommend()` returns `[]Recommendation{}` (empty, not error) when
`!Ready`. The CLI prints a friendly cold-start banner with current
counters:

```
leah suggest: not enough data yet (need 50 rows + 7 days; have 31 rows / 4d).
```

Both gates are required (AND) — 50 rows in 1 day = a single bursty
session, not a habit; 7 days of 3 rows/day = sparse, noise-dominated.

## 4. Recommendation surfacer

### 4.1 `leah suggest` CLI

```
leah suggest                 # rank by (current ctx, current hour)
leah suggest --context X     # override current ctx
leah suggest --llm           # phrase via LLM (opt-in; ~$0.005/call)
```

Default output (no `--llm`):

```
Leah suggests:
  1. leah ask          (you ask after entering 'leah' context 70% of the time)
  2. regatta status    (typical at 09:00 — 60% of mornings)
  3. leah retro        (Sunday evening cadence — last fired 6d ago)
```

`--llm` opt-in re-renders the same three rows as a 1-paragraph natural
summary via the existing reasoner. Default off keeps cost zero
(`feedback_default_simpler`). Expected per-call cost when on: ~$0.005
(input ~1k tokens, output ~150 tokens, Sonnet pricing).

### 4.2 Weekly retro append

`selflearn/retro.go::Generate` gains a section:

```
## What Leah noticed about you this week

- You `leah ship` mostly between 09:00-11:00 (12/15 ships this window).
- After switching to 'leah' context, you `leah ask` 8/10 times.
- Weekly `leah retro` runs Sunday evening — held steady this week.
```

### 4.3 Morning-brief

Pre-fetches top-3 recommendations for `(today's date, hour=09)` and
surfaces in the morning notification body. Brief calls `Recommend()`
and renders.

## 5. Adversarial review — findings inline

| # | Severity | Concern | Resolution |
| - | -------- | ------- | ---------- |
| 1 | LOW      | Privacy: profile = habit snapshot on disk | Deferred per memory.db (Phase-X encryption); same blast radius as existing memory tables. Documented; no change. |
| 2 | HIGH     | Goodhart: recs self-fulfill → drift away from real preferences | §3.3 decay + §3.4 override counter. Both surfaced to operator. |
| 3 | HIGH     | Cold-start noise | §3.5 double gate (50 rows AND 7 days); explicit cold-start banner |
| 4 | MED      | Timezone confusion (laptop sleep/wake, travel) | Observers take explicit `*time.Location`; defaults to `time.Local`. Document trade-off: travel will skew the time_of_day class for ~1 halflife. Acceptable for single-operator. |
| 5 | HIGH     | Context-transition pattern feedback loop (rec → action → rec reinforced) | §3.3 decay applies equally; §3.4 override counter must be checked at suggest-time. Recommend NEVER auto-fires — purely on-demand `leah suggest`. |
| 6 | LOW      | LLM cost at suggest time | Default off; doc'd ~$0.005/call when --llm. |
| 7 | MED      | Overlap with `patterns` | §2.2 explicit differentiation; storage disjoint; output channels disjoint. |
| 8 | LOW      | Goodhart in cadence class (recommend "leah retro Sunday" → operator runs Sunday → reinforced forever) | Same §3.3 decay; cadence rows naturally decay if operator stops the habit. |

No CRITICAL findings; HIGH items resolved inline before scaffolding.

## 6. CLI surface (spec only; scaffolding deferred)

```
leah suggest [--context NAME] [--llm]
  Print up to 3 recommended next actions given current (context, time).
  Refuses with a friendly banner when profile is not Ready.

  --context NAME : override the active context (default: ctxmgr.Active)
  --llm          : re-phrase via Claude (opt-in; ~$0.005/call)
```

No flags for window-size or halflife — `default simpler`. Tweak via env
if needed (`LEAH_OPERATOR_MODEL_WINDOW_DAYS`,
`LEAH_OPERATOR_MODEL_HALFLIFE_DAYS`).

## 7. Build order

1. Spec + schema v4 + `operatormodel` package
   (`profile.go`, `observe.go`, `recommend.go`) with TDD; daemon weekly
   task #4 wired in `cmd/leah-daemon/main.go::buildWeeklyTasks`.
2. `cmd/leah/suggest.go` CLI + `selflearn/retro.go`
   append-section + morning-brief integration.

## 8. Cuts (Phase-X reopen-trigger: external customer ask or 90 days of audit data)

- **Autonomous action**: no `--auto` mode that runs recommended actions
  without confirmation. Permanent cut.
- **Embedding-based clustering**: no vector store; SQL aggregation only.
  Reopen if SQL ranking demonstrably under-performs and operator volunteers.
- **LLM-required modeling**: model itself is pure SQL. LLM only at
  suggest-time phrasing, opt-in.
- **Multi-operator drift correction**: single operator; no cross-operator
  averaging. Phase-X.
- **Halflife auto-tuning**: hard-coded 14d. Reopen after 90 days of real data.
- **Cross-class joins (e.g. "Sunday morning + leah context → leah retro")**:
  start with single-class ranking; joins next iteration if signal weak.

## 9. References

- Pattern recognition: `docs/specs/2026-06-09-pattern-recognition.md`
- Self-learning: `docs/specs/2026-06-09-self-learning-personal.md`
- Daemon loop: `internal/daemonloop/loop.go::WeeklyTask`
- Audit shape: `internal/audit/audit.go::Entry`
- Context switches: `internal/memory/schema.sql` (`context_switch_log`)

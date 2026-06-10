# Leah learn → recommend → apply loop

Status: design draft (Wave 15 prep)
Owner: operator (tri@maydow.com)
Scope: MVP — single operator, single laptop. No multi-tenant.

Closes the gap between "Leah observes operator behavior" (already shipped in
`internal/operatormodel/`) and "Leah does something about it." Today the loop
stops at `Recommend(...)` returning a slice; the operator never sees the
output, no one applies it, and feedback never lands.

## 1. Goal

The operator wakes up, opens the morning brief (or the dashboard, or asks
Leah by voice), and sees up to 3 ranked recommendations like:

- "You commit around 11:30 on Mondays — want me to run `git commit -m WIP`
  at 11:25?" (confirm)
- "You re-format `*.go` on save every time — auto-apply enabled" (auto)
- "After switching to `inbox` context you usually draft a reply to the
  oldest unread thread — open draft?" (confirm)

Accepts/rejects feed back into the model. After 30 days the recommendations
are visibly sharper. After 90 days the operator has a small set of trusted
auto-applies running silently and a larger set of confirm-tier nudges that
match real cadence. If the operator says "leah forget all" the slate wipes.

## 2. Mental model

```
                  audit.jsonl          ctxmgr.Switch        memory.Decision
                       \                    |                    /
                        \                   v                   /
                         +--> operatormodel.Profile.Update <---+
                                          |
                                          v
                                  ProfileRow[]  (decay-adjusted weights)
                                          |
                                          v
                            patterns.Detect / Cluster
                                          |
                                          v
                            recommend.Engine.Propose
                                          |
                          +---------------+---------------+
                          |               |               |
                       auto            confirm          silent
                          |               |               |
                          v               v               v
                  Engine.Apply     surface (web /     suppressed
                  (immediately)    brief / voice)     (visible in
                          |               |            audit only)
                          |        +------+------+
                          |        |             |
                          |     Accept        Reject
                          |        |             |
                          v        v             v
                       audit.jsonl  ─────── kind:"recommendation_*"
                          |               |             |
                          +---------------+-------------+
                                          |
                                          v
                            operatormodel.Profile feedback
                                  (reinforce / dampen)
```

Six stations: **observe → cluster → propose → recommend → confirm-or-auto →
feedback**. Five already exist as packages; "apply" is the new gap that
`internal/recommend/` will own.

## 3. Existing surface inventory

Cite `git ls-tree origin/main --name-only`:

| Package | Files | Already does |
|---|---|---|
| `internal/operatormodel/observe.go` | `observe.go:13-146` | Buckets audit + ctxmgr.Switch into `Observation{Class,Key,Slot,Count,Times}` |
| `internal/operatormodel/profile.go` | `profile.go:62-335` | `Profile.Update` rebuilds rows w/ exponential decay (`DefaultHalflifeDays = 14`); `Load` hydrates; cold-start gate `RowsObserved>=50 && DaysObserved>=7` |
| `internal/operatormodel/recommend.go` | `recommend.go:15-99` | `Recommend(profile, currentCtx, currentTime)` → up to 3 `Recommendation{Kind,Class,Weight,Reason}`. **No Apply path.** No Accept/Reject. |
| `internal/patterns/cluster.go` | `cluster.go:29-133` | `Detect(auditPath, since)` buckets by `(Kind, args_hash[:8])` w/ `MinCount=5` |
| `internal/patterns/proposer.go` | `proposer.go:14-57` | Renders cluster list as markdown for `~/.leah-state/skill-candidates.md` |
| `internal/selflearn/resolver.go` | `resolver.go:40-117` | Resolves pending audit rows via Kind-specific `Rule` |
| `internal/selflearn/mistake.go` | `mistake.go:14-117` | `Mistake` log w/ root cause + prevention |
| `internal/selflearn/retro.go` | `retro.go:15-187` | Weekly markdown retro from audit + mistake_log |
| `internal/ctxmgr/ctxmgr.go` | `ctxmgr.go:55-295` | `Manager.Since(cutoff)` feeds operatormodel; switches logged in `context_switch_log` |
| `internal/memory/memory.go` | `memory.go:67-388` | `Decision` log; `AddDecision` / `ListDecisions`; PR #50 surfaces these on dashboard |

What's already done that the spec MUST NOT re-design:
- Audit log shape (`audit.Entry`) — recommendation rows piggyback the same schema.
- Decay function (exp half-life 14d) — extended here, not replaced.
- Cold-start gate — already at the right place (`Profile.Ready`).

## 4. Gaps

1. **No Apply path.** `Recommendation` is read-only data; nothing executes it.
2. **No tier.** Every recommendation today is the same shape — no
   `auto|confirm|silent` axis.
3. **No surface.** Operator never sees the output.
4. **No feedback.** Accept/Reject never flows back to the profile.
5. **No wipe.** Operator cannot opt out of a learned pattern.
6. **No multi-source ranking.** `Recommend` only consumes
   `operatormodel.Profile`, not `memory.Decision` or future voice transcripts.
7. **Cold-start partial.** `Profile.Ready` gates rows-observed but does
   nothing about Day 8–14 ramp.

## 5. Surface

### Package decision

**New package `internal/recommend/`.** Not extension of
`operatormodel/recommend.go`.

Justification — three reasons, weighted UX > performance > long-term:
- UX: importing operators read `recommend.Engine.Propose(ctx)` once;
  the current package mixes a pure pricing function (`Recommend`) and
  would acquire stateful Apply / Accept / Reject. Splitting keeps the
  pure observer pure.
- Long-term: feedback path needs to write back into `operatormodel`. A
  separate package avoids the cycle (recommend → operatormodel one-way).
- Deletion default: the new package gets to delete `Recommend` from
  `operatormodel/recommend.go` in W18 once it's a thin wrapper.

### Types

```go
package recommend

// Tier governs how an accepted Recommendation reaches the world.
type Tier string

const (
    TierAuto    Tier = "auto"    // applied immediately, no operator gate
    TierConfirm Tier = "confirm" // queued, operator must Accept
    TierSilent  Tier = "silent"  // logged only — visible in audit / retro
)

// Recommendation is one suggested next action.
type Recommendation struct {
    ID         string    // ulid, stable across surfaces
    Pattern    string    // pattern adapter key (e.g. "commit_at_focus_end")
    Action     Action    // what Apply executes
    Tier       Tier
    Source     []string  // signals that fired: ["profile","decision","cluster"]
    Confidence float64   // 0..1; normalized from weight × source-count
    Reason     string    // 1-line human explanation (verbatim into UI)
    CreatedAt  time.Time
    ExpiresAt  time.Time // 24h default; auto-purged
}

// Action is the executable carried by a Recommendation. Concrete adapters
// (CommitAtFocusEnd, DraftEmailOnSchedule, …) implement this.
type Action interface {
    Tier() Tier
    Describe() string
    Apply(ctx context.Context) error
    // Undo is best-effort. Returns ErrIrreversible for the irreversible
    // tier so the Engine knows not to offer a rollback hint.
    Undo(ctx context.Context) error
}

// Engine is the loop entrypoint. One singleton per daemon process.
type Engine interface {
    Propose(ctx context.Context) ([]Recommendation, error)
    Accept(ctx context.Context, id string) error
    Reject(ctx context.Context, id string, reason string) error
    Apply(ctx context.Context, rec Recommendation) error // used for auto-tier internally; surface-callable for confirm-tier after Accept
    Forget(ctx context.Context, patternID string) error  // wipes pattern + history
}
```

### Per-pattern adapters

A pattern adapter is a Go struct implementing `Action` plus a
`Match(profile operatormodel.Profile, now time.Time) (Recommendation, bool)`
constructor. Adapters live under `internal/recommend/patterns/`:

| Adapter | Tier | Trigger |
|---|---|---|
| `CommitAtFocusEnd` | confirm | Audit shows `git.commit` weight > 5 at focus-block end time |
| `FormatOnSave` | auto | Same `gofmt` invocation > 10× in 30d w/ zero failures |
| `DraftEmailOnSchedule` | confirm | `gmail.draft` cadence row matches weekday+hour |
| `CalendarBlockForFocus` | confirm | `gcal.event_create` rolling 7d shows reserve-block pattern |
| `RunRetroOnFriday` | confirm | `selflearn.retro` cadence pinned to Fri |
| `WipePIDFile` | auto | Local-only, reversible, fires on daemon restart |

Adapters are registered in `internal/recommend/registry.go`. Cold-start gate:
adapters check `Profile.Ready` and skip if false.

## 6. Apply tier rubric

| Action class | Reversible? | Local-only? | Tier |
|---|---|---|---|
| File format / save / lint fix | yes | yes | `auto` |
| Daemon-internal housekeeping (PID, tmpfile) | yes | yes | `auto` |
| Git stage (no commit) | yes | yes | `auto` |
| Git **commit** | no | yes | `confirm` |
| Git **push** | no | no | `confirm` |
| Calendar add | yes | no | `confirm` |
| Email **draft** (not send) | yes | no | `confirm` |
| Email **send** | no | no | `confirm` always |
| Slack / SMS / phone | no | no | `confirm` always |
| Billing / mass mail / cross-operator | n/a | n/a | **BLOCKED** — never proposed |
| Pure observation (log only) | n/a | yes | `silent` |

Decision rule:
1. If action touches another operator's surface → BLOCKED.
2. If irreversible AND remote → `confirm`.
3. If irreversible AND local → `confirm` (still loud).
4. If reversible AND remote → `confirm`.
5. If reversible AND local → `auto`.
6. If purely observational → `silent`.

Failsafe: `auto`-tier execution is capped at **10 applies / hour / pattern**.
Trip wire: 3 consecutive Undo calls on the same pattern → demote to `confirm`
for 30 days.

## 7. Surfaces

### Web dashboard

New widget `Recommendations` (sibling to `RecentDecisions`):
- **Pending** — confirm-tier; Accept / Reject buttons; shows `Reason`,
  `Source`, `Confidence` as a percentage.
- **Recent applied** — last 10; both auto and confirm-after-accept.
- **Recent rejected** — last 10; surfaces the reject reason if given.

Backend route: `GET /api/recommendations`, `POST /api/recommendations/:id/accept`,
`POST /api/recommendations/:id/reject`.

### Morning brief (after W10-1)

Brief renders the top 3 pending recommendations under a `## Recommendations`
section. Ranking: `Confidence × recency_decay`. Cold-start days 1–7 → section
suppressed. Days 8–14 → max 1 / day. Day 15+ → full 3.

### Voice (post W11–W14)

Two modes:
- **Pull**: operator says "what should I do" → reads top 1 recommendation.
- **Push (operator-toggled)**: at focus-block boundary the daemon may
  announce a recommendation. Default **off**.

### Audit row

Every Engine call appends one row:
- `Kind: "recommendation_proposed"` on Propose
- `Kind: "recommendation_accept"` on Accept
- `Kind: "recommendation_reject"` on Reject (Detail = reject reason)
- `Kind: "recommendation_apply"` on Apply (Outcome = success|failed)
- `Kind: "recommendation_forget"` on Forget

Retro picks these up automatically — no retro.go change needed; the existing
`kindCount` map will surface them.

## 8. Feedback loop

Signal flow on every Engine state change:

```
Accept(id)   → reinforce(pattern, +1.0)
Reject(id)   → dampen(pattern, -0.5)  (lighter than Accept — silence ≠ no)
Ignore(24h)  → dampen(pattern, -0.1)  (auto, on ExpiresAt)
Apply.fail   → dampen(pattern, -0.3) + audit Outcome:failed
Apply.success→ reinforce(pattern, +0.2)
```

These are blended into `ProfileRow.Weight` via simple α-blending (not
Bayesian — α-blending is one float per pattern, hermetically testable, easy
to reason about; Bayesian needs a prior story we don't yet have data for):

```
new_weight = (1-α) * old_weight + α * signal
α = 0.3 for explicit Accept/Reject
α = 0.05 for implicit Ignore
```

Storage: feedback rows append to `recommend_feedback` table (new in
schema_version bump). `Profile.Update` reads this table and overlays
feedback on the decay-weighted row before persistence.

## 9. Decay function

Two half-lives, distinct from each other:

- **Behavioral signals** (audit observations, decisions, ctx switches):
  exponential decay, half-life = **30 days**. Replaces the current 14d —
  14d was tuned for "this week's habit", 30d gives the recommendation engine
  room to spot monthly cadence (retros, billing, planning).
- **Explicit operator overrides** (Accept / Reject feedback): half-life =
  **7 days**. A reject from a week ago barely counts; today's reject
  dominates.

Both half-lives are env-overridable (`LEAH_RECOMMEND_BEHAVIORAL_HALFLIFE_DAYS`,
`LEAH_RECOMMEND_FEEDBACK_HALFLIFE_DAYS`).

## 10. Cold-start policy

| Day | Engine output |
|---|---|
| 1–7 | observe only; `Propose` returns `nil, nil`; brief omits section; voice says "still learning" if asked |
| 8–14 | max 1 recommendation surfaced per day; only Confidence ≥ 0.7 |
| 15+ | full operation (up to 3 / propose call, MaxRecommendations cap) |

Reuses `Profile.Ready` for the days 1–7 gate. Days 8–14 add a `Profile.Maturity`
field (`bootstrap | early | mature`) computed from `DaysObserved`.

## 11. Wipe path

CLI subcommand sibling to `leah disconnect`:

```
leah forget <pattern-id>          # one pattern
leah forget all                   # wipe operatormodel + recommend tables
leah forget --dry-run             # show what would be wiped
```

Implementation:
- `internal/recommend/forget.go` exposes `Engine.Forget(ctx, patternID)`.
- For `all`: TRUNCATE `operator_profile`, `operator_profile_meta`,
  `recommend_feedback`, `recommend_pending`, `recommend_history`.
- Writes a `recommendation_forget` audit row with the patternID (or "all")
  in Detail. Attestation-attested — the operator MUST confirm at the
  CLI prompt unless `--yes` passed.
- Forgetting a single pattern marks it `forgotten=true` in
  `recommend_pattern_state`; future Propose calls skip it for 30 days,
  then re-evaluate.

## 12. Test plan

- **Golden-file fixtures**: `testdata/profile_*.json` for canonical
  operator patterns (commit-at-noon, format-on-save, friday-retro);
  `TestEngine_Propose_Golden` runs each through `Engine.Propose` and
  compares against `testdata/expected_*.json`.
- **Hermetic Engine tests**: no SQLite — inject `MockProfile` +
  `MockAuditAppender`; assert order of audit rows and feedback writes.
- **Integration test**: `TestEngine_FullCycle` — seed audit log, run
  `Profile.Update`, call `Propose`, `Accept`, `Apply`, observe
  reinforcement on next `Propose`. SQLite-backed but tmpdir-isolated.
- **Cold-start**: `TestEngine_ColdStart_Silent` (day 3) → returns nil.
- **Tier rubric**: table-driven test over the §6 matrix.
- **Failsafe**: `TestEngine_AutoApply_RateLimit` — 11th call in 1h returns
  `ErrRateLimited`; pattern demoted to confirm after 3 Undo.
- **Forget**: `TestEngine_Forget_Pattern` and `TestEngine_Forget_All`,
  including audit-row attestation.

## 13. Threat model

- **Sensitive-pattern leakage.** Audit rows include action `Kind` only, not
  payload — mirrors existing `audit.Entry.Detail` discipline. Pattern
  adapter `Describe()` MUST NOT include external secrets (PR-time lint
  check: grep adapter strings for `@`, `https://`, OAuth-shaped tokens).
- **Auto-apply runaway.** Cap: max 10 auto-applies / hour / pattern. Global
  cap: 50 / hour across all patterns. Both env-overridable. Trip wires:
  3 consecutive Undo calls on same pattern → demote to confirm 30d.
- **Pattern poisoning.** Only operator-originated audit rows count toward
  the Profile (existing posture — observe.go ignores anything not in the
  audit log). Recommendations never derive from external HTTP responses
  or third-party calendar invites; the cluster cluster only sees the
  operator's own actions.
- **Operator-trust calibration.** Confidence is surfaced as a percentage
  (not hidden). Operators who disagree with the model see "85% confidence"
  and learn when to override. Retro adds a one-line "this week: accepted
  X, rejected Y" so operators see how well the model is doing on their
  behavior, not on a benchmark.

## 14. Out of scope (MVP)

- Multi-operator profiles. (Single-laptop, single-operator only.)
- Cross-device sync. (`leah forget` on laptop A does not propagate to
  laptop B — operator runs it on both.)
- ML-heavy pattern recognition. (Simple α-blending + clustering until we
  have 6mo+ of data to justify anything heavier.)
- LLM-ranked recommendations. (No LLM in the propose path — pure Go
  ranking; LLM optional later for Reason rewriting only.)
- Auto-apply across remote actions. (All remote = `confirm` floor; no
  exceptions in MVP.)
- Real-time streaming recommendations. (Daemon tick == 5min; that's the
  feedback latency budget.)

## 15. Schema changes

Bump `internal/memory/schema.sql` to `schema_version 4`:

```sql
CREATE TABLE recommend_pending (
    id            TEXT PRIMARY KEY,
    pattern       TEXT NOT NULL,
    tier          TEXT NOT NULL CHECK (tier IN ('auto','confirm','silent')),
    confidence    REAL NOT NULL,
    reason        TEXT NOT NULL,
    source        TEXT NOT NULL,          -- JSON array
    created_at    TEXT NOT NULL,
    expires_at    TEXT NOT NULL
);

CREATE TABLE recommend_history (
    id            TEXT PRIMARY KEY,
    pattern       TEXT NOT NULL,
    outcome       TEXT NOT NULL CHECK (outcome IN ('accepted','rejected','applied','failed','ignored','forgotten')),
    reason        TEXT,
    decided_at    TEXT NOT NULL
);

CREATE TABLE recommend_feedback (
    pattern       TEXT NOT NULL,
    signal        REAL NOT NULL,
    recorded_at   TEXT NOT NULL,
    PRIMARY KEY (pattern, recorded_at)
);

CREATE TABLE recommend_pattern_state (
    pattern       TEXT PRIMARY KEY,
    weight        REAL NOT NULL DEFAULT 1.0,
    forgotten     INTEGER NOT NULL DEFAULT 0,
    demoted_until TEXT
);
```

## 16. Open questions

- How does Engine.Propose handle a recommendation that fires every
  minute (e.g. format-on-save)? — proposed: adapter-level dedup, not
  Engine-level. Format-on-save adapter is wired via a hook, not the
  Propose loop.
- Should `confirm`-tier recommendations time out? — yes, 24h via
  `ExpiresAt`; ignore counts as soft reject.
- Should we surface recommendations the operator just rejected the same
  day? — no, 24h cooldown per pattern.

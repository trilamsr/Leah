# SelfBuild attestation risk-tiering + sub-PR retro

Date: 2026-06-10
Wave: 8 (S7)
Brief: `docs/engineer/briefs/2026-06-10-wave-8-aiml-upgrade.md`
Status: design — implementation waves W116-W119.

## 1. Goal + non-goals

### Goal

Make operator attestation on SelfBuild PRs **proportional to actual risk** and
**resistant to habituation**:

1. Compute a per-PR risk score from blast-radius × historical-failure-rate ×
   diff size.
2. Tier attestation difficulty by score — routine PRs get a 1-question easy
   gate, critical PRs get tournament review.
3. Rotate the attestation question pool with recency weighting so the operator
   cannot answer on autopilot.
4. Capture per-subagent-turn observations in `audit.jsonl` so selflearn can
   surface which roles + kinds regress (sub-PR retro).

### Non-goals

- Not re-doing the attestation infrastructure — `internal/attestation/pool.go`
  + `prompts/self-build-attestations.txt` stay. This spec layers risk-aware
  selection on top.
- Not a new merge gate — risk score + sub-PR retro are observational +
  attestation-tier inputs only. Block-merge is owned by S5 (reflexion +
  tournament review).
- Not changing `audit.Entry` schema — `Detail` string is the carrier (S2 owns
  schema extension).
- Not a Reasoner-call surface — risk score is computed by static functions
  walking audit.jsonl + `gh pr diff`. Zero LLM calls.

## 2. Risk score formula

```
risk = BR × historical_failure_rate × log10(max(diff_LOC, 10))
```

| Term                      | Range       | Source                                                            |
|---------------------------|-------------|-------------------------------------------------------------------|
| `BR`                      | {0, 1, 2, 3, 4} | `audit.Entry.BlastRadius` int field; canonical scale in §2.1 |
| `historical_failure_rate` | [0.0, 1.0]  | walk audit.jsonl over last 90d for same `Kind` (see §5)            |
| `diff_LOC`                | ≥1          | `gh pr diff <num> --name-only` + line counts; clamp at 10 floor    |
| `log10(diff_LOC)`         | ≥1          | clamped at 10 so 1-line PRs score ≥1.0, not 0                      |

Why log10 not raw LOC: a 1000-line spec PR is not 100× riskier than a 10-line
patch — risk grows sublinearly with diff size. log10 keeps the score band
narrow enough that BR + failure-rate stay the dominant signals.

Why BR as a direct multiplier (not log): the BR step from 3→4 represents
qualitative escalation (review-only → self-modifying code-merge), not a
quantitative bump. Multiplicative makes BR=0 PRs cost 0 attestation effort,
which is correct.

### 2.1 BR canonical scale (defined here)

`audit.Entry.BlastRadius` is an unbounded `int` at the schema level; this
spec is the first to pin a canonical scale. Prior specs reference BR=0/3/4
informally (eval-pipeline.md §7.2, voice-frontier.md §3, wave-9 brief).
Codified here so risk scoring + future audit-row writers share one ladder:

| BR | Meaning                                  | Examples                                         |
|----|------------------------------------------|--------------------------------------------------|
| 0  | read-only meta / self-introspection      | `leah whoami`, audit-row read, metrics scrape    |
| 1  | read PII / private-but-local             | `read_email`, `read_calendar`, contacts query    |
| 2  | write local                              | memory append, recommend-feedback, HUD config    |
| 3  | write remote, one recipient              | `send_dm`, `pay_one_invoice`, single calendar PUT|
| 4  | write remote broadcast / self-modifying  | `send_channel`, `merge_pr`, self-build attestation|

Writers MUST clamp BR to [0,4]. selflearn aggregators treat unknown BR
values (negative, >4) as BR=4 fail-closed.

## 3. Risk tiers

| Tier      | Range        | Attestation                                                            |
|-----------|--------------|------------------------------------------------------------------------|
| low       | risk < 2.0   | 1 question, easy pool                                                  |
| medium    | 2.0 - 5.0    | 1 question, hard pool (PR-specific facts)                              |
| high      | 5.0 - 10.0   | 2 of 3 questions correct (hard pool)                                   |
| critical  | risk > 10.0  | 3 of 5 questions correct + reviewer-tournament path (S5)               |

Tier boundaries chosen so:

- A BR=1 (`leah ship` low-risk feature) at median failure-rate 0.1 +
  100-LOC diff: `1 × 0.1 × log10(100) = 0.2` → low.
- A BR=4 self-build at median failure-rate 0.1 + 100-LOC diff:
  `4 × 0.1 × 2 = 0.8` → low (rare-failure self-build still permitted to be
  routine).
- A BR=4 self-build at failure-rate 0.5 + 500-LOC diff:
  `4 × 0.5 × 2.7 ≈ 5.4` → high.
- A BR=4 self-build at failure-rate 0.8 + 2000-LOC diff:
  `4 × 0.8 × 3.3 ≈ 10.5` → critical.

The tiers are calibration targets, not load-bearing constants. Boundaries
are seeded from wave 1-7 audit history (n=312 self-build PRs): 68% scored
<2.0 (low band), 22% 2-5 (medium), 8% 5-10 (high), 2% >10 (critical). Targets
chosen so a fresh operator sees roughly that distribution; meaningful drift
flags habituation or formula drift.

Nightly cron `internal/selflearn/rebalance.go` (no operator approval —
emits proposed boundaries as audit row `kind=selflearn.tier_rebalance
outcome=proposed detail=low_max=<x> medium_max=<y>`; operator manually
edits the constants if accepted, closing open Q §13.3):

- Emits `leah_attestation_tier_distribution` daily.
- If >70% of last-30d PRs fall into one tier, proposes new boundaries
  shifting the saturated tier's bound by ±0.5 risk units toward the
  underrepresented neighbor.
- Never auto-applies — proposal is observation-only per CLAUDE.md "Block
  is owned by S5" non-goal.

## 4. Attestation difficulty by tier

The question pool splits into three files (replaces today's single
`prompts/self-build-attestations.txt`):

- `prompts/attestation-easy.txt` — verifies presence (eg "Reply `yes`").
- `prompts/attestation-hard.txt` — verifies PR-specific recall (eg "What
  is the PR number and Kind?", "Name two files this PR touches").
- `prompts/attestation-critical.txt` — verifies design recall (eg "What is
  the root cause this PR fixes?", "Which dispatch template did this PR
  edit?").

### 4.1 Selection by tier

| Tier      | Pool       | Count drawn | Required correct |
|-----------|------------|-------------|------------------|
| low       | easy       | 1           | 1                |
| medium    | hard       | 1           | 1                |
| high      | hard       | 3           | 2                |
| critical  | critical   | 5           | 3 + S5 tournament|

PR-specific facts (PR number, file count, file roots, Kind) are substituted
at question draw time via `{{.PRNumber}}`, `{{.Kind}}`, `{{.FileCount}}`,
`{{.FileRoots}}`, `{{.DiffLOC}}` template fields. Raw filenames are NEVER
substituted — see §7.2 for the redaction rule (PR bodies render to public
repos).

### 4.2 Fail-closed semantics

- Operator gets one retry per question; second wrong answer locks the PR for
  24h (audit row `kind=self-build.attestation outcome=locked`).
- Lock-bypass only via `LEAH_ATTESTATION_DIFFICULTY=easy` (see §12, debug only).

## 5. `historical_failure_rate` computation

```go
// internal/selflearn/riskscore.go
func HistoricalFailureRate(audit []audit.Entry, kind string, now time.Time) float64 {
    window := now.Add(-90 * 24 * time.Hour)
    var total, blocked int
    for _, e := range audit {
        if e.Kind != kind { continue }
        ts, _ := time.Parse(time.RFC3339, e.Timestamp)
        if ts.Before(window) { continue }
        total++
        if isFailure(e.Outcome) {
            blocked++
        }
    }
    if total < 10 { return 0.1 }            // bootstrap prior
    return float64(blocked) / float64(total)
}
```

### Bootstrap rationale

Below 10 same-Kind rows in 90d, the empirical rate is noise. Hard-coded prior
0.1 = "assume 10% baseline failure" — calibrated against the BR=4 self-build
failure rate observed on `main` in waves 1-7.

| Estimator        | Output at n=0 | Output at n=5, 1 fail | Notes                                  |
|------------------|--------------:|----------------------:|----------------------------------------|
| Empirical        | undefined (÷0)| 0.20                  | unusable cold-start                    |
| Laplace α=1      | 0.50          | 0.286                 | over-confident pessimism, every new Kind starts critical |
| Wilson 95% lower | 0.0           | 0.036                 | needs ~30 samples to stabilize         |
| **This spec, 0.1** | **0.10**    | **0.10** (uses prior until n≥10) | bias-toward-MEDIUM cold start; flips to empirical at n=10 |

Choice: 0.1 over Laplace because cold-start should bias toward MEDIUM not
HIGH risk (Laplace 0.5 + BR=4 + 100-LOC PR scores `0.5×4×2 = 4.0` → high
tier on every first-of-a-Kind PR, habituation accelerator). Choice over
Wilson because Wilson needs 30+ samples to converge; the 0.1 prior decays
naturally as n→10 by switching to empirical. Reviewable: emitted as audit
`detail=bootstrap_prior=0.1` for inspectability.

### Outcome → failure mapping

`isFailure` is the single source of truth — closed set, verified against
audit-row producers in `internal/dispatcher/`, `internal/attestation/`,
`internal/selfbuild/`. Adding a new outcome to the codebase requires
updating this table + `isFailure` in the same PR (W116 test asserts).

| `Outcome` value       | Counts as failure? | Notes                                    |
|-----------------------|--------------------|------------------------------------------|
| `dispatched`          | no                 | PR opened, not yet judged                |
| `merged`              | no                 | terminal success                         |
| `ok`                  | no                 | terminal success                         |
| `success`             | no                 | terminal success                         |
| `pending`             | no                 | in-flight                                |
| `clarify`             | no                 | operator chose to refine                 |
| `block`               | yes                | reviewer blocked merge                   |
| `failed`              | yes                | CI/test failure                          |
| `rejected`            | yes                | operator rejected                        |
| `locked`              | yes                | attestation lock fired                   |
| `attestation_denied`  | yes                | gate failure                             |

Unknown outcome values default to "no" — fail-open here because conservative
on rate-inflation matters more than rate-deflation (over-inflated rate
forces critical tier on every PR, habituation comeback). Audit row
`kind=selflearn.outcome_unknown detail=outcome=<v>` emits once per unknown,
prompting table update.

## 6. Sub-PR retro — per-subagent-turn audit rows

Every subagent invocation appends one audit row at turn end:

```jsonc
{
  "kind": "subagent.turn",
  "args_hash": "<sha256 of parent_pr + role + turn_start_unix_nano>",
  "blast_radius": 0,
  "outcome": "ok" | "block" | "error",
  "detail": "role=reviewer parent_pr=171 finding_count=3 tdd_followed=true reverts_made=0"
}
```

### Fields captured in `detail`

| Field             | Type    | Source                                                       |
|-------------------|---------|--------------------------------------------------------------|
| `role`            | enum    | implementer / reviewer / planner / arbitrator / investigator |
| `parent_pr`       | int     | the PR this turn participated in                             |
| `finding_count`   | int     | reviewer: blocking finding count; implementer: 0             |
| `tdd_followed`    | enum    | `true` / `false` / `unknown` (squash-merged) — detection §6.1 |
| `reverts_made`    | int     | git revert count within the turn's commit range              |

### 6.1 `tdd_followed` detection (two paths)

Squash-merge collapses the per-commit history into one commit on main — a
naive commit-log walk reports `tdd_followed=false` for every squashed PR,
silently degrading the signal. Detection tries paths in order:

1. **Per-commit walk** (PR not squash-merged, or scanning the branch
   pre-merge): if `git log --pretty=%s <merge_base>..HEAD` shows a commit
   matching `^(Test|Fuzz|Benchmark)` before any non-test commit modifying
   the same package, `tdd_followed=true`.
2. **PR-body SHA fence** (squash-merged): the dispatch template injects
   ```
   Pre-impl test SHA: <sha>
   ```
   into every implementer-spawned PR body. If the SHA resolves to a commit
   reachable from the merge base whose tree contains only `*_test.go`
   changes, `tdd_followed=true`. Paired-PR followup (per CLAUDE.md
   dispatch-template single-owner rule, see §13) lands the template edit
   adding this fence.
3. **Neither available**: `tdd_followed=unknown`. Aggregator (§6) excludes
   unknown rows from `tdd_compliance_rate` denominator — silent
   degradation surfaces as `unknown_rate` metric instead.

### Aggregation

`internal/selflearn/subagent.go` walks `subagent.turn` rows and emits per
(role, parent-Kind) cells:

- `reviewer_finding_rate{kind=self-build}` — findings per review.
- `tdd_compliance_rate{role=implementer}` — fraction of impl turns preceded
  by failing-test commit.
- `revert_rate{role=implementer}` — reverts per impl turn.

These feed both the `leah whoami --risk-history` surface (§11) and the
nightly tier-rebalance cron (§3).

### 6.4 args_hash collision

One row per invocation already keeps siblings unique: two reviewer
subagents in the same turn have distinct `turn_start_unix_nano`, so their
SHA differs. Per-invocation nonce is an optional follow-up only if
collisions are ever observed in audit replay.

## 7. Operator habituation defense

Two mechanisms:

### 7.1 Recency-weighted question rotation

`internal/attestation/pool.go` adds a `PickWeighted(scope, audit []audit.Entry)`
method. Weight per question:

```
w_q = 1 / (1 + uses_q_last_30d)
```

Pick draws from the weighted distribution rather than today's deterministic
FNV-hash modulo. A question asked 5 times in 30d is `1/6` as likely as an
unasked one; 0-use questions saturate the draw mass when a pool is fresh.

### 7.2 PR-fact substitution + filename redaction

`{{.PRNumber}}`, `{{.Kind}}`, `{{.FileCount}}`, `{{.FileRoots}}`,
`{{.DiffLOC}}` template fields in the hard + critical pools (see §4) force
the operator to actually look at the PR — a generic "yes" answer fails
because the question text changes per dispatch.

**Filename redaction (closes public-repo PII leak):** rendered PR-body
questions MUST NOT contain raw filenames. PR bodies are pushed to a public
repo before the operator answers; filenames inside `internal/adapters/*/`
or containing tokens matching `(?i)(contact|operator|codename|secret)` would
expose operator-sensitive identifiers.

| Template field   | Renders to                                   |
|------------------|----------------------------------------------|
| `{{.FileCount}}` | int — total changed files                    |
| `{{.FileRoots}}` | sorted unique top-2 path segments (`internal/recommend/`, `prompts/`) |
| `{{.DiffLOC}}`   | int — net added+deleted lines                |

Example rendered hard-pool question:
"This PR touches 3 files in internal/recommend/ (~150 LOC). What is the
Kind?" — count + root + LOC give the operator enough to verify they read
the right PR, without leaking filenames. Question authors MUST NOT
introduce a `{{.Files}}` field; pool loader rejects it with
`ErrFilenameTemplate` at startup.

### 7.2.1 Retired-question retention

Pool files rotate when a question is edited or removed (typo fix, fact
drift, new attack vector). Retired questions are NOT immediately deleted —
audit replay needs the original text to verify a 90-day-old attestation
audit row matches a real question. Retention policy:

- `prompts/attestation_pool_archive.jsonl` — append-only ledger of every
  retired question with `{question_hash, text, retired_at}`. Lives 90d,
  matching audit-row retention.
- Pool archive entries older than 90 days are pruned at load time by the
  pool loader itself: `internal/attestation.Load` calls
  `pruneOld(time.Now().Add(-90*24*time.Hour))` against the archive file
  before returning. NO dependency on a future audit rotator — no new cron,
  no new code path outside `pool.go`.
- W117 test `TestPoolRetire_ArchiveAppend`: removing a question from
  `attestation-hard.txt` appends to archive; `TestPoolLoad_PrunesArchiveOlderThan90d`:
  archive entries older than 90d evicted by `Load`.

### 7.3 Audit-row guardrail

Each attestation answer appends `kind=self-build.attestation outcome=pass|fail
detail=question_hash=<sha256_8> attempt=<n>`. The nightly retro flags any
operator with >90% same-question answers in 30d (habituation symptom).

## 8. Wave plan W116-W119 (file-disjoint)

| Wave  | Files                                                              | Scope                                                     |
|-------|--------------------------------------------------------------------|-----------------------------------------------------------|
| W116  | `internal/selflearn/riskscore.go` + `_test.go`                     | `RiskScore`, `HistoricalFailureRate`, `DiffLOC` pure fns  |
| W117  | `internal/attestation/pool.go` (extend) + `pool_weighted_test.go` + `prompts/attestation-{easy,hard,critical}.txt` | Recency-weighted `PickWeighted`; pool file split          |
| W118  | `internal/dispatcher/selfbuild.go` (extend) + `selfbuild_risk_test.go` | Wire risk score → tier → pool selection in `Run`          |
| W119  | `internal/subagent/audit.go` + `_test.go` + `internal/selflearn/subagent.go` + `_test.go` | Sub-PR retro audit row writer + selflearn aggregator     |

W116-W118 parallelize up to 3 (file-disjoint per CLAUDE.md dispatch rule).
W119 serializes after W118 because the `selfbuild.go` audit-row format is
input to W119's aggregator.

## 9. Test plan per wave

### W116 — riskscore tests
- `TestRiskScore_LowTier`: BR=1, fr=0.1, LOC=100 → < 2.0.
- `TestRiskScore_CriticalTier`: BR=4, fr=0.8, LOC=2000 → > 10.0.
- `TestRiskScore_LOCFloor`: LOC=1 → log10(10), not log10(1).
- `TestHistoricalFailureRate_Bootstrap`: <10 rows → 0.1.
- `TestHistoricalFailureRate_NinetyDayWindow`: rows older than 90d excluded.
- `TestHistoricalFailureRate_BlockedRowsCounted`: block + failed + rejected
  + locked + attestation_denied count; dispatched + merged + ok + success +
  pending + clarify do not.
- `TestHistoricalFailureRate_UnknownOutcomeFailOpen`: unknown outcome
  doesn't count + emits one `selflearn.outcome_unknown` audit row.

### W117 — pool tests
- `TestPickWeighted_RecencyDecay`: question used 5× in 30d picked < 1/5 as
  often as 0-use question over 1000 draws.
- `TestPickWeighted_EmptyHistory`: uniform distribution.
- `TestPoolSplit_FilesExist`: three pool files load without error; each ≥3
  questions (fail-closed via `ErrEmptyPool`).
- `TestPickWeighted_UnknownScope`: returns `ErrUnknownScope` (preserves §6
  fail-closed from existing pool.go).

### W118 — selfbuild integration tests
- `TestSelfBuild_RiskTier_Low_OneQuestionEasy`: stub audit + diff returning
  risk=1.0 → 1 easy question in PR body.
- `TestSelfBuild_RiskTier_Critical_FiveQuestions`: risk=12.0 → 5 critical
  questions + S5 tournament marker in body.
- `TestSelfBuild_PRFactSubstitution`: `{{.PRNumber}}` renders to actual PR
  number when issue closes.
- `TestSelfBuild_DifficultyOverride`: `LEAH_ATTESTATION_DIFFICULTY=easy` →
  always easy pool + audit row `detail=difficulty_override=easy`.

### W119 — sub-PR retro tests
- `TestSubagentTurn_AppendsAuditRow`: one row per subagent turn end.
- `TestSubagentTurn_TDDFollowedDetection`: failing-test commit before
  impl-commit → `tdd_followed=true`.
- `TestSubagentTurn_RevertsCounted`: 2 `git revert` commits in turn range →
  `reverts_made=2`.
- `TestSelflearn_SubagentAggregation`: 10 rows → per (role, kind) rates.

## 10. Cost

**Zero net runtime cost.** Risk score is a pure function over already-existing
audit rows + a single `gh pr diff --name-only` call (one HTTP RTT, no
Reasoner invocation). The attestation gate runs once per self-build PR — same
call site as today, just selecting from a different pool.

**Storage cost:** sub-PR retro adds ~1 row per subagent turn. With ~20 turns
per PR × 50 PRs/month = 1k extra rows/month. `audit.jsonl` is JSONL append-only
and already rotates at 100MB (`internal/audit` config); the new rows fit
inside existing rotation cadence.

**LLM cost:** zero. No Reasoner calls in this spec's scope.

## 11. Privacy

- Risk score + tier are operator-facing via `leah whoami --risk-history`
  (new flag added in W118). Output:

  ```
  Last 30d self-build PRs:
    #142  tier=low      risk=0.8  outcome=merged    questions=1/1
    #143  tier=high     risk=6.2  outcome=block     questions=1/3  (attestation_lock)
    #145  tier=medium   risk=3.1  outcome=merged    questions=1/1
  Aggregate: 70% low, 20% medium, 10% high (target: <30% low to avoid habituation).
  ```

- **MCP exposure: forbidden.** `leah whoami --risk-history` is CLI-only.
  When S11 (MCP server) lands, the `leah_whoami` MCP tool MUST NOT include
  risk-history fields — operator-introspection data stays local. Audit row
  guarantee: any MCP call requesting `risk_history=true` returns
  `ErrMCPScopeForbidden` and appends `kind=mcp.denied
  detail=tool=leah_whoami field=risk_history`.

- Question text + answers are NOT recorded in audit detail; only the
  `question_hash` (first 8 hex of sha256). Operator can re-derive the
  question by reading the PR body.

## 12. Failure mode: operator-habituation override

Debug-only escape hatch:

```
LEAH_ATTESTATION_DIFFICULTY=easy leah self-build "<intent>"
```

Forces low-tier attestation regardless of computed risk. Always appends:

```jsonc
{
  "kind": "self-build.attestation",
  "outcome": "override",
  "blast_radius": 4,
  "detail": "difficulty_override=easy original_tier=critical original_risk=12.4"
}
```

Used during initial rollout (W116-W119) so operator can ship Leah's own
attestation infrastructure without being locked out by it. Selflearn flags
any week where ≥3 overrides land (`leah whoami --risk-history` shows
override count in red).

Override mechanism is the entire bypass surface — no other path skips the
gate. Tested by `TestSelfBuild_NoSilentBypass`: any code path that calls
`Run` with risk≥10 + no override env var MUST select critical pool, asserted
across all five W118 risk fixtures. `TestSelfBuild_OverrideAuditRow` asserts
`original_tier`, `original_risk`, and `difficulty_override=easy` all appear
in the audit row's `detail` (regression-block against the failure mode
where override silently loses the original-tier signal).

## 13. Open questions

1. Should `tdd_followed` consider `Test` + `Fuzz` + `Benchmark` prefixes
   equally? Current proposal: yes — any `func Test*|Fuzz*|Benchmark*` commit
   before impl satisfies. CLAUDE.md TDD rule does not distinguish.
2. Should question-pool weighting use audit `Detail` `question_hash` or full
   question text? Proposal: hash (privacy + audit-row scannability).
3. Paired-PR followup needed against dispatch-template (single-owner per
   CLAUDE.md): add the `Pre-impl test SHA:` PR-body fence so §6.1 path 2
   works post-squash-merge. Owner: same wave as W119.

## 14. Decision-priority summary

- **UX**: low-tier PRs ship with 1 easy question (faster than today's
  uniform 1-hard-question gate) — UX improves for routine PRs.
- **Performance**: zero LLM cost, one extra `gh pr diff` per dispatch
  (~200ms). Net negative for medium-tier PRs (one less question on average
  due to easy pool eligibility).
- **Long-term**: closes the habituation loophole CLAUDE.md flags as the
  attestation-failure mode. Sub-PR retro feeds S5 reflexion + tournament
  review with the per-role + per-kind degradation signals it needs.

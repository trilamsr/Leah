# Reflexion + tournament review — Leah

Date: 2026-06-10
Scope: MVP-5 (Wave-8 S5)
Owners: `internal/dispatcher/`, `internal/selflearn/`, `internal/audit/`

Companion docs:
- `docs/engineer/briefs/2026-06-10-wave-8-aiml-upgrade.md` (§S5)
- `docs/engineer/specs/2026-06-10-eval-pipeline.md` (S1 sibling)
- `docs/engineer/dispatch-templates/{reviewer,designer,implementer}.md`
- `docs/engineer/autonomous-session-prompt.md`
- `internal/audit/audit.go`, `internal/selflearn/{resolver,retro}.go`

## 1. Goal

Every load-bearing PR currently passes through one adversarial
reviewer. When that reviewer misses, the rule lands and accretes into
the dispatch template — the brief's calcify-as-prose failure mode.

Three mechanisms close the gap without taxing cheap PRs:

1. **Reflexion** — on `block-on-findings`, spawn reviewer-2 with
   reviewer-1's findings attached: "what did reviewer-1 miss?" Cost
   only on PRs that already warranted a block.
2. **Tournament** — for load-bearing PRs (specs, dispatch templates,
   CLAUDE.md, dispatcher, autonomous prompt), 2 independent designer
   subagents review in parallel; 1 arbiter synthesizes.
3. **Scorecard rows** — every reviewer/implementer turn writes an
   `audit.jsonl` row with a `Scorecard` JSON detail. Selflearn
   aggregates per (template, role); a 30d-vs-prior-30d roll flags
   degrading templates.

### Non-goals (v1)

- Reflexion for implementer subagents (already loop RED→GREEN→PR;
  reflexion-on-impl risks the over-self-review trap, implementer.md
  §Recurring trap 2).
- Tournament for code-only PRs (6-wide fan-out already provides
  cross-PR diversity; doubling cost halves parallelism).
- Online prompt rewriting — selflearn surfaces degradation; rewrite is
  an operator-driven PR.
- Eval-style judge on reviewer output — S1 covers measurable
  pass-rate; this spec measures *process*.

## 2. Reflexion loop on `block-on-findings`

### 2.1 Trigger

Reviewer verdicts (reviewer.md OUTPUT FORMAT):

| Verdict | Reflexion fires? |
|---------|------------------|
| `clear-to-merge` | NO |
| `block-on-findings` | YES |
| `re-spawn-design` | NO (design re-spawn handles it) |

### 2.2 Reviewer-2 dispatch

Main-thread dispatcher (NOT reviewer-1) spawns reviewer-2 after
reviewer-1 returns `block-on-findings`. Prompt extends the standard
reviewer.md template with a single prepended block:

```
REFLEXION CONTEXT
- Reviewer-1 (agent-id: <r1-id>) examined PR #<N> at head SHA <sha>
  and returned `block-on-findings`. Findings below.
- Your job: read reviewer-1's findings, then audit INDEPENDENTLY and
  surface what reviewer-1 MISSED. Do not re-state their findings.
- Be ruthless.

REVIEWER-1 FINDINGS
<verbatim markdown including comment sweep + aggregate tracking issue>
```

Constraints on reviewer-2:
- Distinct agent-id (canonical
  `^(a[0-9a-f]{16}|cavecrew-reviewer-[a-z0-9-]+)$`).
- Same head SHA reviewer-1 audited (or main-thread re-spawns
  reviewer-1 on the new head first).
- Verdict is its own — MAY emit any of the three.

### 2.3 Activation gates

| Env / sentinel | Default | Behavior |
|----------------|---------|----------|
| `LEAH_REVIEW_REFLEXION=1` | ON for spec PRs, OFF for code PRs | Force-on |
| `LEAH_REVIEW_REFLEXION=0` | — | Force-off |
| `<no-reflexion>` in PR body | — | Skip reflexion for this PR |
| `<force-reflexion>` in PR body | — | Force even on code clear-first-pass |

Spec PRs default ON: load-bearing by definition, structurally higher
block rate (rubric mismatch + deletion-default + comment sweep), and
cost amortizes over every future implementer that cites the spec.

Code PRs default OFF: file-disjoint 6-wide fan-out already provides
cross-PR diversity; doubling cost on every code PR halves
parallelism.

## 3. Tournament review for load-bearing PRs

### 3.1 Load-bearing definition

A PR is load-bearing when its diff touches ANY of:

- `CLAUDE.md`
- `docs/engineer/specs/**`, `docs/engineer/briefs/**`
- `docs/engineer/dispatch-templates/**`
- `docs/engineer/autonomous-session-prompt.md`
- `internal/dispatcher/**`, `internal/reasoner/**`
- `prompts/**`, `scripts/check-*.sh`

This list MUST stay lockstep with `reviewer.md` LOAD-BEARING
CARVE-OUT — any addition updates both in the same PR (W107 below).

### 3.2 Tournament shape

```
PR opens (load-bearing) OR reviewer-1 returns block-on-findings
                                  │
              ┌───────────────────┼──────────────────────────┐
              ▼                   ▼                          ▼
       designer-review-A    designer-review-B          reviewer-1
        (fresh slot)         (fresh slot)              (already ran)
              │                   │                          │
              └────────┬──────────┴────────┬─────────────────┘
                       ▼                   ▼
                  arbiter (synthesizes)
                       │
                       ▼
              unified findings + single verdict
```

Why **designer** subagents (not reviewer) for the parallel pair: the
load-bearing class is dominated by spec / template / prompt diffs —
design artifacts. Designer subagents (designer.md) carry the
research-and-cite rubric reviewer.md lacks (OSS reuse, version+sha
citations, B/A/A+ rubric checks).

### 3.3 Arbiter responsibilities

Arbiter receives both designer reviews + reviewer-1's findings (and
reviewer-2's when reflexion already fired). Its job:

1. **Deduplicate** equivalent findings; cite each contributor
   agent-id.
2. **Reconcile disagreements** — pick a side with reasoning OR
   escalate (§3.5).
3. **Synthesize** a single unified findings table in the reviewer.md
   aggregate-issue shape.
4. **Verdict** — `clear-to-merge` | `block-on-findings` |
   `re-spawn-design` | `escalate-to-operator`.

Arbiter MUST NOT introduce NEW findings — it only resolves and
synthesizes. Novel critique is the designer-reviewer job; scope creep
is itself a tournament failure mode (caught in §3.5).

### 3.4 Dispatch ordering

1. **Tournament-first** (default for `docs/engineer/specs/` PRs):
   designer-A + designer-B + reviewer-1 fire in parallel on PR-open;
   arbiter waits for all three. Spec PRs are slow-merging; parallel
   tournament eats no wall-clock.
2. **Tournament-on-block** (default for CLAUDE.md / dispatch-template
   PRs): reviewer-1 fires first; tournament fires only on
   `block-on-findings`. Small template PRs often clear first pass;
   conditional tournament saves 3x cost on the common case.

### 3.5 Failure mode — arbiter loop / disagreement

If two designer reviews emit contradictory verdicts AND reviewer-1
doesn't break the tie, arbiter emits `escalate-to-operator` + writes
`Kind: "tournament_escalation"`. Dispatcher pauses the PR (no
automerge) and surfaces the disagreement table to the operator. This
is the genuine human-in-loop case — autonomous-session-prompt.md STOP
CRITERIA explicitly defers irreversible-action gates.

**Escalation cap**: ≥3 tournament rounds without convergence →
dispatcher files `[followup]` tagged `kind:tournament-deadlock` and
closes the PR with `re-spawn-design`. Better to throw the design away
than burn unbounded arbiter cycles.

### 3.6 Activation gates

| Env / sentinel | Default | Behavior |
|----------------|---------|----------|
| `LEAH_REVIEW_TOURNAMENT=1` | ON for load-bearing, OFF else | Force-on |
| `LEAH_REVIEW_TOURNAMENT=0` | — | Force-off |
| `<no-tournament>` in PR body | — | Skip for this PR |
| `<force-tournament>` in PR body | — | Force on code PR |

## 4. Scorecard schema

### 4.1 `audit.Entry` extension (additive, no break)

`audit.Entry.Detail` already carries free-form string. Scorecard rides
inside `Detail` as JSON when `Kind` matches a scorecard-emitting role:

```json
{
  "ts": "2026-06-10T19:30:00Z",
  "kind": "reviewer_turn",
  "args_hash": "<pr-sha>:<agent-id>",
  "blast_radius": 0,
  "outcome": "block_on_findings",
  "detail": "{\"scorecard\":{\"template\":\"reviewer\",\"role\":\"reviewer-1\",\"tdd_order_followed\":true,\"comment_density_ok\":true,\"reviewer_clean_first_pass\":false,\"rebase_conflicts\":false,\"n_block_rounds\":1,\"pr\":172,\"head_sha\":\"abc123\"}}"
}
```

| Field | Type | Set by | Semantics |
|-------|------|--------|-----------|
| `template` | string | dispatcher | `reviewer` / `designer` / `implementer` / `arbiter` |
| `role` | string | dispatcher | `reviewer-1` / `reviewer-2` / `designer-A` / `designer-B` / `arbiter` / `implementer` |
| `tdd_order_followed` | bool | implementer rows | `git log --reverse` shows RED before GREEN |
| `comment_density_ok` | bool | implementer rows | `scripts/check-comment-density.sh` green on diff |
| `reviewer_clean_first_pass` | bool | reviewer-1 rows | verdict == `clear-to-merge` on first dispatch |
| `rebase_conflicts` | bool | implementer rows | rebase produced conflicts |
| `n_block_rounds` | int | all turn rows | block-cycle count before merge |
| `pr` | int | dispatcher | PR number |
| `head_sha` | string | dispatcher | head SHA audited |

Fields nullable per role. Selflearn (§5) treats `null` as "n/a", not
"0".

Why JSON-in-`Detail` not new fields: `internal/audit/audit.go`
explicitly states the schema is stable — "renaming breaks the entire
downstream consumer set". Riding inside `Detail` keeps every existing
reader correct.

### 4.2 Emission points

| When | Kind | Who writes |
|------|------|-----------|
| Implementer subagent completes | `implementer_turn` | implementer (via existing `audit.Logger.Append`) |
| Reviewer emits verdict | `reviewer_turn` | main-thread dispatcher |
| Arbiter emits verdict | `arbiter_turn` | main-thread dispatcher |
| Tournament escalation | `tournament_escalation` | main-thread dispatcher |
| Reflexion fires | `reflexion_dispatch` | main-thread dispatcher |

Subagents must NOT mutate the operator's audit log directly; main
thread is the right writer for reviewer/arbiter rows. Implementer
already writes its own `audit.Entry` rows (selfbuild flow) — it
extends with scorecard.

### 4.3 Privacy

Scorecard rows carry NO operator PII: `pr` (public), `head_sha`
(public), `template`/`role` (identifiers), booleans (telemetry),
`args_hash` (SHA). Safe to ship under future S11 MCP
`leah_search_audit` read-only tool.

## 5. Selflearn aggregation

### 5.1 `internal/selflearn/scorecard.go`

```go
type ScorecardAggregate struct {
    Template string
    Role     string
    Window   time.Duration
    N        int
    Clean    int  // reviewer_clean_first_pass == true (reviewer rows)
    TDDOk    int  // tdd_order_followed == true (impl rows)
    DensOk   int  // comment_density_ok == true (impl rows)
    Rebase   int  // rebase_conflicts == true (impl rows)
    Blocks   int  // sum(n_block_rounds)
}

type ScorecardWalker struct {
    AuditPath string
    Now       func() time.Time
}

func (w *ScorecardWalker) Aggregate(window time.Duration) ([]ScorecardAggregate, error)
```

### 5.2 Degradation detection

`internal/selflearn/retro.go:Generate` gains a `## Dispatch template
scorecard` section with two tables:

1. **Current 30d window** — one row per `(template, role)` showing
   `Clean%`, `TDDOk%`, `DensOk%`, `Rebase%`, `Blocks` (raw).
2. **30d vs prior-30d delta** — same rows, delta column on each
   metric. Drops >10 pp (configurable `LEAH_SCORECARD_DEGRADE_PP=10`)
   flag the row `⚠ degrading`.

A `⚠ degrading` row signals: the template's prose has drifted, its
rule-set is missing a recent failure mode, OR the dispatch shape is
leaking adversarial coverage. The fix is a template-edit PR — which
is itself load-bearing (§3.1) so it re-enters tournament review.

### 5.3 Cadence

Retro runs weekly per `autonomous-session-prompt.md` cadence.
Scorecard aggregation hooks the same run — no new scheduler.
On-demand inspection via `leah retro --week=2026-W23 --scorecard`
(adds one flag to existing `cmd/leah/retro.go`).

## 6. Activation gates (consolidated)

| Surface | Env flag | Default | Skip sentinel | Force sentinel |
|---------|----------|---------|---------------|----------------|
| Reflexion | `LEAH_REVIEW_REFLEXION` | ON for spec, OFF for code | `<no-reflexion>` | `<force-reflexion>` |
| Tournament | `LEAH_REVIEW_TOURNAMENT` | ON for load-bearing, OFF else | `<no-tournament>` | `<force-tournament>` |
| Scorecard | `LEAH_SCORECARD_EMIT` | ON | (telemetry only) | — |
| Degrade PP | `LEAH_SCORECARD_DEGRADE_PP` | 10 | — | — |

Env flags read at dispatcher startup; sentinel scan against PR body
at dispatch time (one `gh pr view --json body` call already in loop).

Sentinels are LAST-RESORT. Operators who reach for `<no-...>`
repeatedly are signaling template drift, which the scorecard catches.

## 7. Wave plan W105-W108

| Wave | Touches | TDD anchors |
|------|---------|-------------|
| W105 | `internal/audit/scorecard.go` (+ test). Encodes Scorecard sub-object into `Entry.Detail`; no field changes. | `TestScorecard_RoundTrip`, `TestScorecard_RejectsUnknownFields`, `TestScorecard_NilFieldsByRole` |
| W106 | `internal/selflearn/scorecard.go` (+ test); retro report `## Dispatch template scorecard` section. | `TestScorecardWalker_AggregatesByTemplateRole`, `TestScorecardWalker_DegradationWindow`, `TestRetro_ScorecardSectionRendersDelta` |
| W107 | `internal/dispatcher/` reads env flags + PR-body sentinels; conditional reflexion/tournament dispatch; emits `reviewer_turn`, `arbiter_turn`, `reflexion_dispatch`, `tournament_escalation`. Lockstep edit to `docs/engineer/dispatch-templates/reviewer.md` LOAD-BEARING CARVE-OUT (CLAUDE.md root-file rule — single owner). | `TestDispatcher_ReflexionFiresOnBlockOnly`, `TestDispatcher_TournamentLoadBearingOnly`, `TestDispatcher_SentinelOverridesEnvFlag`, `TestDispatcher_EscalationCapAtThreeRounds` |
| W108 | `cmd/leah retro --scorecard` flag wires §5 into CLI. | `TestRetroCLI_ScorecardFlagRendersTable` |

W105 || W106 (file-disjoint) → W107 (imports both) → W108 (imports
W106). Critical path: 3 sequential dispatches.

## 8. Cost analysis

| PR class | Dispatches (mean) | Cost (Sonnet ~$0.05/dispatch) |
|----------|-------------------|-------------------------------|
| Code, clear first pass | 1.0 | $0.05 |
| Code, block round | 2.0 | $0.10 |
| Spec, clear first pass | 4.0 (reviewer + 2 designer + arbiter) | $0.20 |
| Spec / load-bearing, block round | 5.0 | $0.25 |

At prior 14d ~25% block rate, ~15% load-bearing mix, ~30 PRs/wk:
weighted sum ≈ **$2.56/wk** (well under S1's $15/wk cap; lives in the
same `internal/budget` envelope). Scorecard rows are free (one append
per existing audit write).

## 9. Failure modes

| Mode | Mitigation |
|------|-----------|
| Reflexion recursion (reviewer-3 spawned) | Hard cap N=1; reviewer-2 verdict final per turn |
| Arbiter novel findings outside §3.3 | Reviewer.md trap entry; arbiter scorecard flagged in retro |
| Tournament wall-clock | §3.4 ord. 1 parallel; wall ≈ slowest subagent + arbiter, not sum |
| Scorecard schema drift | W105 test rejects unknown fields → audit `Outcome: "schema_drift"` |
| Env-flag stuck | PR-body sentinel always wins (operator escape) |
| Habitual `<no-tournament>` | Retro surfaces sentinel-skip rate per template; >50% = rewrite signal |
| Stale reviewer-2 (reviewer-1 ran on old SHA) | Dispatcher re-spawns reviewer-1 on current head first (§2.2) |
| Arbiter deadlock | §3.5 escalate-to-operator + 3-round cap → `[followup]` + `re-spawn-design` |

## 10. Grade rubric

- **B (minimum)**: reflexion for spec PRs; scorecard rows from
  reviewer + implementer turns; retro weekly scorecard section.
- **A (target)**: B + tournament for load-bearing (designer-A +
  designer-B + arbiter); >10pp degradation flag; PR-body sentinel
  override; §8 cost within 20% over 4-week window.
- **A+ (stretch)**: A + tournament-on-block ordering (§3.4 ord. 2);
  arbiter escalation cap with `[followup]` (§3.5); scorecard rows
  safe for S11 MCP read-only exposure (§4.3); `leah retro --scorecard
  --since=YYYY-WW` ad-hoc inspection.

## 11. OSS prior art

- **Reflexion** (Shinn et al. 2023, arxiv.org/abs/2303.11366; ref impl
  github.com/noahshinn/reflexion, MIT, tag v0.1.0): source of the
  "what did the first agent miss?" framing. Divergence: agent-to-agent
  (reviewer-1 → reviewer-2), not agent-to-self — self-critique was
  the observed implementer over-self-review trap (implementer.md
  §Recurring trap 2).
- **CAMEL** (github.com/camel-ai/camel, Apache-2.0, tag v0.2.16):
  parallel-agents + arbiter shape. Divergence: hard-cap at 3, matching
  reviewer.md's 2-of-3 attestation pattern (S7 risk tier).
- **AutoGPT-style critique loops**: explicitly rejected — unbounded
  recursion was the documented failure. §3.5 cap is the structural
  disagreement.
- **LangGraph `Send` API** (github.com/langchain-ai/langgraph, MIT,
  tag 0.2.x): topology match for §3.2 fan-out + arbiter join. Borrow
  the shape, not the framework (Go, not Python).
- **S7 2-of-3 attestation** (wave-8 brief §S7): designer-A + B is the
  same idea applied to spec PRs instead of SelfBuild dispatches.

## 12. Constraints inherited

- Spec PR serializes (CLAUDE.md).
- Code waves file-disjoint (W105: audit, W106: selflearn, W107:
  dispatcher + lockstep template, W108: cmd/leah).
- No AI signatures.
- No self-approve — every wave PR gets independent reviewer; W107
  enters tournament because it touches the load-bearing surface it
  defines.
- Worktree discipline; relative paths only.
- Root cause > symptom — scorecard-surfaced degradation demands
  template-rewrite, not metric suppression.
- TDD: failing-test-first; meta-tests for scorecard schema land in
  W105 before any production scorecard write.

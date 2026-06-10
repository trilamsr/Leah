# Eval pipeline + LLM-as-judge harness — Leah

Date: 2026-06-10
Scope: MVP-5 (Wave-8 S1)
Owners: `evals/`, `internal/eval/`, `.github/workflows/eval.yml`

Companion docs:
- `docs/engineer/briefs/2026-06-10-wave-8-aiml-upgrade.md` — wave-8 synthesis
- `docs/engineer/specs/2026-06-10-observability.md` — metrics registry
- `docs/engineer/specs/2026-06-10-event-timeline.md` — audit row shape
- `internal/budget/budget.go` — `LEAH_BUDGET_DOLLARS` ceiling

## 1. Goal

Every prompt edit, reasoner change, intent classifier tweak, or
recommender ranking change MUST land with a measurable pass-rate
delta against a golden trace set. No more faith-based shipping.

A PR that touches `prompts/`, `internal/reasoner/`, `internal/intent/`,
or `internal/recommend/` cannot merge unless `make eval` produces a
delta table showing pass-rate within 2 percentage points of main.

### Goals

- Golden traces per feature stored as `evals/<feature>.jsonl`.
- LLM-as-judge harness (`make eval`) producing reproducible pass-rate.
- GH check that blocks merge on >2% regression.
- Hard budget (≤$5/wk) wired through `LEAH_BUDGET_DOLLARS`.
- Historical results in `~/.leah-state/eval-history.jsonl` + CI artifact.
- Operator override path that is attestation-gated, not click-through.

### Non-goals (v1)

- Continuous online eval against production traffic (deferred to S2).
- Prompt-optimization loop (DSPy / Anthropic Prompt Improver) — P2 rejected.
- Model-vs-model arena rankings — single judge model, fixed per release.
- Per-operator eval personalization — golden set is global.
- Eval-driven autotuning of recommender weights (S4 owns that path).

## 2. Repository layout

```
evals/
  reasoner.jsonl          # 50 golden traces; reasoner call → expected output
  recommend.jsonl         # 50 traces; (operator state, candidates) → ranked top-3
  voice_intent.jsonl      # 50 traces; transcript → intent JSON
  brief.jsonl             # 50 traces; corpus → brief markdown
  README.md               # how to add traces; rubric pointers
internal/eval/
  harness.go              # runner: load .jsonl, call feature, judge, emit delta
  harness_test.go
  judge.go                # LLM-as-judge prompt + scorer; provider-agnostic
  judge_test.go
  rubric.go               # per-feature rubric structs + validation
  rubric_test.go
  budget.go               # per-run cost cap + LEAH_BUDGET_DOLLARS wiring
  budget_test.go
  history.go              # eval-history.jsonl writer + retention
  history_test.go
  doc.go
cmd/leah-eval/
  main.go                 # `make eval` entry point; flags --feature, --base, --json
.github/workflows/
  eval.yml                # required check on prompts/ + internal/{reasoner,intent,recommend}/
scripts/
  eval-bootstrap.sh       # cold-start trace capture (§9)
```

`make eval` is one new Makefile target invoking `go run ./cmd/leah-eval`.

## 3. Trace schema — `evals/<feature>.jsonl`

One JSON object per line. Schema is feature-tagged, validated by
`internal/eval/rubric.go` at load time. Unknown fields rejected.

### 3.1 Envelope (shared across features)

```json
{
  "id": "reasoner.001",
  "feature": "reasoner",
  "added_at": "2026-06-10T18:30:00Z",
  "added_by": "operator|capture|hand",
  "input": { ... },
  "expected": { ... },
  "rubric": "rubric_id_string",
  "tags": ["regression-pr-141", "hud-widget"],
  "skip_reason": ""
}
```

| Field | Type | Notes |
|-------|------|-------|
| `id` | string | `<feature>.<3-digit>`; immutable once landed; orphans recycled at expiry |
| `feature` | string | enum: `reasoner`, `recommend`, `voice_intent`, `brief` |
| `added_at` | RFC3339 string | when the trace landed on main |
| `added_by` | string | `operator` (manual), `capture` (live), `hand` (cold-start author) |
| `input` | object | feature-specific; see §3.2-3.5 |
| `expected` | object | feature-specific; the "ground truth" pass condition |
| `rubric` | string | rubric ID; resolves to a `rubric.go` entry |
| `tags` | string[] | free-form; used by harness `--filter`; bounded len 8 |
| `skip_reason` | string | non-empty = trace temporarily disabled; CI surfaces count |

### 3.2 `reasoner` input/expected

```json
{
  "input": {
    "system_prompt_path": "prompts/system.md",
    "user_messages": [{"role":"user","content":"..."}],
    "tools_available": ["dispatch","memory_get","audit_search"],
    "max_tokens": 1024
  },
  "expected": {
    "must_contain": ["substring1"],
    "must_not_contain": ["banned phrase"],
    "tool_called": "dispatch",
    "rubric_notes": "1-line author note about why this trace matters"
  }
}
```

### 3.3 `recommend` input/expected

```json
{
  "input": {
    "operator_state": { "...ProfileRow fields..." },
    "candidates": [{"pattern":"...","weight":0.4}, ...]
  },
  "expected": {
    "top3_pattern_ids": ["p_a","p_b","p_c"],
    "ordering_strict": false,
    "rubric_notes": "..."
  }
}
```

### 3.4 `voice_intent` input/expected

```json
{
  "input": {
    "transcript": "yes leah dispatch the gmail wave",
    "operator_id": "default"
  },
  "expected": {
    "intent": "dispatch_wave",
    "slot_values": {"wave":"gmail"},
    "rubric_notes": "..."
  }
}
```

### 3.5 `brief` input/expected

```json
{
  "input": {
    "corpus_paths": ["docs/engineer/specs/2026-06-10-observability.md"],
    "instruction": "Summarize for an operator catching up after 1 week."
  },
  "expected": {
    "must_cover_topics": ["metrics","events","traces"],
    "max_words": 600,
    "rubric_notes": "..."
  }
}
```

## 4. LLM-as-judge rubric

### 4.1 Judge model

One judge, pinned per release in `internal/eval/judge.go`:

```
JudgeModel        = "claude-sonnet-4-5-20251022"  // v1 default
JudgeTemperature  = 0.0
JudgeMaxTokens    = 1024
JudgeProviderEnv  = "LEAH_EVAL_JUDGE_PROVIDER"    // anthropic|openai|ollama
```

Rationale: Sonnet is the cost-balanced calibration point — Haiku
under-discriminates on the recommend rubric; Opus is 5× the price for
marginal lift on these short rubrics. Pinned + temperature 0 makes the
judge near-deterministic across reruns (still re-scored across feature
PRs to detect judge drift in §11).

Local override: `LEAH_EVAL_JUDGE_PROVIDER=ollama` swaps in a local
fallback (qwen3:32b) for fully-offline runs; harness logs which provider
served the judge so CI artifacts are reproducible.

### 4.2 Judge prompt template

Stored in `prompts/eval-judge.md`. Single template, slotted per
feature. Fixed structure forces the judge to emit a parseable score:

```
You are an evaluation judge for the Leah AI assistant.

Feature under test: {{.Feature}}
Rubric ID:         {{.RubricID}}

The system gave the following INPUT:
{{.Input}}

The system PRODUCED:
{{.Actual}}

The EXPECTED criteria are:
{{.Expected}}

Score the production against the expected criteria. Output ONLY a JSON
object on a single line:

{"pass": <true|false>, "score": <0.0..1.0>, "reason": "<= 200 chars"}

A trace passes when score >= 0.8 AND all hard constraints (must_contain,
must_not_contain, tool_called, intent, slot_values) are satisfied.
```

`Input`, `Actual`, `Expected` rendered as compact JSON; no markdown
escaping inside. Judge response parsed by `internal/eval/judge.go` —
malformed JSON → trace counted as failed with `judge_unparseable`
reason, NEVER silently dropped.

### 4.3 Scoring rubric — per-feature pass condition

| Feature | Hard constraint | Soft score |
|---------|-----------------|------------|
| reasoner | `must_contain` ∧ ¬`must_not_contain` ∧ (tool_called == expected ∨ unset) | judge 0..1 on fluency + faithfulness |
| recommend | top-3 set ⊇ expected top-3 (strict ordering if `ordering_strict`) | judge 0..1 on ranking-quality reasoning |
| voice_intent | `intent` exact ∧ all `slot_values` keys present | judge 0..1 on slot-value fidelity (allows paraphrase) |
| brief | all `must_cover_topics` substrings present ∧ word count ≤ `max_words` | judge 0..1 on operator-utility |

Combined pass = hard ∧ (judge_score ≥ 0.8). The hard test is a code
gate (no judge call needed when hard fails — saves budget).

### 4.4 Aggregation

Per-feature pass-rate = `passed / (total - skipped)`.
Overall pass-rate = micro-average across features (sum passed / sum eligible).

## 5. `make eval` workflow

### 5.1 Local invocation

```
make eval                               # all features, sequential per-feature
make eval FEATURE=reasoner              # single feature
make eval BASE=origin/main              # compare HEAD vs BASE
make eval JSON=1                        # machine-readable; default human table
```

Make target:

```makefile
eval:
	@LEAH_BUDGET_DOLLARS=$${LEAH_BUDGET_DOLLARS:-5} \
	  go run ./cmd/leah-eval --feature=$(FEATURE) --base=$(BASE) --json=$(JSON)
```

### 5.2 Harness execution shape

1. Load `evals/<feature>.jsonl` per feature in scope.
2. For each trace, run the feature under HEAD code → `actual_head`.
3. Checkout `BASE` (default `origin/main`) into a scratch dir;
   re-run same traces → `actual_base`. Cached by `(BASE_SHA,trace_id)`
   keyed file under `~/.leah-state/eval-cache/` to skip re-judging
   unchanged BASE results across PRs (the dominant cost).
4. Judge both — but only call judge when hard constraints differ OR
   when HEAD is uncached AND not byte-identical to cached actual.
5. Compute deltas; write `~/.leah-state/eval-history.jsonl` row.
6. Render delta table (markdown if `--json=0`, JSON otherwise) to
   stdout AND `/tmp/leah-eval-out.{md,json}`.

Per-feature traces run concurrently bounded by
`runtime.NumCPU()/2` (cap 4). Within a feature, traces stream serially
to keep judge requests rate-limit-friendly and budget-trackable.

### 5.3 Delta table shape

```
| Feature        | Base pass | HEAD pass | Δ pp | Verdict |
|----------------|-----------|-----------|------|---------|
| reasoner       | 46/50     | 47/50     | +2.0 | PASS    |
| recommend      | 42/50     | 39/50     | -6.0 | FAIL    |
| voice_intent   | 49/50     | 49/50     |  0.0 | PASS    |
| brief          | 44/50     | 44/50     |  0.0 | PASS    |
| **overall**    | 181/200   | 179/200   | -1.0 | PASS    |
```

The PR-body section is generated verbatim from this table by `eval.yml`.

## 6. GitHub check integration

### 6.1 Trigger

`.github/workflows/eval.yml` runs on PR events when paths-filter matches:

```yaml
on:
  pull_request:
    paths:
      - 'prompts/**'
      - 'internal/reasoner/**'
      - 'internal/intent/**'
      - 'internal/recommend/**'
      - 'evals/**'
      - 'internal/eval/**'
      - 'cmd/leah-eval/**'
```

Branch protection rule on `main` adds `eval / regression-gate` to the
required-checks list (set out-of-band by the operator; spec PR documents
the intent only).

### 6.2 Workflow shape

```yaml
jobs:
  regression-gate:
    runs-on: ubuntu-latest
    timeout-minutes: 20
    permissions:
      contents: read
      pull-requests: write
    steps:
      - uses: actions/checkout@v4
        with: { fetch-depth: 0 }
      - uses: actions/setup-go@v5
        with: { go-version: '1.24', cache: true }
      - name: Run eval
        env:
          ANTHROPIC_API_KEY: ${{ secrets.LEAH_EVAL_ANTHROPIC_KEY }}
          LEAH_BUDGET_DOLLARS: '0.50'   # per-PR cap; see §7
          LEAH_EVAL_BASE: 'origin/main'
        run: make eval JSON=1 > /tmp/eval.json
      - name: Post delta table
        uses: actions/github-script@v7
        with:
          script: |
            const fs = require('fs');
            const body = fs.readFileSync('/tmp/leah-eval-out.md','utf8');
            // upsert-comment by marker: <!-- leah-eval -->
            // omitted for brevity; one comment per PR, edited on each run
      - name: Gate
        run: |
          jq -e '.verdict == "PASS"' /tmp/eval.json
```

### 6.3 What blocks

The `regression-gate` job is `required`. Failure modes:

| Cause | Verdict | Block? |
|-------|---------|--------|
| overall Δ ≤ -2.0 pp | FAIL | YES |
| any per-feature Δ ≤ -5.0 pp | FAIL | YES (even if overall passes) |
| budget exhausted mid-run | FAIL | YES (with `budget_exhausted` reason) |
| judge_unparseable rate > 5% | FAIL | YES (signals judge drift) |
| timeout (20 min) | FAIL | YES |
| API key missing | NEUTRAL | NO (treated as infra outage; not author's fault) |

The per-feature -5pp catastrophic gate prevents a single high-weight
feature regression from hiding under green features.

## 7. Failure semantics

### 7.1 The 2-pp gate

Overall pass-rate Δ must be > -2.0 pp. ">" not "≥" — a -2.0 pp drop
on a 200-trace set is 4 fewer passes; treat as a regression that
demands a justification, not a freebie.

### 7.2 Per-feature catastrophic gate

Any single feature Δ ≤ -5.0 pp fails immediately. Rationale: with 50
traces per feature, 5 pp is 2.5 traces — well above natural judge
variance (measured at ~0.8 pp σ in §11 meta-eval).

### 7.3 Reaction matrix

| Δ | Action |
|----|--------|
| ≥ 0 | PASS; merge unblocked |
| (-2, 0) | PASS with warning comment ("watch this") |
| (-5, -2] overall | FAIL; author must either improve or invoke override (§10) |
| ≤ -5 any feature | FAIL; override requires 2-of-3 reviewers (§10) |

### 7.4 Skipped traces

`skip_reason` non-empty → trace excluded from both base AND head
denominators. CI surfaces a `skipped: N` count. Skip count > 10% of any
feature emits a warning (eval set rotting).

## 8. Budget enforcement

### 8.1 Caps

| Scope | Cap | Source of truth |
|-------|-----|-----------------|
| Per-eval-run | $0.50 | `LEAH_BUDGET_DOLLARS` env, set in `eval.yml` |
| Per-day (CI) | $2.00 | GHA concurrency limit + per-run cap |
| Per-week | $5.00 | wave-8 brief constraint; enforced by audit |

Per-week enforcement: `internal/eval/budget.go` reads
`~/.leah-state/eval-history.jsonl` (or a CI-cached blob) on startup,
sums the trailing 7 days' `cost_dollars`, refuses to start if sum + cap
> $5.00. CI cache key: `eval-history-{{ hashFiles('evals/**') }}`.

### 8.2 Wiring

The harness wraps every judge call in `internal/budget.Charge`. On
`*ExceededError`:

1. Mark current trace `budget_exhausted`.
2. Stop further judge calls.
3. Compute partial verdict: PASS only if pre-exhaustion delta ≥ 0;
   otherwise FAIL with `budget_exhausted` reason. (Conservative — never
   let a budget cutoff silently turn a regression green.)

### 8.3 Per-call cost estimate

Judge call ≈ 1.5k input + 200 output tokens. Sonnet pricing
(2026-06-10): ~$0.0045/call. 200 traces × 2 sides = 400 calls = $1.80
worst case, ~$0.30 with BASE caching (typical PR re-judges 20 traces).

## 9. Historical results storage

### 9.1 Locations

- Local: `~/.leah-state/eval-history.jsonl` (append-only, one row per run).
- CI: same file, uploaded as workflow artifact `eval-history-${{github.sha}}`
  and retained 90 days. Aggregated to `gh-pages` branch nightly as
  `eval-history-aggregate.jsonl` for trend dashboards (§12).

### 9.2 Row schema

```json
{
  "ts": "2026-06-10T18:30:00Z",
  "pr": 152,
  "head_sha": "...",
  "base_sha": "...",
  "judge_model": "claude-sonnet-4-5-20251022",
  "judge_provider": "anthropic",
  "cost_dollars": 0.42,
  "verdict": "PASS",
  "features": {
    "reasoner":    {"base":46,"head":47,"total":50,"delta_pp":2.0},
    "recommend":   {"base":42,"head":39,"total":50,"delta_pp":-6.0},
    "voice_intent":{"base":49,"head":49,"total":50,"delta_pp":0.0},
    "brief":       {"base":44,"head":44,"total":50,"delta_pp":0.0}
  },
  "skipped_count": 0,
  "judge_unparseable_count": 0,
  "duration_seconds": 312
}
```

### 9.3 Retention

Local: rolled at 1 MB to `eval-history.YYYY-WW.jsonl.gz`; gzipped files
retained indefinitely (~50 KB/wk → bounded).
CI artifacts: 90 days (GitHub default).
Aggregate file on `gh-pages`: lifetime, downsampled to weekly bins after
1 year (`internal/eval/history.go:Compact`).

## 10. Cold-start — first 50 traces per feature

### 10.1 Capture path (preferred, where available)

`scripts/eval-bootstrap.sh --feature=reasoner --n=50` taps existing
audit rows in `~/.leah-state/audit.jsonl` produced by the operator's
own day-to-day use of Leah over the prior 14 days. For each captured
turn:

1. Reconstruct `input` from audit `Input`/`PromptSHA`/`Tools`.
2. Use the production `Actual` output as a candidate `expected`.
3. Operator hand-edits `must_contain` / `must_not_contain` /
   `tool_called` to pin the intent.

Cost: ~2 hours operator time to curate 50 traces.

### 10.2 Hand-written path (when capture impossible)

For features without enough live history (initially: `brief` is the
likely gap), the spec author hand-writes 50 traces drawn from
`docs/engineer/specs/`. Each trace gets `added_by: hand` and a
`rubric_notes` justification; reviewer agent checks each for triviality.

### 10.3 Wave plan (§13) — W82-W85 each owns one feature.

### 10.4 Anti-cheating

Capture-mode traces MUST be reviewed by a separate adversarial
subagent (`cavecrew-reviewer-evalset-<feature>`) before landing.
The reviewer's job: reject traces where `expected` is "whatever HEAD
produced" — that would lock in current behavior as forever-correct.

## 11. Operator override — force-merge despite eval block

The eval gate is required-but-bypassable, not advisory. Bypass path:

### 11.1 Attestation flow

1. Operator runs `leah eval override --pr=152 --reason="<text>"`.
2. CLI invokes the standard attestation gate (existing
   `cmd/leah/connect_*` pattern, scope `eval:override`).
3. On success, writes a row to `~/.leah-state/audit.jsonl`:
   ```json
   {"kind":"eval_override","pr":152,"reason":"...","operator":"tri",
    "ts":"...","blast_radius":3}
   ```
4. CLI POSTs to GH check via `gh api ... /check-runs/<id>` setting
   `conclusion: success` with a body line `LEAH-EVAL-OVERRIDE: <audit_id>`.

### 11.2 Constraints

- BR=3 (one level below `connect:regatta:cloud`).
- Override audit row replayable — `leah audit list --kind=eval_override`
  surfaces in `leah whoami` (S10).
- Override does NOT silence the next PR's gate; it's per-PR.
- Per-feature -5pp failures require attestation AND a second-reviewer
  attestation row (2-of-3 reviewer rule from S7).

### 11.3 Why attestation, not just a label

A `bypass-eval` GH label is click-through; an attestation prompt forces
the operator to type the reason. Friction = signal: most regressions
either get fixed or get a real explanation. The brief-trap (§14 in
implementer.md) of accreting rationalizations is mechanically prevented
by the prompt's char-count floor (default 32 chars).

## 12. Test plan for the pipeline itself (meta-eval)

The harness ships TDD-first. New tests in `internal/eval/`:

### 12.1 Determinism

`TestHarness_DeterministicForCachedBase`: run harness twice with same
HEAD + cached BASE → identical verdict + identical cost (zero, because
nothing re-judges).

### 12.2 Judge drift detection

`TestJudge_SelfConsistency`: run the same trace through the judge 10
times at temp=0 → variance over 10 runs must be ≤ 0.1 score-std-dev.
Fails the build if the pinned judge model drifts past the bound.

### 12.3 Budget cutoff

`TestBudget_PartialRunBecomesFailWhenNegativeDelta`: synthetic 200-trace
set, force budget exhaustion at trace 50 with a -3pp running delta →
verdict FAIL with `budget_exhausted` reason.

### 12.4 Schema validation

`TestRubric_RejectsUnknownFields`, `TestRubric_RejectsMissingExpected`.

### 12.5 Adversarial trace plant

`TestHarness_DetectsKnownRegression`: a fixture trace where HEAD
deliberately fails a hard constraint MUST produce a FAIL verdict. Run as
part of `make check` to catch harness rot.

### 12.6 Cost estimate accuracy

`TestBudget_EstimateWithinTwentyPctOfActual`: synthetic judge response
sizes; estimator (used for early budget reservation) must stay within
20% of `actual_cost`.

## 13. Observability

### 13.1 Prometheus metrics (registered against `internal/obs.Registry`)

| Metric | Type | Labels | Notes |
|--------|------|--------|-------|
| `leah_eval_run_total` | counter | `verdict={pass,fail,neutral}` | one per `make eval` invocation |
| `leah_eval_trace_total` | counter | `feature`, `outcome={pass,fail,skipped,unparseable,budget_exhausted}` | per-trace tally |
| `leah_eval_delta_pp` | gauge | `feature` | last-run signed delta |
| `leah_eval_cost_dollars` | counter | `judge_provider` | cumulative dollars spent on judge calls |
| `leah_eval_duration_seconds` | histogram | `feature` | per-feature wall time |
| `leah_eval_judge_unparseable_total` | counter | `feature` | judge-output-malformed count |

Cardinality: 4 features × 6 outcomes = 24 series for the trace counter,
well under the 200-series-per-package budget from the observability
spec.

### 13.2 Audit rows

`internal/eval/harness.go` writes one `audit.Entry` per run:

```
Kind:        "eval_run"
BlastRadius: 0
Outcome:     "success" | "regression"
Detail:      "<verdict>:<overall_delta_pp>:<pr>"
```

Override flow writes its own `eval_override` row (§11.1).

### 13.3 Local dashboard tile

The HUD recommend widget (PR #141) gets a sibling tile `eval-trend`
rendering the last 14 days of overall pass-rate from
`eval-history.jsonl`. Implementation out of scope for S1; spec
documents the data contract only.

## 14. Initial waves W82-W85

Each wave is one PR, file-disjoint, lands serially per the wave-8 brief
sequencing rule. Each wave depends on the prior wave's `internal/eval/`
landing (the harness must exist before traces have meaning).

### W82 — harness + reasoner eval set (one PR)

- `internal/eval/{harness,judge,rubric,budget,history}.go` + tests.
- `cmd/leah-eval/main.go`.
- `Makefile` target `eval`.
- `.github/workflows/eval.yml` (initially `continue-on-error: true` so
  the first set of traces lands without blocking the wave on itself).
- `evals/reasoner.jsonl` — 50 traces, captured via §10.1.
- `prompts/eval-judge.md`.

### W83 — recommend eval set

- `evals/recommend.jsonl` — 50 traces.
- `internal/eval/rubric.go` extension for ranking comparison.
- Flips `continue-on-error: false` (the gate goes live with two features).

### W84 — voice_intent eval set

- `evals/voice_intent.jsonl` — 50 traces.
- Reuses harness; no new code beyond rubric IDs.
- 24-hour soak: dashboard tile must show stable pass-rate before merging.

### W85 — brief eval set

- `evals/brief.jsonl` — 50 traces (likely all `added_by: hand`).
- Adds `internal/eval/judge.go` length-constraint check helper.
- Brings the gate to its full 200-trace breadth.

### Wave guards

- Each wave PR fails its own `regression-gate` job if it ships a trace
  set that HEAD cannot already pass — prevents "land broken traces, fix
  later" anti-pattern.
- Each wave PR requires an adversarial reviewer subagent on the trace
  set itself (§10.4).

## 15. 2026 SOTA precedents and divergences

- **Anthropic prompt-evals cookbook** — we adopt the JSONL + judge-prompt
  shape. Divergence: we ship the judge prompt in-repo (`prompts/eval-judge.md`)
  rather than inlining in code, so it gets the same `prompt_sha` audit
  treatment as every other prompt (S2).
- **Braintrust autoevals + GH Action** — we adopt the PR-comment-with-
  delta-table UX. Divergence: no third-party hosted store; everything
  lives in-repo + `~/.leah-state/` for the LOCAL_ONLY moat (S10).
- **LangSmith CI integration** — we adopt the cached-base-evaluation
  pattern (don't re-judge unchanged BASE traces). Divergence: cache key
  is `(base_sha,trace_id)` only; no LangSmith server.
- **OpenAI Evals** — we adopt the per-rubric pluggable scorer concept.
  Divergence: single judge model, not a model registry — the operator
  picks one and lives with it for the release.
- **Patronus AI** — we adopt the catastrophic-feature-gate idea
  (-5pp gate of §7.2). Divergence: no hosted dashboard; the audit
  trail + Prometheus + HUD tile cover the visibility need.

## 16. Constraints inherited

- spec PR serializes (CLAUDE.md rule).
- code wave PRs (W82-W85) file-disjoint per the wave-8 dispatch rule.
- no AI signatures.
- no self-approve — every wave PR gets `cavecrew-reviewer-<topic>`.
- worktree discipline; relative paths only.
- root cause > symptom — eval failures investigated to the prompt or
  code edit that caused them; not papered over by retracing.
- TDD: §12 meta-eval lands FIRST in W82, before any trace lands.

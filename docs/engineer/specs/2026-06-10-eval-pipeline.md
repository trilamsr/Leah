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
- Hard budget (≤$15/wk) wired through a dedicated eval counter that
  is independent of `internal/budget.DefaultCeiling` ($5 per-process).
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
  budget.go               # per-run cost cap + LEAH_EVAL_BUDGET_DOLLARS wiring
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
	LEAH_EVAL_BUDGET_DOLLARS=$${LEAH_EVAL_BUDGET_DOLLARS:-3} \
	  go run ./cmd/leah-eval --feature=$(FEATURE) --base=$(BASE) --json=$(JSON)
```

(Unsuppressed; matches house Makefile style — recipe lines are
grep'able in CI logs.)

### 5.2 Harness execution shape

1. Load `evals/<feature>.jsonl` per feature in scope.
2. For each trace, run the feature under HEAD code → `actual_head`.
3. Checkout `BASE` (default `origin/main`) into a scratch dir;
   re-run same traces → `actual_base`. Cached by
   `sha256(BASE_SHA || trace_id || judge_prompt_sha || judge_model)`
   keyed file under `~/.leah-state/eval-cache/` to skip re-judging
   unchanged BASE results across PRs (the dominant cost). Editing
   `prompts/eval-judge.md` OR bumping `JudgeModel` MUST bust the cache —
   omitting either component would silently serve stale BASE scores
   and mask regressions caused by judge changes.
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
      - name: Gate fork PRs
        if: github.event.pull_request.head.repo.full_name != github.repository
        run: |
          echo "::warning::eval skipped on fork PR (secret not exposed); a maintainer must re-run via workflow_dispatch after manual approval"
          exit 78   # neutral exit; gate treats as NEUTRAL not FAIL (see §6.3)
      - name: Run eval
        env:
          ANTHROPIC_API_KEY: ${{ secrets.LEAH_EVAL_ANTHROPIC_KEY }}
          LEAH_EVAL_BUDGET_DOLLARS: '3.00'   # per-run cap; see §8.1
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

### 7.1 The gate (paired, not unpaired)

Both gates compare BASE and HEAD on the **same trace set** — every
trace is judged twice (BASE code vs HEAD code) and the unit of
analysis is the per-trace flip, not the raw pass-rate delta. This is
a paired comparison (McNemar's test), not two independent samples.

The independent-sample binomial SE on a 50-trace pass-rate at p≈0.9
is √(0.9·0.1/50) ≈ 4.2 pp at 1σ — and at p≈0.5 it's ~7 pp at 1σ —
which would put a -5 pp gate inside the noise floor. The paired
formulation collapses that noise: only traces that flip between BASE
and HEAD contribute, so for a typical PR with ~5–10 disagreements
the McNemar SE is roughly √(b+c)/n ≈ 0.5–1 pp, well below the
gate thresholds.

The §12.2 self-consistency σ ≈ 0.8 pp captures judge-determinism
variance on the same actual; the paired gate captures BASE-vs-HEAD
disagreement on the same trace. These are different statistics —
the spec calls out both explicitly so future eyes don't conflate
them.

Overall pass-rate Δ must be > -2.0 pp. ">" not "≥" — a -2.0 pp drop
on a 200-trace paired set means 4 traces flipped BASE-pass→HEAD-fail
net; treat as a regression that demands a justification, not a
freebie.

### 7.2 Per-feature catastrophic gate

Any single feature Δ ≤ -5.0 pp fails immediately. With 50 paired
traces per feature, -5 pp net is ≥2.5 net flips from BASE→fail; the
McNemar SE under a true-null (no real regression) for ~5
disagreements is ~3 pp, so the gate sits at ~1.5σ above noise —
high-signal without being prone to false-fail on judge wobble.

If a future operator widens the eval set or relaxes the paired
constraint, the gate MUST be re-derived; bumping the trace set to
n≥384 per feature would let the gate hold under an unpaired
(±5 pp at 95 % CI) interpretation, which is the fallback if paired
mode is ever disabled.

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
| Per-eval-run | $3.00 | `LEAH_EVAL_BUDGET_DOLLARS` env, set in `eval.yml` |
| Per-day (CI) | $6.00 | GHA concurrency limit + per-run cap |
| Per-week | $15.00 | enforced by `internal/eval/budget.go` against `~/.leah-state/eval-history.jsonl` |

Per-call cost at 1.5k input + 200 output tokens against Sonnet 4.5 is
$0.0075 (§8.3); the 200-trace × 2-side worst case (400 uncached calls)
is $3.00, which sets the per-run cap. At 30 PRs/wk eval-running × $0.50
typical-cost-with-caching, the weekly bill stays well under the $15 cap.

The eval cost-counter is a separate persistent counter from the
`internal/budget` per-process ceiling: `internal/budget.DefaultCeiling = 5.0`
governs a single `cmd/leah` invocation, while `LEAH_EVAL_BUDGET_DOLLARS`
+ the eval-history file enforce per-run and rolling-7d caps across CI
runs. Mixing the two via `LEAH_BUDGET_DOLLARS` would either starve the
eval (cap too low) or weaken the daemon's own budget (cap too high).

Per-week enforcement: `internal/eval/budget.go` reads
`~/.leah-state/eval-history.jsonl` (or a CI-cached blob) on startup,
sums the trailing 7 days' `cost_dollars`, refuses to start if sum + cap
> $15.00. CI cache key: `eval-history-{{ hashFiles('evals/**') }}`.

### 8.2 Wiring

The harness wraps every judge call in `internal/budget.Charge`. On
`*ExceededError`:

1. Mark current trace `budget_exhausted`.
2. Stop further judge calls.
3. Compute partial verdict: PASS only if pre-exhaustion delta ≥ 0;
   otherwise FAIL with `budget_exhausted` reason. (Conservative — never
   let a budget cutoff silently turn a regression green.)

### 8.3 Per-call cost estimate

Judge call ≈ 1.5k input + 200 output tokens. Sonnet 4.5 pricing
(2026-06-10): $3/MTok in + $15/MTok out → 1500·$3/M + 200·$15/M
= $0.0045 + $0.0030 = **$0.0075/call**. 200 traces × 2 sides = 400
calls = **$3.00 worst case**. Typical PR re-judges only ~20 traces
(BASE cached for the other 180) ≈ $0.30 — the worst case is the
rebaseline run after a judge-prompt or `JudgeModel` edit that busts
the cache (§5.2).

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
2. **Redact PII before write.** Run the candidate through
   `scripts/eval-redact.sh` which strips: email addresses
   (`[A-Za-z0-9._%+-]+@…` → `<email>`), E.164 / NANP phone numbers
   (`<phone>`), absolute paths under `$HOME` (`<home>/…`),
   `~/.leah-state/` operator-id segments, and contact-name tokens
   pulled from `~/.leah-state/contacts.jsonl`. The redactor refuses
   to emit a trace if any unmatched `@` survives.
3. Use the redacted `Actual` output as a candidate `expected`.
4. Operator hand-edits `must_contain` / `must_not_contain` /
   `tool_called` to pin the intent.
5. `.gitattributes` MUST add `evals/*.jsonl  filter=eval-redact`
   so a re-staged trace re-runs the redactor — defense-in-depth
   against an operator manually editing in PII later.

Cost: ~2 hours operator time to curate 50 traces.

### 10.2 Hand-written path (when capture impossible)

For features without enough live history (initially: `brief` is the
likely gap), the spec author hand-writes 50 traces drawn from
`docs/engineer/specs/`. Each trace gets `added_by: hand` and a
`rubric_notes` justification; reviewer agent checks each for triviality.

### 10.3 Wave plan (§13) — W82-W85 each owns one feature.

### 10.4 Anti-cheating

A same-session adversarial reviewer subagent is not sufficient on its
own — author and reviewer share the operator's context and can
silently collude on "whatever HEAD produced = expected" lock-in. The
bootstrap set MUST clear BOTH guards before landing:

1. **Independent attestation (2-of-3).** Two adversarial reviewer
   subagents, dispatched from independent operator sessions or with
   distinct agent-ids (`cavecrew-reviewer-evalset-<feature>-{a,b}`)
   and given the same dispatch prompt, must each approve the trace
   set. The author's own self-review does NOT count toward the two.
   This mirrors the S7 2-of-3 attestation rule applied to traces.
2. **Inverted-trace canary.** 10 % of the bootstrap traces (5 of 50)
   are deliberately broken — `expected` set to a known-wrong tool,
   wrong intent, or banned-substring violation — and shipped in the
   same `evals/<feature>.jsonl` with `tags: ["canary-inverted"]`. A
   working judge MUST fail every canary; if the bootstrap reports
   any canary pass, the trace set is rejected and the judge prompt
   is re-tuned. Canary rows are filtered out of the merge-gate
   denominator at runtime by `internal/eval/harness.go`, so they
   serve as a continuous integrity check rather than depressing the
   reported pass-rate.

Guard (1) raises the collusion cost; guard (2) is mechanically
verifiable, so a broken judge or a captured reviewer pair still trips
the canary.

## 11. Operator override — force-merge despite eval block

The eval gate is required-but-bypassable, not advisory. Bypass path:

### 11.1 Attestation flow

1. Operator runs `leah eval override --pr=152 --reason="<text>"`.
2. CLI invokes the standard attestation gate (existing
   `cmd/leah/connect_*` pattern, scope `eval:override`).
3. On success, writes a row to `~/.leah-state/audit.jsonl`:
   ```json
   {"kind":"eval_override","pr":152,"reason":"...","operator":"tri",
    "ts":"...","blast_radius":2}
   ```
4. CLI POSTs to GH check via `gh api ... /check-runs/<id>` setting
   `conclusion: success` with a body line `LEAH-EVAL-OVERRIDE: <audit_id>`.

### 11.2 Constraints

- BR=2. Justification: override mutates the merge-gate decision for
  one PR, no production-state change, no money spent beyond the
  judge call already accounted in §8. The earlier draft cited
  `connect:regatta:cloud` as a one-level-up reference, but the
  regatta-integration spec does not assign a numeric BR to that
  scope — that comparison was invented and is dropped. Future
  cross-spec BR ordering belongs in a dedicated BR-registry doc, not
  inferred here.
- Override audit row replayable — `leah audit list --kind=eval_override`
  surfaces in `leah whoami` (S10).
- Override retention **exceeds** the default 30-day audit retention.
  `eval_override` rows are the exact rows a future operator needs >30d
  later to detect cumulative override drift (e.g. "have I bypassed the
  gate four times in two months on the same feature?"). Implementation:
  `internal/audit` retention sweeper skips rows with `Kind == "eval_override"`,
  and `internal/eval/history.go` mirrors each override into the
  lifetime `eval-history.jsonl` aggregate on the `gh-pages` branch so
  the row survives even if local audit is rotated.
- Override does NOT silence the next PR's gate; it's per-PR.
- Per-feature -5pp failures require attestation AND a second-reviewer
  attestation row (2-of-3 reviewer rule from S7).

### 11.3 Why attestation, not just a label

A `bypass-eval` GH label is click-through; an attestation prompt forces
the operator to type the reason. Friction = signal: most regressions
either get fixed or get a real explanation. The CLI enforces a
char-count floor on the typed `--reason` (default 32 chars) so a
one-word "fine" cannot serve as a bypass — accreting rationalizations
are mechanically deterred by the cost of typing.

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

### 12.7 Judge-bias eval

LLM-as-judge has three well-documented biases that meta-eval must
guard against — none are caught by §12.2 self-consistency, which only
measures temp=0 determinism.

1. **Position bias.** `TestJudge_PositionSwap`: for every paired
   trace, run the judge twice with `(Actual, Expected)` and
   `(Expected, Actual)` swapped in the prompt. A working judge
   produces identical pass/fail in ≥95 % of pairs.
2. **Length bias.** `TestJudge_LengthControl`: synthetic identical-
   content traces padded to 100 / 500 / 1500 tokens. Pass-rate
   spread across length buckets must stay within ±5 pp.
3. **Self-preference.** Quarterly job (`make eval-cross-judge`) re-
   scores the full 200-trace set with a non-Anthropic judge (GPT-4o
   or local `qwen3:32b`, selected via `LEAH_EVAL_JUDGE_PROVIDER`).
   Overall pass-rate divergence > 10 pp between Sonnet and the
   cross-judge raises an alert and pins the next eval-pipeline PR to
   recalibrate.

### 12.8 Judge-model deprecation playbook

Pinned `JudgeModel` going EOL (Anthropic deprecation notice, vendor
sunset, or non-200 response on the calibration trace) triggers:

1. CI gate `regression-gate` flips to NEUTRAL (does not block merges)
   until re-baselined. Audit row `Kind: "eval_judge_deprecated"`
   written.
2. Operator swaps `JudgeModel` (or sets `LEAH_EVAL_JUDGE_PROVIDER` to
   the fallback) and re-runs the full bootstrap; the cache is busted
   automatically because `judge_model` is part of the cache key (§5.2).
3. Re-baseline window: 1 week. After 7 days with NEUTRAL gate and no
   new baseline, the daemon emits a `leah_eval_judge_stale_total`
   counter increment per HUD render — friction without block.

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
well under the few-hundred-values-per-high-cardinality-dimension bound
in observability.md §4.14.

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

Each wave is one PR, **serialized** (one in flight at a time) sharing
`internal/eval/` ownership — W83 extends `rubric.go`, W85 extends
`judge.go`, so they are NOT file-disjoint and parallelizing them
would re-introduce the stale-base regression that CLAUDE.md spec-PR
serialization exists to prevent. Each wave also depends on the prior
wave's `internal/eval/` landing (the harness must exist before traces
have meaning), reinforcing serial order on its own.

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
- code wave PRs (W82-W85) serialize too — they share `internal/eval/`
  ownership and cannot be file-disjoint (§14).
- no AI signatures.
- no self-approve — every wave PR gets `cavecrew-reviewer-<topic>`.
- worktree discipline; relative paths only.
- root cause > symptom — eval failures investigated to the prompt or
  code edit that caused them; not papered over by retracing.
- TDD: §12 meta-eval lands FIRST in W82, before any trace lands.

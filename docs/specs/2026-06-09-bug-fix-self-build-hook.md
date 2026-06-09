# Bug-fix self-build hook

Status: shipped (rule landed; daemon wiring deferred to follow-up issue)
Date: 2026-06-09
Closes: closed-loop-architecture.md §5 step 3 ("Leah-fixes-Leah" sequence)

## 1. Goal

Close the self-fix loop: panic surfaces in obs metrics → selflearn drafts
a regatta-issue candidate → operator reviews + manually dispatches
`leah self-build`. The loop never auto-dispatches; operator approval is
the only structural defense against self-modification drift.

This spec ships the detection + drafting half. Operator-facing CLI
(`leah self-build`) is a sibling spec.

## 2. Trigger

`PanicRateRule.Detect(ctx, stateDir)` runs from the weekly selflearn
tick. For each `leah_panic_total|name=<n>` counter in
`<stateDir>/metrics/latest.json`:

- delta = current[name] − baseline[name]
- if delta ≥ Threshold (default 3) → emit `Candidate{Name, Delta, Total, PanicFile, PanicStack}`

Baseline persists at `<stateDir>/selflearn/panic-baseline.json` and is
written by the caller (daemon wiring) on each successful Detect. First
run primes the baseline implicitly — Detect returns `(nil, nil)` on
cold-start (missing snapshot).

Threshold rationale: 3 is empirically large enough to filter
single-run noise from a SafeGo target that panics on a transient
network blip, while still firing within a week on a real defect (one
panic per day). Tunable per-rule via `PanicRateRule.Threshold`.

## 3. Output

`Candidate` (in-memory) + markdown candidate file at
`~/.leah-state/bug-fix-candidates.md` (daemon writes; this spec's
rule returns the slice, daemon handles persistence).

Per-candidate `BuildIssueBody(c)` renders the regatta-issue body:

```
# Leah self-noticed panic-rate spike: <name>

- goroutine name: `<name>`
- panics this window: <delta>
- cumulative total: <total>
- most-recent panic file: `<path>`

## Raw panic stack

Leah does not summarize this — investigate the raw bytes.

```
<raw bytes of obs.writePanicFile output>
```
```

The body is intentionally raw-context-only — see §7 Goodhart
mitigation.

## 4. Operator gate (always)

Detect → write candidate file → notify operator (push). NEVER call
`regatta` or `leah self-build` autonomously. The operator reviews the
candidate file + manually pastes the body into a `gh issue create`
flow (or runs `leah self-build` which scripts the same). This is the
only structural defense against self-modification drift; mechanical
enforcement is the absence of any regatta-dispatch code path on the
detect side (verified by `git grep -rn "regatta\|self-build" internal/selflearn/rules/` returning empty).

## 5. Schema

Plain JSON for baseline (compatible with obs snapshot shape):

```json
{ "counters": { "leah_panic_total|name=daemon.tick": 12, ... } }
```

Plain markdown for candidate file (human-readable, diffable, no
DB migration cost). SQLite was considered + rejected — operator
inspection cadence (weekly, manual) makes the markdown overhead
trivial and the SQLite schema premature.

## 6. Build order

1. **Rule** — `internal/selflearn/rules/panic_rate.go` + tests
   (this PR).
2. **Registration** — `selflearn.PanicDetector` marker interface +
   `Resolver.PanicDetectors` slice (this PR, additive).
3. **Daemon wiring** — daemon constructs `PanicRateRule{Threshold: 3}`,
   appends to `Resolver.PanicDetectors`, weekly tick iterates +
   persists candidates + notifies operator. (Tracked: follow-up issue
   pending daemon-side wiring; not in this PR's scope — see
   `internal/selflearn/resolver.go::PanicDetector` doc.)

## 7. Adversarial review

Hunted:

- **Threshold N=3** — too low risks noise from one-off network
  panics; too high delays signal. 3 = ≥1/day for a week; tunable.
  Reopen-trigger: ≥5 false-positive candidates in operator review.
- **Panic-file freshness** — `mostRecentPanicFile` picks lex-max
  filename (timestamp-prefixed by obs). Old panics from prior
  binary versions still surface; mitigation deferred (operator
  notices version mismatch on review). Reopen-trigger: operator
  hits a stale-panic miscorrelation.
- **Auto-dispatch refusal** — documented + structurally enforced
  (no regatta calls in the rule code path).
- **Goodhart** — Leah notices its own panics + drafts spec; the
  regatta agent then implements. The candidate body could
  prompt-inject the regatta agent (e.g. a panic message containing
  `IGNORE PREVIOUS INSTRUCTIONS`). Mitigation: body wraps raw
  bytes in a triple-fence code block + omits any Leah-narrative
  interpretation. Reviewer subagent on the regatta side is the
  second line of defense.
- **Cost** — zero LLM calls; pure file IO + counter math.
- **Cold-start** — first run primes baseline silently; no signal
  until the second weekly tick.
- **Race** — Detect is read-only; baseline write is daemon-side
  + sequential within the weekly tick mutex.

## 8. Cuts

- **No auto-dispatch.** Operator gate only.
- **No LLM-required draft.** V1 ships raw stack only; LLM
  enrichment is a Phase-X follow-up.
- **No cross-package panic attribution.** SafeGo `name` parameter
  is the only granularity. Callers pass package-ish names by
  convention; we don't parse stack frames to attribute.
- **No SQLite.** Plain markdown candidate file; revisit when
  operator-cadence exceeds weekly.
- **No daemon wiring in this PR.** Rule + interface only;
  daemon-side wiring (file write, push notify, baseline update)
  is sequenced to a follow-up to keep this PR file-disjoint with
  the daemon package.

---
title: Leah — self-learning, personal-use Tier 1 subset
status: draft
phase: design
owner: tri
created: 2026-06-09
parent: 2026-06-09-leah-tier1-self-improvement.md
defers_to: 2026-06-09-leah-phase-x-multi-operator-roadmap.md
---

# Self-learning — personal-use slice

Three capabilities. No A/B harness. No fixture freeze. No demoralization
guard. No time-saved estimator. Default simpler.

## 1. Goal

Close the learn-from-experience loop for a single operator at low call
volume. Make Leah's outcomes *visible* and *annotatable* with the
minimum machinery that survives one operator using one Mac:

- **outcome-resolver**: turn `pending` audit rows into
  `success|failed|unknown` by probing the world (no operator labor)
- **mistake-log**: operator records *why* a row failed + how to prevent it
- **weekly-retro**: markdown report (stdout) — wins, mistakes,
  cost-vs-budget, drift-from-stated-prefs

That's the loop. Anything more is Phase X.

## 2. Outcome resolver

Cron-fired walker. Reads last-N-days audit rows (default 7d) where
`Outcome == "pending"` and dispatches each to a per-`Kind` rule.
Rule returns one of `success | failed | unknown` and an optional
`Detail` suffix. Resolver appends a new audit row of kind
`resolver.update` with `Detail` `"resolved <orig-key> -> <new-outcome>"`
— **never mutates existing rows** (JSONL append-only; mutation would
fight the file format and break `dispatcher.Status`).

Row identity: composite `(Timestamp, Kind, ArgsHash)`. Stable across
restarts; no schema change to `audit.Entry`.

### 2.1 Per-Kind rule table (initial)

| Kind                | Probe                                                     | success                 | failed                          | unknown                              |
| ------------------- | --------------------------------------------------------- | ----------------------- | ------------------------------- | ------------------------------------ |
| `ship`              | `gh pr view <N> --json state,mergedAt` (PR# from `Detail` URL) | `state==MERGED`         | `state==CLOSED && mergedAt==nil` | `state==OPEN` AND age <7d → leave pending; ≥7d → `unknown` |
| `ask`               | n/a — `Outcome` set inline by dispatcher                  | (skipped)               | (skipped)                       | (skipped)                            |
| `review`            | `gh pr view <N> --json reviews` — find Leah review at/after `Timestamp` | review posted | API 404 / no review found | network error                   |
| `daemon.transition` | terminal state observation — already final                | (skipped, `observed`)   | (skipped)                       | (skipped)                            |
| `*` (unknown kind)  | log + skip (no `unknown` row written)                     | —                       | —                               | —                                    |

Default: rules live in `internal/selflearn/rules/`; one file per Kind.
`regatta_pr.go` is the first implemented rule; other kinds stub to
`unknown` and gain rules as kinds appear in practice.

### 2.2 Scheduling

Default: **inline in `leah-daemon` tick loop**, gated to one-fire-per-24h
via last-fire timestamp file at `$STATE_DIR/.last-resolver`. Rationale:

- daemon already running; no second cron surface to monitor
- daemon already owns `audit.Logger`; no file-lock dance
- one-per-24h cap keeps gh-api quota cost bounded

Operator override: `leah resolve --since 7d` runs out-of-band.

Cron clash with daemon append: `audit.Logger.Append` opens the file
`O_APPEND`; POSIX guarantees atomic writes for `<PIPE_BUF` (4KiB on
macOS) — entries are ~300 bytes. Two appenders are safe. No mutex
needed.

### 2.3 Cost

Per-Kind probe budget: 1 `gh` call per pending row per resolver fire.
At current volume (≤10 ship/day), 7d × 10 = 70 calls/day worst-case;
gh's authenticated quota is 5000/hr. **3 orders of magnitude under
quota.** No throttle needed.

## 3. Mistake log

### 3.1 Schema

Lives in memory.db (`internal/memory/`). Appended to
`internal/memory/schema.sql`.

```sql
CREATE TABLE IF NOT EXISTS mistake_log (
  id          TEXT PRIMARY KEY,    -- ulid
  created_at  TIMESTAMP NOT NULL,  -- RFC3339 UTC
  audit_ts    TEXT NOT NULL,       -- audit.Entry.Timestamp (composite key part)
  audit_kind  TEXT NOT NULL,       -- audit.Entry.Kind         (composite key part)
  audit_hash  TEXT NOT NULL,       -- audit.Entry.ArgsHash     (composite key part)
  root_cause  TEXT NOT NULL,       -- short tag, e.g. "wrong-pr", "bad-prompt"
  prevention  TEXT NOT NULL        -- free-form operator note
);
CREATE INDEX IF NOT EXISTS mistake_log_created ON mistake_log(created_at);
```

No `workspace_id` (single workspace per phase-x deferrals).
No `decision_id` (decisions table is Phase X).

### 3.2 CLI

```
leah mistake add --audit-ts <RFC3339> --audit-kind <kind> --audit-hash <hash> \
                 --root-cause <tag> --prevention "<text>"
leah mistake list [--week YYYY-WW]
```

Convenience: `--audit-ts/kind/hash` may be replaced with a single
`--audit-id <ts>:<kind>:<hash>` token to match the form `leah status`
prints.

## 4. Weekly retro

```
leah retro [--week YYYY-WW]   # default = current ISO week
```

Markdown report to stdout (operator pipes to a file or scrolls).
No HTML, no PDF, no scheduled email.

### 4.1 Report skeleton

```markdown
# Leah retro — week 2026-W23

## Summary
- actions taken: <N>
- success: <N> / failed: <N> / pending: <N> / unknown: <N>
- cost: $<X.XX> of $<budget> ceiling
- top action_kind: <kind> (<N> calls)

## Wins (top 5 success)
- 2026-06-03 ship  https://github.com/.../pull/123  $0.18
- ...

## Mistakes (top 5 by root_cause frequency)
- wrong-pr ×3   prevention: "double-check the repo arg"
- ...

## Cost vs budget
| day | spent  | budget | %    |
| --- | ------ | ------ | ---- |
| Mon | $1.20  | $5.00  | 24%  |
| ... |        |        |      |

## Drift from stated prefs
(stub — Phase X surfaces operator-prefs table; for v1, print "n/a")
```

### 4.2 Implementation

- read audit.jsonl + apply same parser as `dispatcher.Status`
- read mistake_log table
- bucket by ISO week (`time.Time.ISOWeek()`)
- render via `text/template`

No fatigue guard. If operator skips week 7, week 8's report still
runs the same query.

## 5. Build order (4 tasks max)

1. **Resolver core** — `internal/selflearn/resolver.go` + per-Kind rules
   interface + `regatta_pr.go` implementation + TestResolverUpdatesPendingRow.
2. **Mistake log** — `internal/selflearn/mistake.go` + schema +
   `leah mistake add` CLI + TestMistakeAddRoundtrip.
3. **Retro report** — `internal/selflearn/retro.go` + `leah retro`
   CLI + TestRetroIncludesMistakes.
4. **Daemon wiring** — `internal/daemonloop/loop.go` calls
   `selflearn.Resolver.Run` once per 24h. Integration test gated by
   `-tags integration`.

Each task is one PR. File-disjoint.

## 6. Cuts (Phase X)

All cited in `2026-06-09-leah-phase-x-multi-operator-roadmap.md` §"Tier 1 self-improvement":

- **A/B harness + SPRT** — call rate too low; eyeball suffices.
- **Frozen-fixture mechanical immutability + CODEOWNERS gate** — solo
  operator; convention beats gate.
- **Sunday demoralization-prevention** — operator just won't read the
  bad section when tired. No code needed for that decision.
- **Time-saved estimator** — Goodhart-bait; subjective weekly ask.
- **Reviewer-prompts separate approval queue** — same Sunday review
  covers both.
- **Workspace-tagged rows** — single default workspace; no column.
- **Decisions table + decision_id audit link** — Phase X.
- **Prompt mutation pipeline** — Phase X.

---

## Adversarial review

Hunting circularity, selection bias, fatigue, serialization, cost.
Severity-tagged.

### CRITICAL — none

### HIGH — outcome-inference circularity (partial mitigation, accept residual)

**Finding.** Resolver grades Leah's own audit rows using probes Leah
herself wrote (the rule code). If the rule is wrong (e.g. `ship` rule
treats `state==CLOSED && mergedAt==nil` as `failed`, but operator
actually `gh pr close`'d it because they reconsidered the intent),
Leah self-promotes a false `failed` row, which then leaks into the
retro report and biases the operator's view of Leah.

**Mitigation in spec.** Resolver writes `resolver.update` row but
**never mutates the original**. The mistake_log lets the operator
override: `leah mistake add --root-cause "resolver-misjudged" …`.
Retro report MUST surface `resolver.update` rows with attribution so
the operator can spot-check. §2 mandates appended row instead of
in-place mutation; §4.1 wins/mistakes sections MUST display the
resolving rule name in parentheses.

**Accepted residual.** Operator-grades-Leah's-self-grading is honor
system at single-operator scale. A second independent grader is Phase X.

### HIGH — mistake-log selection bias (accept; document)

**Finding.** Operator annotates the obvious failures (`ship` PR
closed-without-merge). Silent failures (Leah drafted a worse email
than the operator would have, but it still sent) are invisible to
the log. Retro will overweight `ship`-style failures and miss `ask`
quality drift entirely.

**Mitigation.** Spec adds §4.1 "Drift from stated prefs" stub — when
prefs table lands (Phase X memory follow-up), Leah self-flags rows
that deviated from operator-stated preferences (e.g. tone) and surfaces
them in the retro under a separate header so the operator is prompted
to annotate. **For v1: documented gap; no code.** Operator should
treat retro as "lower bound on mistakes," not "full picture."

### MED — retro fatigue (accept)

**Finding.** Operator skips after week 3. Retro becomes vestigial.

**Why not fix.** Spec already cuts the Sunday-review demoralization
guard per phase-x. Adding "you skipped 3 weeks" nag would re-introduce
the same anti-pattern. If operator skips, retro file just keeps
working when they come back. No metric to chase. No streak.

### MED — daemon ↔ resolver serialization

**Finding.** Resolver runs *inside* daemon tick (§2.2). Daemon ticks
every PollEvery (default 60s); resolver gated to 24h. If resolver
takes >60s (e.g. 70 `gh pr view` calls × 500ms latency = 35s — OK)
the next tick will see resolver still running and skip its
notify-transition work.

**Mitigation.** Resolver runs in a goroutine launched from tick with
a `sync.Mutex` guard on the singleton `Resolver`. Tick that finds
mutex held just continues with notify-transition work. §2.2 mandates:
"Resolver MUST be invoked via `go r.Run(ctx)` from tick with a
sync.Mutex held inside r.Run; tick itself never blocks on resolver."

### MED — cost burst on first run

**Finding.** First resolver fire after 30d of dormant audit could
walk hundreds of pending rows × 1 gh call each = quota concern if
operator ran a regatta cleanup batch.

**Mitigation.** Spec §2 — resolver walks **last 7d only** by default.
Older pending rows are left alone (operator can opt in via
`leah resolve --since 30d` if they care). 7d × 10/day worst case
= 70 calls; 1 order of magnitude under per-hour quota.

### LOW — `unknown` outcome ambiguity

**Finding.** Resolver writes `unknown` when the probe can't tell.
Without explanation, operator reading retro sees `unknown: 12` and
shrugs. Useless signal.

**Mitigation.** `resolver.update` row's `Detail` MUST cite the probe
result that drove the verdict (e.g. `"state=OPEN age=8d -> unknown"`).
Retro `unknown` bucket prints the top-3 reasons (§2 + §4.1).

### LOW — composite key fragility

**Finding.** `(Timestamp, Kind, ArgsHash)` could collide if two
identical actions fire in the same RFC3339 second. RFC3339 has no
sub-second resolution as written today.

**Mitigation.** Probability negligible at personal-use volume (1 op
running 1 daemon). When/if it happens, resolver writes one
`resolver.update` covering both; operator notices in retro. Not
worth pre-fixing. If it ever recurs, switch `audit.Logger` to
RFC3339Nano (1-line additive change).

### Verdict

APPROVE with the inline §2 / §2.2 / §4.1 mitigations captured above.
Accepted-residual items are explicit personal-use tradeoffs, not
oversights.

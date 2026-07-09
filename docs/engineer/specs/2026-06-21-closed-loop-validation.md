---
title: Closed-loop validation harness (M2 exit)
status: proposed
owner: tri
created: 2026-06-21
---

# Closed-loop validation harness

## 1. Goal

Prove — mechanically, with a single command — that the 9-step self-build loop
fires end-to-end for one real feature, and surface a pass/fail receipt. The
substrate is all shipped; what is missing is a way to *assert the loop closed*
rather than eyeballing the audit log. Without this, "closed loop runnable"
(M2 exit) is a claim, not a verified state.

Outcome: `leah self-build-status [--args-hash <h>]` reads the audit log and
reports, per dispatched self-build, which of the loop's checkpoints have fired
and which are pending — a deterministic receipt the operator (and the audit-
session skill) can read to declare M2 closed.

## 2. Producer it depends on (verified present on main @ 7621dbb)

All checkpoint emitters already exist — this wave is a **consumer**, which is
exactly why it is the safe first wave (no missing-producer risk):

- `internal/audit/audit.go` — `Entry{Timestamp, Kind, ArgsHash, ...}`,
  append-only JSONL. **Present.**
- `internal/dispatcher/selfbuild.go` — emits `kind=self-build` (dispatch),
  `kind=self-build.outcome` (terminal state), paired by `ArgsHash`; `ship`
  row via `Ship.Run`. **Present** (`appendAuditOutcome`, line 288).
- `internal/daemonloop/loop.go:411` — emits `kind=daemon.transition` with
  from/to/pr attrs on regatta state change. **Present.**
- `internal/regattaclient` — PR/agent state read. **Present.**

No new producer. No `go.mod` edit.

## 3. Interface surface

A pure read+classify package plus a thin CLI:

```go
// internal/selfbuild_status/status.go (new package, consumer-only)

// Checkpoint is one observable step of the closed loop.
type Checkpoint struct {
    Name string // "dispatched","shipped","pr_opened","merged","outcome"
    Done bool
    TS   string // RFC3339 of the audit row that satisfied it, "" if pending
}

// Loop is the per-self-build receipt keyed by the dispatch ArgsHash.
type Loop struct {
    ArgsHash    string
    Checkpoints []Checkpoint
    Closed      bool // every checkpoint Done
}

// Classify folds the audit entries for one ArgsHash into a Loop receipt.
// Pure function over already-read entries — no IO, fully table-testable.
func Classify(entries []audit.Entry) []Loop
```

Checkpoint → audit-kind mapping (derived from verified emitters):

| Checkpoint | Audit kind                         |
|------------|------------------------------------|
| dispatched | `self-build`                       |
| shipped    | `ship` (same ArgsHash window)      |
| pr_opened  | `self-build.outcome` (issueURL set)|
| merged     | `daemon.transition` to=merged      |
| outcome    | `self-build.outcome` terminal      |

CLI `cmd/leah/self_build_status.go`: read the audit log, call `Classify`,
print one line per loop — `<args-hash>  [x dispatched][x shipped][ pr][ merged]
CLOSED|PENDING`. Exit 0 when at least one loop is `Closed` (so CI / audit-
session can gate on it); non-zero only on read error, not on "pending"
(pending is a normal state, not a failure).

## 4. Test plan (TDD — failing test first)

- `Classify` over a synthetic audit slice with all 5 kinds for one ArgsHash
  returns one `Loop` with `Closed=true`.
- Partial slice (dispatch + ship only) returns `Closed=false` with the right
  two checkpoints Done and the rest pending — asserts the receipt is honest
  about an in-flight loop.
- Two distinct ArgsHashes return two independent `Loop`s (no cross-talk).
- A `daemon.transition` with `to != merged` does NOT satisfy the merged
  checkpoint (guards against counting any transition as a merge).
- CLI exit code: 0 when a closed loop exists, non-zero on a malformed audit
  file, 0-with-PENDING when only in-flight loops exist.

## 5. Risk

- **ArgsHash correlation** — `ship` and `self-build` rows must share the
  ArgsHash window for "shipped" to bind to the right dispatch.
  `selfbuild.go` already comments the BR=3 ship / BR=4 self-build pairing;
  validate the hashes actually match in an integration test against a real
  dispatch before declaring the mapping correct (do not assume).
- **merged detection** — relies on `daemon.transition` carrying a PR ref that
  maps back to the self-build's issue. If that linkage is weak, "merged" may be
  under-counted; the receipt errs toward PENDING (safe — never falsely CLOSED).
- **No live dispatch in CI** — `Classify` is pure and tested in isolation; the
  end-to-end live run is an operator-run integration check, not a CI gate (CI
  has no regatta/GitHub credentials). Document the manual run-once procedure.

## 6. Out of scope

- Triggering the loop — this is read-only validation, not a self-build runner.
- Auto-merge — operator-merge stays mandatory (README.md / self-build PRs
  never auto-merge).
- Surfacing the receipt in the HUD/dashboard — a follow-up; this wave ships the
  CLI + pure classifier only.
- Grading outcome quality (good/bad self-build) — that is selflearn's job; this
  wave only asserts the loop *closed*, not that the feature was *good*.

# Closed-loop live validation — 2026-06-09

**Operator**: tri (`trilamsr` on GitHub)
**Target repo**: `trilamsr/Leah` (public)
**Leah HEAD at validation**: `7e130c04f76f51cf7ff19c6602388fdd5235328f` (main, dirty: cmd/leah/brief.go modifications unrelated to this run)
**Architecture under test**: `docs/specs/2026-06-09-closed-loop-architecture.md`
**Validation kind**: live end-to-end; no mocks; real GitHub issue, real Anthropic API spend.

---

## Verdict

**Status: substrate-validated — regatta orchestrator not running, so the auto-pickup → PR → review hop is deferred.**

Steps 1-4 of the closed-loop spec (operator types `leah self-build` → Reasoner drafts spec → `gh issue create` with correct shape) executed successfully and are reproducible. Step 5 (regatta picks up issue → opens PR) and step 6 (`leah review`) require a running regatta instance targeting `trilamsr/Leah`; the only regatta container on this host (`regatta:stage2`) was last configured against the regatta self-host repo and is currently `Exited (0)`.

Two real defects were uncovered during the run and are documented as backlog items below.

---

## Pre-checks

| Check | Command | Result |
|---|---|---|
| Build | `go build -o leah ./cmd/leah` | ok (21 MB binary) |
| Version | `./leah version` | `0.0.1-mvp5` |
| API live | `./leah ask "say hi in 3 words"` | success, charged $0.000897 |
| gh auth | `gh auth status` | logged in as `trilamsr` |
| Docker stack | `docker ps` | grafana, prometheus, alertmanager up; `regatta` container exited |
| Regatta CLI | `which regatta` | not in `$PATH` |

---

## Regatta availability

- `regatta:stage2` container last run 2 h before this validation; logs show it was polling `regatta/agent-NN` branches on the regatta self-host repo, not `trilamsr/Leah`.
- No `regatta` binary on `$PATH`; the dispatcher watcher emits `dispatcher.watch.regatta_list_error err="regatta agents list: regatta: exec: \"regatta\": executable file not found in $PATH"` every 30 s.
- Configuring + restarting regatta against `trilamsr/Leah` is out of scope for this validation slot (would need to compose a target-repo config, mount creds, and let it run unattended past the validation window). Deferred as a follow-up live test.

---

## Self-build dispatch — what happened

### Run 1: argument-parsing trap

```
./leah self-build --help
```

`--help` was treated as the intent string. Reasoner correctly emitted a clarifying-questions block; audit row written as:

```
2026-06-09T23:00:24Z self-build BR=4 clarify cost=$0.0059 detail=prompt_sha=7d63e6442921f859
```

This is correct behavior under the current Reasoner contract, but is also a usability papercut — see Defect 2.

### Run 2: first real dispatch — label missing

```
./leah self-build "add a --version flag to leah CLI that prints the version (same as 'leah version' subcommand). Should work as both 'leah --version' and 'leah -v'."
```

Reasoner drafted the spec (29 ms, $0.029), dispatcher wrote the issue template to a temp file, then `gh issue create` failed:

```
gh: could not add label: 'ready-for-agent' not found
```

Audit row:

```
2026-06-09T23:01:19Z ship BR=3 failed cost=$0.0290 detail=gh issue create: ... 'ready-for-agent' not found
```

This is Defect 1 below — the dispatcher assumes the label exists on the target repo but never creates it.

### Run 3: label added, retry, issue created

After running `gh label create ready-for-agent --repo trilamsr/Leah --color 0E8A16`, the same dispatch succeeded:

```
2026-06-09T23:02:05Z ship BR=3 pending cost=$0.0261 detail=https://github.com/trilamsr/Leah/issues/1
```

### Issue #1 inspection

`gh issue view 1 --repo trilamsr/Leah --json title,labels,body,createdAt` confirms:

- Title: `[SELF-BUILD] add a --version flag to leah CLI that prints...` (correct `[SELF-BUILD]` prefix; 60-char truncation)
- Labels: `ready-for-agent` (only)
- Body contains the regatta-issue template sections: `## Title`, `## Motivation`, `## Files to create or modify`, `## Code shape`, `## Acceptance criteria`, `## Test plan`, `## Deferred`, `## Self-build context`
- Correlation comment present: `<!-- leah-dispatched: 01KTQ9Y7QRVN8QB3BN1CJEV9RM -->`
- Intent-source comment present: `<!-- intent-source: terminal -->`
- Attestation footer present:
  > Before merging this PR, operator (`tri-lamsr`) must answer the following question in a PR comment whose first line starts with `Attestation:`.
  >
  > If this PR were reverted in 30 days, which symptom would tell you it should be reverted?

All issue-shape contracts satisfied (correlation comment, attestation footer, label, title prefix, template body — as emitted by `internal/dispatcher/ship.go` + `prompts/self-build-feature.md`; the architecture doc covers these at a narrative level in §"The closed loop").

Issue closed at end of run with cleanup comment (regatta won't pick it up).

---

## Audit chain verification

```
2026-06-09T22:59:50Z ask        BR=0 success      cost=$0.0009  (pre-check smoke)
2026-06-09T23:00:24Z self-build BR=4 clarify      cost=$0.0059  (--help mis-parse)
2026-06-09T23:01:19Z ship       BR=3 failed       cost=$0.0290  (label missing)
2026-06-09T23:02:05Z ship       BR=3 pending      cost=$0.0261  (issue #1 created)
```

**Missing row (Defect 3)**: `internal/dispatcher/selfbuild.go:233-260` (`appendAuditSuccess`) is contractually meant to write a `kind=self-build outcome=dispatched BR=4` row after a successful ship, paired with the BR=3 `ship` row (per the in-source comment: "Ship writes a BR=3 'ship' audit row; we follow up with the BR=4 'self-build' row so retro queries (WHERE kind = 'self-build') count exactly the self-builds"). But the call is gated behind `inner.Run(ctx, intent)`, which blocks indefinitely in the regatta watcher polling loop. Operator-initiated SIGTERM (Ctrl-C; this run used `kill 62535`) kills the process before the deferred audit row is appended. Result: a successful self-build dispatch leaves an orphan BR=3 `ship` row and no `self-build` row, defeating the in-source rationale for the second row.

---

## End-to-end timing

| Step | Duration | Wall-clock |
|---|---|---|
| Reasoner draft (BR=4) | 31 s | 23:00:48 → 23:01:19 |
| Ship: template render + gh issue create (Run 3) | <1 s | 23:02:05 |
| Total: intent → issue live on GitHub | ~35 s | — |
| Regatta pickup → PR open | N/A | regatta not running |
| `leah review` verdict | N/A | no PR to review |

---

## Cost

```
leah cost --since 1d
  total   $0.0634
  rows    11
  ship          $0.0551
  self-build    $0.0059
  ask           $0.0023
```

No explicit cost envelope in the architecture spec to compare against; for reference, a clean single-shot loop (skipping the `--help` mis-parse and the failed-label retry) would cost roughly $0.035 (one BR=4 spec draft + one BR=3 ship template render).

---

## Defects found

### Defect 1 — `ready-for-agent` label not auto-created on target repo

- **Severity**: HIGH (blocks first-ever self-build on any new target repo)
- **Surface**: `internal/dispatcher/ship.go` (`gh issue create --label ready-for-agent`)
- **Repro**: any `leah self-build` against a target repo whose label set lacks `ready-for-agent`.
- **Symptom**: `gh issue create: gh: could not add label: 'ready-for-agent' not found` → `ship` row outcome `failed` → $0.029 spent on Reasoner draft, no issue filed.
- **Fix sketch**: dispatcher SHOULD attempt `gh label create ready-for-agent --color 0E8A16 --description "..."` and ignore "already exists" — once per dispatcher boot, cached.
- **Backlog**: file as `trilamsr/Leah` issue once the regatta loop is live, OR as a leah repo issue if leah is the source of truth.

### Defect 2 — `self-build --help` parsed as intent

- **Severity**: LOW (papercut; operator quickly learns `--help` is not supported)
- **Surface**: `cmd/leah/main.go::main` switch on `os.Args[1]`
- **Symptom**: `./leah self-build --help` spends $0.0059 + Reasoner cycle to emit clarifying questions instead of printing flag usage.
- **Fix sketch**: detect `os.Args[2]` ∈ {`-h`, `--help`} before invoking dispatcher; print a one-line usage. (Same pattern probably worth applying to `ask`, `ship`, `review`.)

### Defect 3 — `self-build dispatched` audit row not written when operator aborts watcher

- **Severity**: HIGH (breaks retro counting; in-source comment at `internal/dispatcher/selfbuild.go:138-141` explicitly justifies the row as the canonical signal for "WHERE kind = 'self-build'" retro queries)
- **Surface**: `internal/dispatcher/selfbuild.go:142` — `appendAuditSuccess` runs after `inner.Run(ctx, intent)`, which blocks on the regatta watcher loop.
- **Repro**: operator runs `leah self-build "..."` against any target without a running regatta instance, waits for issue-created log line, Ctrl-Cs.
- **Symptom**: ship row written (`outcome=pending`, `detail=<url>`); self-build row NEVER written.
- **Fix sketch**: emit the `self-build dispatched` row as soon as `inner.LastURL != ""` (i.e. immediately after `gh issue create` returns success), BEFORE entering the watch loop. The watch loop already has its own retry/notify path; the audit row should not depend on its return.

---

## Closed-loop status

**Status: substrate-validated-but-regatta-not-running.**

- ✅ Operator → Leah Reasoner (BR=4 spec draft): proven working live.
- ✅ Leah Reasoner → GitHub issue with correct shape (`[SELF-BUILD]` prefix, template body, `ready-for-agent` label, correlation comment, attestation footer): proven working live.
- ✅ Audit row for `ship` BR=3 with issue URL: proven working live.
- ❌ Audit row for `self-build` BR=4 `dispatched`: never appended on this run (Defect 3).
- 🚧 Regatta orchestrator → PR: deferred (no regatta instance targeting `trilamsr/Leah`).
- 🚧 `leah review trilamsr/Leah <pr#>`: deferred (no PR exists).
- 🚧 Operator merge + retro: deferred.

---

## Backlog items

1. **leah/ship**: auto-create `ready-for-agent` label on target repo if missing (Defect 1).
2. **leah/cmd**: intercept `--help` / `-h` on subcommand args before dispatching to Reasoner (Defect 2).
3. **leah/selfbuild**: emit `kind=self-build outcome=dispatched` audit row immediately after `gh issue create` success, not after watcher loop returns (Defect 3).
4. **leah/ops**: document how to stand up regatta against `trilamsr/Leah` so the auto-pickup hop can be validated.
5. **leah/regression**: re-run this validation after Defect 3 fix to confirm the audit row appears.

---

## Reproducibility

```bash
cd /Users/treedesk/Desktop/Projects/leah
set -a && source .env && set +a
go build -o leah ./cmd/leah
gh label create ready-for-agent --repo trilamsr/Leah --color 0E8A16 \
  --description "Issue ready for autonomous agent (regatta) pickup" || true
./leah self-build "add a --version flag to leah CLI that prints the version (same as 'leah version' subcommand)."
# In another shell, after seeing dispatcher.issue.created log line:
gh issue view 1 --repo trilamsr/Leah --json title,labels,body
./leah status --json
./leah cost --since 1d
```

Issue URL from this run: <https://github.com/trilamsr/Leah/issues/1> (closed).

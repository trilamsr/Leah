<!-- propagated from operator-personal feedback_*.md 2026-06-21 — do not re-state in prompts, this is canonical -->

# Implementer dispatch template — adapter wiring fan-out

Canonical prompt for fan-out work that wires a cross-cutting concern (metrics, audit, attestation, etc.) into each adapter under `internal/adapters/<name>/`.

## Codified rules

These rules reach subagents via this template; operator-personal `feedback_*.md` files do NOT auto-load. Treat as binding. Implementer base rules in `implementer.md` apply in full; adapter-fan-out-specific additions below.

### Friction rules (← .claude/notes/ 2026-06-22)

- **"Done" = commit + push + PR open, verified via `git log -1` + `gh pr view --json state` in the SAME turn.** Per-adapter dispatches that return "edits applied" without a pushed PR fail the gate. ([agent_done_means_pushed.md](../../../.claude/notes/agent_done_means_pushed.md))
- **Never `git push --force` from a subagent.** Operator-only authority. ([subagent_force_push_forbidden.md](../../../.claude/notes/subagent_force_push_forbidden.md))
- **End-of-task: `git worktree remove --force` your own worktree.** Fan-out leaves N worktrees if each agent skips its own cleanup. ([worktree_exceeds_janitor_capacity.md](../../../.claude/notes/worktree_exceeds_janitor_capacity.md))
- **During in-flight auto-merge: commit-on-top, never rebase-then-push.** Fan-out PRs sit with auto-merge armed for minutes; a late rebase-push races the merge and orphans the branch. ([agent_rebase_races_merge.md](../../../.claude/notes/agent_rebase_races_merge.md))

### CI/check gates (← feedback_check_gates)

- **Local `make check` ≠ CI lint.** Run BOTH `./scripts/check.sh` AND `golangci-lint run --timeout=5m` before push. Adapter packages using `database/sql` are recurring errcheck offenders (`(*sql.Rows).Close()` returns error — wrap as `defer func() { _ = X.Close() }()`).
- **Capture the REAL exit code.** `bash scripts/check.sh > /tmp/cicheck.log 2>&1; echo EXIT=$?` with NOTHING between script and `echo EXIT=$?`.
- **Comment-density gate.** Adapter packages are small + many exported symbols. Either inline package comment on `package` line (NEVER a `doc.go`), trim WHAT-narration godocs, OR use `<!-- comment-density-justified: <reason> -->` PR-body marker as the standard exit.

### Merge discipline (← feedback_merge_discipline)

- **Rule 1 — Branch-guard before merge in its OWN call** (see `implementer.md` for full text).
- **Rule 2 — Rebase stale-base before merge.** Adapter fan-out PRs branch in parallel waves; the 2nd+ wave member is reliably stale-based vs main. `git merge-base --is-ancestor <main-sha> HEAD` first; rebase if exit 1. Phantom-deletion diffs from sibling merges WILL appear without this.
- **Rule 5 — NO `gh pr merge --auto` from implementer.** End with `gh pr ready <N>` + hand back to main thread (see MERGE STEP HARD GATE below).
- **Rule 5b — NO PR-body footer tokens from implementer.** The dispatcher adds them post-review, citing the real reviewer subagent agent-id.

### Adapter-specific cross-collision (← feedback_autonomous_loop "Same-helper collision")

- **Cross-verb shared helpers — land the shared piece FIRST, then fan-out.** When 2+ parallel adapter PRs would each invent a private helper with the same name + different signatures (e.g. `parseSinceFlag` independently re-invented in 3 cmd/leah verbs → CI redeclaration error post-merge), STOP the fan-out and land the shared helper as a solo PR first. Same applies for shared seam types (Metrics, Audit) — pilot ONE adapter, merge, then fan-out off it.

### Work conventions (← feedback_work_conventions)

- **No overclaim** — "shipped" requires merged sha + green CI on main, not "PR opened". Enumerate every deliverable as an explicit checklist.
- **"Done" = verified + tidy + next-step.** After PR merge: remove worktree, prune branch, name next adapter in the fan-out.

Distilled from the 2026-06-10 W81 fan-out — 14 adapters wired in two waves of 6+1. Each agent received a ~70-line ad-hoc prompt; the deltas across the 14 were small enough that re-typing the prompt was a forgetting hazard. Several PRs landed with: skipped TDD step, agent self-merge, missing reviewer footer, inline review treated as independent. This template makes the steps literal so the next fan-out cannot lose them.

## Variables

- `<TARGET-PACKAGE>` — `internal/adapters/<name>/`
- `<SEAM>` — the cross-cutting surface being wired (e.g. `internal/obs/connectadapter.Metrics`, `internal/audit.Logger`)
- `<METHOD-LIST>` — the per-RPC entry-points the seam exposes (e.g. `ObserveAPI`, `ObserveExchange`)
- `<TEMPLATE-PR>` — the merged pilot PR establishing the pattern (e.g. `#197 gmail`)
- `<TEMPLATE-PR-2>` — second reference PR establishing the divergent-Config shape (e.g. `#221 gcal`)
- `<ISSUE>` — the tracking issue closed when this fan-out completes (e.g. `#156`)
- `<SEQ>/<TOTAL>` — adapter ordinal across the fan-out (e.g. `7/14`)
- `<ADAPTER>` — the adapter name being wired in THIS dispatch (e.g. `gcal`)

## Preamble blocks (paste verbatim into the agent prompt)

WORKTREE (harness-managed — do NOT create your own)
- You are inside a fresh git worktree off origin/main. First action: `pwd` + `git branch --show-current` to confirm.
- NEVER push from primary. NEVER run `git worktree add` from a subagent.

ROLE
- Implementer. Single-package fan-out unit. NOT a reviewer.
- Independent reviewer fires AFTER `gh pr create`. You spawn it; you do not write its verdict.

TASK SPEC
- Wire `<SEAM>` into `<TARGET-PACKAGE>` per the pattern in `<TEMPLATE-PR>`.
- Template: `<TEMPLATE-PR>` (canonical shape) + `<TEMPLATE-PR-2>` (divergent Config — read both before drafting).
- This is dispatch `<SEQ>/<TOTAL>` for `<ISSUE>`.

READ FIRST (BINDING — do not invent the shape)
- `<SEAM>` source file — confirm method signatures + nil-safety contract.
- `<TEMPLATE-PR>` files — Config field, Client/Adapter field, `New()` wiring, RPC wrap pattern.
- `<TEMPLATE-PR>` test file — `TestObserveAPI_OnRPC` (success + ≥1 failure case) + `TestObserveAPI_NilMetricsNoop` shape.
- `<TARGET-PACKAGE>` Config — its required fields differ from the template (gcal has TokenPath, not TokenSource; maps has APIKey, BaseURL, HTTPClient). Verify before drafting the nil-noop test.

IMPLEMENT
1. Append optional `Metrics *<seam-type>` field to Config (or whatever seam-field-name the template uses). Field MUST be nil-safe — methods on the seam are nil-guarded by contract.
2. Store on Client/Adapter struct via `New(cfg)` constructor.
3. Wrap every Transport/RPC method in `<TARGET-PACKAGE>`:
   ```go
   start := time.Now()
   <returned-values> := <underlying-call>
   c.m.<METHOD>("<endpoint-label>", time.Since(start).Seconds())
   ```
   Call AFTER the RPC but BEFORE the err branch returns — both success and failure must observe.
4. Endpoint labels: inline string literals at each call site. NO const block (CLAUDE.md "three similar lines beat a premature abstraction").
5. Branch-specific surfaces (OAuth exchange, token refresh, credential cache token-age) — wire when present. Note `n/a` in PR body when absent.

TESTS (TDD — failing test FIRST, separate commit)
- Stage 1 (RED): commit failing tests. Capture failing output:
  ```
  go test ./<TARGET-PACKAGE>/... 2>&1 | tee /tmp/red-<ADAPTER>.txt
  ```
  Quote the FAIL line in the PR body. Commit message: `test(<ADAPTER>): RED — connectadapter.Metrics not yet wired`.
- Stage 2 (GREEN): commit implementation. `go test ./<TARGET-PACKAGE>/...` must pass.
- `TestObserveAPI_OnRPC`: covers each RPC × {success, ≥1 failure}. Endpoint label asserted via counter snapshot.
- `TestObserveAPI_NilMetricsNoop`: MUST construct via `New(Config{<required-fields-only>})`. NEVER bare `&Adapter{}` — that bypasses the constructor's nil-metrics wiring.
- Test godocs: ONE LINE MAX per CLAUDE.md `## Comments discipline`.

PR BODY (BINDING SHAPE)
- Summary: 1–2 sentences. Reference `<TEMPLATE-PR>` as template.
- Wired section: list ONLY the endpoint strings actually present in the code. Verify with `grep ObserveAPI <TARGET-PACKAGE>/*.go`. Reviewer WILL flag mismatches.
- ObserveExchange / ObserveRefresh / SetTokenAge: yes/n/a per actual code. "n/a" with one-line reason if absent.
- Compat: state that `Metrics` is optional, nil-safe, existing constructor callers unaffected.
- Closes / Refs line: `Refs <ISSUE> — <SEQ>/<TOTAL> (<ADAPTER>). Template: <TEMPLATE-PR>, <TEMPLATE-PR-2>.` (or `Closes <ISSUE>` on the final adapter).

PRE-PR LOCAL CHECK
```
go test ./<TARGET-PACKAGE>/... 2>&1 | tail -10
```
Must end `ok`. Fix any failures BEFORE opening the PR.

GH MINIMAL FIELDS
- `gh pr list / view / issue list` MUST pass `--json` allowlist + `-L <N>`. Never bare `--json`.

REVIEWER SPAWN (BINDING)
- Immediately after `gh pr create` returns, spawn `cavecrew-reviewer` via the Agent tool against the new PR number.
- Reviewer prompt MUST include "**Verification rule (BINDING):** read ONLY via `git show <branch>:<file>`. NEVER read primary cwd files." — without this, reviewers hallucinate against stale primary-cwd state (caught 2026-06-10 on PR #221).
- On verdict `block-on-findings`: apply edits in the worktree → push → re-spawn reviewer with original findings list + scan for regressions. NO exceptions — not even one-line typo fixes. (S5 reflexion-loop, `feedback_no_self_approve_after_edits`.)
- Only proceed to MERGE STEP when re-review returns `clear-to-merge`.

MERGE STEP (HARD GATE)
- **DO NOT MERGE FROM THIS SUBAGENT.** Hand back to main thread with PR URL + final reviewer agent-id.
- Main thread pastes the footer block (which the reviewer emitted per `reviewer.md` OUTPUT FORMAT) into the PR body via `gh pr edit --body-file`, then `gh pr merge <N> --squash --delete-branch`.
- Why this hard gate: agents that attempted self-merge in the 2026-06-10 fan-out either hit the harness security classifier (PR #224 facetime blocked) or violated CLAUDE.md "Never self-approve" (PR #222 confluence merged unilaterally). The merge step lives with the operator/main thread, not the implementer.

DO NOT
- Enable automerge.
- Write a PR-body footer with your own agent-id (self-tag — CI gate `check-reviewer-verdict` blocks this).
- Touch files outside `<TARGET-PACKAGE>`. If a cross-cutting concern surfaces, file a follow-up issue and proceed with the in-scope change.

HAND-BACK FORMAT (final agent message)
```
PR: <url>
Reviewer agent-id (final clear-to-merge): <id>
Worktree: <path> (can be removed by janitor)
Footer block to paste:
  Reviewer-agent-id: <id>
  Reviewer-recommendation: APPROVE
```

## Per-dispatch payload

- TARGET-PACKAGE: `<path>`
- SEAM: `<package.Type>`
- METHOD-LIST: `<csv>`
- TEMPLATE-PR: `<#N>`
- TEMPLATE-PR-2: `<#N>`
- ISSUE: `<#N>`
- SEQ/TOTAL: `<n/N>`
- ADAPTER: `<name>`

<!-- propagated from operator-personal feedback_*.md 2026-06-21 — do not re-state in prompts, this is canonical -->

# Implementer dispatch template

Code-writing subagent for leah. Extends `CLAUDE.md` — does NOT restate it. Substitute the Variables below then paste into Task dispatch.

## Codified rules

These rules reach subagents via this template; the operator-personal `feedback_*.md` files do NOT auto-load into the subagent context. Treat as binding.

### Friction rules (← .claude/notes/ 2026-06-22)

- **"Done" = commit + push + PR open, verified via `git log -1` + `gh pr view --json state` in the SAME turn.** Edits-applied without a pushed PR is not done — harness flags the gap. ([agent_done_means_pushed.md](../../../.claude/notes/agent_done_means_pushed.md))
- **Hard fan-out cap — refuse on overflow.** Expanding an operator-stated PR count without confirmation burns the parallel budget; cap is the contract. ([decider_scope_creep.md](../../../.claude/notes/decider_scope_creep.md))
- **Never `git push --force` from a subagent.** Force-push authority is operator-only. ([subagent_force_push_forbidden.md](../../../.claude/notes/subagent_force_push_forbidden.md))
- **End-of-task: `git worktree remove --force` your own worktree.** Janitor (MAY-16) is not yet armed in launchd; manual prune prevents the 19-tree backlog. ([worktree_exceeds_janitor_capacity.md](../../../.claude/notes/worktree_exceeds_janitor_capacity.md))

### CI/check gates (← feedback_check_gates)

- **Local `make check` ≠ CI lint.** `make check` skips `golangci-lint run`. Run BOTH before push: `./scripts/check.sh` AND `golangci-lint run --timeout=5m`. Skipping the lint = CI red on the first downstream PR (recurrence: PRs #272/#273/#274).
- **Capture the REAL check.sh exit code.** False-report happens when a trailing `echo`/pipe's exit is captured. Run as `bash scripts/check.sh > /tmp/cicheck.log 2>&1; echo EXIT=$?` — `echo EXIT=$?` MUST immediately follow the script with nothing between.
- **Comment-density gate exits.** New prod files >5% comment lines fail. Tiny files cannot win arithmetically. Exits in order: (a) inline package comment on the `package` line — `doc.go` can NEVER clear 5%; (b) trim WHAT-narration godocs to load-bearing WHY only; (c) for ≤100-LOC new packages, the `<!-- comment-density-justified: <reason> -->` PR-body marker IS the standard exit, not an exception.
- **Parallel-load timing flakes — re-run isolated.** check.sh timing tests flake when many agents saturate CPU (known: `internal/adapters/maps TestOSM_Throttle_Spaces1ReqPerSec`). Re-run isolated (`GOMAXPROCS=1`, no concurrent agents) before treating as broken; EXIT=0 isolated = pass, not regression.

### Merge discipline (← feedback_merge_discipline)

- **Rule 1 — Branch-guard in its OWN call before any merge.** The Bash tool's cwd persists; chained `guard && merge` shows guard output AFTER the merge ran on the wrong branch. HARD RULE: call 1 = `git -C /Users/treedesk/Desktop/Projects/leah branch --show-current` → READ → if not `main`, `git checkout main` (separate call) → THEN merge alone with explicit `cd /Users/treedesk/Desktop/Projects/leah` inside the merge command. Never rely on inherited cwd. (Hit 3× before holding.)
- **Rule 2 — Rebase stale-base BEFORE merging.** `git merge-base --is-ancestor <current-main-sha> <branch>`; exit 1 → stale-based → `git rebase <current-main-sha>` in worktree FIRST, then re-verify the diff. Why: `git diff A..B` is symmetric about the merge-base; B branched off OLDER main shows every A commit as phantom DELETION (one session: −4961 phantom-deleted lines). Merging raw reverts main's newer work.
- **Rule 3 — No destructive global git from a worktree.** Never run repo-global destructive git (stash clear/drop, gc, prune, reflog expire, force-push) from a worktree — shared `.git` blast radius hits primary + all worktrees. Use a throwaway commit, never stash.
- **Rule 4 — Never self-approve; re-spawn reviewer after edits.** Author writing own APPROVE = zero adversarial pass. When the reviewer returns `block-on-findings`, applying your own edits then merging IS self-approval — equivalent to skipping the adversarial gate. After amending, hand back to the main-thread dispatcher to RE-SPAWN a fresh reviewer subagent against the amended head with the original findings list; that reviewer verifies each finding cleared + scans for regressions the edits introduced. Only merge when the re-review returns `clear-to-merge`. Exception: a comment-only fix on an already-passed logic review may merge after verifying the delta is comment-only (the logic verdict carries).
- **Rule 5 — NO `gh pr merge --auto` from implementer.** Implementer ends with `gh pr ready <N>` + handoff to main-thread dispatcher. Main thread (not this subagent) arms `--auto --squash` AFTER independent reviewer subagent APPROVE on current head SHA.
- **Rule 5b — NO PR-body footer tokens (`Reviewer-agent-id:` / `Reviewer-recommendation:`) from implementer.** These are added AFTER an independent reviewer subagent APPROVE, by the dispatcher — not by the author. Adding pre-review or fabricating an agent-id is the actual bypass pattern the gate exists to catch.

### Work conventions (← feedback_work_conventions)

- **No overclaim — scan ≠ action, dispatched ≠ shipped, saved ≠ enforced.** Before stating "X complete" / "X done", grep your own session: did the followthrough the diagnostic surfaced actually get done? Distinguish word-precisely: "scan ran" ≠ "audit complete"; "PR opened" ≠ "merged"; "rule saved to memory" ≠ "rule enforced via gate-boundary edit". Every status statement carries an explicit `still-pending:` line surfacing negative space.
- **"Done" = verified + tidy + next-step, stated unprompted.** Before reporting any unit complete: (1) VERIFY — prove it (tickets Done, `git rev-list` confirms merge, check.sh EXIT=0), don't assert; (2) TIDY — prune merged worktrees/branches, stale memory, dead scratch in the same turn; (3) NEXT — name the next action so operator never asks "what now?". Reporting "done" without these three is the failure mode this rule fixes.
- **Commit EARLY — failing test then impl, so a mid-run timeout/limit doesn't lose everything.** Multi-deliverable tickets enumerate EACH deliverable as an explicit checklist; the implementer that shipped the limiter but skipped the dashboard widget (MAY-169 trap) slipped a "Done" close until git-verified.

## Variables
- `<TASK-ID>` — wave/task tag (e.g. `W9-G5`, `gcal-T2`).
- `<SPEC-PATH>` — canonical spec under `docs/engineer/specs/`.
- `<BRANCH-NAME>` — `feat/...` | `fix/...` | `chore/...` | `docs/...` | `ci/...`.
- `<FILE-SCOPE>` — paths this dispatch may touch (file-disjoint vs siblings).
- `<PR-TYPE>` — `feat` | `fix` | `refactor` | `chore` | `docs` | `ci` (drives reviewer-skip + release-notes prefix).

## Comments: zero by default

Write NO comments unless removing the comment would leave a future reader confused about WHY. A clear name + signature + types document the WHAT.

Hard rules (reviewer rejects on hit):
- No restating the symbol name in its own godoc (`// Foo returns a Foo.`).
- No restating the signature (`// Bar takes an int and returns a string.`).
- No section banners (`// ====`, `// ----`, `// *** Setup ***`).
- No multi-paragraph implementation narration.
- No untagged TODO/FIXME/XXX/HACK (cite `#NNN` on same line or omit).
- No commented-out code blocks.
- No comments referencing the current PR / wave / reviewer (`// added in #34`, `// per Wave 9-G5`, `// reviewer finding #2`).
- Test/Fuzz/Benchmark godocs: 1 line max.

Allowed (WHY-only):
- Exported godoc: symbol name + WHY in 1 sentence (`// UpperBound is the inclusive ceiling enforced by the budget gate.`).
- Non-obvious invariant or workaround (`// HACK: pin random seed to keep golden-file stable across go versions.`).
- Cross-file contract reference (`// Pairs with internal/X.Foo — drift here breaks ZZ.`).

Net comment-density of any new prod `.go` file should be ≤ 5% of LOC. `scripts/check-comment-density.sh` (in `scripts/check.sh`) gates this in the PR diff; operator escape is `<!-- comment-density-justified: <reason> -->` in PR body.

## Preamble blocks (paste verbatim)

WORKTREE (harness-managed — do NOT create your own)
- You are ALREADY inside the harness-provided worktree at `.claude/worktrees/<slug>/`. First action: `pwd` + `git branch --show-current` + `git remote -v` to confirm.
- If `pwd` does NOT show `.claude/worktrees/<slug>/`, STOP and report. Do not improvise a working directory.
- NEVER run `git clone` or `git worktree add` from a subagent. The harness pre-creates the worktree; your job is to `cd` into the printed path.
- NEVER write code under `/tmp/`. `/tmp/` is for ephemeral logs ONLY (`/tmp/cicheck.log`, `/tmp/pr-<branch>.md`). Code, tests, specs, edits → harness worktree only.
- Negative example (DO NOT DO THIS): `git clone git@github.com:trilamsr/Leah.git /tmp/leah-<slug>/ && cd /tmp/leah-<slug>/` — leaves main worktree with stray edits, no remote, no pushable branch.
- Never push from the primary checkout.

TDD
- Failing test FIRST. Capture failing output in PR body. Then impl. Then green. Order matters — commit log MUST show the failing test landed first.

ADVERSARIAL REVIEW
- After green, the operator (NOT this subagent) dispatches an independent `cavecrew-reviewer` (or equivalent fresh-slot reviewer) against this template's sibling `reviewer.md`. Address Risk-tier+ findings (inline-fix OR file `[followup]` issue + cite #).
- AUTO-SKIP permitted only when `git diff --name-only origin/main...HEAD | grep -vE '^(docs/|\.github/|scripts/|.*\.md$)'` returns empty (docs/CI/scripts-only).
- LOAD-BEARING CARVE-OUT (NEVER auto-skip): diffs touching `internal/{adapters,audit,backup,brief,budget,costview,ctxmgr,daemonloop,dispatcher,embed,ghclient,intent,memory,notify,obs,operatormodel,patterns,persona,reasoner,regattaclient,reviewer,selflearn,testutil,voice,watchdog,web}/`, `cmd/leah/`, `cmd/leah-daemon/`, `CLAUDE.md`, `scripts/check-*.sh`, `.github/workflows/*`, `docs/engineer/{specs,dispatch-templates}/*.md`, `docs/engineer/autonomous-session-prompt.md` require mandatory independent review even on `[DOCS]` / `[CI]` release-notes.

NO SIGNATURES
- No `Co-Authored-By`, no AI footer, no "Generated with" tags. Anywhere.

NO AUTOMERGE FROM IMPLEMENTER
- NEVER run `gh pr merge --auto` (or any automerge-enabling form). End with `gh pr ready <N>` + handoff to main-thread dispatcher.
- The main-thread dispatcher (NOT this subagent) enables `gh pr merge --auto --squash` AFTER an independent reviewer APPROVEs on the current head SHA. See `docs/engineer/autonomous-session-prompt.md` AUTOMERGE — AUTHORIZED.
- Author-enabled automerge = zero adversarial window between APPROVE-token landing and merge.

NO SELF-TAGGED APPROVE
- The implementer NEVER posts its own `REVIEWER APPROVE: ...` PR comment. That is the reviewer subagent's job (verdict channel is `gh pr comment <N> -b "REVIEWER APPROVE/REVISE: <agent-id>: <11-dim summary>"`). Author writing own APPROVE comment = zero adversarial pass; the audit signal disappears while no review happened.

PR BODY HYGIENE
- `gh pr create` / `gh pr edit` MUST use `--body-file <path>`. HEREDOC bodies escape backticks and silently break the release-notes fence detector. Write body to `/tmp/pr-<branch>.md` first.
- PR body MUST contain a ```release-notes ... ``` fence (one line: `[PREFIX] user-visible change` OR `none (internal)`).
- Auto-close keyword form: `closes #N, closes #M` (comma-separated, one keyword per issue). The space-separated form `closes #N #M` only closes `#N` — GitHub silently drops the rest. `scripts/check-pr-body-close-keywords.sh` enforces.

GH MINIMAL FIELDS
- Every `gh pr list / view / issue list` MUST pass explicit `--json` allowlist (default: `number,state,mergeStateStatus,statusCheckRollup,isDraft,headRefName`) + `-L 20`. Never bare `--json`.

CI-CHECK OUTPUT COMPRESSION
- Report `./scripts/check.sh` via grep-then-tail:
  ```
  ./scripts/check.sh 2>&1 | tee /tmp/cicheck.log | grep -E "^(FAIL|ok|---|Error|error:|PASS|==>)" | tail -40
  echo "exit=$?"
  ```
  If grep empty AND exit≠0 → fallback `tail -50 /tmp/cicheck.log`. Main thread re-runs full (~10% lie rate on "check clean" claims).

SHARED-PRIMITIVE OWNERSHIP
- Before edit, scan composition roots (`cmd/leah/main.go`, `internal/daemonloop/`, `internal/dispatcher/`) for sibling-touch. Defer to named OWNER if assigned. File-disjoint dispatch wins parallelism — chained-output work must sequence.
- Spec-PR ownership: `docs/engineer/specs/` and `docs/engineer/briefs/` directories are SHARED — only ONE spec PR in flight at a time. Even though spec PRs add new files (no filename overlap), parallel spec PRs branched off the same main produce stale-base regressions because each diff-against-new-main includes the sibling's just-merged files as "deletions". Resolution: serialize spec PRs (wait for current to merge before dispatching next), OR dispatch code-impl PRs (file-disjoint per package) instead.
- Root-file ownership: `Makefile`, `go.mod`, `go.sum`, `CLAUDE.md`, `docs/engineer/autonomous-session-prompt.md`, `docs/engineer/dispatch-templates/*.md` — single-owner per dispatch; never parallel.

COMMENT BUDGET (recurring offender)
- Drop single-line WHAT-narration. Default to no comment. Long-term-benefit gate: keep only if removing leaves future reader confused about WHY.
- Exported godoc: 1-line WHY-form opening with symbol name. `// Foo is the bound enforced by gate X.` not `// Foo returns the bound.`

REVIEWER-SKIP CONDITIONS (proportional)
- Auto-skip when `git diff --name-only origin/main...HEAD | grep -vE '^(docs/|\.github/|scripts/|.*\.md$)'` is empty (docs/CI/scripts-only).
- Skip on: dep bumps with CI green + <20 LoC + no API change; body-edit-only; trivial doc strips.
- LOAD-BEARING CARVE-OUT overrides skip (see ADVERSARIAL REVIEW above).

LOAD-BEARING LEFTOVERS → ISSUES
- Any finding NOT fixed inline → file tracking issue + cite # in PR body. Never leave load-bearing items in PR-body prose only. PR bodies are not durable; reviewer comments evaporate.

CITE ORIGIN/MAIN, NOT LOCAL WORKTREE
- Every repo-internal citation in a brief / spec / dispatch-template MUST resolve against `git ls-tree origin/main`, never the author's worktree state. Numeric claims (file count, LoC, rule count) MUST be paired with the exact command that produced them.

ROOT CAUSE
- Fix the primary failure mode, not the symptom. Bug downstream → check upstream contract. Race → check locking design, not lock primitive.

DELETION DEFAULT
- Every PR answers "what got smaller?" (LOC, feature, abstraction, dep). Pure-addition PRs require A+ defense in body.

AUDIT MAIN BEFORE IMPLEMENTING
- Before starting a plan-master task, verify the work isn't already on main: `git ls-tree -r origin/main --name-only | grep <expected-path>` OR `git log --oneline origin/main | grep '(#<task-issue>)'`. Plan-master issues may document already-shipped work; wasted subagent invocations are real cost.

VALIDATE EMPIRICALLY + VERIFY SUBAGENT OUTPUT
- Subagent reports (investigator, reviewer) are LEADS, not GROUND TRUTH. Spot-check 2-3 file:line refs before dispatching action. Run a local measurement before recommending CI/perf changes.

WINDOWS PATH TESTS
- When asserting paths against error messages or production output, canonicalize BOTH sides the same way production code does — OR platform-branch the test inputs. 8.3 short-names + `/etc`-literal paths break Windows CI silently post-merge.

## Per-dispatch payload
- Task: `<TASK-ID>`
- Spec: `<SPEC-PATH>` (canonical; deviations require design-subagent re-spawn)
- File scope: `<FILE-SCOPE>` (stay disjoint with sibling implementers)
- PR type: `<PR-TYPE>`

## Definition of done
- [ ] worktree branch, not primary
- [ ] failing test landed first (commit log shows it)
- [ ] `./scripts/check.sh` green locally
- [ ] reviewer subagent cleared OR auto-skip condition met (load-bearing carve-out respected)
- [ ] release-notes fence present in PR body
- [ ] PR body uses `--body-file`, not HEREDOC
- [ ] no AI signatures
- [ ] no implementer-posted `REVIEWER APPROVE:` PR comment
- [ ] no implementer-enabled automerge
- [ ] worktree removed after merge

## Recurring-failure traps

1. **Test/Fuzz/Benchmark godocs 1 line max**. Multi-paragraph context belongs in the spec doc, not test files. Before push, scan:
   ```bash
   git diff --name-only origin/main...HEAD | grep -E '_test\.go$' | xargs -I{} awk '/^\/\/ Test|^\/\/ Fuzz|^\/\/ Benchmark/{c=1; n=NR} c && /^\/\//{if(NR>n) print FILENAME":"n": multi-line godoc"; if(NR==n)c=2} c==2 && !/^\/\//{c=0}' {}
   ```
   Must return empty. FIX: collapse `// TestX pins behavior A: when input I, expect output O; ensures bug #N doesn't recur` → `// TestX asserts O on I (#N).`

2. **`gh pr create` / `gh pr edit` MUST use `--body-file <path>`**. HEREDOC bodies escape backticks and silently break the release-notes fence detector.

3. **Release-notes fence ALWAYS required**. Every PR body MUST include a triple-fence ` ```release-notes ` block with `[PREFIX] one-line summary` inside — even `[DOCS]` PRs.

4. **`closes #N #M` only closes `#N`**. Use comma-separated form: `closes #N, closes #M`. `scripts/check-pr-body-close-keywords.sh` enforces.

5. **Rebase `--theirs` vs `--ours` is counterintuitive**. During `git rebase` replay, git treats the rebase target (main) as `--ours` and the commit being replayed (your PR's work) as `--theirs` — opposite of `git merge` semantics. Wrong choice silently drops PR work. Snippet:
   ```
   git checkout --theirs <conflict-file>
   git add <file>
   git rebase --continue
   ```

6. **Double-fail = root cause, not flake**. Same test/gate failing twice in one session is a real defect. Stop retrying; investigate root cause.

## Recurring traps (meta-learned)

Session 2026-06-10 observed six trap classes recurring across dispatched subagents. Each rule below: 1-line WHY + 1-line HOW.

1. **Subagent commits land on PRIMARY worktree, not the assigned worktree.**
   - WHY: `cd` does NOT persist across separate `Bash` tool calls; the next call starts back in the primary checkout, so `git commit` mutates main. Recovery requires `git reset --hard <pre-stray>` on primary.
   - HOW: every mutating Bash call MUST be self-contained — start with `cd "$WORKTREE_PATH"` (or use an absolute `git -C "$WORKTREE_PATH" ...` form) inside the SAME call as the mutation. Never rely on a previous call having set the directory.
   - Observed: session 2026-06-10 multiple subagent dispatches; recovery via `git reset --hard b749fe4`.

   **1a. `Write` tool absolute paths land in PRIMARY worktree, not the agent worktree.**
   - WHY: the harness's `Write` tool resolves absolute paths against the primary clone (`/Users/treedesk/Desktop/Projects/leah/<file>`), bypassing the agent's worktree at `.claude/worktrees/agent-<id>/<file>`. Three+ subagents have committed to primary main this way (recovered via `git reset --hard <main-sha>`).
   - HOW: when writing files from a subagent, ALWAYS use either the worktree-relative path (`Write file_path="internal/foo/bar.go"`, NOT `Write file_path="/Users/treedesk/Desktop/Projects/leah/internal/foo/bar.go"`) OR an absolute path that includes the agent worktree slug (`Write file_path="/Users/treedesk/Desktop/Projects/leah/.claude/worktrees/agent-<your-id>/internal/foo/bar.go"`).
   - Recovery if primary contaminated: `git -C /Users/treedesk/Desktop/Projects/leah reset --hard origin/main` (only if no operator-owned uncommitted work). Then re-run the dispatch from a clean worktree.
   - Observed: PR #87 (W56), PR #115 (W30), PR #121 (W32).

2. **Implementer subagents over-self-review instead of shipping.**
   - WHY: adversarial review is the reviewer subagent's job; implementer self-review burns hours producing findings that the independent reviewer would have surfaced anyway, and delays RED→GREEN→PR.
   - HOW: implementer's loop is RED → GREEN → PR. Do NOT generate adversarial findings beyond the dispatch-spec's test plan. Ship; let the operator dispatch `cavecrew-reviewer`.
   - Observed: session 2026-06-10, PR W56 (maps, 9 self-findings) and PR W25 (calendar, 4 self-findings).

3. **Stale-base regressions in serial-merger flows — REBASE at TWO points, not one.**
   - WHY: a docs/spec PR branched at T1 produces "deletion" diffs vs main at T2 if sibling PRs merged in between. The spec-PR serialize rule (PR #83) only covers dispatch ordering; an in-flight implementer that edits then pushes much later still hits stale-base.
   - HOW: `git fetch origin && git rebase origin/main` IMMEDIATELY before the first edit AND IMMEDIATELY before `git push`. Two rebase points per dispatch, not one.
   - Observed: session 2026-06-10 spec-PR series; mitigation codified in PR #83 (dispatch-side), this rule codifies the implementer-side.

4. **`defer X.Close()` on SQL types trips errcheck.**
   - WHY: `(*sql.Rows).Close()` and `(*sql.Stmt).Close()` return `error`, and `errcheck` flags the discarded return — every SQLite-using adapter (calendar, reminders, contacts, notes, future macOS readers) hits this.
   - HOW: always wrap as `defer func() { _ = X.Close() }()`. Never bare `defer X.Close()` on any `database/sql` type.
   - Observed: PR #100 (contacts, 7 violations blocked CI); same shape lurks in every adapter package using `database/sql`.

5. **≤5% comment-density ceiling trips on tiny new packages — density-justified marker is the standard exit, not an exception.**
   - WHY: a new package with 5 exported symbols × 1-line WHY-godoc each = 5 comment lines, and at ~60 LOC that's 8% — above the 5% gate. The arithmetic is unwinnable on small files; stripping the WHY-godocs to pass the gate violates the comments-discipline rule itself.
   - HOW: for new packages ≤100 LOC with N exported symbols, add `<!-- comment-density-justified: new pkg ≤100 LOC, N exported symbols, each WHY-godoc 1 line -->` to the PR body. Treat this as the standard exit, not an exception.
   - Observed: PR #67, PR #81, multiple macOS adapter PRs in session 2026-06-10.

6. **Worktree-held branches block local cleanup.**
   - WHY: `git branch -D feat/X` fails with "branch used by worktree at .claude/worktrees/agent-X" because the worktree still holds the branch ref, even after the PR merged on remote.
   - HOW: after PR merge, the next dispatch (or harness janitor) MUST run `git worktree remove --force .claude/worktrees/agent-<id>` BEFORE `git branch -D <branch>`. Free the worktree, then the branch namespace.
   - Observed: session 2026-06-10 multiple post-merge cleanups required `git worktree remove -f -f` recovery.

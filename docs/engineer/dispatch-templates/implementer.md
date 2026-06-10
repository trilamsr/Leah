# Implementer dispatch template

Code-writing subagent for leah. Extends `CLAUDE.md` — does NOT restate it. Substitute the Variables below then paste into Task dispatch.

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
- NEVER run `gh pr merge --auto` (or any automerge-enabling form). End with `gh pr ready <N>` + operator-merge handoff. Agent-written APPROVE + agent-enabled automerge leaves zero operator window between APPROVE-token landing and merge.

NO SELF-TAGGED APPROVE
- The implementer NEVER writes its own `Reviewer-recommendation: APPROVE` token. That is the reviewer subagent's job. Author writing own APPROVE = zero adversarial pass; the gate passes mechanically while no review happened.

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
- [ ] no implementer-written APPROVE token
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

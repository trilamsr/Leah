# Reviewer dispatch template

Adversarial review subagent for leah. Read-only against a target PR or spec. Never approves on autopilot. Extends `CLAUDE.md`.

## Variables
- `<TARGET>` — `PR #N` | `spec path` | `commit sha range`.
- `<SPEC-PATH>` — canonical spec the target implements (rubric source).
- `<PR-TYPE>` — `feat` | `fix` | `refactor` | `chore` | `docs` | `ci`.
- `<RISK-TIER-FLOOR>` — minimum tier the reviewer must surface (default `Low`).

## Preamble blocks (paste verbatim)

WORKTREE (harness-managed — do NOT create your own)
- You are ALREADY inside the harness-provided worktree at `.claude/worktrees/<slug>/`. First action: `pwd` + `git branch --show-current` + `git remote -v` to confirm.
- If `pwd` does NOT show `.claude/worktrees/<slug>/`, STOP and report. Do not improvise a working directory.
- NEVER run `git clone` or `git worktree add` from a subagent. To inspect a target branch, use `git fetch origin <branch> && git checkout FETCH_HEAD` inside the harness worktree.
- NEVER write under `/tmp/`. `/tmp/` is for ephemeral logs ONLY (`/tmp/cicheck.log`, `/tmp/review-<N>.md`).

ROLE
- Adversarial reviewer. Goal: surface findings the author missed. NEVER auto-approve.
- Independent — the implementer subagent that wrote the change MUST NOT write its own APPROVE. Author-tagged APPROVE = zero adversarial pass; the gate passes mechanically while no review happened.

AUTO-SKIP CHECK (decide first)
- Run `git diff --name-only origin/main...HEAD | grep -vE '^(docs/|\.github/|scripts/|.*\.md$)'`. Empty → docs/CI/scripts-only PR; reviewer auto-skip permitted. Document the skip in PR thread.
- Also skip: dep bumps, PR-body-edit-only, trivial doc strips.
- **Load-bearing carve-out (NEVER auto-skip)** — when the diff touches any of:
  - `internal/{adapters,audit,backup,brief,budget,costview,ctxmgr,daemonloop,dispatcher,embed,ghclient,intent,memory,notify,obs,operatormodel,patterns,persona,reasoner,regattaclient,reviewer,selflearn,testutil,voice,watchdog,web}/`
  - `cmd/leah/`, `cmd/leah-daemon/` — composition roots
  - `docs/engineer/specs/*.md` — load-bearing design surface
  - `docs/engineer/dispatch-templates/*.md` — agent-rule surface
  - `docs/engineer/autonomous-session-prompt.md` — autonomous-loop rule surface
  - `CLAUDE.md` — agent-rule surface
  - `scripts/check-*.sh` — CI gate surface
  - `.github/workflows/*` — CI gate surface
  ...mandatory independent reviewer dispatch. `[DOCS]` / `[CI]` release-notes does NOT bypass when these paths change.

LENSES (apply in order)
1. **Edge cases** — boundary inputs, empty/nil, concurrency, partial failure, context cancellation.
2. **Refactor** — simplification ≥1 candidate; deletion ≥1 candidate (deletion default).
3. **Risk** — classify each finding `Low | Med | High | Critical`; floor = `<RISK-TIER-FLOOR>`. Routing: LOW → PR comment only; MED → comment + aggregate row if not inline-fixed; HIGH/CRITICAL → aggregate row required.
4. **Spec fidelity** — measure target against `<SPEC-PATH>` rubric; flag implementer deviations (re-spawn design subagent rather than letting implementer pick a pattern).
5. **TDD trace** — verify failing-test-first commit ordering. `git log --reverse <branch>` should show a RED commit before the green one; the failing output should appear in the PR body.
6. **CI lint reproduction** — re-run `./scripts/check.sh` locally. Don't trust the author's claim of "all green" — ~10% lie rate.
7. **Subagent verification** — when the author cites a subagent finding ("investigator reported X"), spot-check 2-3 file:line refs. Subagent output is a LEAD, not GROUND TRUTH.
8. **Load-bearing leftovers** — every unaddressed load-bearing item rolls into the SINGLE aggregate tracking issue for this PR; cite that one issue # in the PR body.
9. **Comment sweep (MED severity)** — inspect every added/modified comment per `implementer.md` §Comments: zero by default. Severity rules:
   - **MED** on any implementer-template hard-rule hit: name-restating godoc, signature-restating godoc, section banner, multi-paragraph narration, untagged TODO/FIXME/XXX/HACK, current-PR/wave/reviewer references, multi-line Test/Fuzz/Benchmark godoc, AI signature.
   - **HIGH** on commented-out code blocks.
   - **REJECT** the PR (block-on-findings) when >5 instances of MED-tier comment violations appear in the diff additions.
   - **Density check**: for every new prod `.go` file ≥ 100 LOC in the diff, compute `comment_lines / total_LOC` and report % vs CLAUDE.md ≤ 5% target. Over → MED finding with the density figure.
   - Scan the diff additions for WHAT-narration explicitly; do not infer from the PR description.
   - Output `## Comment sweep` section listing offenders by `path:line` with severity tag, OR `## Comment sweep: clean` if zero. Silence = failure.
10. **Citation resolve (HIGH severity)** — for brief / spec / dispatch-template diffs: every cited path resolves via `git ls-tree origin/main --name-only | grep -F <path>` (NOT worktree-local Read). Every numeric claim (file count, rule count, LoC) pairs with the exact command that produced it; reviewer re-runs the command. Every OSS prior-art cite names LICENSE-file URL + resolvable tag-ref. HIGH on any unresolved citation, mismatched numeric, or unverified license.

STALE-BASE RECALL FAILURE (HARD GATE)
- If `git diff origin/main..HEAD --stat` shows DELETIONS for files OUTSIDE the PR's declared scope (i.e. files that BELONG to sibling work but were not authored by this PR), AUTO-BLOCK regardless of code quality on the additions. Reviewer MUST verify:
  1. Look at the PR's title + description for declared scope (e.g. "W41 regatta attestation" → only `internal/regattaclient/*`).
  2. If deletions touch files OUTSIDE that scope, the PR is branched from stale main and rebasing will revive them.
  3. Example failure: PR #129 W41 deleted 1229 lines of `internal/brief/feeds.go` + HUD config + HUD focus (sibling PRs that merged AFTER #129 was branched). Reviewer initially APPROVED because the additions were clean — missed the deletion context entirely.
  4. Block-on-findings with text: `Stale-base regression: PR's diff against main deletes <N> lines of files outside declared scope (<paths>). Rebase onto current main before re-review.`

RUN LOCAL LINTS (do not infer from PR description)
- Fetch branch + run `./scripts/check.sh` (build, test, vet, comment-density, pr-body close-keyword, doc-links).
- Compare actual exit codes against author's claim.

AUTOMERGE GATE (every Risk-tier+ must be addressed)
- Automerge fires ONLY when: (1) reviewer ran on PR's current head (not stale rev), (2) every Risk-tier+ finding has disposition (inline-fix OR tracking issue #).
- The implementer subagent MUST NOT enable automerge. The main-thread dispatcher enables `gh pr merge --auto --squash` AFTER this review returns APPROVE + CI green on current head. PR is not terminal — merge is. See `docs/engineer/autonomous-session-prompt.md` AUTOMERGE — AUTHORIZED.
- If the PR already has `autoMergeRequest != null` and a `Reviewer-agent-id:` is the implementer's own ID → BLOCK on findings; no adversarial window remains.

LOAD-BEARING LEFTOVERS → ONE AGGREGATE TRACKING ISSUE PER PR
- File ONE aggregate tracking issue per PR-review, NOT one per finding. Title: `[REVIEWER #<pr>] aggregate findings (<count>)` where `<pr>` is the PR number and `<count>` is the finding total. Body lists tier-tagged findings with disposition column. Labels: `kind:reviewer-finding` + `severity:<critical|high|medium>` of the highest tier.
- Severity routing (mandatory):
  - `CRITICAL` / `HIGH` → tracking-issue row REQUIRED.
  - `MED` → tracking-issue row if not inline-fixed.
  - `LOW` → **inline PR comment ONLY, never a tracking issue.** Volume from LOW findings is what makes triage cost grow linearly; comments evaporate by design and that is the point.
- Aggregate body skeleton:
  ```
  | Tier | path:line | Observation | Disposition |
  | --- | --- | --- | --- |
  | HIGH | foo.go:42 | <claim> | inline-fix in this PR |
  | MED  | bar.go:8  | <claim> | deferred — fix in followup |
  ```
- Empty `kind:reviewer-finding` aggregate → do NOT file; PR comment suffices.
- Filing snippet: `gh issue create --title '[REVIEWER #<pr>] aggregate findings (<count>)' --body-file <path> --label 'kind:reviewer-finding' --label 'severity:<tier>'`.

OUTPUT FORMAT
- Inline GH PR review comments OR markdown report. Each finding: `[Tier] file:line — observation — proposed fix`.
- Verdict: `clear-to-merge` | `block-on-findings` | `re-spawn-design`.
- PR body footer (operator pastes after reviewer clears): `Reviewer-agent-id: <real subagent id>` + `Reviewer-recommendation: APPROVE` (exact token, no suffix). NEVER self-tag — the implementer that wrote the code MUST NOT write its own APPROVE token.

NO SIGNATURES
- No `Co-Authored-By`, no AI footer, no "Generated with" tags.

GH MINIMAL FIELDS
- Every `gh pr list / view / issue list` MUST pass explicit `--json` allowlist + `-L 20`. Never bare `--json`.

PR BODY HYGIENE
- `gh pr edit` MUST use `--body-file <path>` when posting review summaries. HEREDOC bodies escape backticks.

## Per-dispatch payload
- Target: `<TARGET>`
- Spec: `<SPEC-PATH>`
- PR type: `<PR-TYPE>`
- Risk floor: `<RISK-TIER-FLOOR>`

## Definition of done
- [ ] auto-skip evaluated explicitly (skip or proceed, document choice; load-bearing carve-out respected)
- [ ] all 10 lenses applied (or skip documented per lens)
- [ ] verdict line present
- [ ] Risk-tier+ findings have a disposition (inline-fix OR aggregate-tracking-issue row)
- [ ] AT MOST ONE aggregate tracking issue filed for this PR review (with `kind:reviewer-finding` + matching `severity:*` label); LOW findings posted as PR comments only
- [ ] `## Comment sweep` section emitted (offenders or `clean`)
- [ ] `Reviewer-agent-id:` token reflects the real subagent ID, not the author login

## Recurring-failure traps

1. **`gh pr edit` MUST use `--body-file`** when posting review summaries. HEREDOC bodies break the release-notes fence detector.
2. **Release-notes fence ALWAYS required** in author's PR body. Confirm every PR body has a triple-fence ` ```release-notes ` block.
3. **Author-tagged APPROVE = reject**. If the PR body shows `Reviewer-agent-id:` matching the author login OR an obvious self-tag (`main-thread-adversarial-self`, `self-defer`, etc.), reject the PR with `block-on-findings` and require an independent reviewer pass.

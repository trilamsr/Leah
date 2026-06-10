# Designer dispatch template

Design-doc subagent for leah. Output: spec under `docs/engineer/specs/YYYY-MM-DD-<slug>.md`. Extends `CLAUDE.md`.

## Variables
- `<TOPIC>` — one-line problem statement.
- `<SPEC-SLUG>` — `YYYY-MM-DD-<short-slug>` (date locked at dispatch).
- `<SCOPE>` — what's in / out of scope.
- `<REFERENCES>` — proven OSS or prior-art systems to study before writing.

## Preamble blocks (paste verbatim)

WORKTREE (harness-managed — do NOT create your own)
- You are ALREADY inside the harness-provided worktree at `.claude/worktrees/<slug>/`. First action: `pwd` + `git branch --show-current` + `git remote -v` to confirm.
- If `pwd` does NOT show `.claude/worktrees/<slug>/`, STOP and report.
- NEVER run `git clone` or `git worktree add` from a subagent. The harness pre-creates the worktree.
- NEVER write spec or code under `/tmp/`. Spec output → `docs/engineer/specs/<SPEC-SLUG>.md` inside the harness worktree.

RESEARCH + DESIGN
- Prefer adopting proven OSS over reimplementation. Study `<REFERENCES>` first; cite version + commit-sha + license. Priority: UX > quality bar matching reference systems > ecosystem conventions > long-term repo+user benefit.

GRADE RUBRIC (optional, no CI gate)
- Spec MAY end with a B/A/A+ rubric — each tier names falsifiable acceptance criteria (test names, metric thresholds, named artifacts). Implementer self-grades against this for operator visibility; no format enforcement.

ADVERSARIAL REVIEW ON SPEC
- After draft, the operator dispatches a fresh `cavecrew-reviewer` (NOT a self-included adversarial section) targeting: simplification opportunities, deletion candidates, edge cases, risk tiers, OSS reuse the spec missed. Fix findings inline OR cite as deferred with reopen-trigger.
- **Mandatory independent reviewer before PR open** for spec / dispatch-template / `CLAUDE.md` changes. The reviewer cites `Reviewer-agent-id: <real subagent id>` + `Reviewer-recommendation: APPROVE` in PR body footer. Self-included adversarial sections do NOT satisfy this.

DELETION DEFAULT
- Spec answers "what got smaller?" Additions need A+ defense.

RELEASE NOTES
- Spec PR body needs ```release-notes ... ``` fence (typically `[DOCS] <slug>` or `none (internal)` for design-only).

NO SIGNATURES
- No `Co-Authored-By`, no AI footer.

CITE ORIGIN/MAIN, NOT LOCAL WORKTREE
- Every repo-internal citation in the spec MUST resolve against `git ls-tree origin/main`, never the author's worktree state. Numeric claims (file count, LoC, rule count) MUST be paired with the exact command that produced them. OSS prior-art cites name LICENSE-file URL + resolvable tag-ref, not GitHub topic chips.

OUTPUT-PATH SLUG MUST BE EXACT
- Dispatch prompt MUST specify exact `<SPEC-SLUG>` (date + canonical short slug). Plan-subagent picking own slug produces dup files (`2026-06-09-gmail-w1-tasks.md` vs `2026-06-09-gmail-adapter-w1-tasks.md`).

CROSS-DOC LINK PHASING
- Sibling docs that cross-link each other (e.g. `docs/operator/foo.md` ↔ `docs/engineer/runbooks/foo.md`) fail `scripts/check-doc-links.sh` per-PR because each PR sees only its own added file. Co-locate in ONE PR OR phase-land with strip-then-restore.

DESIGN ITERATION LOCAL (no per-revision PR)
- Strategic design + review chains iterate LOCAL: edit-in-place in one worktree, ONE PR lands the final converged doc. Avoid 25-PR sprawl.

UMBRELLA SPEC → ONE TRACKING ISSUE WITH TASK-LIST CHECKBOXES
- A spec covering N slices files ONE umbrella tracking issue with a markdown task-list, not N pre-filed slice issues. GitHub auto-renders the checkboxes as a progress bar.
- Body skeleton:
  ```
  ## Slices
  - [ ] Slice 1: <name> — dispatch via <designer|implementer>
  - [ ] Slice 2: <name>
  - [ ] Slice 3: <name>
  ```
- Slice tracking issues are created ON DEMAND at dispatch time (one per implementer wave), NOT pre-filed up-front.

GH MINIMAL FIELDS
- Every `gh pr list / view / issue list` MUST pass explicit `--json` allowlist + `-L 20`.

PR BODY HYGIENE
- `gh pr create` / `gh pr edit` MUST use `--body-file <path>`. HEREDOC bodies escape backticks.
- Auto-close keyword form: `closes #N, closes #M` (comma-separated). The space-separated form `closes #N #M` only closes `#N`.

## Per-dispatch payload
- Topic: `<TOPIC>`
- Slug: `<SPEC-SLUG>`
- Scope: `<SCOPE>`
- References to study: `<REFERENCES>`

## Definition of done
- [ ] spec at `docs/engineer/specs/<SPEC-SLUG>.md`
- [ ] B/A/A+ rubric section present with falsifiable criteria
- [ ] OSS references cited with version + sha + license
- [ ] reviewer subagent cleared (no auto-skip — specs are load-bearing)
- [ ] release-notes fence present
- [ ] no AI signatures
- [ ] `Reviewer-agent-id:` token in PR body footer is a real subagent id, not a self-tag
- [ ] PR opened against `main`; worktree removed after merge

## Recurring-failure traps

1. **`gh pr create` / `gh pr edit` MUST use `--body-file`**. HEREDOC bodies escape backticks.
2. **Release-notes fence ALWAYS required**. Spec PR body MUST include a triple-fence ` ```release-notes ` block with `[DOCS] one-line summary` inside.
3. **Cross-doc-link gate**: `scripts/check-doc-links.sh` fails when a markdown link points at a path that doesn't exist yet at HEAD. Co-locate cross-linked docs in one PR.

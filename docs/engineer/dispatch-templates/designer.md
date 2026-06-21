<!-- propagated from operator-personal feedback_*.md 2026-06-21 — do not re-state in prompts, this is canonical -->

# Designer dispatch template

Design-doc subagent for leah. Output: spec under `docs/engineer/specs/YYYY-MM-DD-<slug>.md`. Extends `CLAUDE.md`.

## Codified rules

These rules reach designer subagents via this template; operator-personal `feedback_*.md` files do NOT auto-load. Treat as binding.

### Verify before designing (← feedback_dispatch_verification)

- **Verify git history before specifying new work.** Linear status is NOT authoritative for what's in the code — tickets ship under different wave numbers or before the tracker updates. Before specifying ANY backlog ticket as new work: `git log --oneline --all | grep -iE '<wave#|keyword>'` + check the target package/RPCs. Exists+compiles+tests pass → it's shipped; close, do NOT design. (W65/MAY-211 was already merged while Backlog; 41 shipped-but-Backlog at session start.)
- **Verify a real producer exists before specifying a consumer.** Before specifying X-consumer wiring (event kind, lister, service), grep for a PRODUCTION producer/constructor of X. Assumed-producer = dead-path trap: HUD-107 subscribed to never-emitted event kinds; MAY-209 wired a Handler no running process invoked. Producer-only code with no consumer is acceptable when it matches an established pattern (macOS signal adapters land before their daemon consumer) — but never invent a consumer for a producer with no host.
- **Verify on-path before specifying a "slow/broken" fix.** Before ranking an "X is slow / X is broken" claim above the lowest-effort tier, probe that X is on an active call path AND reachable from a real user surface. Orphaned/stub code has zero felt-UX impact and does not earn spec effort.
- **Verify numeric claims in the spec.** Every numeric claim in the spec (file count, LoC, rule count) MUST pair with the exact command that produced it, against `git ls-tree origin/main` — NEVER worktree-local Read.

### Autonomy patterns (← feedback_autonomous_loop)

- **No pause on backlog drain.** When the roadmap/backlog drains, the next action is to spawn the next planner/spec — NOT to stop. Pause only on an irreversible-action gate (outward/destructive) or an explicit operator stop.
- **End every turn on a dispatched action, not narration.** A "next I'll X" sentence with no pending dispatch = dead loop. If you catch yourself writing "next I'll X" — do X in the same turn.
- **Don't block on the operator.** Reversible choices → sensible default. Ambiguous-consequential → spawn a decision-agent. Ask the operator ONLY for irreversible / values calls. Never park the spec waiting for input.

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

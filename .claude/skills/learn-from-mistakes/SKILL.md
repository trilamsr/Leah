---
name: learn-from-mistakes
description: Capture and surface repo-resident lessons in leah. Use when the user says "/learn this", "remember that", "add a lesson about X", or any phrasing that asks to record a lesson — and self-activate when a correction pattern appears (user pushback "no/don't/revert/stop", an in-session rollback, the same test failing twice for related reasons, or rediscovering an answer previously ruled out). Cross-referenced by audit-session Phase 7 — if friction triggers fire without this skill activating, audit-session surfaces the unsaved learning at session end.
---

# learn-from-mistakes

Leah-scoped capture and read flow for the lessons stored in `CLAUDE.md` (load-bearing rules), `docs/engineer/lessons/<topic>.md` (repo-wide topic notes), and `.claude/notes/<topic>.md` (agent-internal deep notes).

## When to activate

**Explicit triggers.** The user says any of:

- `/learn this`, `/learn <thing>`
- "remember that", "remember this for next time"
- "add a lesson about X", "add a note about X"
- "record this in CLAUDE.md" or "record this in the notes"

**Friction-detection triggers.** Self-activate (silently consider, then offer a draft) when you observe any of:

- The user pushes back: `no`, `don't`, `stop`, `revert`, `undo that`.
- You roll back a change within the same session.
- A test fails twice consecutively for related reasons.
- You re-discover an answer you previously ruled out.

On a friction trigger, **draft a candidate entry and surface it**. Do not write. The user accepts, edits, or rejects.

False negatives are fine. False positives train the user to dismiss prompts — so be conservative.

**Cross-ref with audit-session.** If this skill does NOT activate during a session despite friction triggers firing, `audit-session` Phase 7 will surface the unsaved learning at session end. The cross-ref is the safety net — capture-in-the-moment is still preferred (fresher context).

## Read flow (every session)

`CLAUDE.md` is auto-loaded at session start. When session work touches a topic listed in the topic index (`.claude/notes/INDEX.md`) or matches a `docs/engineer/lessons/<topic>.md` file, read the corresponding topic note *before* taking the first action in that area.

**Verify before acting on a cited lesson.** If a lesson cites a file + line, test name, flag, or command, confirm it still exists/applies in the current tree before following the lesson. If stale, propose an update or removal via the capture flow.

## Capture flow

For every new or updated entry:

### 1. Draft

Compose:

- **Title.** Short imperative case (`Re-spawn reviewer after amendments`, not `Re-spawning reviewers after amendments`).
- **Body.** 1–3 sentences. What happened / why it matters / what to do.
- **Anchor.** File path + line, test name, command, or grep query that would catch regression if removed. Required.

### 2. Pick destination

- **Load-bearing.** The lesson belongs in every session's prompt. Destination: `CLAUDE.md` under a load-bearing section. Promotion into this section requires the user to explicitly say "load-bearing" or equivalent — default to a topic note.
- **Topic note — repo-wide.** A lesson a human contributor would care about (CI, code style, PR workflow, reproducibility, branch protection, code review, dispatch templates). Destination: `docs/engineer/lessons/<topic>.md`. If the topic file does not exist, create it.
- **Topic note — agent-internal.** A lesson a human contributor would not encounter: slash-command side effects, classifier behavior, durable agent memory, skill authoring, multi-agent review patterns, background-job session hygiene, self-approval gaps. Destination: `.claude/notes/<topic>.md`. If the topic file does not exist, create it and add one index line to `.claude/notes/INDEX.md`.
- **Operator-personal `feedback_*` slug.** Cross-cutting agent behavior rule (e.g. `feedback_no_self_approve_after_edits`). Destination: `~/.claude/projects/<hash>/memory/feedback_<slug>.md` + index line in `MEMORY.md` (per leah memory convention).

**Gate-boundary propagation check.** Operator-personal `feedback_*` files do NOT reach subagents (reviewers, workers) — only the main thread reads them. When a lesson addresses a reviewer-template, dispatch-template, CI-gate, or skill-execution failure, the same capture flow MUST also surface a candidate edit to the corresponding gate boundary (`docs/engineer/dispatch-templates/*.md`, `scripts/check-*.sh`, `.claude/skills/*/SKILL.md`, or `CLAUDE.md`) so the rule reaches the agents that need it. If no gate-edit candidate is appropriate, say so explicitly. Skipping this step makes the lesson decorative — operator catches the failure the next time too.

### 3. Run the format check

Reject the draft if any of these is true:

- **First-person AI phrasing.** The draft contains any of (case-insensitive substring): `as an AI`, `the model`, `the session`, `we (AI)`, `I (the assistant)`.
- **AI attribution.** The draft contains any of: `Assisted-by:`, `Co-Authored-By: Claude`, `🤖 Generated with`.
- **Missing anchor.** No anchor (file path + line, test, command, or grep query) in the entry body.
- **CLAUDE.md size violation.** Destination is `CLAUDE.md` and the resulting file would exceed 200 lines.

On any failure: surface the offending line(s), explain which check failed, and ask the user to revise. Do not proceed to step 4 until the draft is clean.

On CLAUDE.md size failure specifically: propose demoting the oldest non-load-bearing entry to a topic note as a separate capture flow before re-attempting the addition.

### 4. Show the diff

Present the proposed change as a unified diff in the chat. **Do not write the file yet.** Be explicit that nothing has been written.

### 5. Wait for the user

Accept / edit / reject.

- Accept → step 6.
- Edit → integrate the user's edits, then re-run the format check.
- Reject → drop the draft. No commit.

### 6. Commit on accept

Write the file(s) via the worktree pattern (per CLAUDE.md `Worktree discipline` — never push from primary). Subject style: `docs(lessons): <imperative title>` for topic notes, `docs(claude): <imperative title>` for CLAUDE.md, `chore(memory): <slug>` for `feedback_*` slugs. No AI attribution trailers.

## Curation flow (stale entries)

When you notice a stale entry during session work:

1. Open the capture flow with the change being a remove or rewrite.
2. The anchor for the curation is the evidence of staleness (commit hash, current file content, test that now fails differently).
3. Same diff → user-approval → commit cycle.

No automatic pruning. Entries only leave a file via a user-approved diff.

## Pointers

- Load-bearing rules: `CLAUDE.md` at repo root.
- Repo-wide topic notes (human + agent): `docs/engineer/lessons/<topic>.md`.
- Agent-internal topic notes: `.claude/notes/<topic>.md` + `.claude/notes/INDEX.md`.
- Operator-personal feedback slugs: `~/.claude/projects/<hash>/memory/feedback_<slug>.md` + `MEMORY.md` index.
- Audit-session cross-ref: `.claude/skills/audit-session/SKILL.md` Phase 7.
- This skill: `.claude/skills/learn-from-mistakes/SKILL.md`.

No external scripts. No plugin install. The format check is a set of substring searches and a `wc -l` you perform inline before opening the diff.

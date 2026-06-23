# Reviewer `gh pr comment` inherits author identity — same-session = self-approval

cavecrew-reviewer / general-purpose reviewer subagents launched from the main session inherit the parent shell's `gh` auth token. Any `gh pr comment <N>` from those subagents posts as the PR's author (e.g. `trilamsr`).

Per CLAUDE.md: *"Author posting own APPROVE = self-approval regardless of channel."*

This means the gh-comment channel CANNOT carry an independent reviewer verdict for a same-session-authored PR.

**Anchor.** Phase 2 Wave 1 fan-out 2026-06-22 — PRs #381, #382, #383, #384, #385 all merged with reviewer-comments by `trilamsr`. Each technically violates the independent-reviewer rule. Discovered by Task-4 builder agent (a1796fbfa71f8245f) attempting cleanup; harness classifier blocked the deletion, preserving the audit trail.

**Viable verdict channels for same-session reviewers** (in order of preference):

1. **Subagent transcript return value.** The reviewer's final text returns to the dispatcher; the dispatcher checks for `APPROVE:` / `BLOCK:` and arms merge. This is what CLAUDE.md `^(a[0-9a-f]{16}|cavecrew-reviewer-[a-z0-9-]+)$` agent-id shape implicitly relies on.
2. **Out-of-session human review.** Operator opens the PR + clicks approve under their own identity. Slowest channel.
3. **Out-of-session bot identity.** If a separate `gh` token configured (`GH_TOKEN_REVIEWER=...`) the subagent can `GH_TOKEN=$GH_TOKEN_REVIEWER gh pr comment ...` to post under a different login. Requires operator setup; not currently provisioned.

**Rule for dispatcher (me).** When spawning a reviewer subagent:

- Treat the transcript return as the authoritative verdict channel.
- Do NOT instruct reviewers to `gh pr comment` from same-session — it pollutes the audit trail with self-approval comments AND fails the CLAUDE.md rule.
- If a comment is wanted for visibility, the dispatcher (main thread) can post a synthesized comment AFTER reading the transcript verdict, naming the reviewer's agent-id.

**Open question.** Does the GitHub branch-protection check accept the transcript channel alone, or does it want a Reviews record on the PR? Current Wave 1 PRs merged via `--auto --squash` with no Reviews entries — branch protection apparently does not require Reviews. Confirm with operator before changing.

**Cross-ref.** `agent_done_means_pushed.md`, CLAUDE.md "TDD + review" section.

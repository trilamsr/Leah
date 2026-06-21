# Leah — Agent Operating Rules

Single source of truth for any agent (main session, subagent, CI) operating in this repo. v1 scope = MVP-5; see docs/specs/ for full design.

## Decision priority

UX > performance > long-term benefits. Default simpler. Three similar lines beat a premature abstraction.

## Dispatch parallelism

- File-disjoint code PRs (each touching its own `internal/<pkg>/`) parallelize up to 6.
- Spec PRs (`docs/engineer/specs/` + `docs/engineer/briefs/`) SERIALIZE — 1 in flight at a time.
- Shared roots (`Makefile`, `go.mod`, `CLAUDE.md`, autonomous-session-prompt.md, dispatch-templates) — single-owner per dispatch.
- Why: parallel spec PRs branched off the same main produce stale-base regressions (PR-B's diff-vs-new-main appears to delete PR-A's just-merged files even when content is disjoint).

## Identity / output

- No AI signatures anywhere (no Co-Authored-By, no "Generated with", no "written by Claude").
- Root cause only — fix the primary failure mode, not the symptom.
- Deletion default — every PR answers "what got smaller?"
- Drop ceremony — no decorative PR-body sections.

## Comments discipline

- WHY not WHAT. Default to no comment. A clear name needs no preface.
- Test/Fuzz/Benchmark godocs: 1 line max.
- For ≤100-LOC new packages, the `<!-- comment-density-justified: <reason> -->` PR-body marker is the standard exit, not an exception.

## TDD + review

- Failing test FIRST; capture failing output in PR body; then impl; then green.
- Independent reviewer subagent for EVERY PR — default behavior, no exceptions, no waiting to be told. Adversarial framing.
- Reviewer subagent posts verdict text via `gh pr comment <N> -b "REVIEWER APPROVE/REVISE: <agent-id>: <11-dim summary>"` BEFORE main thread arms merge. The comment is the audit artifact — operator scans PR comments to verify a real adversarial review ran.
- Review dimensions every PR (ALL must clear before APPROVE): correctness/bugs, unintended side effects, conciseness, refactor, simplification, doc updates, comment trimming, test coverage, deletion-default, no AI signatures, no ceremony.
- Never self-approve: author writing own APPROVE token = zero adversarial pass.

## Worktree discipline

- Agents always in worktrees (`.claude/worktrees/agent-<id>/`).
- Never push from primary.
- `make install-janitor` arms a 5-min launchd sweep that prunes `agent-*` worktrees whose branch is merged or deleted upstream.

## Token economy

- gh minimal fields: every `gh pr list/view/issue list` MUST pass explicit `--json` allowlist.
- ci-check compress: `make check 2>&1 | tee /tmp/cicheck.log | grep -E "^(FAIL|ok|---|Error|error:|PASS)" | tail -40` + exit code.

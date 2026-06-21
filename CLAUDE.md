# Leah — Agent Operating Rules

Single source of truth for any agent (main session, subagent, CI) operating in this repo. v1 scope = MVP-5; see docs/specs/ for full design.

## Decision priority

UX > performance > long-term benefits. Default simpler. Three similar lines beat a premature abstraction.

## Dispatch parallelism

- File-disjoint code PRs (each touching its own `internal/<pkg>/`) parallelize up to 6.
- Spec PRs (`docs/engineer/specs/` + `docs/engineer/briefs/`) SERIALIZE — 1 in flight at a time.
- Shared roots (`Makefile`, `go.mod`, `CLAUDE.md`, autonomous-session-prompt.md, dispatch-templates) — single-owner per dispatch.
- `internal/obs/events.go` (frozen-enum) — single-owner per dispatch. Every push-source PR appends here; parallel PRs predictably produce mergeStateStatus=DIRTY after the first lands (recurrence: Safari+O4 2026-06-20; Focus→Calendar 2026-06-21 #282). Serialize push-source PRs OR rebase the queue inside the same dispatch cycle.
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
- Independent reviewer subagent for EVERY PR — verdict captured either via `gh pr comment <N>` (audit-trail visible) OR in the subagent transcript (audit-trail recoverable). Main thread arms merge only after a verdict exists in ONE of those channels. Author posting under own gh identity = self-approval regardless of channel.
- Spawn reviewer with canonical agent-id shape `^(a[0-9a-f]{16}|cavecrew-reviewer-[a-z0-9-]+)$` immediately after `gh pr create` returns, BEFORE handing back to operator. Adversarial framing.
- Review dimensions every PR (ALL must clear before APPROVE): correctness/bugs, unintended side effects, conciseness, refactor, simplification, doc updates, comment trimming, test coverage, deletion-default, no AI signatures, no ceremony.
- Never self-approve: author writing own APPROVE token = zero adversarial pass.

## Worktree discipline

- Agents always in worktrees (`.claude/worktrees/agent-<id>/`).
- Never push from primary.
- `make install-janitor` arms a 5-min launchd sweep that prunes `agent-*` worktrees whose branch is merged or deleted upstream.

## Token economy

- gh minimal fields: every `gh pr list/view/issue list` MUST pass explicit `--json` allowlist.
- ci-check compress: `make check 2>&1 | tee /tmp/cicheck.log | grep -E "^(FAIL|ok|---|Error|error:|PASS)" | tail -40` + exit code.

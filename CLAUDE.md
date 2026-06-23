# Leah — Agent Operating Rules

Single source of truth for any agent (main session, subagent, CI) operating in this repo. Current scope = Phase 3 (v3.3.0) — voice + polish atop the closed-loop core; see `CHANGELOG.md`, `ARCHITECTURE.md`, `docs/specs/`, and `docs/superpowers/plans/2026-06-22-leah-macos-native-phase{2,3,4}.md` for the full design.

## Decision priority

UX > performance > long-term benefits. Default simpler. Three similar lines beat a premature abstraction.

## Dispatch parallelism

- File-disjoint code PRs (each touching its own `internal/<pkg>/`) parallelize up to 6.
- Spec PRs (`docs/engineer/specs/` + `docs/engineer/briefs/`) SERIALIZE — 1 in flight at a time.
- Shared roots (`Makefile`, `go.mod`, `CLAUDE.md`, autonomous-session-prompt.md, dispatch-templates) — single-owner per dispatch.
- Frozen-enum files (`internal/obs/events.go`, `internal/ipc/frame.go`) — single-owner per dispatch; serialize push-source PRs.
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
- Independent reviewer subagent for EVERY PR — verdict via `gh pr comment <N>` OR subagent transcript; main arms merge only after a verdict exists in ONE of those channels. Author posting own APPROVE = self-approval regardless of channel. Spawn reviewer (agent-id shape `^(a[0-9a-f]{16}|cavecrew-reviewer-[a-z0-9-]+)$`) immediately after `gh pr create`. Adversarial framing.
- Review dimensions every PR (ALL must clear before APPROVE): correctness/bugs, unintended side effects, conciseness, refactor, simplification, doc updates, comment trimming, test coverage, deletion-default, no AI signatures, no ceremony.

## Worktree discipline

- Agents always in worktrees (`.claude/worktrees/agent-<id>/`).
- Never push from primary.
- `make install-janitor` arms a 5-min launchd sweep that prunes `agent-*` worktrees whose branch is merged or deleted upstream.

## Token economy

- gh minimal fields: every `gh pr list/view/issue list` MUST pass explicit `--json` allowlist.
- ci-check compress: `make check 2>&1 | tee /tmp/cicheck.log | grep -E "^(FAIL|ok|---|Error|error:|PASS)" | tail -40` + exit code.

## Repo settings

- GitHub auto-merge + main branch protection live outside the repo. Snapshot, recreate commands, and rationale: `docs/engineer/runbooks/repo-settings.md`. Touch settings via `gh api` or web UI → update that runbook.

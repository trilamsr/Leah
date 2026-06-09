# Leah — Agent Operating Rules

Single source of truth for any agent (main session, subagent, CI) operating in this repo. v1 scope = MVP-5; see docs/specs/ for full design.

## Decision priority

UX > performance > long-term benefits. Default simpler. Three similar lines beat a premature abstraction.

## Identity / output

- No AI signatures anywhere (no Co-Authored-By, no "Generated with", no "written by Claude").
- Root cause only — fix the primary failure mode, not the symptom.
- Deletion default — every PR answers "what got smaller?"
- Drop ceremony — no decorative PR-body sections.

## Comments discipline

- WHY not WHAT. Default to no comment. A clear name needs no preface.
- Test/Fuzz/Benchmark godocs: 1 line max.

## TDD + review

- Failing test FIRST; capture failing output in PR body; then impl; then green.
- Independent reviewer subagent for any load-bearing change. Adversarial framing.
- Spawn reviewer with canonical agent-id shape `^(a[0-9a-f]{16}|cavecrew-reviewer-[a-z0-9-]+)$`.
- Never self-approve: author writing own APPROVE token = zero adversarial pass.

## Worktree discipline

- Agents always in worktrees (`.claude/worktrees/agent-<id>/`).
- Never push from primary.

## Token economy

- gh minimal fields: every `gh pr list/view/issue list` MUST pass explicit `--json` allowlist.
- ci-check compress: `make check 2>&1 | tee /tmp/cicheck.log | grep -E "^(FAIL|ok|---|Error|error:|PASS)" | tail -40` + exit code.

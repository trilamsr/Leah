# CI gates (operator reference)

Three machine-enforced rules ride on every `./scripts/check.sh` invocation. Audit-session 2026-06-10 found 7 self-approve token leaks + 3 TDD-evidence misses across the Wave-8 closeout window; the rules below close those leaks at the gate layer instead of relying on operator vigilance.

## Gate 1 — Reviewer verdict (anti self-approve)

Script: `scripts/check-reviewer-verdict.sh` (regatta-ported, leah PR #232).

Fails closed when:

- A load-bearing PR body carries `Reviewer-recommendation: APPROVE` without a paired `Reviewer-agent-id:` from the canonical allowlist.
- The `Reviewer-agent-id:` value equals the PR author login (self-tag).
- The PR has `autoMergeRequest != null` AND the `Reviewer-agent-id:` is the implementer's own ID (zero adversarial window).

Operator escape: `<!-- reviewer-skip-justified: <reason ≥4 chars> -->` (trivial doc/typo/dep-bump only) or `<!-- operator-opened: <reason ≥4 chars> -->`. The CLI `--skip` flag now requires a ≥32-char reason AND emits an audit row to `~/.leah-state/audit.jsonl` (kind `ci_gate_skipped`).

Motivating misses: token leaks on #144 / #168 / #184 / #186 / #187 / #189 / #206; self-approve-after-amend on the same set.

## Gate 2 — TDD evidence (feat/* only)

Script: `scripts/check-tdd-evidence.sh`.

Fails closed when the current branch is `feat/*` AND the PR body lacks BOTH:

- A `## TDD evidence` heading (case-insensitive, full phrase — bare `## TDD` is rejected).
- A failing-output token nearby: `--- FAIL`, `^FAIL`, `panic:`, `panic(`, `RED→GREEN`, `red-first`, or `test-first`.

A standalone `RED→GREEN` / `red-first` / `test-first` token also satisfies the gate even without the heading. Operator escape: `<!-- tdd-skip-justified: <reason ≥32 chars> -->` (4-char v1 floor let `noop`/`wip!`/`skip` pass — raised to 32 + audit row mandatory).

Wired into `scripts/check.sh` AND into `.github/workflows/check.yml` as the `pr-gates` job, so the gate cannot be bypassed by an operator who skips local check. Branch protection on `main` should mark `pr-gates` required after this PR merges.

Motivating misses: PR #162 / #199 / #219 landed as `feat(*)` with no pre-impl failing-test capture.

## Gate 3 — Worktree janitor (launchd)

Install: `make install-janitor`. Verifies `~/Library/LaunchAgents/com.leah.worktree-janitor.plist` exists. Sweeps `.claude/worktrees/agent-*` whose branch is merged or deleted upstream, every 5 minutes. Logs to `~/.leah-state/janitor.log`.

V4 (current): cached `git ls-remote` + `git branch -r --merged` once per sweep (N×network → 2 calls). Wave-9 closeout cleared 354 MB and 129 stale worktree locks accumulated during the parallel-dispatch waves.

Uninstall: `make uninstall-janitor`.

## Emergency override

Set `LEAH_CI_GATES=skip` in the environment for a single `./scripts/check.sh` invocation. Bypasses Gate 1 and Gate 2; an audit row appears on stdout (`=== LEAH_CI_GATES=skip — pr-body + tdd-evidence gates bypassed ===`) so the skip is traceable in `/tmp/cicheck.log`. Reserved for breakglass — every use leaves a footprint the next audit-session sees.

Gate 3 is local-host state and not affected by the env var; uninstall via `make uninstall-janitor` if you need it gone.

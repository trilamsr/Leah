# CI gates (operator reference)

PR #287 dropped the reviewer-token + TDD-evidence CI gates in favor of a reviewer-comment audit channel. The remaining machine-enforced rules are listed below; reviewer audit covers what the dropped scripts previously enforced.

## Gate 1 — Reviewer-verdict PR comment (audit channel, replaces former token gate)

Channel: `gh pr comment <N> -b "REVIEWER APPROVE/REVISE: <agent-id>: <11-dim summary>"` posted by the independent reviewer subagent against the PR's current head SHA. See `CLAUDE.md` § TDD + review and `docs/engineer/dispatch-templates/reviewer.md` § OUTPUT FORMAT for the binding rules.

The comment is the audit artifact — operator scans PR comments to verify a real adversarial review ran before arming merge. The former regatta-ported `check-reviewer-verdict.sh` body-footer gate was removed because:

- Footer tokens produced false-pass on operator-pasted text.
- Body edits forced empty-commit refreshes to re-run the gate.
- The audit signal we care about is "did a fresh adversarial review run", which a reviewer-authored PR comment captures more durably than a regex-matched body footer.

Self-tag rejection rule: an operator-posted `REVIEWER APPROVE: ...` comment OR a comment whose `<agent-id>` matches the PR author login → zero adversarial pass. Reviewer subagents enforce this inline per `docs/engineer/dispatch-templates/reviewer.md` § Recurring-failure traps.

Historical motivation (preserved for context): token leaks on #144 / #168 / #184 / #186 / #187 / #189 / #206 motivated the original 2026-06-10 token gate; #287 found the token mechanism itself fragile and moved enforcement to the comment channel.

## Gate 2 — TDD evidence (now reviewer-enforced inline)

The former `scripts/check-tdd-evidence.sh` script + `pr-gates` CI job were removed in #287. Reviewer subagents now enforce the rule inline per `docs/engineer/dispatch-templates/reviewer.md` Definition-of-done: `feat/*` PR bodies must carry a `## TDD evidence` heading paired with a `FAIL` / `panic` / `RED→GREEN` token, OR an explicit `<!-- tdd-skip-justified: <reason ≥32 chars> -->` marker.

Historical motivation (preserved for context): PR #162 / #199 / #219 landed as `feat(*)` with no pre-impl failing-test capture; the original script enforced this until #287.

## Gate 3 — Worktree janitor (launchd)

Install: `make install-janitor`. Verifies `~/Library/LaunchAgents/com.leah.worktree-janitor.plist` exists. Sweeps `.claude/worktrees/agent-*` whose branch is merged or deleted upstream, every 5 minutes. Logs to `~/.leah-state/janitor.log`.

V4 (current): cached `git ls-remote` + `git branch -r --merged` once per sweep (N×network → 2 calls). Wave-9 closeout cleared 354 MB and 129 stale worktree locks accumulated during the parallel-dispatch waves.

Uninstall: `make uninstall-janitor`.

## Emergency override (scoped)

`LEAH_CI_GATES=skip` still bypasses the remaining `./scripts/check.sh` PR-body gates (currently `check-pr-body-close-keywords.sh`); an audit row (`=== LEAH_CI_GATES=skip — pr-body gate bypassed ===`) appears in `/tmp/cicheck.log`. Reserved for breakglass — every use leaves a footprint the next audit-session sees.

Gate 3 is local-host state and not affected by the env var; uninstall via `make uninstall-janitor` if you need it gone.

# Closed-loop validation steps 5-7 — Wave 9 subagent B report

**Date**: 2026-06-09
**Operator**: tri (`trilamsr`)
**Target repo**: `trilamsr/Leah`
**Branch (leah)**: `refactor/daemon-split-main`
**Predecessor**: `docs/research/2026-06-09-closed-loop-live-validation.md` (Wave 7-II)

## TL;DR

Steps 5-7 (regatta picks up `ready-for-agent` issue → opens PR → `leah review` cycle) are **still blocked on the same root cause documented in Wave 7-II**: no regatta orchestrator is running against `trilamsr/Leah`. No code/config change inside leah or regatta will unblock this — operator action required to provision a regatta service instance.

No new issue dispatched; doing so would replicate issue #1's orphan state.

## Findings

### Regatta runtime — absent

- `which regatta` → not on PATH.
- No process matching `regatta|orchestrator` (`ps aux`).
- No launchd entry (`launchctl list | grep regatta` → empty).
- No service state dir (`~/Library/Application Support/regatta/`, `~/.config/regatta/`, `~/.regatta/` all absent).
- No docker-compose stack up for regatta (`docker compose ps` empty in regatta repo).
- No `regatta.yaml` anywhere reachable from leah (`find leah -maxdepth 3 -name regatta.yaml` empty).

### Regatta capability — present in source, not wired

- `internal/orchestrator/adapter/githubissues/adapter.go` implements `schemas.SpecAdapter` polling GitHub issues by label via `ListIssuesByLabelPaginated`. Default label hardcoded to `autonomous`; selector accepts `label:<name>` (closes #1067), so `selector: "label:ready-for-agent"` in regatta.yaml would target leah's issues.
- `cmd/regatta/serve.go` flag set has `-poll 30s` (matches leah's 30s watcher cadence) + `-repo` (worktree root for spawner) + `-spawner claude` (real Claude agent). No `--target-repo` flag — the target repo is sourced from `regatta.yaml::spec_adapter` (type `github_issues` plus repo derived from gh API context / env).
- `internal/orchestrator/spawner/claude.go::defaultPromptBuilder` already supports target-repo CLAUDE.md auto-load + bundled-default fallback (spec 2026-06-08-regatta-on-arbitrary-repo §3 L1) — substrate is ready for an arbitrary target.

### Leah dispatch — working end-to-end

- `internal/dispatcher/selfbuild.go::SelfBuildRepo` is hardcoded to `"trilamsr/Leah"` and locks `--repo` against override (`ErrSelfBuildRepoLocked`).
- `internal/ghclient/ghclient.go::CreateIssue` plus EnsureLabel retry path (Defect-1 fix landed in commit `9f5d483`) reliably files an issue with `ready-for-agent` label.
- Issue #1 from Wave 7-II is `CLOSED` (operator-closed; no PR ever produced because regatta never polled).
- `./leah audit tail` last 11 rows: only `ask` + `self-build clarify` + 2x `ship` (the 23:01:19 failure then 23:02:05 pending). No `self-build dispatched` row, no agent activity, no PR review activity — consistent with no regatta consumer.
- `./leah backlog trilamsr/Leah` confirms: 0 active agents, 0 open ready-for-agent issues, 0 recent PRs.

### Why no test dispatch was filed

Filing a new `leah self-build "add LICENSE file (MIT) at repo root"` issue would:

1. Produce the exact same orphan state as issue #1 (Wave 7-II already proved steps 1-4).
2. Spend Anthropic API budget on a Reasoner spec-draft call (~$0.005-$0.02 per dispatch) with zero validation reward.
3. Add backlog noise on `trilamsr/Leah` that the operator would have to manually close.

Per task constraints ("If regatta service is not running, do NOT start it — file a blocker note") and decision-priority rule (long-term > short-term, validate empirically before spending), the correct call was to report rather than dispatch.

## Dispatched

None.

## Blockers (operator actions)

1. **Provision a regatta orchestrator targeting `trilamsr/Leah`.** Two viable paths:
   - **Native install**: `cd /Users/treedesk/Desktop/Projects/regatta && make install && regatta install-service --name leah --healthz-url http://127.0.0.1:8081/healthz` — then drop a `regatta.yaml` (under the path the install-service unit references) with `spec_adapter: {type: github_issues, selector: "label:ready-for-agent"}` and ensure `GITHUB_TOKEN` (or the secret store wired via `wire_secrets.go`) reaches the process with `repo` scope on `trilamsr/Leah`. Bind `--addr :8081` to coexist with leah's default port 8080.
   - **Docker compose**: bring up the regatta stack documented in `docs/operator/docker-compose.md` (regatta repo) with the same `regatta.yaml` mounted.
2. **Decide regatta-instance vs leah co-location.** Spec `docs/specs/2026-06-08-regatta-on-arbitrary-repo.md` says the substrate is ready; no engineering work is needed inside leah for this. Decision is purely operational: where does the regatta process run, how is its token stored, who watches its logs.
3. **Document the chosen path** in leah `README.md` "First-time install" section — currently line 28-30 says "(assumes you cloned regatta separately; cd into it and `make install` or your usual)" which is too thin to reproduce against a fresh machine and is the underlying cause of backlog item #4 in Wave 7-II.

## Next steps (3-5 bullets for follow-up dispatch waves)

- **Wave 10-A** (operator-driven, not subagent-dispatchable): provision regatta against `trilamsr/Leah` per blocker #1; verify `./leah backlog trilamsr/Leah` shows the regatta process responding within 60s of `leah self-build` dispatch.
- **Wave 10-B** (subagent, after 10-A green): re-dispatch a trivial validation issue (`leah self-build "add LICENSE file (MIT) at repo root"`), watch through PR open → `leah review trilamsr/Leah <pr#>` → operator merge; capture timings + audit rows in a `2026-06-XX-closed-loop-live-validation-v2.md` follow-up.
- **Wave 10-C** (parallel with 10-B): write the operator runbook `docs/operator/regatta-against-leah.md` covering native + compose setup, env vars, log locations, healthz check, troubleshooting — closes Wave 7-II backlog item #4.
- **Defer**: do not pre-build any drift gates / lint scripts for the "regatta target-repo allowlist" surface until at least 2 distinct production target repos exist (per `feedback_default_simpler` in CLAUDE.md).

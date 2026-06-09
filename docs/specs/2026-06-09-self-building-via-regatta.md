---
title: Leah — self-building via regatta orchestrator
status: draft-v1
version: 1.0
phase: design
owner: tri
created: 2026-06-09
parent: 2026-06-09-leah-tier1-self-improvement.md
---

# Self-building via regatta — close the loop

Leah grows Leah. Operator describes a feature in natural language; Leah's Reasoner drafts a structured Leah-feature-spec; the spec routes through the existing dispatcher (`internal/dispatcher/ship.go`) as a regatta issue against `trilamsr/Leah`; regatta opens a PR; Leah's existing `leah review` validates the PR; operator merges. Result: Leah adds a feature to itself with one CLI call.

This spec is the THIN wrapper that closes the loop. It does NOT rebuild dispatch, reasoning, or review — those exist. It adds one prompt, one wrapper struct, and a tagged audit row.

## 1. Goal — the closed loop

```
operator: leah self-build "add a flag to leah ask that prints raw tokens"
   ↓
Reasoner (with prompts/self-build-feature.md system prompt)
   ↓ structured Leah-feature-spec markdown
SelfBuild → Ship (Repo locked to trilamsr/Leah, BR=4, [SELF-BUILD] title)
   ↓
regatta issue → regatta agent opens PR
   ↓
leah review <PR>  (independent reviewer subagent)
   ↓
operator reads + merges manually (no automerge ever)
   ↓
next `leah` invocation runs the new feature
```

Loop closure = compounding capability. Every successful self-build expands the surface Leah can subsequently self-build against.

## 2. CLI usage

```
leah self-build "<feature description>"
```

Example:

```
leah self-build "add --json flag to leah status that emits agent states as JSON for shell scripting"
```

Behavior:

1. Operator intent passed to Reasoner with `prompts/self-build-feature.md` as system prompt (instead of `prompts/regatta-issue.md`).
2. Reasoner returns a structured Leah-feature-spec.
3. SelfBuild wraps a `Ship` with `Repo: "trilamsr/Leah"`, `Title: "[SELF-BUILD] <derived from intent>"`, body = spec.
4. Ship files the issue + watcher fires (same code path as `leah ship`).
5. Audit row: `kind: "self-build"`, `blast_radius: 4`.

Flags:

- `--repo`: **REFUSED** (returns error). Repo is locked to `trilamsr/Leah` so an operator typo can't accidentally self-build into a customer repo, a fork, or `trilam/regatta` itself.
- `--watch` / `--no-watch`: same semantics as `leah ship`. Default `--watch`.
- All other `leah ship` flags forwarded.

## 3. Prompt design — `prompts/self-build-feature.md`

Reasoner system prompt that turns operator intent into a structured Leah-feature-spec. Output sections (Reasoner emits exactly these H2s, no other prose, no greeting):

- `## Title` — single line, prefixed `[SELF-BUILD]` (Reasoner enforces; SelfBuild also asserts).
- `## Motivation` — 2–4 lines: what operator wants + why this advances Leah's self-improvement loop.
- `## Files to create or modify` — bulleted list of paths under `internal/` / `prompts/` / `cmd/` / `docs/specs/`.
- `## Code shape` — sketches of signatures + struct fields. Not full implementation; enough to constrain the regatta agent.
- `## Acceptance criteria` — operator-visible behaviour the merged PR must demonstrate. Bulleted. Each criterion must be observable via `leah` CLI or a Go test.
- `## Test plan` — unit tests + 1 integration test at minimum. Reasoner names test functions.
- `## Deferred` — explicitly out-of-scope follow-ups, prevents scope creep.
- `## Self-build context` — fixed footer Reasoner copies verbatim: BR=4, repo locked, manual merge, independent reviewer mandatory, no automerge.

Reasoner is told (in the system prompt):

- Do not draft prompt-file edits. `prompts/*.md` mutations route through the Tier-1 prompt-review queue, never through `self-build` (see §4 / §7 cuts).
- Do not draft credential / secret / network-side-effect features without naming `BR=4 self-build` + an operator approval gate.
- If operator intent is ambiguous, draft a `## Clarifying questions` H2 INSTEAD of the spec, and leave other sections empty. SelfBuild detects this case and aborts before filing the issue.

## 4. Safety guards

Self-build = self-modifying code. The safety stack:

1. **BR=4 audit row.** `audit.Entry.Kind = "self-build"`, `BlastRadius: 4`. Distinct from `"ship"` (BR=3). Tier-1 retro queries can `WHERE kind = 'self-build'` to count weekly cadence.
2. **Repo lock.** `SelfBuild.Run` ignores any caller-supplied repo and always sets `Ship.Repo = "trilamsr/Leah"`. `--repo` CLI flag returns `error: --repo not allowed on self-build`.
3. **Title prefix.** Every self-build issue title starts `[SELF-BUILD] `. Operator's `gh pr list --repo trilamsr/Leah` distinguishes Leah-built PRs at a glance.
4. **Manual merge only.** Spec body Reasoner emits states `automerge: never`. The regatta-issue body footer template includes a hard line: `Operator merges manually. Do not enable automerge. Do not self-tag Reviewer-recommendation: APPROVE.`
5. **Independent review required.** Regatta's reviewer-verdict gate already requires `Reviewer-agent-id:` on `internal/`-touching PRs; `trilamsr/Leah` paths are load-bearing by definition. No bypass.
6. **Out-of-scope refusal.** Reasoner is instructed (system-prompt §5) to refuse drafting: (a) prompt-file edits, (b) credential-rotation features, (c) network egress to non-allowlisted hosts. If operator intent maps to any of these, Reasoner emits a `## Clarifying questions` block, SelfBuild aborts.
7. **Clarifying-question abort path.** When Reasoner returns `## Clarifying questions`, SelfBuild prints the questions, files NO issue, audit row records `outcome: "clarify"` with `blast_radius: 4`. Operator answers and re-invokes.
8. **No prompt edits via self-build.** `prompts/self-build-feature.md` and `prompts/system.md` are explicitly named in the Reasoner system prompt as forbidden edit targets. Prompt edits route through the Tier-1 prompt-review queue (separate spec).

## 5. Self-improvement integration

Every self-build audit row feeds Tier-1 retro:

- `audit_log` query: `SELECT count(*), strftime('%Y-W%W', ts) AS week FROM audit_log WHERE kind = 'self-build' GROUP BY week`.
- Weekly retro report includes: N self-build PRs filed, M merged, K aborted at clarify-step, mean operator-merge-latency.
- When merged-rate < 50%, retro flags Reasoner prompt drift — `prompts/self-build-feature.md` likely needs revision.
- When clarify-rate > 30%, retro flags operator intent quality — CLI may need an interactive flow.

Cross-link: Tier 1 spec §2.5 already names "self-build cadence" as a retro metric (forward-fit).

## 6. Build order (max 3 tasks)

1. **Prompt + dispatcher wrapper + tests.** `prompts/self-build-feature.md`, `internal/dispatcher/selfbuild.go`, `internal/dispatcher/selfbuild_test.go`. Validates: repo lock, BR=4 audit, title prefix, clarify-abort path. NO CLI wiring yet.
2. **CLI wiring.** `case "self-build":` in `cmd/leah/main.go`. Sequenced AFTER (1) merges to avoid main.go churn collisions with parallel Wave-1 agents. Adds an end-to-end test that exercises the CLI path with a fake Reasoner + fake GHClient.
3. **Tier-1 retro query + dashboard tile.** SQL view + CLI subcommand `leah self-build stats` returning weekly counts + merged-rate + clarify-rate. Blocks on Tier-1 Memory M2 landing first.

## 7. Cuts (explicit non-goals)

- **Automerge on self-build PRs**: never. Operator-merge mandatory.
- **Multi-repo self-build** (e.g. `leah self-build --repo trilamsr/leah-extras`): never. Repo locked.
- **Auto-deploy after merge**: never. Operator runs `go install` themselves.
- **Self-modifying prompts via this path**: never. Prompt edits route through a separate Tier-1 prompt-review queue.
- **Self-building credentials, secrets, or network egress features**: never via this path. Reasoner refuses; operator routes through manual-implement.
- **Self-build chains** (one self-build PR triggering another self-build): never. Each self-build is an isolated operator-initiated action.
- **`--from-issue <N>` shortcut** that takes an existing Leah issue and self-builds against it: deferred. First validate the manual-intent path.
- **Interactive clarify loop** that re-asks Reasoner with operator answers: deferred. v1 prints questions + exits; operator re-invokes with refined intent.

## 8. Adversarial review (self-reviewed; spawn independent reviewer before merging this spec)

Self-modifying systems demand adversarial scrutiny. Findings below are tagged HIGH / MED / LOW with the spec mitigation that addresses each.

### HIGH-1: Recursive feedback / circular validation

Leah self-builds, regatta opens PR, `leah review` validates the PR — Leah reviews Leah. Independent reviewer subagent (canonical agent-id, fresh slot) satisfies the regatta mechanical gate, but the reviewer subagent ITSELF runs inside Leah's process, with Leah's system prompt, on Leah's model snapshot. If Leah has a blind spot, the reviewer subagent inherits it.

Mitigation in spec: §4.5 + §4.6 + §4.4 + §4.3. **Additional mitigation added inline**: §6 task 3 includes a Tier-1 retro metric `external-reviewer-disagreement-rate` — operator (the only truly external reviewer) records each merged self-build PR as "approved as-is" / "approved with edits" / "rejected". Disagreement rate > 20% over a rolling window triggers `prompts/self-build-feature.md` review. Cannot be eliminated; can be measured.

### HIGH-2: Operator habituation to merging self-build PRs without reading

Once self-build is reliable, operator drift: "they all pass; just merge." This is the classic auto-pilot failure mode for any human-in-the-loop safety control. Operator-merge stops being a real review and becomes a rubber stamp.

Mitigation added inline (§4.9): every `[SELF-BUILD]` PR description ends with a randomised 1-line operator-attestation: "Confirm you read the diff by answering: <random factual question about the diff>, e.g. 'How many test functions does this PR add?'" Operator merges by posting the answer as a PR comment + then merging — no answer = no merge ritual. Cost is friction; benefit is preserving the only non-circular safety check.

### HIGH-3: Goodhart's law on "self-improvement features"

Operator says "build a feature that makes Leah self-improve faster." Reasoner builds whatever syntactically resembles self-improvement (e.g. a meaningless metric counter) — not what operator meant. Reward (PR-shipped) is gamed by metric (any PR that compiles + has tests).

Mitigation added inline (§3 + §4.6): Reasoner system prompt §6 explicitly says "if operator intent is abstract or unmeasurable, emit `## Clarifying questions` requesting a specific observable behaviour." SelfBuild aborts on clarify. Forces operator to specify observable acceptance criteria before regatta is dispatched. Cannot solve Goodhart fully; can require explicit operator-named success criterion as a precondition.

### HIGH-4: Lock-out via broken build

A self-build PR merges (operator misses a regression in test output). Next `leah self-build` invocation: `go test ./...` fails on master. Tier-1 retro can't run. Operator can't run `leah` at all if the regression is in `cmd/leah/main.go`. Self-build becomes self-bricking.

Mitigation added inline (§4.10): SelfBuild precondition check — before filing the issue, run `go build ./...` against the operator's local `trilamsr/Leah` checkout (path discovered via `$LEAH_HOME` env or git config). If build fails, abort with "local Leah build is broken — fix master before self-building." Audit row records `outcome: "precondition_fail"`. Cheap, catches the case where prior self-build already bricked master.

### MED-1: Prompt drift — who reviews `prompts/self-build-feature.md`?

The prompt that drafts self-build specs IS itself a Leah artifact that controls Leah's self-modification. If it drifts (operator edits it locally without committing, or a future PR sneaks a change through), every subsequent self-build inherits the drift.

Mitigation: noted as DEPENDENCY in §4.8 + §7 — out of scope for this spec; handled by the Tier-1 prompt-review queue. SelfBuild does NOT itself constrain prompt edits. **Added inline (§4.11)**: SelfBuild logs the sha256 of `prompts/self-build-feature.md` in the audit row's `detail` field at dispatch time. Retro can detect prompt mutation across self-build invocations even before the Tier-1 prompt-review queue lands.

### MED-2: Regatta rate-limit on issues filed via Leah

Operator's GitHub PAT has a 5000 req/hr rate limit (REST) and a separate 2nd-class abuse-detection limit for issue creation specifically. Operator typo-loop (`leah self-build "x" && leah self-build "y" && ...`) could trip abuse detection — locking out the operator's PAT for all of Leah AND regatta.

Mitigation added inline (§4.12): SelfBuild rate-limit — refuse to file more than 3 self-build issues per rolling 24h window (counted from `audit_log WHERE kind = 'self-build' AND outcome != 'precondition_fail'`). Override flag `--rate-limit-override` for one-off bursts. 3/day is a soft ceiling that catches accidental loops, not deliberate work.

### MED-3: Malicious self-build PR exfiltrating credentials

A regatta agent (or a future supply-chain-compromised model) ships a PR that adds code which reads `~/.config/leah/.env`, posts to an attacker-controlled URL, then deletes the call site. Code review by operator catches it IF operator reads the diff (see HIGH-2). If they don't, Leah self-exfiltrates.

Mitigation added inline (§4.13): operator-side defence already exists (manual merge, attestation question). **Additional**: `prompts/self-build-feature.md` §5 (Reasoner system prompt) names `~/.config/leah/`, `~/.aws/`, `~/.ssh/`, `~/.npmrc`, `$HOME/.netrc`, and `os.Environ()`-iteration as forbidden source patterns. Reasoner will emit a clarifying-question abort if operator intent maps to credential access. Not a full defence (Reasoner is the same process being defended), but raises the bar from "silent" to "would need to defeat Reasoner refusal AND operator attestation AND independent reviewer subagent."

### MED-4: Self-build PR introduces a memory-corrupting dep

Self-build PR adds a new module dep. `go mod tidy` runs in regatta agent's sandbox; `go.sum` updates. Operator merges. Next `leah` run pulls the dep, which has a malicious init() function (typo-squat or supply-chain). Defence-in-depth — not unique to self-build, but self-build accelerates the attack surface by automating dep additions.

Mitigation added inline (§4.14): Reasoner system prompt §5 forbids `## Files to create or modify` from naming `go.mod` or `go.sum`. Dep additions route through manual operator PRs only. Self-build is feature-build, not dep-build.

### LOW-1: Watcher hang on stuck regatta agent

If regatta agent picks up the issue but never reaches `merged`/`escalated`/`failed`, watcher polls until `MaxPolls`. Operator gets no notification of stuck state, just silence.

Mitigation: already present in `Ship.watch` via `MaxPolls`. **Improvement added inline (§4.15)**: SelfBuild sets `MaxPolls` such that total watch window ≤ 30 min default; on timeout, emits a notify "self-build watcher timed out — check regatta dashboard." Not a defect, just better UX.

### LOW-2: `$LEAH_HOME` discovery for §4.10 precondition build check

If operator runs `leah self-build` from a directory that isn't the Leah checkout AND `$LEAH_HOME` isn't set, precondition build check fails open or fails with unclear error.

Mitigation added inline (§4.10): precondition order — (1) `$LEAH_HOME` env, (2) walk up from `pwd` looking for `go.mod` with `module github.com/trilam/leah`, (3) skip precondition with WARN log "could not locate Leah checkout; skipping build precondition." Skip-with-warn is correct: lock-out is a defence-in-depth control, not a primary safety control.

### Severity summary

| Severity | Count | Status |
| --- | --- | --- |
| HIGH | 4 | All mitigated inline (§4.9, §4.10, §3 clarify-abort, §6 retro disagreement metric) |
| MED  | 4 | All mitigated inline (§4.11, §4.12, §4.13, §4.14) |
| LOW  | 2 | Mitigated inline (§4.15, §4.10 fallback) |

### Operator decision required before merging this spec

- Confirm the 3/day rate ceiling is right (or set a different number).
- Confirm the attestation-question flow is acceptable friction (HIGH-2 mitigation) — alternative is a typed `--i-read-the-diff` confirmation flag on `gh pr merge`, less friction but easier to drift past.
- Confirm `$LEAH_HOME` discovery order vs. requiring explicit env var.

These three are operator preferences, not safety-load-bearing — spec ships with defaults and operator overrides on first invocation.

## 8.5 Safety guards (revised — augments §4)

The mitigations in §8 add the following items to §4. Implementation references them by number.

- §4.9 — operator attestation question on every `[SELF-BUILD]` PR body.
- §4.10 — `go build ./...` precondition with `$LEAH_HOME` discovery fallback.
- §4.11 — `prompts/self-build-feature.md` sha256 in audit detail.
- §4.12 — 3-per-24h rate ceiling with `--rate-limit-override` escape.
- §4.13 — Reasoner system prompt §5 forbidden source patterns (credentials, env iteration).
- §4.14 — Reasoner system prompt §5 forbidden file targets (`go.mod`, `go.sum`).
- §4.15 — watcher 30-min default timeout with notify.

## 9. References

- `internal/dispatcher/ship.go` — wrapped, not modified.
- `prompts/regatta-issue.md` — pattern reference, not edited.
- `docs/specs/2026-06-09-leah-tier1-self-improvement.md` — parent (retro integration §5).
- `CLAUDE.md` in `trilam/regatta` — the rules regatta agents will follow when picking up self-build issues (TDD, independent reviewer, no self-tagged APPROVE).

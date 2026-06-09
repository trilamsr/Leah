---
title: Leah — Phase X multi-operator / external-share roadmap
status: deferred
phase: x-forward-fit
owner: tri
created: 2026-06-09
parent: 2026-06-09-leah-overview.md
---

# Phase X — Multi-operator + external-share items (deferred)

These items remain in spec for "right for multi-operator robustness / shareable / external-auditable" use. **Out of scope for active build on personal-use Leah** (tri-only consumption, single-Mac primary, no SaaS pitch).

Reopen trigger for any item below: explicit operator decision OR external interest (e.g., someone else wants to run Leah).

Per regatta's `feedback_default_simpler`: don't pre-build for hypothetical drift. Three similar lines beat a premature abstraction.

## Deferred items

### Architecture

- **Cedar policy engine + 22 fixtures** (overview §4.4). Personal-use replacement: 5-10 if-statements in action gateway; per-action-class blast-radius constants in code. Reopen if action_kind count exceeds 30 OR external operator joins.

- **Litestream + SQLCipher bifurcated backup** (overview §4.7a). Personal-use replacement: `restic` to local USB-3 + offsite B2 ($0.005/GB-mo). Single binary, single config. Reopen if WAL replication latency becomes load-bearing OR multi-device sync needed.

- **Per-device API tokens + tailnet ACL + iOS Shortcut bridge** (overview §2.1). Personal-use: laptop-only Phase 1; phone fallback via SSH + claude CLI. Reopen at iPhone-secondary milestone.

- **CAP/RISC OAuth revocation push** (overview §4.7). Personal-use: per-account `leah account revoke <id>` CLI suffices when operator notices breakage. Reopen if compromise scenarios become realistic.

### Workspace dimension

- **Workspace as first-class Memory column on every table** (overview §3.5). Personal-use: defer until tri has 2nd active context (e.g., side project requiring isolation from work email). Memory v1 uses single implicit "default" workspace.

- **Per-workspace persona + signature + voice + accounts** (Tier 3 §10). Personal-use: one persona at a time; workspace switcher is optimization for the future. Reopen at 2nd-workspace event.

- **Knowledge-firewall cross-workspace BR-2** (Tier 2 §3.18). Personal-use: no cross-workspace queries exist for one workspace. Moot.

- **Workspace-tagged audit / mistake / decision rows** (Tier 1). Personal-use: implicit "default" workspace_id. Add column from day 1 BUT skip query-filter logic until 2nd workspace.

### Tier 1 self-improvement

- **Frozen seed-fixture mechanical immutability + CODEOWNERS gate** (Tier 1 §2.16 + §C6). Personal-use: operator IS the only fixture author + reviewer; "frozen" is convention, not gate. Reopen if non-tri fixture contributions become possible.

- **A/B harness SPRT + tool-mock layer** (Tier 1 §2.7). Personal-use volume too low for statistical significance; operator-eyeball-on-Sunday-review suffices. Reopen when call rate > 200/day.

- **Sunday review demoralization-prevention out-of-band burnout warning** (Tier 1 §2.10 + §3a.6). Personal-use: just don't read the bad-news section if you're tired. Reopen if Sunday-review attrition becomes signal-loss problem.

- **Operator-time-saved estimator** (Tier 1 §3.8). Personal-use: subjective; ask yourself weekly. Skip building the heuristic + Goodhart guard.

- **Reviewer-prompts/ separately-versioned approval cadence** (Tier 1 §5 + overview §4.4a). Personal-use: operator approves both prompt sets in the same Sunday review. Keep the separate directory (independence-principle is cheap structurally); drop the separate-approval-track ceremony.

### Tier 2 SWE productivity

- **Spec drafter adversarial subagent review for Leah-own specs** (Tier 2 §3.12). Personal-use: operator reads own drafts; subagent-review for Leah's own design overkill. Reopen if Leah drafts external-facing specs.

- **Cross-PR dependency tracker** (Tier 2 §2.21). Personal-use: 1-2 active PRs at a time; eyeball sufficient. Reopen at >5 active PRs in flight.

- **SPOF surfacer "only you know X"** (Tier 2 §3.17). Personal-use: solo operator = always 100% bus-factor by definition. Moot until collaborator joins.

- **Multi-repo `merge-in-order` rollup state machine + dispatch-gating** (Tier 2 §2.2). Personal-use: rare cross-repo task; operator can sequence manually. Reopen at 3rd cross-repo refactor.

### Tier 3 schedule + comms

- **HMAC-rolling-token email-to-Leah inbox** (Tier 3 §9.1). Personal-use: static signed-token in subject sufficient for single operator. Reopen at multi-operator OR if signed-token leaks once.

- **DKIM verification + sender allowlist** (Tier 3 §9.1). Personal-use: operator's own forwarding address only. Implement minimal sender-domain check (operator's verified accounts); skip full DKIM.

- **Per-contact tone calibration × workspace × account_scope fallback ladder** (Tier 3 §8.5). Personal-use: too sparse to train meaningfully; single-tone-per-contact suffices. Reopen if operator notices per-account voice mismatch.

- **Phone-tag detector + hot-thread tracker + friend-ghost-detector × keep-in-touch unified relationship enum** (Tier 3 §9.4 + §9.14 + §10.27). Personal-use: build only when felt-pain surfaces. Defer all until tri reports specific recurring annoyance.

- **Identity-correct meeting joiner per-OS mechanism (Chrome --profile-directory, Zoom URL scheme)** (Tier 3 §10.7). Personal-use: operator manually picks Chrome profile. Reopen if friction.

- **Hand-off marker `extendedProperties.private[leah.handoff]`** (Tier 3 §10.8). Personal-use: defer; not load-bearing without workspace dimension active.

- **Vacation autopilot per-account fixed-string OOO templates** (Tier 3 §10.21). Personal-use: write OOO manually for now; ~3min effort, twice a year.

- **Operator-mode state machine (focus/standby/asleep/travel/sick/vacation) with 6h auto-revert + OS Focus read-only** (Remaining-tiers cross-tier section). Personal-use: 2-3 modes max (working / not-working); avoid the enum complexity.

### Remaining tiers

- **Plaid + SimpleFIN bank coverage pre-flight + workspace-categorization ladder** (T7.1 + T7.13-17). Personal-use: manual CSV import from each bank if operator wants spend tracking. Defer Plaid + SimpleFIN integration until operator hits ledger-update fatigue.

- **Tax-bucketing handoff (CSV + TXF + receipts.zip)** (T7.15). Personal-use: existing accountant workflow; revisit pre-tax-season Jan 2027.

- **Sherpa visa proxy + freshness pin + 24h cache + consulate-scrape fallback** (T6.4). Personal-use: 1-3 trips/year; just google it. Reopen if frequent traveler.

- **Per-tier cost ceiling enforcement table** (Remaining-tiers cross-tier section + Overview §4.0). Personal-use: single global $cap suffices. Per-tier sub-budget overkill until cost surprises occur.

- **Privacy-tier enumerated consumer matrix + mechanical lint** (Remaining-tiers cross-tier section). Personal-use: tri is the only privacy boundary; operator reviews each integration manually.

- **Universal operator-inbox concurrency model + approval_request idempotency** (Remaining + Tier 1 §3.9). Personal-use: one operator, one device active at a time mostly; race conditions are theoretical.

### Cross-cutting / infra

- **Multi-account OAuth blast-radius + per-account encrypted token at rest + YubiKey gate on refresh** (Overview §4.7 + Tier 3). Personal-use: macOS Keychain alone suffices for OAuth tokens; YubiKey gate optional later.

- **Provider data-flow matrix with per-workspace overlay** (Overview §4.8). Personal-use: single global matrix sufficient; per-workspace overlay adds nothing for one workspace.

- **launchd daemon + healthchecks.io heartbeat + Pushover + Twilio fallback** (Overview §4.7). Personal-use: launchd KeepAlive + healthchecks.io + Pushover sufficient. Skip Twilio SMS fallback until heartbeat misses become problem.

- **Banned-phrase gate on prompts/ + narration outputs** (Overview §9). Personal-use: operator self-edits; CI gate overkill.

- **6 additional regatta CLAUDE.md rule ports** (audit-main-before-implementing, test-coverage-audit-per-wave, trap-projection, double-fail-root-cause, validate-before-ship, keep-orchestrator-branch-name) — Overview §9. Personal-use: keep them in mind, skip mechanical enforcement.

## Reopen-trigger glossary

| Trigger | Items to reopen |
|---|---|
| External operator joins | All multi-operator items above |
| 2nd workspace activated | Workspace dimension items |
| Frequent traveler pattern (>4 trips/yr) | Sherpa visa proxy + travel autopilot |
| 30-day cost surprise on metered API | Per-tier cost ceiling enforcement |
| iPhone secondary milestone | Per-device tokens + tailnet ACL + iOS bridge |
| Multi-Mac home server | Litestream + SQLCipher bifurcation |
| Collaborator added to any repo | SPOF surfacer + spec-drafter-adversarial-review |
| ≥3 active workspaces | Per-workspace tone calibration fallback ladder |
| Compromise of any OAuth token | CAP/RISC + YubiKey gates |
| > 200 LLM calls/day sustained | A/B harness SPRT |
| > 30 action_kind types | Cedar policy engine |
| External-facing Leah spec drafted | Spec drafter adversarial subagent review |

## What stays personal-use

These items remain personal-use because they protect tri-the-operator from tri-the-operator's own forgetfulness / mistakes / runaway cost / data loss. Not multi-operator concerns.

- Hard $cap per process
- JSONL audit (every action one line)
- healthchecks.io heartbeat
- Independent reviewer subagent on regatta PRs (regatta gate requires it anyway)
- Backup + restore drill (restic to USB + B2)
- Memory primitives once usage proves need
- Self-improvement audit→outcome→retro loop once N weeks of data exist
- Reviewer-prompts/ separate directory (cheap structurally)
- Workspace_id column from day 1 on Memory tables (cheap to add; expensive to retrofit)

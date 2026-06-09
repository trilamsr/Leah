---
title: Leah — Tier 1, self-improvement + meta-improvement
status: draft
phase: design
owner: tri
created: 2026-06-09
parent: 2026-06-09-leah-overview.md
---

# Tier 1 — self-improvement + meta-improvement

Leah learning Leah. The compounding layer. Every other tier gets better when this works. Built BEFORE Tier 2 and Tier 3 are mature, because without it the system drifts: prompts go stale, mistakes recur, value is unmeasured.

## 1. Scope

Tier 1 captures:

- What Leah did
- What she intended
- What you wanted
- Whether the outcome matched
- How she can do better next time

It does NOT:

- Auto-modify Leah's own code without operator approval
- Ship prompt changes silently — every prompt mutation reviewable + rollback-able
- Train models on operator data without explicit opt-in
- Operate without operator-visible audit
- **Self-promote fixtures or prevention rules** (independence principle, overview §4.4a — fixture-add + mistake→prevention promotions are separate operator-approval queues, NOT batched with prompt changes)

## 2. Capabilities (expanded from the 20-item list)

### 2.1 Action audit log (foundation)

Append-only structured log of every action Leah takes. Lives in `audit_log` table in Memory's SQLite (SQLCipher at rest per overview §4.3). Backbone for every other Tier 1 capability.

Schema:

```sql
CREATE TABLE audit_log (
  id              TEXT PRIMARY KEY,            -- ulid
  occurred_at     TIMESTAMP NOT NULL,
  intake_event_id TEXT,                        -- backlink to intake (nullable for cron-initiated)
  workspace_id    TEXT,                        -- §3.5 overview (nullable for cross-workspace)
  action_kind     TEXT NOT NULL,               -- "email.draft", "regatta.dispatch", ...
  args_json       TEXT NOT NULL,               -- ProposedAction.Args (redacted; see below)
  body_hash       TEXT,                        -- sha256 of any args.body content
  body_blob       BLOB,                        -- encrypted per-row key wrapped by OS keychain
  why             TEXT,                        -- Reasoner's rationale
  confidence      REAL,                        -- 0..1
  blast_radius    INTEGER NOT NULL,
  approval_path   TEXT NOT NULL,               -- "auto", "notify-after", "approved-by-operator", "denied"
  approval_at     TIMESTAMP,
  outcome         TEXT,                        -- "unknown", "success", "failed", "partial", "rolled-back", "pending"
  outcome_detail  TEXT,                        -- dispatcher-specific (gh issue #, draft id, ...)
  outcome_at      TIMESTAMP,
  reversibility   TEXT NOT NULL,               -- enum: 'trivial-undo' | 'windowed-undo' | 'revert-with-side-effects' | 'irreversible'
  undo_window_s   INTEGER,                     -- non-null when reversibility='windowed-undo' (e.g. Gmail send 30s)
  rollback_path   TEXT,                        -- if rolled back, how
  cost_tokens     INTEGER,
  cost_dollars    REAL,
  operator_signal TEXT,                        -- "thumbs-up", "thumbs-down", "correction", null
  signal_detail   TEXT,
  prompt_versions TEXT                         -- json: {prompt_name: git_sha} at action time (trace replay)
);
CREATE INDEX audit_log_kind_time ON audit_log(action_kind, occurred_at);
CREATE INDEX audit_log_outcome_at ON audit_log(outcome_at);
CREATE INDEX audit_log_workspace ON audit_log(workspace_id, occurred_at);
```

**Redaction layer on write path**: a redactor wraps the audit insert. Sensitive fields (`args.body`, `args.html`, `args.subject` for email; `args.transcript` for voice) are split: short hash in `body_hash` + encrypted blob in `body_blob` (per-row symmetric key, wrapped by OS Keychain master). Queries needing plaintext require explicit `audit unlock <id>` (BR-2 surfaced action). Closes audit-log-as-PII-leak risk.

`prompt_versions` column captures `{prompt_name: git_sha}` at action time so §3.1 trace-replay can pin against the exact prompt set.

Every dispatcher writes one row per execution. Every operator approval/denial appends signal. Every rollback records path.

CLI reads: `leah audit recent`, `leah audit show <id>`, `leah audit grep <pattern>` (only over non-redacted fields), `leah audit since <date>`, `leah audit unlock <id>` (per-row plaintext decrypt, logged).

**Operator-unlock UX**: Touch-ID OR passphrase per unlock; rate-limit 10/hr; auto-relock after 5min idle. Every unlock writes an `audit_log_unlock` row (`actor`, `reason`, `unlocked_id`, `at`). Compacted-row unlock surfaces "body purged, hash retained" instead of attempting decrypt.

### 2.2 Outcome tracking

Did the action achieve intent? Three signals, in priority:

1. **Explicit operator signal** — thumbs/correction/"redo that"
2. **Inferred world signal** — PR merged, email got reply, calendar invite accepted, draft was sent (vs deleted)
3. **Time-decay default** — no negative signal in N days → `unknown` (NOT `success`)

**Outcome default = `unknown`.** Calibration (§2.11) uses only resolved-positive + resolved-negative buckets; surfaces unknown-rate separately. Treating no-signal as success silently inflates calibration.

Outcome inference rules per action_kind live in `internal/selfimprove/outcome/rules/<kind>.go`. Each rule is a small function: `func(action AuditRow, world WorldProbe) (Outcome, confidence float64)`.

World probes (called by outcome tracker on cron):

- `regatta.PRMerged(pr#) → bool` via gh API
- `gmail.DraftStillUnsent(draft_id) → bool` after N days = negative signal
- `gmail.GotReply(thread_id) → bool` after Leah's send = positive
- `gcal.EventStillOnCalendar(id) → bool` (operator may have deleted Leah's block)

Outcome backfill is async — first writes are `pending`, cron job advances them. Ops dashboard surfaces stale `pending` rows for manual resolution.

### 2.3 Feedback capture

Lightweight UI for operator to react to Leah actions. Three surfaces:

- **In-terminal**: every interactive Leah turn ends with hidden `?` prompt; `?+` or `?-` types into next prompt logs feedback for last action.
- **Push notification**: action notifications carry reaction buttons. **macOS UNNotificationAction caps at 4 per category** (https://developer.apple.com/documentation/usernotifications/unnotificationaction accessed 2026-06-09) — design accordingly: morning-brief notification uses at most 4 actions (typical: `dismiss`, `approve-all`, `inbox`, `snooze-1h`).
- **Voice**: "Leah, that was wrong" parses as correction signal for last action; "Leah, good call" as positive.

Corrections (`✏ ` / `that was wrong because ...`) are higher-signal than thumbs: they include WHY. Stored as `operator_signal = "correction"` + `signal_detail = <reason>`. Feeds mistake log directly.

### 2.4 Mistake log

Curated subset of audit-log rows where outcome was negative. Each entry annotated with:

- Root-cause tag (`misclassified_intent`, `wrong_tone`, `missing_context`, `bad_timing`, `stale_memory`, `policy_violation`, `dispatcher_bug`)
- Prevention rule (text, op-readable)
- **Detector predicate** (executable; see §C2 below)
- Reference to audit row(s)
- Status (`open`, `prevented`, `accepted`)

Schema:

```sql
CREATE TABLE mistake_log (
  id                 TEXT PRIMARY KEY,
  created_at         TIMESTAMP NOT NULL,
  workspace_id       TEXT,
  audit_ids          TEXT NOT NULL,                  -- json array
  root_cause         TEXT NOT NULL,
  prevention         TEXT NOT NULL,
  detector_predicate TEXT,                           -- cel-go expression; runs at intake-event time
  workspace_scope    TEXT NOT NULL DEFAULT 'same',  -- 'same' (default fires same-workspace only) | 'any'
  status             TEXT NOT NULL,
  prevented_at       TIMESTAMP,
  fire_count         INTEGER NOT NULL DEFAULT 0,    -- # times predicate matched + Leah avoided the mistake
  notes              TEXT
);
```

**Predicate language: cel-go ONLY** (google/cel-go, https://github.com/google/cel-go accessed 2026-06-09, Apache-2.0). Sandboxed, typed, designed for predicates (Kubernetes admission policy + Google IAM conditions use it). Example:

```cel
event.kind == "email.received"
  && event.sender.endsWith("@recruiter.example")
  && contains(event.body, "opportunity")
```

**Prevention predicate workspace scope**: default `workspace_scope = 'same'` — fires only on intake events in the same workspace as the mistake's origin. `workspace_scope = 'any'` requires explicit operator-set + surfaces in Sunday review.

**Mistake → prevention closure metric**: `fire_count` increments each time the predicate matches a fresh IntakeEvent that Leah would have repeated the mistake on. `fire_count > 0` = loop closed for that mistake. Sunday review surfaces zero-fire predictions as suspect.

Mistake → prevention path: each entry produces a candidate `feedback_*` style rule that ends up in Leah's own `CLAUDE.md` after operator-approved Sunday review (see §2.20). Mirrors regatta's lessons-learned mechanism but automated. **Per overview §4.4a**: promotion goes through a SEPARATE operator-approval queue, not the prompt-change batch.

### 2.5 Decision journal

Non-trivial Leah choices logged with reasoning. Different from audit log (which logs *every* action). Decision journal logs choices that weren't obvious — Reasoner emits a journal entry when confidence < 0.85 OR when blast-radius ≥ 4 OR when the chosen path had a credible alternative.

Schema:

```sql
CREATE TABLE decision_journal (
  id                    TEXT PRIMARY KEY,
  occurred_at           TIMESTAMP NOT NULL,
  workspace_id          TEXT,                  -- null for cross-workspace decisions
  referenced_workspaces TEXT,                  -- json array; non-null when workspace_id is null
  audit_id              TEXT NOT NULL,
  question              TEXT NOT NULL,
  alternatives          TEXT NOT NULL,
  chosen                TEXT NOT NULL,
  chosen_reason         TEXT NOT NULL,
  outcome_quality       INTEGER,               -- 1..5, backfilled from outcome + operator signal
  retro_notes           TEXT
);
```

Reviewable weekly. Operator can see HOW Leah thinks, catch systematic biases. Decision-journal entries also feed §2.16 calibration.

### 2.6 Prompt versioning

Every Leah prompt lives in `prompts/<name>.md`. Each prompt has YAML frontmatter with `version`, `parent` (predecessor version), `purpose`. Prompts are in git — full diff history, rollback via `git revert`.

Prompts under management:

- `intent-classifier` — top-level intake → intent route
- `email-triage` — classify inbound email urgency + needs-reply
- `email-drafter` — first-draft reply
- `meeting-prep` — meeting brief
- `daily-brief` — morning summary
- `evening-shutdown`
- `weekly-review`
- `regatta-dispatch` — formats issue body
- `independent-reviewer` — dispatched to subagent reviewing regatta PR
- `intent-classifier-voice` — variant for voice-input (different priors)

Active version pinned per prompt in `prompts/active.yaml`. Changes require commit; CI gate (later) rejects uncommitted prompt files at runtime. **Boot-time load only**: `prompts/active.yaml` is loaded at daemon process boot; swap requires daemon restart. Avoids "did the file change apply" surprise mid-run.

### 2.7 A/B harness

When a prompt mutation is proposed, run new (B) and active (A) in parallel. Both produce a `ProposedAction`; only A's action executes; B's action is logged + scored against A's eventual outcome.

**Sample size: sequential Bayesian (SPRT-style) — NOT fixed N=20.** Run continues until posterior probability that B is better crosses upper threshold OR lower threshold OR a max-sample cap (default 100) is hit. Avoids the under-powered comparisons fixed-N produces.

**Tool-mock layer for B's would-be side-effecting actions** — when B would call `gmail.SendDraft`, route through `gmail.MockDispatcher` that records intent without sending. Lets A/B run on BR≥2 prompts without doubling real-world side-effects. Cost-doubling (both A and B incur reasoning cost) is acknowledged and accounted for in the §4.0 cost-cap budget (Tier 1 daily budget).

After threshold cross, produce a comparison report: outcome quality, confidence calibration, cost. Operator approves B → A swap or keeps A. If neither dominates, log inconclusive and keep A.

A/B is OFF by default for blast-radius ≥ 4 prompts on live data — running an experimental email-drafter against real outbound mail still requires mock dispatchers OR explicit opt-in per prompt.

**SPRT prior + cap**: H0 = parity (effect-size 0); H1 = δ ≥ 0.05 (5 percentage-point lift). Per-experiment cost ceiling $5 (Tier 1 budget); max **3 concurrent experiments**.

**Per-workspace pooling**: non-persona-sensitive prompts (e.g. `intent-classifier`) may pool across workspaces for sample-size. Persona-sensitive prompts (`email-drafter`, `meeting-prep`) **NEVER pooled** — per-workspace voice would be diluted.

### 2.8 Recurring-pattern detector

Cron job (weekly) over audit_log: cluster actions by `action_kind + args.shape + intake.source + workspace_id`. Surface clusters with frequency ≥ N OR identical operator-signal outcome ≥ M times. Workspace partitioned: a pattern in `acme` is not a pattern in `personal`.

Examples:

- "You drafted-and-sent identical 'thanks!' replies to 12 recruiter emails this week" → propose template
- "You ran `git rebase --interactive` in 5 different worktrees" → propose skill
- "You replied 'no thanks' to 8 podcast invitations" → propose polite-decline template

Output: candidate skill specs (text) for operator to approve. Approved skills become first-class Leah actions and shorten future Reasoner work.

### 2.9 Skill discovery

Sibling of §2.8. Detector watches operator's terminal commands (opt-in shell hook) and outside-Leah workflows. Same surface — proposes Leah-managed skill where operator is doing rote work.

Skill specs include: trigger condition, action plan, blast radius, expected frequency, **workspace scope**.

### 2.10 Self-critique loop (Sunday review)

Once per week (Sunday morning by default, configurable), Leah generates a Self-Review report from the past 7 days of audit + outcome + mistake + decision data. Per-workspace breakdown + overall rollup. Report sections:

1. **Outcomes**: success/fail/unknown ratio by action_kind (workspace-bucketed); biggest wins; biggest misses
2. **Mistakes**: new entries since last review; root-causes; detector predicates with `fire_count`
3. **Calibration**: confidence vs outcome (resolved buckets only)
4. **Cost vs value**: tokens + dollars spent (per workspace, per feature) vs operator-reported usefulness
5. **Drift**: rules in CLAUDE.md / preferences that Leah systematically violated
6. **Top-3 improvements**: concrete, ordered by impact × ease
7. **Skill candidates** (from §2.8 + §2.9)
8. **A/B results** (from §2.7) ready for decision
9. **3-option presenter** outputs for the week (see §C-decisions)

Operator reviews + approves/rejects each item in **independent queues** per overview §4.4a:

- Prompt-change batch (composable diff)
- Fixture-addition queue (separate; see §2.16)
- Mistake→prevention promotion queue (separate)
- Skill-registration queue
- CLAUDE.md-rule-add queue

Approved → applied. Rejected → archived with reason (so we don't re-propose).

Self-review is itself a Leah action with blast radius 4 — produces a markdown report dropped into operator's inbox + saved under `reviews/YYYY-WW.md`.

### 2.11 Confidence calibration

For every audit row with both `confidence` and final RESOLVED `outcome` (i.e. excluding `unknown`), plot calibration: at confidence 0.7, what % actually succeeded? Ideal = identity line.

Surface miscalibration in Sunday review. Persistent over-confidence → tune confidence-emission prompts down. Persistent under-confidence → flag operator may be over-correcting low-confidence proposals.

Implementation: simple bucketing into deciles + Brier score over resolved set. Stored in `calibration_snapshots` table for trend. Unknown-rate also surfaced.

**Per-workspace + overall**: Sunday review surfaces both per-workspace bucket AND overall rollup. **Data-sparsity fallback**: when a workspace has < 30 resolved actions, that workspace's per-workspace card falls back to overall + flags "low-sample."

### 2.12 Anti-pattern catalog

Leah's own `feedback_*` style rules. Each rule has: name (kebab-case slug), trigger (what would violate it), why (operator-citation rationale), how-to-apply (when in execution to consult).

Storage: `prompts/lessons/<slug>.md`. Frontmatter includes `name`, `description`, `metadata.type`, body has `Why:` + `How to apply:` + linked `[[other-lesson]]`.

Inserted at the top of relevant prompts at run-time (via prompt-templating layer).

### 2.13 Adversarial self-review

For ProposedActions with blast radius ≥ 4, Leah's Reasoner outputs a `Plan`; then a separate critic subagent receives `(Plan, full context)` and tries to refute. If critic finds material flaw → Reasoner re-plans OR action escalates to operator with both views shown.

Critic uses a separate prompt (`prompts/critic.md`) with different framing: assume the plan is wrong; find why. Different from Reasoner — never re-uses Reasoner's reasoning text directly. **Critic runs in fresh subagent runtime, not the Reasoner's context** (overview §4.4a).

Tradeoff: doubles inference cost on high-blast actions. Worth it. **Skip predicate**: `reversibility in ('trivial-undo', 'windowed-undo') AND confidence >= 0.95` (configurable).

### 2.14 Drift detector

Periodic (weekly + on-demand): scan operator's preferences + CLAUDE.md rules vs recent action patterns. Flag drift.

Sunday review surfaces drift; operator confirms (rule still applies, prompt needs tuning) or accepts (rule out of date, retire).

### 2.15 Cost / value ledger

Per feature (action_kind family): token spend + dollar cost + operator-reported usefulness. **Per workspace** (Tier 7 expense isolation alignment). **Cross-workspace attribution**: a Reasoner call spanning workspaces (e.g. cross-workspace KB query) attributes to `cross-workspace` bucket; Sunday review surfaces its share.

Sunday review surfaces:

- Features in top decile of cost AND bottom decile of value → kill candidates
- Features in top decile of cost AND top decile of value → safe; ensure A/B harness keeps them tuned
- Features in bottom decile of cost AND top decile of value → champion; expand use

Inspired by regatta's `feedback_deletion_default` — every Sunday review answers "what got smaller?"

### 2.16 Personal benchmark suite (frozen + open subsets)

Two subsets:

- **Frozen seed-fixture set** — operator-curated fixtures Leah CANNOT extend autonomously. Drift on the frozen set is a high-signal regression flag (something fundamental broke).
- **Open fixture set** — grown over time from operator-approved fixture-addition queue (per overview §4.4a).

**Bootstrap**: `leah bench seed --workspace <ws> --intent <kind>` CLI; operator manually adds 5-10 fixtures per major intent BEFORE T1.16 ships. Without seed, the frozen set is empty and no regression signal exists.

**Close-trigger**: each frozen fixture-set is closed when operator runs `leah bench freeze --intent <kind>` — adds a git tag `frozen/<intent>/v1` + updates `bench/frozen/<intent>/.frozen-at`. Re-opening requires `leah bench unfreeze --intent <kind> --reason <text>` (BR-4 logged action).

**Mechanical immutability**: CI gate `scripts/check-frozen-fixtures.sh` rejects any commit touching `bench/frozen/**` unless commit body carries `Operator-frozen-edit: <reason>` token AND commit is operator-signed (GPG signature key in `~/.leah/operator-key.pub`). Cited in §6 build order T1.16.

Adding to the **open set** is a Leah PROPOSAL, not a Leah ACTION: after a mistake is logged + prevention proposed, the original intake + corrected outcome is surfaced as a candidate fixture in the **fixture-addition queue** — separate from the prompt-change batch. Operator approves before it enters the regression suite.

Storage: `bench/frozen/<intent>/<fixture-id>.json` + `bench/open/<intent>/<fixture-id>.json`. Tests live in `internal/reasoner/bench_test.go`; frozen set runs at every CI, full set runs on prompt-change PRs.

### 2.17 Memory accuracy probe

Cron (daily): pick a random row from `verified_facts` → generate a question Leah should be able to answer correctly → ask Leah → compare to ground truth.

**Ground truth comes from `verified_facts` table only** — populated EXCLUSIVELY by operator-confirmed corrections (mistake-log `signal_detail` rows where `operator_signal = "correction"`). Probe NEVER asks Leah about unverified Memory entries (would test against potentially-wrong-self → useless).

```sql
CREATE TABLE verified_facts (
  id            TEXT PRIMARY KEY,
  workspace_id  TEXT,
  fact_kind     TEXT NOT NULL,                 -- "contact-attr", "decision-recall", "vocab", "org-detail", ...
  question      TEXT NOT NULL,
  answer        TEXT NOT NULL,
  source_audit  TEXT NOT NULL,
  verified_at   TIMESTAMP NOT NULL,
  expires_at    TIMESTAMP                      -- per-fact_kind TTL; see below
);
```

**Per-`fact_kind` TTL**:

- `contact-attr` — 6 months
- `decision-recall` — indefinite (no expiry)
- `org-detail` — 12 months
- `vocab` — indefinite

Expired rows surface in the Sunday review for batch re-confirmation; rows not re-confirmed move to `archived`.

Wrong answers create a `memory_probe_failure` row + open a tracker. Memory layer treated as load-bearing; probe is regression-test against operator-visible mistakes.

### 2.18 Output-diff explanations

When Leah's behavior changes meaningfully (active prompt swap, A/B win, new rule in catalog), she narrates: "I used to summarize Slack threads in 3 bullets; now I lead with the decision. Reason: your 2026-05-22 correction." Surfaces in Sunday review + on-demand `leah explain "<feature>"`.

Storage: `behavior_diff` log, generated automatically on prompt commits via post-commit hook.

### 2.19 External feedback ingest

When operator tells someone else "Leah said X and was wrong", they can email Leah's dedicated address (`leah@tri.example`) → ingested as correction signal, attached to last matching action by content match. Tier 3 §9.1 prompt-injection hardening applies (allowlist + DKIM + shared-secret subject).

Lower priority — useful when Leah operates more autonomously (Phase 5+). Phase 1 just exposes the inbox; matching is manual.

### 2.20 Self-update cadence (Sunday batch + sibling queues)

All proposed changes land in independent queues at the Sunday review surface — see §2.10. Each queue applies atomically; Leah auto-runs benchmark suite (frozen subset) on every queue apply; on regression, reverts and re-surfaces.

## 3. Newly identified capabilities

### 3.1 Trace replay

For any historical audit row: replay the same intake event through current prompts; show what Leah would do today vs. what she did then. Uses the `prompt_versions` column (§2.1) to pin the original prompt set and current `prompts/active.yaml` to pin today's.

CLI: `leah trace replay <audit_id>`. Read-only; never re-executes the action.

### 3.2 Causality graph

For any operator-reported failure ("Leah messed up X yesterday"), walk back the chain. CLI: `leah why <audit_id>`. Mirrors regatta's `feedback_root_cause`.

### 3.3 Privacy ledger

Track which IntakeEvents went to which LLM provider with which data, mapped against the overview §4.8 provider matrix. For every cloud call, log: provider, model, data class, prompt token count, completion tokens, redaction policy applied, workspace. Operator runs `leah privacy report` to see "I sent N emails to Anthropic last month, K Slack messages to OpenAI, …" Mechanical lint catches any cell violation (e.g. medical → cloud).

### 3.4 Confidence floor enforcement

Per action_kind, a minimum confidence threshold below which Leah will NOT propose. Default thresholds in `internal/actiongateway/policy.schema.cue::confidence_floor`. Below-threshold actions go to "Leah is asking" queue.

### 3.5 Counterfactual outcome marker

When operator says "no, I would have done X", capture X as the counterfactual. Lets §2.7 A/B harness use operator-labeled counterfactuals as a comparison signal.

### 3.6 Leah's "I don't know" disposition

Explicit signal Leah can emit. Triggered when: confidence below floor (§3.4), OR Memory has conflicting entries, OR question references unknown vocabulary (acronym not in workspace's vocab).

### 3.7 Audit log compaction

Audit log grows fast. Compaction policy:

- Last 90 days: full rows (including encrypted `body_blob`)
- 90d–1y: kind + outcome + signal + redaction hashes only; **`body_blob` purged**; `body_hash` retained so outcome trace stays auditable
- > 1y: aggregate-only (counts per kind per month per workspace)

Compacted-row `audit unlock <id>` surfaces "body purged, hash retained" instead of attempting decrypt.

Mistakes, decisions, calibration snapshots, verified_facts survive compaction.

### 3.8 Operator-time-saved estimator

For each completed action, estimate the operator-minutes-saved. Per-kind heuristics in `internal/selfimprove/timesaved.go`. Sunday review surfaces aggregate per workspace.

### 3.9 Operator-inbox concurrency model (approval_request table)

Multi-device approval surface (overview §2.1) needs atomic claim. Schema:

```sql
CREATE TABLE approval_request (
  id                 TEXT PRIMARY KEY,           -- ulid
  proposed_action_id TEXT NOT NULL,              -- audit_log ref
  workspace_id       TEXT,
  state              TEXT NOT NULL,              -- 'pending' | 'claimed-by-<device>' | 'approved' | 'denied' | 'expired'
  claimed_at         TIMESTAMP,
  claimed_by         TEXT,                       -- device hostname
  decided_at         TIMESTAMP,
  decided_by         TEXT,                       -- 'operator-mac', 'operator-iphone'
  idempotency_key    TEXT NOT NULL UNIQUE,
  expires_at         TIMESTAMP,                  -- default created + 24h; surfaces 'auto-deny + audit row'
  created_at         TIMESTAMP NOT NULL
);
```

Semantics:

- Approval acted on twice (mobile + desktop simultaneously): atomic `UPDATE WHERE state='pending'` returning rowcount; second click no-ops with "already decided" notification.
- Mode-switch (`leah mode focus`): pending approvals stay pending but suppress notification fires; on mode-clear, re-surface in batch.
- Stale approvals: `expires_at` (default 24h) → auto-deny + audit row.

## 3a Decision-support capabilities

These cover meaningful operator decisions where Leah's role is to clarify, frame, and (within blast-radius bounds) decide.

### 3a.1 3-option presenter

For decisions surfaced to operator: Leah presents exactly 3 options, each annotated with `tradeoff` + `recommendation` + `reversibility`. Format:

```
Decision: <topic>
1. <option-a> — tradeoff: <…>. (reversible, recommend)
2. <option-b> — tradeoff: <…>. (reversible)
3. <option-c> — tradeoff: <…>. (irreversible)
```

Forces structure on otherwise unbounded "what should I do" prompts. Logged in decision_journal.

### 3a.2 "You decide" default-decision mode

Operator enables per-topic (`leah you-decide email-replies`, `leah you-decide tier4-tabs`). In that topic, Leah picks per stated prefs, logs the decision in decision_journal, and acts — BUT only within configured blast-radius ceiling (default ≤ 3). The "stated prefs" are pulled from Memory.preferences + recent operator corrections. Above ceiling → falls back to 3-option presenter.

### 3a.3 Reversibility-first framing (enum-driven)

Every decision presented to operator carries an explicit `reversibility` enum:

- `trivial-undo` — pure local revert (draft delete, calendar block remove). Nudge: *"decide in 10s, fully local."*
- `windowed-undo(N s)` — short undo window (Gmail send 30s, Slack post 30s). Nudge: *"decide in 30s; you have N seconds to undo."*
- `revert-with-side-effects` — reversible but observers notified (calendar invite-then-cancel, PR closed-after-open). Nudge: *"reversible, but others will see."*
- `irreversible` — no undo (email read by recipient, password-reset, money movement). Nudge: *"this one sticks; sleep on it if unsure."*

Audit_log `reversibility` column carries this enum (§2.1).

### 3a.4 No-decide-after-9pm rule

Configurable quiet hours (default 21:00 - 07:00 local): Leah holds non-urgent decision surfaces until morning brief. Urgent = BR-5 OR explicit operator override. Reduces decision-fatigue + bad late-night calls.

### 3a.5 "You decided X 3mo ago" recall

When a topic resurfaces and decision_journal has a prior entry: surface the prior decision + outcome quality before re-deciding. Avoids re-litigating settled questions; respects operator's past self.

### 3a.6 Burnout early-warning (mid-week proactive ping, NOT Sunday review)

Mid-week proactive ping (out-of-band from Sunday review). Trigger: **3+ consecutive days** of (declining commit cadence OR missed daily briefs OR thumbs-down spike OR correction-rate ≥ 2× rolling 30-day avg). Action: TTS + push: *"I'm noticing X. Want me to redirect non-urgent to morning brief for next 24h?"* — one-tap action button.

Multi-signal classifier over audit log + journal + calendar + sleep budget (Tier 3) + energy load (Tier 3). Threshold tuned operator-specific.

Sunday review **weights wins-first** (≥ 50% of body); demoralization signals only mention "burnout-warning fired N× this week, see mid-week log." Prevents Sunday review itself from becoming a stressor.

## 4. Data model

### 4.1 Tables

Inline schemas; 5-7 columns each:

```sql
CREATE TABLE calibration_snapshots (
  id            TEXT PRIMARY KEY,
  taken_at      TIMESTAMP NOT NULL,
  workspace_id  TEXT,                          -- null = overall rollup
  decile_json   TEXT NOT NULL,                 -- {0.0..0.1: {n, success_rate}, ...}
  brier_score   REAL NOT NULL,
  unknown_rate  REAL NOT NULL
);

CREATE TABLE behavior_diff (
  id           TEXT PRIMARY KEY,
  occurred_at  TIMESTAMP NOT NULL,
  prompt_name  TEXT NOT NULL,
  before_sha   TEXT NOT NULL,
  after_sha    TEXT NOT NULL,
  summary      TEXT NOT NULL                   -- operator-facing one-liner
);

CREATE TABLE memory_probe_failure (
  id            TEXT PRIMARY KEY,
  occurred_at   TIMESTAMP NOT NULL,
  workspace_id  TEXT,
  verified_fact_id TEXT NOT NULL,
  leah_answer   TEXT NOT NULL,
  ground_truth  TEXT NOT NULL,
  resolved_at   TIMESTAMP                      -- non-null when tracker closed
);

CREATE TABLE ab_experiments (
  id             TEXT PRIMARY KEY,
  prompt_name    TEXT NOT NULL,
  variant_b_sha  TEXT NOT NULL,
  started_at     TIMESTAMP NOT NULL,
  workspace_id   TEXT,                         -- non-null when not pooled; null = pooled cross-workspace
  status         TEXT NOT NULL                 -- 'running' | 'concluded-a' | 'concluded-b' | 'inconclusive'
);

CREATE TABLE ab_results (
  experiment_id     TEXT PRIMARY KEY,
  ended_at          TIMESTAMP NOT NULL,
  samples_n         INTEGER NOT NULL,
  posterior_b_win   REAL NOT NULL,             -- 0..1
  effect_size       REAL,                      -- δ (B - A) on outcome quality
  cost_total_usd    REAL NOT NULL,
  verdict           TEXT NOT NULL              -- 'a-wins' | 'b-wins' | 'inconclusive'
);

CREATE TABLE feedback_signals (
  id           TEXT PRIMARY KEY,
  occurred_at  TIMESTAMP NOT NULL,
  audit_id     TEXT NOT NULL,
  workspace_id TEXT,
  signal_kind  TEXT NOT NULL,                  -- 'thumbs-up' | 'thumbs-down' | 'correction' | 'snooze' | 'redo'
  detail       TEXT
);

CREATE TABLE cost_ledger (
  id              TEXT PRIMARY KEY,
  occurred_at     TIMESTAMP NOT NULL,
  audit_id        TEXT,                        -- null for non-action costs (probe, calibration)
  workspace_id    TEXT,                        -- 'cross-workspace' sentinel for cross-ws Reasoner calls
  action_kind     TEXT NOT NULL,
  cost_tokens_in  INTEGER NOT NULL,
  cost_tokens_out INTEGER NOT NULL,
  cost_dollars    REAL NOT NULL
);
```

Existing tables documented above:

- `audit_log` (§2.1) — redaction layer + prompt_versions + workspace_id + reversibility enum + undo_window_s + audit_log_unlock companion
- `mistake_log` (§2.4) — detector_predicate (cel-go) + workspace_scope + fire_count
- `decision_journal` (§2.5) — workspace_id + referenced_workspaces
- `verified_facts` (§2.17) — operator-confirmed ground truth + per-fact_kind TTL via expires_at
- `approval_request` (§3.9) — multi-device approval atomic claim

### 4.2 Backfill jobs (single coalesced scheduler)

Per overview §3.5 / overview Tier 1 §L4: ONE scheduler goroutine with priority queue + jitter coalesces all cron jobs. SQLite WAL + `busy_timeout = 5000ms` documented in `internal/cron/README.md`. **No parallel cron processes** — prevents lock contention seen in regatta's goose package-global race.

Scheduled jobs:

- Outcome resolver (every N min)
- Memory probe (daily)
- Pattern detector (weekly)
- Calibration snapshot (daily)
- Drift detector (weekly)
- Sunday review generator (Sunday 7am)
- Detector predicate firing check (per intake event, in-process — not cron)

## 5. Self-improvement layer security

Self-improvement layer reads everything but writes carefully.

- Prompt commits authored by Leah signed with a dedicated SSH key kept in OS keychain (separate from operator's personal key). Audit-able.
- Prompt commits go to a `leah-self-update/YYYY-WW` branch and require operator-approved Sunday batch merge — Leah never pushes to `main` without operator sign-off.
- **Fixture-additions are write-allowed by Leah ONLY to the open subset AND only into the operator-approval queue, separate from prompt batch** (§2.16, overview §4.4a). Frozen subset is read-only to Leah.
- **Mistake → prevention promotions go through a separate operator-approval queue**, not the prompt batch. Leah does NOT self-tag a prevention as "applied."
- Memory writes from self-improvement layer are limited to: outcome backfill, mistake log, decision journal, verified_facts (only from confirmed corrections), behavior_diff. Never touches contacts/threads/projects/preferences directly.

## 6. Build order (Tier 1 specific, slots into system M3)

| Step | Deliverable | Dep |
|---|---|---|
| T1.0 | `audit_log` table + redaction layer + every dispatcher writes rows | needs Action gateway (M0) |
| T1.0a | `verified_facts` table | T1.0 |
| T1.1 | `decision_journal` table + Reasoner emits on confidence < 0.85 or BR≥4 | T1.0 |
| T1.2 | Operator-feedback capture: terminal `?+/?-` parser, notification action handlers (≤4 actions) | T1.0 |
| T1.3 | `mistake_log` + manual entry CLI + detector_predicate column + fire_count | T1.0 |
| T1.4 | Outcome resolver cron + per-kind world probes (default = `unknown`) | T1.0 + single scheduler |
| T1.5 | Prompt-versioning layout under `prompts/` + active.yaml + runtime loader | independent |
| T1.6 | Anti-pattern catalog (`prompts/lessons/*.md`) + prompt-template injection | T1.5 |
| T1.7 | Adversarial critic subagent + BR≥4 plan-then-critic flow (fresh-runtime) | T1.5 |
| T1.8 | Cost ledger + per-kind aggregation + per-workspace bucket | T1.0 |
| T1.9 | Confidence calibration daily snapshot (resolved buckets only) | T1.0 |
| T1.10 | Behavior-diff post-commit hook + log table | T1.5 |
| T1.11 | Recurring-pattern detector weekly job (workspace-partitioned) | T1.0 + T1.4 |
| T1.12 | Sunday review generator + sibling-queue split | T1.0–T1.11 |
| T1.13 | A/B harness + SPRT sample-size + tool-mock layer | T1.5 + T1.12 |
| T1.14 | Personal benchmark suite (frozen + open) + CI fixture-runner + `leah bench seed/freeze/unfreeze` CLI + `scripts/check-frozen-fixtures.sh` mechanical immutability + operator-GPG-key signature | T1.5 + T1.4 |
| T1.15 | Memory accuracy probe daily (verified_facts only) | T1.0a + Memory layer (M2) |
| T1.16 | Trace replay (prompt_versions pin) + causality-graph CLIs | T1.0 + T1.5 |
| T1.17 | Privacy ledger + report (mechanical lint vs overview §4.8) | T1.0 |
| T1.18 | Operator-time-saved estimator | T1.0 + T1.4 |
| T1.19 | Confidence-floor enforcement in Action gateway | T1.0 |
| T1.20 | Audit-log compaction job | T1.0 (run only after 90d) |
| T1.21 | 3-option presenter + decision_journal hookup | T1.1 |
| T1.22 | "You decide" mode + per-topic preference store | T1.21 |
| T1.23 | Reversibility-first framing in presenter | T1.21 |
| T1.24 | No-decide-after-9pm scheduler | T1.21 + single scheduler |
| T1.25 | "You decided X 3mo ago" recall | T1.1 |
| T1.26 | Burnout early-warning classifier | T1.0 + Tier 3 sleep/energy |

T1.0 through T1.5 is the minimum viable self-improvement layer. T1.6–T1.12 ships the Sunday review. T1.13–T1.26 are enrichment.

## 7. Open questions

- How granular should `action_kind` taxonomy be? Default 2-level (`email.draft`, `email.send`, `email.archive`).
- Decision journal threshold (confidence < 0.85) tunable; need to see real distribution before locking.
- Cost-of-self-improvement: tier 1 itself burns tokens — budgeted at 15% of cap (overview §4.0); Sunday review surfaces if exceeded.
- Where does the personal benchmark suite live? Same repo as prompts (versioned together).

## 8. Success criteria

After Tier 1 fully delivered (M3 + Sunday-review live for 4 weeks):

- 100% of actions logged in audit with redaction enforced (no plaintext bodies outside `body_blob`)
- ≥80% of audit rows have resolved outcome within 7 days (resolved = success | failed; unknown excluded from "resolved")
- Unknown-rate trends downward week-over-week (target ≤ 25% by week 4)
- Sunday review delivered 4 weeks in a row, ≥3 proposals per week
- Operator approval rate tracked per queue independently
- ≥1 detector_predicate fires (`fire_count > 0`) — confirms mistake → prevention loop closes
- A/B harness runs ≥3 experiments end-to-end with SPRT termination
- Calibration Brier score improves over 4 weeks (resolved set)
- Cost vs value ledger drives ≥1 feature kill OR ≥1 feature expansion
- Per-workspace cost-ledger surfaces dollar share per workspace

## 9. Cuts (Tier 1)

- No auto-deployment of Leah-modifications (always operator-approved via Sunday batch + independent sibling queues)
- No model-finetuning on operator data (only prompt + memory editing)
- No external benchmark publishing (private to operator)
- No multi-operator anti-pattern catalog (single-operator scope)
- No real-time pattern detection (weekly cron sufficient initially)
- **No Leah-self-tagged fixture promotions** (overview §4.4a)
- **No Leah-self-tagged mistake → prevention promotions** (overview §4.4a)


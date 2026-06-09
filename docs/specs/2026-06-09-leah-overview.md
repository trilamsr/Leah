---
title: Leah — personal AI chief-of-staff (system overview)
status: draft-v2.1
version: 2.1
phase: design
owner: tri
created: 2026-06-09
---

# Leah — system overview

Leah is a personal AI chief-of-staff for a single operator (`tri`). She listens across multiple input channels, reasons over a persistent memory of the operator's life and work, takes approval-gated actions in the world, and improves herself over time.

She is **not** regatta. Regatta is a single subsystem Leah dispatches when the work is "ship code via PR." Email, calendar, voice, research, scheduling, finance, etc. are sibling subsystems with their own action surfaces.

This document is the system-level spec. Per-tier specs live alongside:

- `2026-06-09-leah-tier1-self-improvement.md` — meta layer (Leah learns Leah)
- `2026-06-09-leah-tier2-swe-productivity.md` — software-engineering work
- `2026-06-09-leah-tier3-schedule-comms-multi-account.md` — calendar + email + voice
- `2026-06-09-leah-remaining-tiers-reordered.md` — automation, comms-external, social, travel, finance

## 1. Why a new system, not a regatta fork

Regatta's invariants are GitHub-PR-shaped: trigger = issue, work = PR, verification = CI + reviewer-verdict, isolation = git worktree, state = `state.db`, audit = git log. Every assumption is load-bearing for that shape; rewiring them inside regatta is a rewrite with extra constraints.

Leah's invariants are operator-life-shaped: trigger = anything (voice, email, calendar tick, cron), work = anything (reply, brief, booking, decision, code), verification = blast-radius policy per action, isolation = per-domain sandbox, state = persistent memory of `tri`, audit = action log + rollback where possible.

Carrying regatta's frame forward costs more long-term than salvaging principles + small modules into a fresh repo. Regatta stays standalone and Leah calls it through its existing versioned operator surfaces (`regatta agents list --json`, future `regatta query`, `gh issue create`, web dashboard, slog stream).

## 2. Operator + scope

Single operator: `tri`. Single user, multi-device (Mac primary, iPhone secondary, optional web access from anywhere). No multi-tenant, no public-facing surface, no SaaS pitch. Leah's threat model is "Leah running on tri's hardware accessed by tri."

### 2.1 Multi-device trust

- Each device gets a **per-device API token** issued by primary Mac via `leah device register <hostname>` (BR-5; YubiKey-confirm).
- Tokens scoped per device: iPhone is read-only + voice-PTT; cannot dispatch BR≥4 actions without primary-relay (primary Mac receives + acts on the request).
- Revocation: `leah device revoke <hostname>` removes token from secrets vault; tailnet ACL pulls the device.
- Each device-token **rotated every 90 days** (cron-fired prompt).
- iPhone-specific: iOS Keychain stores per-device token; access via iOS Shortcut + URL-scheme `leah://` registered to Leah-companion Shortcut.

Out of scope (deferred indefinitely, see `2026-06-09-leah-remaining-tiers-reordered.md` §Cuts):

- Multi-user / team support
- Public chatbot / shared assistant
- Smart-home / IoT control
- Image / video generation
- Autonomous purchases above explicit threshold
- Always-on wake word in Phase 1 (push-to-talk first)
- Native mobile app (iOS Shortcut + SMS bridge sufficient)

## 3. Architecture (high-level)

```
                           ┌──────────────────────────────────────────┐
                           │              tri (operator)              │
                           │  voice ◇ terminal ◇ phone ◇ web ◇ email  │
                           └──────────────────────────────────────────┘
                                              │ ▲
                                              ▼ │
       ┌──────────────────────────────────────────────────────────────┐
       │                       Leah core                              │
       │                                                              │
       │   ┌───────────┐   ┌────────────┐   ┌──────────────────┐      │
       │   │  Intake   │──▶│  Reasoner  │──▶│  Action gateway  │      │
       │   │ (sources) │   │ (planner)  │   │ (policy + queue) │      │
       │   └───────────┘   └────────────┘   └──────────────────┘      │
       │         ▲                ▲                  │                │
       │         │                │                  ▼                │
       │   ┌─────┴────────────────┴──────┐    ┌──────────────┐        │
       │   │   Memory (operator KB)      │    │  Dispatchers │        │
       │   │   contacts ◇ threads ◇ prefs│    │              │        │
       │   │   projects ◇ vocab ◇ history│    │  ┌─────────┐ │        │
       │   │   workspaces (§3.5)         │    │  │ regatta │ │        │
       │   └─────────────────────────────┘    │  └─────────┘ │        │
       │                  ▲                   │  ┌─────────┐ │        │
       │                  │                   │  │ gmail   │ │        │
       │   ┌──────────────┴────────────────┐  │  └─────────┘ │        │
       │   │ Self-improvement (Tier 1)     │  │  ┌─────────┐ │        │
       │   │  audit ◇ outcome ◇ feedback   │  │  │ gcal    │ │        │
       │   │  mistakes ◇ benchmarks ◇ A/B  │  │  └─────────┘ │        │
       │   └───────────────────────────────┘  │  ┌─────────┐ │        │
       │                                      │  │ slack   │ │        │
       │   ┌──────────────────────────┐       │  └─────────┘ │        │
       │   │ Cross-cutting infra      │       │  ┌─────────┐ │        │
       │   │ secrets ◇ events ◇ ops   │       │  │ browser │ │        │
       │   └──────────────────────────┘       │  └─────────┘ │        │
       │                                      └──────────────┘        │
       └──────────────────────────────────────────────────────────────┘
                                              │ ▲
                                              ▼ │
       ┌──────────────────────────────────────────────────────────────┐
       │  External world: GitHub, Gmail, Calendar, Slack, web, APIs   │
       └──────────────────────────────────────────────────────────────┘
```

## 3.5 Workspaces

Operator carries multiple parallel commitments / contexts. Until v1, Leah was operator-keyed (single `tri`); v2 adds `workspace` as a first-class Memory + reasoning dimension.

- **Definition**: `workspace` = a named context (`personal`, `acme`, `side-project-X`, `oss-foo`, …) the operator declares. Workspaces are flat, no hierarchy.
- **Memory rows carry `workspace_id`** (nullable → cross-workspace shared). Default views filter to the active workspace.
- **Cross-workspace queries explicit**: `leah find --all-workspaces`, `leah threads --workspace acme,personal`.
- **Switch**: `leah workspace <name>` (CLI) or voice "I'm in `<name>` mode" (intent shortcut; see Tier 3 §4.5).
- **Persona settings** keyed per-workspace: tone, signature, voice ID, default repos, default email/cal accounts, default Slack workspace, OOO templates.
- **Briefs, todos, reminders, focus rules** are workspace-scoped by default. Morning brief is per active workspace unless `--all` requested.
- **Knowledge-firewall**: cross-workspace Memory reads are explicit (BR-2 surfaced; never silent). See Tier 2 §3.13.
- **Vocabulary partitioned per workspace** — codename "Phoenix" can mean two different things in two workspaces; Leah resolves by active context.
- **Active workspace** lives in `operator_state` row; surfaced in dashboard banner + every CLI prompt prefix.
- **Auto-infer** on intake — tie-breaker ladder:
  1. Inbound from domain matching ONE workspace's account-domain → that workspace.
  2. Inbound from domain matching ZERO workspaces → `workspace_id=null` (cross-workspace; surfaces in default view).
  3. Inbound from domain matching MULTIPLE workspaces (e.g. vendor used in both work + side-project) → **block + surface to operator**: "Is this `acme` or `side-project-X`? Pick one to apply now + remember for sender." Mirrors Tier 3 §8.4 contact-merge discipline.
  4. Outbound: auto-infer from active workspace; never silent override.
  5. Cron-fired events inherit operator's active workspace.

### 3.5a operator_state schema + multi-process race rules

Singleton row backing active workspace + mode + persona overrides. All readers subscribe to `event_stream` (`operator.state.changed`); never cache active values past 5s.

```sql
CREATE TABLE operator_state (
  id                  INTEGER PRIMARY KEY CHECK(id=1),  -- singleton
  active_workspace    TEXT NOT NULL,
  active_mode         TEXT NOT NULL,                    -- standby/focus/asleep/travel/sick/vacation
  active_persona      TEXT,                             -- per-workspace persona overrides
  updated_at          REAL NOT NULL                     -- julian-day
);
CREATE TRIGGER operator_state_event AFTER UPDATE ON operator_state
  BEGIN INSERT INTO event_stream(kind, data)
    VALUES ('operator.state.changed', json_object('active_workspace', NEW.active_workspace, 'active_mode', NEW.active_mode));
  END;
```

- Daemon-fired cron jobs read live state at job-start; in-flight job completes on old state; next job picks up new state.
- Mid-job switch never preempts.

Threading per tier:

- Tier 1: `audit_log`, `mistake_log`, `decision_journal` gain `workspace_id`. Calibration + cost-vs-value bucketed per workspace.
- Tier 2: `repos.yaml` carries `workspace_id` per repo entry; multi-repo tasks workspace-tagged.
- Tier 3: `accounts.yaml` carries `workspace_id` per account; meeting briefs workspace-aware; per-contact tone trained per workspace × contact.
- Remaining: per-workspace expense isolation, subscription audit, tax bucketing (Tier 7); per-workspace todos + capture (Tier 4).

## 4. Modules

### 4.0 Cost cap (M0 invariant)

Hard daily $ ceiling is a M0 acceptance criterion, NOT an open question.

- `cost.Cap` (salvaged from regatta `internal/cost`) wraps every Reasoner / TTS / STT / embedding call.
- Daily ceiling configurable in `~/.leah/cost.yaml`; default `$10/day, $300/month` (30 × daily + week buffer; v2 had a math-inconsistent $200/mo).
- **Tier-degrade ladder** on spend buckets: Opus → Sonnet → Haiku → local 3B (llama.cpp / MLX). Bucket thresholds: 50%, 75%, 90% of daily cap.
- Above 100%: gateway pauses all non-emergency reasoning; emergency = BR-5 + operator-explicit. Surfaces "cap hit" banner in dashboard.
- Per-action `Cost` field (estimated tokens, dollars, side-effects) computed pre-execution; cap blocks the call before spend.
- **Daily-spend banner** surfaces in dashboard + top of every morning brief (email + voice).

Per-Tier budgets (empirically-grounded; re-baseline after 2 weeks of M3 audit data):

| Tier | Monthly cap | Daily cap | Dominant draws + per-call caps |
|---|---|---|---|
| Tier 1 self-improvement | $30 (10%) | $1 | A/B shadow, critic, calibration; A/B shadow $0.10/event; pause when above |
| Tier 2 SWE | $105 (35%) | $3.50 | regatta dispatch, spec/brief drafter, KB Q&A; `leah ask` $0.50/call; `leah build` $5/orchestration |
| Tier 3 schedule + comms | $90 (30%) | $3 | email drafter, meeting prep, daily briefs, TTS; per-email-draft $0.05; classifier moves local-model-first; cloud-Reasoner only for draft generation |
| Tier 4-8 remaining | $45 (15%) | $1.50 | automation, travel (Sherpa $0.30/check), finance, social as called |
| Headroom | $30 (10%) | bursty | spike absorption |

**Post-M3 re-baseline**: after 2 weeks of real audit-log data, re-tune cap + percentages from measured costs (Sunday review item).

Cross-ref: regatta `internal/cost` — salvage the accountant; do not reimplement.

### 4.1 Intake

Pluggable adapters that translate external events into a normalized `IntakeEvent`. Each adapter owns its own polling cadence, auth, deduplication. Adapters run in their own process / goroutine; failures are isolated (one dead adapter does not stall the rest).

**Phase 1 — trimmed (M0–M1):**

- `voice/push-to-talk` — hotkey-triggered, local Whisper, terminal-piped
- `voice/tts` — output side of voice
- `terminal/chat` — `leah` CLI one-shot and REPL
- `regatta` — slog stream tail + `regatta agents list --json` poll (Tier 2)
- `notify` — outbound notifications (desktop / push / SMS)
- `cron` — Leah-internal scheduled tasks

Deferred to M2-M6 (cite `feedback_default_simpler`):

- `gmail` — M5
- `gcal` — M5
- `slack/dm` — M6
- `github-notifications` — M6
- `icloud-mail` (IMAP) — M6+ if needed
- `file-watcher/downloads` — M7 (Tier 4)
- `icloud-cal` — **Phase-X** (no write-capable Go CalDAV library; reopen on lib emergence; see §13 Cuts)

Later adapters: iMessage, SMS, Discord, screen capture, clipboard, RSS, web monitor, browser history.

Normalized event shape:

```go
type IntakeEvent struct {
    ID          string    // ulid
    Source      string    // "gmail/work", "gcal/personal", "voice", ...
    Kind        string    // "email.received", "calendar.upcoming", ...
    OccurredAt  time.Time
    ReceivedAt  time.Time
    Subject     string    // short header for triage
    Body        string    // primary content
    WorkspaceID string    // inferred or operator-active; nullable
    Attrs       map[string]any
    Refs        []EventRef // pointers into Memory (thread, contact, project)
}
```

### 4.2 Reasoner

Takes one or more `IntakeEvent`s plus relevant Memory context, produces a `Plan`. A plan is a sequence of `ProposedAction`s with predicted outcomes and confidence. Plans go to the Action gateway; they are never executed directly by the Reasoner.

The Reasoner is the only module that talks to a frontier LLM by default. Other modules use local heuristics or smaller models where sufficient (intent classification, name extraction, embedding) to keep latency and cost down. Every Reasoner call passes through `cost.Cap` (§4.0).

A `ProposedAction` carries:

```go
type ProposedAction struct {
    Kind        string         // "email.draft", "regatta.dispatch", "calendar.create", ...
    Args        map[string]any
    Why         string         // operator-readable rationale
    Confidence  float64        // 0..1
    BlastRadius int            // 0..5, see §4.4
    Reversible  bool
    Cost        Cost           // estimated tokens, dollars, side-effects
    WorkspaceID string         // active workspace at plan time
}
```

### 4.3 Memory (operator KB)

Persistent, append-mostly knowledge of `tri`. Distinct from per-turn reasoning context.

- **Contacts**: name + accounts (email, Slack, GitHub, etc.) + role + last interaction + notes
- **Threads**: email/Slack/iMessage/issue threads, normalized + linked, `account_scope` tagged (Tier 3 §8.5)
- **Projects**: ongoing work (repos, life projects, topics) + status + related people + related repos
- **Workspaces**: see §3.5; declared list + per-workspace persona
- **Decisions**: significant operator decisions + reasoning + outcomes (feeds Tier 1)
- **Preferences**: operator preferences ("terse replies", "always CC X")
- **Vocabulary**: internal codenames, acronyms, jargon — partitioned per workspace
- **Calendar history**: past meetings, attendee patterns
- **Reading queue**: articles/papers seen + summaries
- **Decision journal**: see Tier 1
- **Audit log**: every Leah action (see Tier 1)
- **Verified facts**: operator-confirmed corrections from mistake_log (Tier 1 §2.17 ground truth)

Storage: **bifurcated SQLite** (§4.7a). Memory DB = plaintext SQLite (Litestream-replicable; per-row `body_blob` encryption for PII columns, key wrapped by macOS Keychain — Tier 1 §2.1). Secrets DB = SQLCipher 256-bit AES (OAuth tokens; NOT Litestream-replicated; restic + age offsite backup). Optional embedding store (sqlite-vec or local Chroma) for semantic search. JSON columns for structured blobs.

Every multi-tenant-shape table carries `workspace_id` (§3.5).

Schema lives in `internal/memory/schema/`. Migrations versioned and tested per regatta's pattern (no duplicate migration numbers; main thread owns the next number).

### 4.4 Action gateway

Single chokepoint for everything that affects the world. No module bypasses it. **Voice intent shortcuts (Tier 3 §4.5) route through Action gateway always — never bypass blast-radius check.**

Action lifecycle:

```
   Reasoner ──▶ ProposedAction ──▶ Gateway
                                    │
                                    ├── cost.Cap check
                                    ├── classify blast radius (policy.schema.cue)
                                    ├── check policy (Cedar)
                                    ├── independence-principle check (§4.4a)
                                    │
                                    ▼
                            ┌───────────────────┐
                            │ Auto?  Notify?    │
                            │ Approve?  2FA?    │
                            └───────────────────┘
                                    │
                  ┌─────────────────┼────────────────┐
                  ▼                 ▼                ▼
              Execute        Enqueue for         Block + ask
              immediately    approval            operator
                  │                 │                │
                  ▼                 ▼                ▼
              Dispatch         Notification     Operator
              to adapter       to operator      decides
                  │                 │                │
                  ▼                 ▼                ▼
              Record in       On approval:      On approve:
              audit log       execute           execute
                              + record          + record
```

Blast-radius tiers (per action class):

| Tier | Name | Examples | Policy |
|---|---|---|---|
| 0 | Read-only | calendar read, email read, regatta state read | Auto, always logged |
| 1 | Internal write | draft saved, memory updated, todo created, audit entry | Auto, logged |
| 2 | Visible local | file moved, app opened, screenshot annotated, cross-workspace read | Notify-after, logged |
| 3 | External low-risk | regatta issue filed, calendar block on own cal, email draft saved | Notify-after, logged, daily summary surface |
| 4 | External high-risk | email send, Slack post, calendar invite to others, meeting decline, web form submit, file mutation (T4.1 / T4.9 until 90d-green) | Approve-before, per-action, audit + rollback window |
| 5 | Irreversible | money movement, file delete, public post, send-no-undo, GH merge | 2FA-confirm, per-action, audit + post-action verify |

**Policy engine: Cedar in-process.** `cedar-policy/cedar-go` is the pinned policy evaluator (Apache-2.0; https://github.com/cedar-policy/cedar-go accessed 2026-06-09). No "or equivalent" hedge.

Policy schema lives in `internal/actiongateway/policy.schema.cue` (CUE-defined, Cedar-emitted). Skeleton:

```cue
#Policy: {
    action_kind:   string
    workspace?:    string
    blast_radius:  int & >=0 & <=5
    approval:      "auto" | "notify-after" | "approve-before" | "2fa-confirm" | "block"
    rate_limit?:   {per_hour?: int, per_day?: int, burst?: int}
    confidence_floor?: float
    independence_required?: bool
}
```

**20 canonical (action_kind, expected_tier) fixtures** live in `internal/actiongateway/fixtures/policy_canonical_test.go`:

```
1.  email.read              → 0
2.  calendar.read            → 0
3.  regatta.state.read       → 0
4.  memory.write             → 1
5.  todo.create              → 1
6.  audit.entry              → 1
7.  email.draft.save         → 1
8.  file.move                → 2
9.  screenshot.annotate      → 2
10. workspace.crossread      → 2
11. regatta.issue.file       → 3
12. calendar.block.self      → 3
13. email.draft.in-gmail     → 3
14. email.send               → 4
15. slack.post               → 4
16. calendar.invite.others   → 4
17. meeting.decline.external → 4
18. file.delete              → 5
19. money.move               → 5
20. gh.pr.merge              → 5
21. workspace.switch         → 1
22. policy_default_tier      → 4   (fail-closed for unknown action_kinds)
```

Per-channel overrides allowed ("Slack #random auto-reply OK, never auto on #legal"). Operator can globally pause or pause per domain ("stop all email actions"). Operator can globally pause per workspace ("stop all `acme` actions").

#### 4.4a Independence principle (universal port of `no-self-tagged-APPROVE`)

For every blast-radius ≥ 3 action, the *approval signal* MUST come from a context independent of the *authoring signal*. Concretely:

- Tier 1: fixture-additions + mistake-log → prevention promotions are hoisted to **operator-approval queue**, separate from the prompt-change batch. Author (Tier 1 self-improvement layer) cannot self-approve a fixture into the regression suite.
- Tier 2: independent-reviewer subagent runs in fresh subagent runtime, NOT main Reasoner context.
- Tier 3: email-drafter cannot self-approve email-send; that gate is operator (or Tier 5 per-template-per-contact whitelist that itself required operator approval at registration).
- All Sunday-review item promotions: independent reasoning pass at apply time, NOT batch-author time.

This rule extends regatta's `feedback_no_self_tagged_approve` to every Leah approval surface.

**Reviewer-prompt-store independence**: reviewer subagent prompts live in `reviewer-prompts/` — a separate directory, separate git track, separately approved on a different cadence than Reasoner prompts in `prompts/`. A Tier-1 Sunday-review prompt-promotion CANNOT touch `reviewer-prompts/`; operator must explicitly opt-in via `leah review-prompts promote <name>`. Reviewer subagent system prompts open: *"Your prompt comes from a separately-versioned track; the Reasoner prompts that planned this action did not write your instructions."* Closes the loophole where reviewer + Reasoner share a `prompt_versions`-tracked store.

### 4.5 Dispatchers

Plugins that the Action gateway routes to. Each implements a small contract:

```go
type Dispatcher interface {
    Kind() string                       // "regatta", "gmail", "gcal", ...
    Execute(ctx, action) (Outcome, error)
    Verify(ctx, outcome) error          // post-action sanity
    Rollback(ctx, outcome) error        // best-effort
}
```

Phase 1 dispatchers (trimmed per §4.1):

- `regatta` — file issue, watch PR, narrate, dispatch independent reviewer
- `notify` — desktop (macOS native), phone push (Pushover), TTS, SMS fallback (Twilio)
- `voice/tts` — speak (Phase 1: `say` + OpenAI tts-1-hd; ElevenLabs only if voice-clone needed; see §4.1 voice pin)
- `terminal` — print to operator session

Phase 2+ dispatchers: `gmail`, `gcal`, `slack`, `browser` (Playwright), `plaid` (read-only finance), `obsidian/notes`, `linear/jira`.

### 4.6 Self-improvement layer (Tier 1)

A side-car that observes the Reasoner + Action gateway + Memory and feeds them learning signal. Full spec in `2026-06-09-leah-tier1-self-improvement.md`.

### 4.7 Cross-cutting infra

- **Secrets vault** — OS keychain (macOS Keychain on primary) backing `secrets.Get(name)`. Per-account encrypted token at rest (key derived per-account from a master key in Keychain). Optional YubiKey unlock on sensitive reads + on token refresh. Token refresh handled centrally. **OAuth lifecycle**:
  - Google OAuth apps published as **"In production (unverified)"** (https://support.google.com/cloud/answer/10311615 accessed 2026-06-09); operator self-grants. Avoids "test mode" 7-day refresh-token expiry trap.
  - **Unverified-app consent UX**: each new device shows Google's "Hasn't been verified by Google" warning → operator clicks "Advanced → Go to leah (unsafe)". Expected; document in operator setup runbook.
  - **CAP / RISC** push revocation handlers wired per provider (Google RISC: https://developers.google.com/identity/protocols/risc accessed 2026-06-09).
  - **Token-refresh failure path**: retry-with-backoff (3 attempts, exponential 1s/4s/16s); on persistent failure → Pushover alert + Twilio SMS + per-account auto-pause (account marked `auth_failed` until operator re-auths).
  - Per-account kill switch: `leah account revoke <id>` zeroizes token + posts revoke to provider.
  - Blast-radius if Keychain breached: every per-account token recoverable via revoke + re-auth; SQLCipher key separate; never both compromised by one keychain dump alone.
- **Event stream** — Leah's slog-style telemetry. Every module emits structured events; self-improvement and ops dashboard subscribe. Schema in `internal/obs/events.go`. Borrow regatta's event-kind taxonomy + reaper.
- **Ops dashboard** — local web UI (tailnet-only Phase 1) showing pending approvals, active threads, intake adapter health, recent actions, regatta state, **last-good-backup age** (§4.7a), active workspace.
- **Kill switch** — single command pauses all gateways. Per-domain pauses too (`leah pause email`). Per-workspace pauses (`leah pause workspace acme`).
- **Watchdog / heartbeat** — replaces "Leah self-checks":
  - **launchd `KeepAlive=true`** restarts the daemon on crash (https://www.launchd.info accessed 2026-06-09).
  - **healthchecks.io** ping every 5 min from the daemon; healthchecks.io alerts on miss (https://healthchecks.io accessed 2026-06-09).
  - On healthchecks.io miss: **Pushover** push to operator's phone (https://pushover.net accessed 2026-06-09).
  - **SMS fallback via Twilio** when Pushover ack not received in 10 min (https://www.twilio.com/messaging accessed 2026-06-09).

#### 4.7a Backup + recovery (M0 acceptance criterion) — BIFURCATED DBs

Backup is not an open question; it's M0. **Litestream + SQLCipher are INCOMPATIBLE** — SQLCipher encrypts the WAL; Litestream WAL-tail cannot stream sane page data (Litestream upstream issue #177, https://github.com/benbjohnson/litestream/issues/177 accessed 2026-06-09). v2 shipped a broken combination; v2.1 resolves by **DB bifurcation**:

**Secrets DB** (`~/.leah/secrets.db`) — OAuth tokens, OS-keychain-wrapped per-row keys, sensitive operator identity:

- **SQLCipher at-rest** (256-bit AES; https://www.zetetic.net/sqlcipher accessed 2026-06-09, BSD-style).
- **NO Litestream** (incompatibility above).
- Backup: periodic `sqlcipher_export` to plaintext intermediate → encrypted offsite via **restic** (https://restic.net accessed 2026-06-09, BSD-2-Clause) + age key (YubiKey-held OR operator passphrase). Intermediate file lives in `tmpfs` ramdisk, zeroed post-upload.

**Memory DB** (`~/.leah/memory.db`) — audit, threads, contacts, projects, decisions, calendar, todos, etc (non-credential content):

- **Plaintext SQLite** (Litestream-compatible).
- **Litestream WAL-tail** to S3 SSE-KMS or Backblaze B2 with per-bucket key (https://litestream.io accessed 2026-06-09, Apache-2.0).
- **Per-row `body_blob` encryption** (Tier 1 §2.1) preserves PII-at-rest even in replicated plaintext DB; the replicated stream carries encrypted blobs.

**Quarterly restore drill** — required, scheduled cron:

- Trigger: 1st Sunday of each calendar quarter.
- Both DBs restored to `/tmp/leah-restore/`; smoke-query returns ≥ 1 last-week row from each.
- Pass: completes < 1h RTO + no schema-version mismatch.
- Drill is itself a regatta-style ticket Leah dispatches to herself; operator verifies; result logged in audit.

**Dashboard** surfaces:

- `last_good_backup_age_memory` (last successful Litestream replication ts). > 1h amber, > 24h red.
- `last_good_backup_age_secrets` (last successful restic snapshot ts). > 24h amber, > 7d red.
- Recovery RTO target: 1h (drill must complete in 1h to count green).

### 4.8 Provider data-flow matrix

Authoritative per-data-class × per-provider policy. Default-deny on money/health/family-PII to any third-party reasoner. Anthropic API retention: **30 days zero-data-retention available + 7 days default, no training by default** (https://www.anthropic.com/legal/commercial-terms accessed 2026-06-09; https://privacy.anthropic.com/en/articles/7996866-how-long-do-you-store-personal-data accessed 2026-06-09).

| Data class \\ Provider | Anthropic Reasoner | OpenAI TTS | OpenAI STT (fallback) | ElevenLabs TTS | Plaid (finance) | GitHub | local-only |
|---|---|---|---|---|---|---|---|
| email body (generic) | allow | n/a | per-call-approve | n/a | n/a | n/a | allow |
| email body (private flag) | never | n/a | never | n/a | n/a | n/a | allow |
| calendar event title | allow | allow | n/a | allow | n/a | n/a | allow |
| calendar event body | allow | per-call | per-call | per-call | n/a | n/a | allow |
| voice transcript (general) | allow | n/a | per-call-approve | n/a | n/a | n/a | allow |
| voice transcript (private) | never | n/a | never | n/a | n/a | n/a | allow |
| journal entry | per-call-approve | n/a | n/a | n/a | n/a | n/a | allow |
| receipt / transaction | never | n/a | n/a | n/a | allow | n/a | allow |
| Slack DM (general) | allow | n/a | n/a | n/a | n/a | n/a | allow |
| Slack DM (HR / legal / health) | never | n/a | n/a | n/a | n/a | n/a | allow |
| code (public repo) | allow | n/a | n/a | n/a | n/a | allow | allow |
| code (private repo) | per-call-approve | n/a | n/a | n/a | n/a | allow | allow |
| decision-journal entry | per-call-approve | n/a | n/a | n/a | n/a | n/a | allow |
| family-PII (DOB / SSN) | never | never | never | never | never | never | allow |
| medical | never | never | never | never | never | never | allow |
| financial-specifics ($ amounts, account #s) | never | never | never | never | allow | never | allow |

Cells: **allow** = default permitted; **per-call(-approve)** = operator gate per call; **never** = mechanical block (action gateway rejects); **n/a** = provider does not handle this class. Privacy ledger (Tier 1 §3.3) records every cell hit.

**Per-workspace overlay rule**: a workspace may declare a stricter row (e.g. `acme` overrides `email body (generic)` from `allow` → `per-call-approve`). Overlays may ONLY tighten, NEVER loosen, the global default. Mechanical lint (`scripts/check-privacy-leak.sh`) asserts overlay rows are strict subsets.

## 5. Regatta interface

Leah talks to regatta through regatta's existing **versioned operator surfaces only**. Regatta receives no Leah-specific changes; Leah is just a programmatic operator.

| Surface | Use |
|---|---|
| `gh issue create` | Leah files issues regatta picks up via poll |
| `gh issue edit --add-label` | route to specific lane / trigger |
| `regatta agents list --json` | poll lifecycle state (BUG-1078, shipped #1108) |
| `regatta query` (future) | extended read queries; versioned CLI surface |
| `gh pr view --json` | poll PR state |
| slog stdout stream | event subscription for narration |
| dashboard `:8080` | iframe in Leah's own dashboard for live view |
| `gh pr review` / `pr comment` | post Leah's independent-reviewer subagent verdict |
| `gh pr merge` | NOT used — operator merges; gates require independent review |

**No `state.db` direct reads.** Direct SQLite reads couple Leah to regatta's internal schema and break on any schema migration. Contract: Leah uses only versioned CLI surfaces. **Contract-test note**: Leah's regatta dispatcher carries a contract test (`internal/dispatchers/regatta/contract_test.go`) that exercises every CLI surface against a pinned `regatta` binary version; bumping the pin requires re-running the test set.

Critical: Leah cannot self-approve regatta PRs. Regatta's `check-reviewer-verdict.sh` enforces canonical `Reviewer-agent-id:` shape; "leah" is not in the allowlist. Leah must spawn an independent reviewer subagent (canonical prefix like `cavecrew-reviewer-*` or hex `a[0-9a-f]{16}`) and pass that agent-id. This is good — the gate designed against autonomous-regatta protects against autonomous-Leah identically.

Per regatta's `feedback_no_implementer_automerge`: Leah cannot enable automerge on a load-bearing PR that carries a `Reviewer-agent-id`. Operator-merge handoff remains the terminal step.

## 6. Data flow examples

### Example A: voice → ship code

```
tri presses hotkey, says "ship the fix for bug 1086"
  → voice/push-to-talk adapter: whisper.cpp large-v3-turbo → text
  → IntakeEvent{Source: "voice", Kind: "voice.utterance", Body: "ship the fix for bug 1086", WorkspaceID: active}
  → Reasoner: intent = "regatta.dispatch"; resolve "bug 1086" via Memory.lookup_issue
  → ProposedAction{Kind: "regatta.dispatch", Args: {issue: 1086}, BlastRadius: 3}
  → Action gateway: tier 3, notify-after policy → cost.Cap check → execute
  → regatta dispatcher: gh issue add-label ready-for-agent
  → narration adapter starts watching regatta agents list + slog events
  → on PR open: spawn independent reviewer subagent
  → on PR merge (or block): notify dispatcher → TTS + desktop push
  → audit log entry + outcome marker
```

### Example B: email → drafted reply

```
gmail adapter polls Gmail Pub/Sub watch() (Tier 3 §2.2a)
  → new thread detected from Sarah
  → IntakeEvent{Source: "gmail/work", Kind: "email.received", WorkspaceID: "acme", ...}
  → Reasoner: classify as "needs reply, low urgency"; load thread + contact memory scoped to acme
  → ProposedAction{Kind: "email.draft", Args: {body: "...", thread_id: ...}, BlastRadius: 1}
  → Action gateway: tier 1, auto-execute
  → gmail dispatcher: save draft via Gmail API
  → audit log entry (acme workspace)
  → notify dispatcher: silent (per policy — drafts batched into morning brief)
  → Memory updated: thread state advanced, account_scope=acme
```

### Example C: calendar tick → meeting prep

```
event-scheduled timer fires at T-20:00 (per Tier 3 §3.3a; NOT 5-min cron sweep)
  → IntakeEvent{Source: "timer", Kind: "meeting.upcoming", Refs: [event_id]}
  → Reasoner: pull attendees from contacts; pull last threads with each; load shared decisions
  → ProposedAction{Kind: "brief.create", Args: {meeting_id, brief_body}, BlastRadius: 1}
  → Action gateway: auto-execute
  → terminal + notify dispatchers: print brief + desktop notification
  → audit log entry
  → on operator-feedback (thumbs up/down): self-improvement layer ingests
```

## 7. Build order (system level)

Per-tier build orders live in each tier spec. System-level milestones (trimmed scope; cite `feedback_default_simpler`):

**M0 — Skeleton** — repo init, CLAUDE.md, secrets vault stub, event stream, audit log table, blast-radius gate (Cedar in-process), **cost.Cap**, **bifurcated DBs: SQLCipher secrets-db + restic backup; plaintext Memory-db + Litestream WAL-tail** (§4.7a), **launchd + healthchecks.io + Pushover**, ops dashboard placeholder, kill switch. Phase-1 intake adapters: voice + terminal + cron + notify + regatta only.

**M1 — Terminal + regatta dispatcher** — Tier 2 minimum: `leah ship`, regatta watch, independent reviewer, narration. Terminal-only intake. No memory yet (in-session state).

**M2 — Memory layer** — operator KB schema (workspace-aware), contacts/threads/projects/decisions tables, embedding store, semantic search.

**M3 — Tier 1 self-improvement** — audit + outcome + feedback + mistake log + decision journal. Sunday review.

**M4 — Voice** — push-to-talk + TTS + desktop notifications. Cuts daily friction.

**M5 — Email + calendar** — Gmail multi-account (Pub/Sub watch primary, history.list reconcile), gcal multi-account, daily brief, meeting prep, decline drafter.

**M6 — Slack + GitHub notifications + file-watcher** — DM ingest, mention watch, drafts; `~/Downloads` watcher.

**M7+ — Onward** — automation, social, travel, finance per `remaining-tiers` spec.

## 8. Repo layout (proposed)

```
leah/
├── CLAUDE.md                         # operator rules (port principles from regatta)
├── cmd/
│   └── leah/                         # main CLI binary
├── internal/
│   ├── intake/
│   │   ├── voice/                    # push-to-talk + tts (M0-M4)
│   │   ├── cron/                     # M0
│   │   ├── regatta/                  # M1 slog tail
│   │   ├── gmail/                    # M5
│   │   ├── gcal/                     # M5
│   │   ├── slack/                    # M6
│   │   ├── github/                   # M6
│   │   └── file/                     # M7
│   ├── reasoner/                     # planner + intent classifier
│   ├── memory/                       # operator KB
│   │   ├── schema/                   # workspace_id everywhere
│   │   ├── contacts/
│   │   ├── threads/
│   │   ├── projects/
│   │   ├── workspaces/               # NEW v2
│   │   └── search/                   # embedding + FTS
│   ├── actiongateway/
│   │   ├── policy.schema.cue         # Cedar policy schema
│   │   ├── fixtures/                 # 20 canonical (action_kind, tier) fixtures
│   │   └── queue/
│   ├── cost/                         # salvaged from regatta; cap + tier-degrade
│   ├── dispatchers/
│   │   ├── regatta/                  # contract_test.go vs pinned regatta binary
│   │   ├── notify/                   # M0
│   │   ├── tts/                      # M4
│   │   ├── gmail/                    # M5
│   │   ├── gcal/                     # M5
│   │   └── slack/                    # M6
│   ├── selfimprove/                  # Tier 1
│   │   ├── audit/
│   │   ├── outcome/
│   │   ├── feedback/
│   │   ├── mistakes/
│   │   ├── benchmark/
│   │   └── ab/
│   ├── secrets/                      # vault + OAuth lifecycle
│   ├── obs/                          # event stream
│   ├── ops/                          # dashboard + kill switch + watchdog
│   ├── persona/                      # per-workspace + global tone settings + voice config (maps to §10 personality)
│   └── backup/                       # Litestream config + restore drill
├── contracts/
│   └── schemas/                      # CUE schemas (mirror regatta)
├── scripts/                          # check-*.sh mechanical gates
└── docs/
    ├── specs/                        # design docs
    └── briefs/
```

## 9. Salvage from regatta

Code/concepts to lift into Leah, cited explicitly:

- **`internal/secrets`** — env scrub patterns; Leah extends to OAuth tokens
- **`internal/cost`** — token + dollar accounting (M0 invariant; see §4.0)
- **`internal/obs`** — slog event taxonomy + reaper pattern
- **`internal/gates`** — policy decision shape (gateway design)
- **`internal/orchestrator/state`** — lifecycle state-machine pattern (apply to Action lifecycle, not agent lifecycle)
- **`contracts/schemas/`** — CUE-as-source-of-truth pattern
- **`scripts/check-*.sh`** — mechanical-gate pattern (extend to action-policy gates + privacy-tier lint; see remaining-tiers §H7)
- **`CLAUDE.md` principles** — decision priority, default-simpler, validate-before-ship, adversarial-review-every-step, root-cause-not-symptom, deletion-default, **no-self-tagged-APPROVE (ported universally; §4.4a)**, comments-discipline (WHY not WHAT), no-AI-signatures
- **Additional ports**:
  - `feedback_audit_main_before_implementing` — before dispatching subagent, check the work isn't already on main (Memory check or repo grep).
  - `feedback_test_coverage_audit_per_wave` — end every dispatch wave with explicit unit + integration + E2E + TDD-order audit BEFORE next wave.
  - `feedback_trap_projection` — recurring operator traps must be fixed at gate / prompt / knowledge boundary, not just per-PR symptom.
  - `feedback_double_fail_root_cause` — same test/gate failing twice in one session is a real defect, not flake; investigate.
  - `feedback_validate_before_ship` — empirically validate before recommending CI/perf/memory changes.
  - `feedback_keep_orchestrator_branch_name` — Leah-spawned subagents push under orchestrator-assigned branch; semantic name belongs in PR title only.
- **Banned-phrase gate** — Leah's `prompts/` + `reviewer-prompts/` + static narration strings scanned for `seamless`, `world-class`, `blazing-fast`, `production-grade`, `cutting-edge`, `state-of-the-art` (and the regatta 11-token list); CI fails on hits. Mechanical: `scripts/check-banned-phrase.sh`.

Explicitly NOT carried:

- Byte-equal-pin gate (specific to refactor PR pattern, irrelevant here)
- PR-body-as-durable-record assumption — PR bodies do not exist for most Leah actions
- Reviewer-agent-id token shape (regatta's allowlist) — Leah has her own action approval model
- Single-repo self-host filter — Leah may span multiple repos and accounts
- Worker-prompt parity gate (`scripts/check-prompt-parity.sh`) — Leah has a different prompt surface

## 10. Personality / voice

Leah is a name, not a persona theme. Tone defaults to terse + warm + dry. Tunable. She uses your name when it matters; she does not "hi tri!" every turn. She narrates state changes proactively when blast-radius is ≥3 ("PR 1112 merged"), silently otherwise. She does not editorialize unsolicited.

Voice defaults to OpenAI `tts-1-hd` (Phase 1) with `say` offline fallback. ElevenLabs Creator tier ($22/mo, 100K credits — https://elevenlabs.io/pricing accessed 2026-06-09) only enabled if voice-clone needed. TTS cache keyed by `text-hash + voice-id + model` to cut repeat-line spend. Voice persona configurable per workspace (see §3.5). **`say` fallback caveat**: macOS system voice ignores persona config — voice persona unavailable in `say` fallback; document in operator-facing setup.

## 11. Open questions

(Several Phase-1 open-questions resolved in v2 and moved into normative sections; remaining unknowns:)

- **Hosting**: laptop-only Phase 1 confirmed; home server (Mac mini) by Phase 3 still open — depends on watchdog + Litestream proving out.
- **Local model floor**: tier-degrade ladder lands `local-3B` at 90% cap; which model exactly (llama-3.1 8B Q4 via MLX vs llama.cpp) — benchmark needed.
- **Web access**: tailnet-only Phase 1 confirmed; Cloudflare Tunnel still deferred.
- **Identity / auth**: per-device API token in OS keychain + tailnet ACL Phase 1 confirmed; mTLS later if needed.

Resolved → normative:

- ~~Hard cost cap~~ → §4.0 (M0 invariant).
- ~~LLM provider mix~~ → §4.1 voice pin + §4.8 matrix.
- ~~OAuth lifecycle~~ → §4.7.
- ~~Backup~~ → §4.7a.
- ~~Watchdog~~ → §4.7.
- ~~Blast-radius classifier shape~~ → §4.4.

## 12. Success criteria (system level)

After M5 (week ~7):

- Leah runs unattended for a full work week without crashes (launchd `KeepAlive` validated)
- Litestream last-good-backup age ≤ 1h continuously; quarterly restore drill green
- Daily $-cap holds; tier-degrade ladder observed (Sonnet → Haiku trip ≥ once without operator pain)
- Leah handles ≥5 inbound channels (terminal + voice + Gmail × 2 + gcal × 2)
- ≥1 regatta PR per day dispatched + reviewed + narrated end-to-end
- ≥80% of low-blast-radius actions execute without operator intervention
- Daily brief delivered every morning, every evening shutdown delivered, **active-workspace banner correct on every brief**
- Self-improvement layer produces a Sunday review with ≥3 concrete proposed improvements
- Operator reports time-saved measurably; subjective "Leah is useful" qualitative pass

Per-tier success criteria are in tier specs.

## 13. Cuts (system level, explicit)

- No multi-user.
- No SaaS / billing / Stripe.
- No public API.
- No mobile native app pre-M7.
- No always-on wake word pre-M7.
- No autonomous money movement, ever.
- No autonomous DMs/posts/replies pre-M5.
- No image/video generation.
- **No iCloud Calendar write** (no write-capable Go CalDAV library as of 2026-06-09; read-only via CalDAV optional; Phase-X reopen trigger = library emergence).
- **No `state.db` direct read** (versioned CLI only; see §5).
- **No OpenAI Whisper STT default** (audio retention policy violation; fallback only with per-utterance operator consent + privacy ledger entry; default STT = whisper.cpp local).

## 14. Changes in v2

- §3.5 **Workspaces** added — first-class Memory + reasoning dimension; threads through Tiers 1/2/3/4/5/7.
- §4.0 **Cost cap** added as M0 invariant; tier-degrade ladder; per-Tier token budget table.
- §4.4 **Cedar** pinned as policy engine; 20 canonical fixtures; `policy.schema.cue` skeleton.
- §4.4a **Independence principle** — universal port of `no-self-tagged-APPROVE` to all BR≥3 approvals.
- §4.5 trimmed Phase-1 dispatchers (terminal + regatta + notify + tts only); rest M2-M6.
- §4.7 OAuth lifecycle pinned (Google "In production unverified" + CAP/RISC + per-account kill switch).
- §4.7 watchdog replaced "Leah self-checks" with launchd `KeepAlive` + healthchecks.io + Pushover + Twilio SMS.
- §4.7a **Backup + recovery** added as M0 (Litestream + SQLCipher + quarterly drill).
- §4.8 **Provider data-flow matrix** added; default-deny money/health/family-PII to third-party reasoners.
- §4.1 trimmed Phase-1 intake; iCloud Calendar → Phase-X; gmail/gcal/slack/github/file deferred to M5-M7.
- §5 dropped `state.db direct read`; versioned CLI only + contract-test note.
- §10 voice pinned: OpenAI tts-1-hd default + `say` fallback; ElevenLabs only with voice-clone need; TTS cache.
- §11 several open-questions resolved → normative (cost, provider mix, OAuth, backup, watchdog, classifier shape).
- §13 added cuts: iCloud Calendar, state.db direct read, OpenAI Whisper default.

## 15. Changes in v2.1

- §2.1 **Multi-device trust** subsection added — per-device API token, scoped capability, 90d rotation, iPhone Keychain + `leah://` URL scheme (H2).
- §3.5 **Workspace auto-infer tie-breaker ladder** — domain-match single / zero / multiple / outbound / cron rules (H6).
- §3.5a **operator_state schema** + event_stream trigger + multi-process race rules (H5).
- §4.0 **Cost cap math fixed**: $300/mo (was $200/mo); empirically-grounded per-Tier % with per-call caps; daily-spend banner; post-M3 re-baseline note (C2).
- §4.4 **Fixtures 21+22 added**: `workspace.switch → 1`, `policy_default_tier = 4` fail-closed for unknown action_kinds (M23).
- §4.4a **Reviewer-prompt-store independence** — `reviewer-prompts/` separate dir + separate promotion CLI; reviewer system prompt opens with provenance disclaimer (H4).
- §4.7 **OAuth UX gap** — unverified-app consent UX documented; token-refresh retry-with-backoff + Pushover/Twilio + auto-pause on persistent failure (H1).
- §4.7a **Bifurcated backup** — Secrets DB SQLCipher + restic (NO Litestream); Memory DB plaintext SQLite + Litestream + per-row `body_blob` encryption; quarterly drill 1st Sunday of quarter < 1h RTO (C1, M36).
- §4.8 **Per-workspace overlay rule** — overlays may tighten, never loosen, global default (M22).
- §9 **Additional CLAUDE.md ports** — audit_main_before_implementing, test_coverage_audit_per_wave, trap_projection, double_fail_root_cause, validate_before_ship, keep_orchestrator_branch_name; **banned-phrase gate** added (L1, L2).
- §8 `internal/persona/` directory clarification (L4).
- §10 `say` fallback persona caveat noted (L5).

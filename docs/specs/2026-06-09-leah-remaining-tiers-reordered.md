---
title: Leah — remaining tiers (4 → 8)
status: draft
phase: design
owner: tri
created: 2026-06-09
parent: 2026-06-09-leah-overview.md
---

# Remaining tiers

After Tier 1 (self-improvement), Tier 2 (SWE productivity), Tier 3 (schedule + multi-account comms + voice) are mature, expand outward.

Ordered by leverage × low-blast-risk × foundation-dependency:

## Priority order

| # | Tier | Rationale |
|---|---|---|
| 4 | **Automation (life ops)** | Low blast, recurring drudgery, compounds with Memory |
| 5 | **External communication (proactive)** | High leverage, gated, builds on Tier 3 inbox |
| 6 | **Travel research** | Read-only research, no auto-actions |
| 7 | **Finance (read-only first, write later)** | Read-only spend/balance huge value |
| 8 | **Social media** | Discretionary, brittle APIs, weakest ROI |

## Tier 4 — Automation (life ops)

### Goal

Eliminate recurring drudgery. Low individual blast radius; the value comes from compounding many small wins.

### Capabilities

| ID | Feature | BR | Notes |
|---|---|---|---|
| T4.1 | File watcher: `~/Downloads/*.pdf` auto-rename + categorize + index | **4 until 90d-green** | File mutation. `.leah-trail.jsonl` sidecar per-action; reversibility window. After 90-day-green-log, BR may be relaxed to 3 (proposal). |
| T4.2 | Screenshot sweeper: auto-tag + archive | 2 | OCR via macOS Vision API + LLM caption |
| T4.3 | Clipboard history + smart actions | 1 | Opt-in only; **mandatory local regex denylist** (AWS/GCP keys, JWT, CC numbers, SSH private keys) BEFORE any LLM call. See cross-tier "Clipboard secrets denylist" below. |
| T4.4 | Recurring local commands | 2 | YAML schedule + report on completion |
| T4.5 | Backup health monitor: verify backups + alert on failure | 3 | Periodic restore-test sample; complements overview §4.7a Litestream |
| T4.6 | Subscription audit (paired with Tier 7) | 1 | Surface unused; never cancels autonomously |
| T4.7 | Domain / cert / token expiry watcher | 1 | DNS + LetsEncrypt + OAuth token introspection |
| T4.8 | Receipt auto-file (email + photo) | 1 | OCR + structured extract; workspace-tagged for expense isolation (Tier 7) |
| T4.9 | Photo cleanup: dedupe + screenshots-no-longer-needed | **4 until 90d-green** | File mutation. `.leah-trail.jsonl` sidecar. Proposes batches; operator approves. Same trail-and-window pattern as T4.1. |
| T4.10 | Note consolidation (Apple Notes + Obsidian + voice memos → unified index) | 1 | Read-only Phase 1 |
| T4.11 | Browser tab archivist: snapshot open tabs as named session | 2 | Restore via `leah tabs restore <name>` |
| T4.12 | Quick-capture inbox (voice/text/photo → universal capture) | 1 | Routes to right downstream after classify; workspace-inferred from context |
| T4.13 | Voice-to-todo from phone (Shortcut + watch complication) | 1 | iOS Shortcut → Leah daemon → todo row; workspace-inferred from time-of-day + active calendar |
| T4.14 | Highlight ingestion (Kindle / iPhone Books → reading list memory) | 1 | Parses Kindle export + Books highlights → Memory.reading_queue |
| T4.15 | Web-clipping inbox (share-sheet target → Leah summarizes + files) | 1 | iOS / macOS share sheet → Leah daemon → summarized + filed by topic + workspace |
| T4.16 | "Remind me when I'm next ..." context-aware | 1 | Triggers on location (next at airport), device-state (next at home Mac), workspace (next in `acme`), time. Uses Tier 3 §6.2 state-based reminders. |
| T4.17 | Idle-thought capture (voice memo while walking → clustered next morning) | 1 | Walking voice memos accumulate; morning brief clusters by topic |
| T4.18 | Forced unplug ("lock me out of email after 9pm" → silent until allowed) | 3 | Pauses Tier 3 email intake events from surfacing; allow override only via typed unlock; aligns with Tier 1 §3a.4 no-decide-after-9pm |

### File-mutation safety (T4.1 + T4.9)

- Sidecar at `~/.leah/trails/<action_kind>.jsonl` records: `{ts, action_id, from_path, to_path, hash_before, hash_after, reversible_by}`.
- Reversibility window: 30 days; `leah trail revert <action_id>` undoes the move/rename if hash matches.
- BR stays at 4 (approve-before per action OR per batch) until 90 consecutive days of green log (zero operator-reverted actions). Then proposal to relax to 3 lands as Sunday-review item.

### Build order

T4.1 + T4.10 + T4.12 + T4.13 + T4.14 + T4.15 (capture surfaces) → T4.5 + T4.7 (alerts) → T4.4 + T4.6 + T4.8 (recurring drudgery) → T4.16 + T4.17 + T4.18 → rest opportunistic.

### Cuts

- No autonomous file deletion (only move/archive)
- No autonomous subscription cancellation (proposes, operator acts)
- No always-on screen recording
- **No clipboard LLM call without denylist scan** (T4.3)

## Tier 5 — External communication (proactive)

### Goal

Move email/Slack from "Leah drafts, tri sends" to "Leah handles low-stakes outbound autonomously." Builds on Tier 3 inbox + memory.

### Capabilities

| ID | Feature | BR | Notes |
|---|---|---|---|
| T5.1 | Auto-send for thanks/ack/short-confirm templates (whitelisted per-contact) | 4 | Per-contact opt-in; reversible within 30s (Gmail undo); subject to "Auto-send rate limits" below |
| T5.2 | Inbox-zero "approve-all-clean" batch | 4 | Per-batch operator approval |
| T5.3 | Recruiter/sales polite-decline auto-send (per-template-per-sender) | 4 | Whitelist by template + sender domain; rate-limited |
| T5.4 | Scheduled-send: send tomorrow 9am | 4 | Per-message approval, deferred execution |
| T5.5 | OOO autoresponder context-aware (traveling, focus-day, sick) | 3 | Templates per context; **fixed strings only — NEVER interpolate calendar context** (template-injection prevention). "Sick" context-aware OOO restricted to allowlisted known-contacts only. |
| T5.6 | Slack low-risk auto-reply (per-channel whitelist) | 4 | E.g. `#random` "thanks for sharing"; never `#legal`; rate-limited |
| T5.7 | iMessage drafts (operator taps send) | 1 | Pure draft Phase 1 |
| T5.8 | Follow-up tracker: "you said you'd send X by Tuesday" → reminder if not sent | 1 | Watches outbound + parses commitments |
| T5.9 | Polite-decline templates: meeting / coffee / podcast / mentor | 4 | Per-template-per-contact whitelisting |
| T5.10 | Thread summarizer: long email/Slack thread → 5-line catch-up | 0 | Read-only |
| T5.11 | Out-of-band escalation: SMS to operator for genuine urgency | 3 | Twilio + classifier "this needs response in < 2h" |
| T5.12 | Per-message style match (mirror sender's tone in reply draft) | 1 | Builds on Tier 3 per-contact tone calibration (per workspace × contact × account_scope) |
| T5.13 | Identity-correct reply enforcement | 4 | Block send on account mismatch (re-stated from Tier 3 §10.11; lives here at send-stage) |
| T5.14 | Awkward-ask drafter (2-3 tone variants) | 1 | Re-stated from Tier 3 §10.16; lives here at send-stage |
| T5.15 | Defer-without-loss (real reminder + thread snooze + invisible-to-sender) | 1 | Re-stated from Tier 3 §10.13; send-stage variant |

### Auto-send rate limits

Every auto-send template enforces:

- **Per-policy cap**: ≤ N sends/hour (default N = 5 per template).
- **Cumulative daily budget**: ≤ 50 auto-sends/day operator-wide.
- **Burst-detect kill switch**: if rate > 2× rolling 24h average over 10 min → pause + alert.
- **60s anti-loop floor**: between a Leah-auto-send and any subsequent Leah-auto-send on the SAME thread (prevents recursive thank-you-to-thank-you loops).
- **Spam-wave circuit breaker**: if Leah is auto-replying to a sender whose volume > 50/hour to the operator (likely a wave / bot), pause auto-reply for that sender + flag.

### Build order

T5.10 + T5.8 + T5.7 (low-risk capture + summary) → T5.5 + T5.9 (templates + OOO) → T5.1 + T5.2 + T5.3 + T5.6 (auto-send whitelist with rate limits) → T5.4 + T5.11 (timing + escalation) → T5.12 + T5.13 + T5.14 + T5.15 enrichment.

### Cuts

- No autonomous send to new contacts (only whitelisted)
- No autonomous Slack post in channels with > 10 members initially
- No autonomous DM to non-contacts
- **No interpolated OOO content** (template-injection-safe; fixed strings only)
- **No auto-send without rate-limit enforcement**

## Tier 6 — Travel research

### Goal

Trip planning + price watching + checklists. Read-only research initially; no booking, no money movement.

### Capabilities

| ID | Feature | BR | Notes |
|---|---|---|---|
| T6.1 | Trip planner: destination + dates + constraints → flight/hotel/itinerary research | 1 | Output: structured plan; no booking |
| T6.2 | Price watcher: flight/hotel route, alert on drop | 1 | Polls + computes baseline; alert via push |
| T6.3 | Itinerary builder: multi-day plan + mapped routes + time budgets | 1 | Read-only output |
| T6.4 | Visa/document checker | 1 | **Sherpa proxy** (https://www.joinsherpa.com accessed 2026-06-09) primary source — authoritative visa data API. **Freshness pin**: cache TTL ≤ 24h. **Mandatory disclaimer in every output**: "Visa rules change; verify with the consulate before travel. This is research, not advice." **NO LLM-only path** — the LLM may reformat the Sherpa response but MUST NOT substitute for the source. **Pricing fallback**: if Sherpa B2B pricing > $200/mo OR access denied → fall back to **consulate-site scrape for top-10 destinations + Reasoner-summarize + mandatory disclaimer + ≤24h cache** (cited sources required; never LLM-only). |
| T6.5 | Pre-trip checklist: packing + holds + OOO + currency + plugs | 2 | Generates checklist; integrates with todos |
| T6.6 | In-trip mode: flight status + gate changes + local-time-aware reminders | 3 | Flight API + timezone shift |
| T6.7 | Post-trip expense gather: receipts from email + photos → expense report | 2 | Combines with T4.8 receipt-file |
| T6.8 | Local recommendations: restaurants + activities at destination | 1 | LLM + research; no booking |
| T6.9 | Conflict surfacer: trip dates conflict with existing meetings? | 1 | Cross with calendar |
| T6.10 | Travel insurance reminder | 1 | Per trip cost threshold |

### Build order

T6.1 + T6.3 (planning) → T6.4 + T6.5 (prep) → T6.2 + T6.6 (watch + in-trip) → T6.7 (post-trip) → enrichment.

### Cuts

- No booking (flights, hotels, restaurants) Phase 1
- No payment
- No location tracking
- No autonomous itinerary modifications mid-trip
- **No LLM-only visa output** (T6.4 mandates Sherpa source)

## Tier 7 — Finance (read-only first, write later)

### Goal

Visibility first. Read-only spend/balance/anomaly/subscriptions across accounts. **Workspace-aware** expense isolation. No autonomous money movement, ever.

### Capabilities

| ID | Feature | BR | Notes |
|---|---|---|---|
| T7.1 | Read-only account aggregation | **2** (re-tiered from 0) | **Plaid risk callout below.** Balances + transactions across all accounts; workspace-tagged. |
| T7.2 | Spend categorization (auto + manual override) | 1 | Weekly + monthly summary; per-workspace |
| T7.3 | Anomaly detection: unusual CC charges | 1 | Statistical + LLM verify; alert via push |
| T7.4 | Subscription auditor (cross with T4.6) | 1 | "Paying $X/mo for Y, last used 6mo ago"; per-workspace audit |
| T7.5 | Bill review: "looks normal" / "investigate this line" | 1 | Comparison vs prior bills |
| T7.6 | Tax doc gathering | 1 | **Primary: email-attachment harvesting**. Playwright portal scraping deferred to Phase-X (brittle + breaks on portal changes); reopen when 90% of operator's tax docs arrive by email + remaining 10% justify the maintenance cost. |
| T7.7 | Budget tracking: monthly targets, surface drift | 1 | Per-category, per-workspace targets in YAML |
| T7.8 | Investment summary read-only | 0 | Portfolio snapshot |
| T7.9 | Net-worth tracker (monthly snapshot) | 0 | Sum of accounts; trend line |
| T7.10 | Receipt + transaction matcher | 1 | Combines T4.8 + T7.2 |
| T7.11 | Bill-pay watcher: surface late bills, autopay status | 1 | Read-only; alerts only |
| T7.12 | ~~Bill pay~~ | — | **Phase-X deferred.** Reopen trigger = explicit operator request + ≥6mo green read-only finance + 2FA hardware key in flow. |
| T7.13 | Per-workspace expense isolation | 1 | Categorize transactions by workspace; surface per-workspace P&L view |
| T7.14 | Per-workspace invoice tracker | 1 | Outstanding, payment status, age; read-only first |
| T7.15 | Tax-bucketing across workspaces | 1 | Structured set so Jan tax-prep is mechanical; per-workspace bucket |
| T7.16 | Reimbursable tracker | 1 | Receipts marked "to-reimburse"; 30-day-unreimbursed alert |
| T7.17 | Per-workspace subscription audit | 1 | "This paid tool charged to acme but unused 3mo" |

### Plaid risk callout

Plaid (https://plaid.com accessed 2026-06-09) stores transaction data **externally on Plaid infrastructure** — operator credentials flow through Plaid + transactions cached server-side. Tiered at BR-2. Privacy ledger (Tier 1 §3.3) records every Plaid call.

**Alternatives evaluated**:

- **SimpleFIN** (https://www.simplefin.org accessed 2026-06-09) — operator-self-hosted bridge; transactions stay on operator infra. More setup work; fewer institutions. Recommended for the privacy-strict default.
- **Manual CSV import** — operator exports per-bank → Leah ingests. Zero third-party exposure. Tedious.

Spec body recommends: **SimpleFIN primary, Plaid secondary** for institutions SimpleFIN doesn't cover. Manual CSV as escape valve.

**Bank coverage pre-flight**: before M10 dispatch, run `leah finance preflight` → enumerate operator's known banks → cross-check SimpleFIN supported-institutions endpoint → produce coverage report. If gap > 20% by transaction volume → fall back to **manual CSV import** for missing banks, OR open **Plaid Trial** for that account specifically (per-account opt-in, not all-or-nothing).

### Build order

T7.1 + T7.2 + T7.8 + T7.9 (read-only foundation) → T7.3 + T7.4 + T7.5 + T7.7 (analysis) → T7.13 + T7.14 + T7.15 + T7.16 + T7.17 (workspace-aware) → T7.6 + T7.10 + T7.11 (workflow integrations). T7.12 deferred (Phase-X).

### Cuts

- No autonomous money movement, ever (BR-5 with 2FA, deferred)
- No autonomous investment trades
- No autonomous bill cancellation (proposes; operator acts)
- No credit-score tracking
- **T7.12 (bill pay)** deferred to Phase-X
- **T7.6 Playwright** deferred to Phase-X; email-harvesting primary

## Tier 8 — Social media

### Goal

Reduce, not amplify, time spent. Drafting + minimal engagement. Demoted on purpose.

### Capabilities

| ID | Feature | BR | Notes |
|---|---|---|---|
| T8.1 | Post drafter: from voice ramble or bullets | 1 | Output: drafts only |
| ~~T8.2~~ | ~~Schedule-post via Buffer/native APIs~~ | — | **Phase-X** (Twitter/LinkedIn write-paths). Brittle APIs + low ROI. |
| T8.3 | Mention monitor: surface mentions worth replying to | 1 | Filter by sentiment + commenter relevance |
| T8.4 | Engagement digest: "this post is doing well/poorly" | 0 | No autonomous engagement |
| ~~T8.5~~ | ~~Cross-post helper~~ | — | **Phase-X.** Tone-adapt drafting may live in T8.1; multi-platform write-path deferred. |
| T8.6 | Reply drafter for selected mentions | 1 | Operator-approved reply pipeline |
| ~~T8.7~~ | ~~Bluesky / Mastodon / etc.~~ | — | **Phase-X** auto-post path. Bluesky AT Protocol (https://atproto.com accessed 2026-06-09) noted as cheaper future alternative if Tier 8 write-paths ever reopen — open API, less rate-limit hostile than X/LinkedIn. |

### Build order

T8.1 + T8.4 (drafting + visibility) → T8.3 + T8.6 (selective engagement). T8.2 + T8.5 + T8.7 Phase-X.

### Cuts

- No autonomous engagement (likes, follows, replies)
- No follower growth tactics
- No autonomous DMs on any platform
- No automated cross-posting
- **T8.2 / T8.5 / T8.7 Phase-X** (write-paths to Twitter/LinkedIn/Bluesky)

## Cross-tier feature additions

### Operator-mode switcher state machine

`leah mode focus|standby|asleep|travel|sick|vacation` — **single enum, NO stacking**. Setting a mode replaces the previous mode. Sequence of operator commands → last-write-wins.

State machine:

```
standby (default)
  → focus     | auto-revert after 6h (configurable)
  → asleep    | auto-revert at 08:00 local
  → travel    | auto-revert on OOO event end
  → sick      | auto-revert after 24h
  → vacation  | manual clear only; dashboard nudge at 12h
```

- 6h auto-revert default for `focus` (prevents stuck-in-focus accidents).
- OS Focus mode (macOS / iOS) is an **input signal, NOT source-of-truth** — when OS Focus turns on, Leah suggests `leah mode focus` but doesn't auto-switch.
- Dashboard banner shows current mode + time-in-mode + revert time.
- Vacation: 12h after operator manual-set, banner nudges "still vacation?" + one-click clear.

Per mode:

- `focus`: pause notifications, decline new meetings, auto-OOO Slack, hold non-urgent
- `standby`: default; everything normal
- `asleep`: no notifications, queue everything for morning brief
- `travel`: trigger Tier 6 in-trip mode + Tier 5 travel OOO
- `sick`: like asleep + cancel today's meetings + auto-decline new
- `vacation`: Tier 3 §10.21 vacation-grade autopilot

### Cross-account financial-vs-time tradeoff

Bridges Tier 6 + Tier 7. Per-workspace hourly value lets Leah compute "this $40 flight saves 3 hours; in `acme` that's worth $X, in `personal` worth $Y."

### Universal "operator inbox"

`leah inbox` extended showing pending approvals across all tiers. Per-workspace by default; `--all` cross-workspace. **Concurrency model**: backed by the `approval_request` table (Tier 1 §3.9) with atomic claim semantics — multi-device taps on the same approval no-op the second click with "already decided." Mode-switch (`leah mode focus`) suppresses notification fires but leaves approvals pending; mode-clear re-surfaces in batch. 24h auto-expire + auto-deny + audit row.

### "Defer to me" pattern

Any action Leah is uncertain about, she defers — adds to operator inbox with reasoning.

### Receipt-finance-tax pipeline

End-to-end: T4.8 receipt-file → T7.10 transaction-matcher → T7.15 tax-bucketing across workspaces → T7.6 tax-doc-gather. Compounds across Tiers 4 + 7. Per-workspace bucket.

### "Surprise me" / "what should I do" prompt

End of brief: operator says "Leah, surprise me" → Leah picks one high-value low-friction action (a paper to read, a friend to message, a system to maintain). Anti-stagnation.

### Privacy-tier override (enumerated consumer table)

For sensitive content (medical, legal, personal-relationships, financial-specifics), operator marks a thread/topic "private". Privacy-tier override is an **enumerated consumer table**, not a vague "exclude from passive analysis."

| Downstream consumer \\ Privacy class | normal | sensitive | private |
|---|---|---|---|
| cloud LLM (overview §4.8 Anthropic) | allow | per-call | never |
| Litestream backup (overview §4.7a) | allow | allow | allow (encrypted) |
| event stream (`internal/obs`) | allow | hash-only | drop |
| audit log (Tier 1 §2.1) | allow | redacted | redacted + no body_blob |
| regatta dispatch (Tier 2) | n/a | n/a | n/a |
| ops dashboard | allow | summary-only | hidden |
| cross-tier KB (Tier 2 §2.17) | allow | excluded | excluded |

Mechanical lint: `scripts/check-privacy-leak.sh` (modeled on regatta `check-phase-x-leak.sh`) walks Memory schemas + dispatcher manifests + privacy-ledger config and fails closed if any consumer is missing a row in the matrix OR if any "private" data class lacks a "never" cell against a third-party reasoner.

### Per-tier cost ceiling

References overview §4.0 cost cap. Per-tier monthly + daily caps + per-call caps:

| Tier | Monthly | Daily | Per-call (where applicable) |
|---|---|---|---|
| 1 self-improvement | $30 | $1 | A/B shadow $0.10/event |
| 2 SWE | $105 | $3.50 | KB Q&A $0.50; spec→plan→execute $5 |
| 3 schedule/comms | $90 | $3 | per email draft $0.05 |
| 4 automation | $15 | $0.50 | file-op classify $0.01 |
| 5 external comms | $15 | $0.50 | per auto-send pre-check $0.02 |
| 6 travel | $5 | bursty | Sherpa $0.30/check |
| 7 finance | $30 | $1 | anomaly LLM-verify $0.05 |
| 8 social | $5 | bursty | post draft $0.10 |

Headroom = $30/mo system-wide spike absorption. Re-baseline after 2 weeks of M3 audit data (overview §4.0).

### Tier-7 shared workspace categorization rule

Single deterministic ladder applied across T7.13–T7.17 (all finance workspace bucketing):

1. **Merchant-tagged workspace** if `merchant_workspace_map` has an entry.
2. **Time-of-day × active calendar** at transaction time (e.g. weekday 9-5 + acme calendar event → acme).
3. **Operator-prompt at week-close batch** (Friday rollup).
4. **"uncategorized" sink** — surfaces in next week-close.

### Tier-7 tax-bucketing handoff format

Per-workspace `tax-YYYY.csv` with Schedule C codes + per-workspace `tax-YYYY.txf` (TurboTax-compatible) + per-workspace `tax-YYYY-receipts.zip`. Generated by T7.15.

### Tier-7 shared-subscription attribution

Rule: **primary-workspace = workspace with > 50% usage in trailing 30d**; tie → operator-prompt at week-close. Subscription continues billed to primary; usage detected via app-activity heuristic per workspace.

### sick + vacation mode combination

Operator-mode enum stays single (no stacking). Nested-state semantics are operator's job via the **mode notes field**: `leah mode sick --note "vacation underway"`. Mode stays single (`sick`); operator-readable note carries the qualifier.

### Pushover privacy row

Add row to the privacy-tier consumer matrix:

| Downstream consumer \\ Privacy class | normal | sensitive | private |
|---|---|---|---|
| Pushover / Twilio push (overview §4.7) | allow (note: lock-screen preview visible) | summary-only (no body content) | drop |

Push-preview text often visible on lock screen; classify accordingly.

### Reviewer-skip footer expectation

Design-doc frontmatter for any spec under `docs/specs/` MUST carry `Reviewer-agent-id:` + `Reviewer-recommendation:` fields (echoes regatta `feedback_adversarial_review_every_step`). Spec revisions require an independent reviewer subagent dispatch to fill them.

### Clipboard secrets denylist

Mandatory mechanical gate before any clipboard → LLM call (also lives in Tier 4 T4.3):

- AWS access key (`AKIA[0-9A-Z]{16}`)
- AWS secret key (`[A-Za-z0-9/+=]{40}`)
- GCP service account key (`AIza[0-9A-Za-z\-_]{35}`)
- JWT (`eyJ[A-Za-z0-9_-]+\.eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+`)
- CC numbers (`(?:\d[ -]*?){13,16}` + Luhn check)
- SSH private keys (`-----BEGIN [A-Z]+ PRIVATE KEY-----`)
- Generic secret-shaped (`[a-zA-Z0-9]{32,}` with high entropy + label-near match)

Hit → block + warn. No "scrub-and-send" — block + tell operator. Lint enforced via `internal/intake/clipboard/denylist_test.go`.

## Overall build sequence (system-level recap)

Reframed as **ordered milestones** (NOT calendar weeks). Re-baseline velocity after M5.

```
M0  Skeleton (infra)                                  [overview §4.0/§4.4/§4.7/§4.7a/§4.8 invariants]
M1  Tier 2 SWE minimum
M2  Memory layer (workspace-aware)
M3  Tier 1 self-improvement
M4  Voice
M5  Tier 3 multi-account email + calendar + daily briefs
    --- re-baseline velocity check; replan downstream milestones based on M0-M5 actual cadence ---
M6  Tier 2 enrichment + Slack + GitHub notifications + spec drafter
M7  Tier 4 automation (capture + recurring + alerts; T4.1/T4.9 BR-4 trail-and-window)
M8  Tier 5 external comms (auto-send templates with rate limits + escalation)
M9  Tier 6 travel research (Sherpa-backed visa)
M10 Tier 7 finance read-only (SimpleFIN primary + Plaid as needed; workspace-aware)
M11+ Tier 8 social drafting (write-paths cut; T8.1/T8.3/T8.4/T8.6 only) + iMessage + wake word
```

Calendar weeks removed — velocity has not been measured for a Leah-shaped repo. After M5 (Tier 3 minimum), the operator gets a re-baselined estimate from actual cadence.

Parallelism trade: regatta can dispatch parts of this once Leah itself is bootstrapped — Leah builds Leah from M4 onward.

## Open questions (cross-tier)

- Where does Leah's own code live: separate `leah/` repo confirmed; cite regatta-as-dispatcher.
- License for Leah: private Phase 1.
- Cloud-vs-local cost: capped at $200/mo (overview §4.0); tier-degrade ladder fires before cap.
- Multi-device sync: Phase 1 single-machine; Phase 2 home-server + thin clients via tailnet.
- When to harden Leah into a product vs keep personal: leave open until M11+.

## Cuts (cross-tier, explicit)

- No multi-user / SaaS
- No model fine-tuning on operator data
- No autonomous money movement, ever
- No autonomous public posts/replies, ever
- No autonomous purchases above explicit per-purchase threshold
- No screen-recording / always-on capture
- No phone-call recording without opt-in
- No location tracking by Leah
- No autonomous social-engagement
- No external customer / SaaS pitch for Leah Phase 1+
- **No file-mutation without trail-and-window** (T4.1 / T4.9)
- **No clipboard LLM call without denylist** (T4.3)
- **No LLM-only visa output** (T6.4)
- **No Twitter/LinkedIn auto-post** (T8.2 / T8.5 Phase-X)
- **No bill pay Phase 1** (T7.12 Phase-X)
- **No tax-portal Playwright Phase 1** (T7.6 deferred)
- **No stacked operator modes** (single enum)
- **No "private" data class routing to third-party reasoners** (privacy matrix)

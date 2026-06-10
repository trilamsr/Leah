# Wave 10 — osevent + push-API migration

Date: 2026-06-10. Successor to wave-9 velocity/responsiveness. Replaces 9 remaining polling sites uncovered by the wave-9 polling-pattern audit (11 total; wave-9 V2 SSE-migration already pushed 2 — `/api/state` and `/hud/recommendations`).

## 1. Goal

Operator-perceived "Leah notices things immediately." Replace cron/poll loops with OS- and vendor-native push so reaction latency goes from poll-interval (15s–60s) to event-arrival (<1s) and idle CPU drops.

## 2. In-scope

### O1 — `internal/osevent/` cgo package (foundation)

Single cgo entry point exposing typed Go channels for NSWorkspace, Contacts, FSEvents. (EventKit deferred — see Out-of-scope.) All in-scope downstream sources (O3, O4, O5) consume from here — no duplicate cgo bridges.

### O3 — Contacts push

`CNContactStoreDidChangeNotification` → `internal/macosmirror/contacts`. Replaces hourly rescan.

### O4 — Messages / Mail / Notes push

FSEvents watcher on the respective SQLite WAL paths (`~/Library/Messages/chat.db-wal`, `~/Library/Mail/V*/MailData/*`, `~/Library/Group Containers/group.com.apple.notes/NoteStore.sqlite-wal`). Coalesce bursts at 500ms; re-open DB on WAL bump.

### O5 — `internal/macos/activeapp/` push

NSWorkspace `NSWorkspaceDidActivateApplicationNotification` + 250ms coalesce + 30s per-pattern debounce (cite: wave-9 brief V8). Removes the 1s active-app poll.

### O6 — `audit.Logger.Subscribe(ch chan<- audit.Event)` push channel

Replaces 3 file-rescans (selflearn.Resolver, dashboard live counter, dispatcher feedback ingest) that today re-read the audit log every 5s. Subscribe-on-write fan-out. Buffer: per-subscriber `chan audit.Event` with capacity 1024; drop-oldest policy on full (newest event always delivered, oldest dropped first); `leah_audit_subscriber_dropped_total{subscriber=...}` counter exposed on `/health` and the existing `/metrics` endpoint.

### O7 — Regatta event-stream (gated on cross-repo API)

Hook `regatta.Subscribe(EventCh)` into daemonloop, dispatcher.ship, dispatcher.selfbuild — kills 3 daemonloop polls that scan regatta state every tick. **Cross-repo gate**: `regatta.Subscribe(EventCh)` does not yet exist in the regatta repo; O7 is blocked until that API ships. If regatta lands the API mid-wave, O7 ships as wave-10b; otherwise defers to wave-11.

### O9 — Per-adapter pull-with-shorter-interval (degraded mode, no push)

Webhook-push for gmail/gcal/linear/notion/jira requires a public HTTPS endpoint; operator has not authorized cloud-tunnel infra (Cloudflare Tunnel / ngrok / Tailscale Funnel), and the loopback-only invariant is sacred. Wave-10 therefore SKIPS the webhook receiver entirely and instead reduces per-adapter pull intervals to the floor each provider's rate-limit allows (typical 30s → 5–10s) as a degraded interim mode. Full push-receiver work (the prior O8 design + per-adapter subscription register) defers to wave-11 once operator authorizes tunnel infra.

File-disjoint per adapter so they parallelize.

## 3. Out-of-scope / Deferred

- **O2 — Calendar + Reminders push (EventKit)** — `docs/engineer/specs/macos-ecosystem-integration.md:158` marks EventKit CGO bridge as "Defer". Spec ADR stands; wave-10 does NOT amend it. EventKit-pushed sources defer to wave-11 (paired with a spec-amendment PR).
- **O8 + webhook push receiver** — gmail Pub/Sub + gcal channels + linear/notion/jira webhooks require a public HTTPS endpoint that contradicts the 127.0.0.1-loopback invariant. Defers to wave-11 once operator authorizes tunnel infra (Cloudflare Tunnel / ngrok / Tailscale Funnel). Wave-10 falls back to O9 degraded-mode pull-with-shorter-interval.
- Slack / Discord / WhatsApp / MS Teams — Socket Mode / Gateway requires persistent vendor session OR public HTTPS endpoint. Deferred wave-11.
- OWM / news / markets feeds — pull is fine; low frequency, no operator-perceived latency.
- Wails native HUD — deferred per existing ADR.
- EventKit deep-history backfill — out-of-scope; brief covers push only.

## 4. Sequencing

- **Phase A (file-disjoint, parallel up to 6)**: O1, O6. Foundation packages with no inbound deps.
- **Phase B (after A)**: O3, O4, O5 (all consume O1). O7 only if cross-repo regatta API has shipped — otherwise defer to wave-11. File-disjoint per package — up to 4 PRs parallel.
- **Phase C (after B)**: O9 per-adapter pull-interval reductions — file-disjoint per adapter, parallel.

Spec PRs serialize per CLAUDE.md dispatch rule; this brief is a single spec PR. Code PRs across in-scope O1/O3/O4/O5/O6/O7/O9 are file-disjoint and parallelize within each phase.

## 5. Adversarial-prior pruning

Carried forward from wave-9 brief V8 + ecosystem-integration ADR:

- **NSWorkspace fires 10–30x/min** under normal Cmd-Tab usage. Per-pattern debounce (30s) is REQUIRED downstream — without it, selflearn churns.
- **EventKit CGO bridge stays deferred.** macos-ecosystem-integration.md marks it deferred-by-design and wave-10 honors that ADR; O2 ships in wave-11 paired with a spec amendment. Avoids spec drift.
- **Operator-trust risk: always-on push = surveillance creep.** Every push source MUST respect the existing active-app blocklist (1Password, banking, private-mode apps). Enforced at the osevent layer, not per-adapter, so a single bypass cannot leak.
- **Loopback invariant is sacred; tunnel infra is wave-11 work.** Wave-10 has no inbound webhook listener at all — no port bound, no HMAC verify path, no surface to misconfigure. O9 degraded-mode pull beats a half-built receiver that drifts toward `0.0.0.0`.
- **WAL FSEvents fire mid-transaction.** O4 must re-open the DB connection on each event and tolerate partial rows; no assumption that a notify == committed write.

## 6. Test plan

Per push source:

- Fake event source emitting recorded fixture sequence.
- Replay test asserts: (a) event count matches fixture, (b) downstream consumer state matches expected, (c) coalesce/debounce windows enforced.
- Drop-on-slow-consumer test for O6: fill subscriber buffer past 1024, assert oldest dropped, newest delivered, drop counter increments.
- Active-app blocklist test for O5 (event suppressed when foreground app is in blocklist).
- O9 pull-interval test: assert reduced interval respects each provider's documented rate-limit floor.

## 7. Constraints

- No AI signatures (Co-Authored-By, "Generated with", etc.).
- File-disjoint PRs within each phase per CLAUDE.md.
- Worktree discipline: every code PR in `.claude/worktrees/agent-<id>/`.
- Independent reviewer subagent on every PR (canonical agent-id shape).
- Deletion default: every PR answers "what got smaller?" — expected wins are the cron tickers in daemonloop, the Sync() goroutines in macosmirror, and the rescan loops in selflearn.
- gh minimal fields on every `gh pr list/view`.

## 8. Success criteria

- Idle CPU (no operator activity, 5min sample) drops measurably vs wave-9 baseline.
- Notify→action latency for OS-event-pushed sources (synthetic test: contact added → HUD shows it; messages WAL bump → mirror picks up) drops from 30s p50 to <2s p50. Calendar/Reminders excluded (O2 deferred).
- Zero polling loops remain in `internal/macosmirror/contacts/`, `internal/macosmirror/messages/`, `internal/macosmirror/mail/`, `internal/macosmirror/notes/`, `internal/macos/activeapp/`, `internal/audit/` after Phase B. (Calendar/Reminders polls stay until wave-11 EventKit work.) `internal/daemonloop/` regatta polls drop only if O7 ships.
- O9 adapters retain pull-with-reduced-interval (degraded mode); full push migration tracked for wave-11 alongside tunnel-infra authorization.

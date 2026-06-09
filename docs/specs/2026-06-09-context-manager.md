---
title: Leah — Context manager (active-context, switch/show/history)
status: draft
phase: design
owner: tri
created: 2026-06-09
parent: 2026-06-09-leah-overview.md
related:
  - 2026-06-09-leah-overview.md   # §3.5 Workspaces (source-of-truth dimension)
  - 2026-06-09-leah-phase-x-multi-operator-roadmap.md  # workspace mostly deferred
---

# Context manager — fast path to "what am I doing right now"

## 1. Goal

Personal-use, single-operator. The operator (`tri`) juggles multiple parallel commitments — work repo, side project, OSS, personal. Today every `leah ask`, `leah ship`, `leah review` call has to re-state which lane it belongs to. That is the friction surfaced by overview §3.5 ("multi-context-heavy lifestyle"); fixing it is on the fast path.

This spec ships the minimum: ONE active context at a time, switched explicitly by CLI, persisted in the same SQLite file used for memory (`internal/memory/schema.sql`). Every Leah command that takes a prompt prepends `Current context: <name>` to the system prompt so downstream reasoners (Anthropic, regatta dispatch) get the lane without the operator typing it.

We do NOT ship: auto-infer from inbound email/calendar, multi-context queries, per-context persona/voice, account_scope taint, knowledge-firewall. All deferred per the audit ladder in §6.

### 1.1 Non-goals

- Concurrent contexts. Exactly one active. Switching is the supported operation.
- Cross-context queries. `leah ask --all-contexts` is sketched in §6 (cut) but not built.
- Daemon-side context inference. Daemon notifications inherit whatever context is active at tick time (see §5.2 + critic finding R-DAEMON).
- Workspace = context. The overview uses "workspace"; this spec uses "context" — same dimension. When the schema upgrades, the column stays `workspace_id` for forward-compat (§2).

## 2. Schema

Append to `internal/memory/schema.sql`. Re-runnable via `CREATE … IF NOT EXISTS`.

```sql
-- schema_version: 2 (additive — ctxmgr tables)

CREATE TABLE IF NOT EXISTS context (
  name         TEXT PRIMARY KEY,            -- 'personal', 'acme', 'side-proj-x'
  created_at   TEXT NOT NULL,
  description  TEXT                          -- optional one-liner
);

CREATE TABLE IF NOT EXISTS operator_state (
  id                INTEGER PRIMARY KEY CHECK(id=1),   -- singleton row
  active_context    TEXT NOT NULL REFERENCES context(name),
  updated_at        TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS context_switch_log (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  from_context  TEXT,                        -- NULL on first switch
  to_context    TEXT NOT NULL,
  switched_at   TEXT NOT NULL,
  reason        TEXT                          -- 'cli', 'voice-intent', 'auto-infer' (future)
);
CREATE INDEX IF NOT EXISTS idx_switch_time ON context_switch_log(switched_at);

-- Seed the default context + singleton if missing.
INSERT OR IGNORE INTO context(name, created_at, description)
  VALUES ('default', strftime('%Y-%m-%dT%H:%M:%SZ','now'), 'Implicit context for unsegmented work');
INSERT OR IGNORE INTO operator_state(id, active_context, updated_at)
  VALUES (1, 'default', strftime('%Y-%m-%dT%H:%M:%SZ','now'));
```

### 2.1 Forward-compat with overview §3.5

Overview §3.5 calls the dimension `workspace_id` and bolts it onto every Memory table. This spec uses `context` as the human noun + table name. Mapping rule for the future Phase-X migration: `context.name == workspace_id`. When the multi-workspace dimension activates, `context` gains `(persona, default_repos, default_account_scopes, …)` columns or is renamed to `workspace`; no data loss.

The decision to use `context` instead of `workspace` now: every other Leah CLI verb (`ask`, `ship`, `review`, `status`, `ctx`) reads like English with "context"; "workspace" reads like a tool noun. The schema rename when Phase X activates is one `ALTER TABLE … RENAME TO` migration — cheap.

### 2.2 Storage location

DB path: `~/.leah/memory.db` (same file the memory layer owns).

## 3. CLI

```
leah ctx new <name> [--desc "<one-line>"]
leah ctx switch <name>
leah ctx show
leah ctx history [--limit N]   # default 20
leah ctx list
```

### 3.1 `ctx new`

```
$ leah ctx new acme --desc "Acme Corp work account"
created context 'acme'
```

Errors:
- `name` already exists → exit 1, message `context 'acme' already exists`.
- Invalid name (regex `^[a-z][a-z0-9-]{0,31}$`) → exit 1.

### 3.2 `ctx switch`

```
$ leah ctx switch acme
switched: default → acme
```

Errors:
- Unknown context → exit 1, `unknown context 'acme'; run 'leah ctx new acme' first`.
- Already active → exit 0, `already in 'acme'` (idempotent).

### 3.3 `ctx show`

```
$ leah ctx show
context: acme
since:   2026-06-09T14:22:10Z (32m ago)
desc:    Acme Corp work account
```

Phase 1.5 — surface "last N actions taken in this context" by joining against `audit_log` once Tier 1 audit lands a `context` column (see §5.1 + critic finding R-AUDIT-TAG). Until then, `ctx show` prints only the active row.

### 3.4 `ctx history`

```
$ leah ctx history --limit 5
2026-06-09T14:22:10Z  default → acme   (cli)
2026-06-09T11:05:33Z  acme    → personal (cli)
2026-06-09T09:18:02Z  default → acme   (cli)
```

### 3.5 `ctx list`

```
$ leah ctx list
* acme       Acme Corp work account
  default    Implicit context for unsegmented work
  personal
```

`*` marks active.

## 4. Integration with leah ask / ship / review

Every entry point that builds a Reasoner prompt:

1. Calls `ctxmgr.Active(ctx)` → `(Context, error)`. ~1ms SQLite read; cached for the duration of the single CLI invocation (no daemon-side cache — see §5.2).
2. Prepends one line to the system prompt: `Current context: <name>. Description: <desc>.` (description omitted if empty).
3. Tags the audit_log entry with `context` field (additive to `audit.Entry`).

Failure mode: if `operator_state` row is missing (corrupted DB), the call returns `Context{Name: "default"}` and logs a WARN to stderr — never blocks the user-facing command.

### 4.1 Wiring sequencing

Implementation order:

1. `ctxmgr.Manager` + schema (this spec, Stage 3).
2. `audit.Entry` gains `Context string` (follow-up issue; one-line additive).
3. `dispatcher/ask.go`, `dispatcher/ship.go`, `reviewer/reviewer.go` call `ctxmgr.Active` and weave (follow-up issue).
4. Daemon (`internal/daemonloop`) reads active context at tick boundary and tags fired notifications (follow-up issue; see §5.2).

This spec stops at step 1. Steps 2-4 are tracked separately to keep the diff narrow.

## 5. Build order (max 4 tasks)

| # | Task | LoC | Tests |
|---|------|-----|-------|
| 1 | Append schema to `internal/memory/schema.sql` (or create `internal/ctxmgr/schema.sql` if memory file missing) | ~30 | n/a (validated by Manager open) |
| 2 | `internal/ctxmgr/ctxmgr.go` — `Manager` w/ `NewContext`, `Switch`, `Active`, `History`, `List` | ~180 | unit |
| 3 | Tests — TDD: TestSwitchPersistsActive, TestHistoryReturnsLastSwitches, TestActiveOnFreshDBReturnsDefault, TestNewContextRejectsDuplicate, TestSwitchUnknownErrors | ~150 | unit |
| 4 | CLI wiring in `cmd/leah` `ctx` subcommand (follow-up issue, NOT in this spec's diff) | ~80 | n/a |

This spec ships tasks 1-3. Task 4 + audit-tagging + dispatcher weaving are follow-up issues filed at scaffolding-commit time.

## 6. Cuts (Phase X — reopen on trigger)

| Cut | Reopen trigger |
|---|---|
| Auto-infer context from inbound email/cal domain (overview §3.5 ladder) | Operator opens 2nd account-scoped context AND reports manual-switch fatigue ≥3×/week |
| `--all-contexts` cross-context queries | Operator runs first cross-context audit need (e.g. "find all decisions about Phoenix across acme + personal") |
| Per-context persona / voice / signature (overview §3.5 bullet 4) | First time wrong persona ships to wrong account |
| `account_scope` taint propagation (Tier 3 §8) | First Tier 3 email-drafter integration |
| Voice intent shortcut "I'm in `<name>` mode" (Tier 3 §4.5) | TTS intake online + Tier 3 §4.5 lands |
| Knowledge-firewall BR-2 cross-context reads (Tier 2 §3.18) | Second context active AND knowledge-leak surfaces |
| Daemon-side per-tick context inference | See §5.2 — for now daemon inherits active-at-tick; reopen when this misroutes |
| Operator-mode state machine (focus/asleep/travel) integration | Mode dimension is orthogonal; combine when both stabilize |

## 7. Critic findings (addressed inline)

CRITICAL / HIGH revisions are folded into §§2-5 above. MED / LOW captured as follow-up issues at scaffolding-commit time.

---

## Adversarial review

Findings:

### R-RACE [HIGH] — multi-process write race on operator_state singleton

**Claim**: CLI process and daemon process can both hold the SQLite file open. If `leah ctx switch acme` fires while `leah-daemon` is mid-tick reading `active_context`, the daemon may read stale value, fire a notification tagged `default` while the user already switched to `acme`.

**Resolution**: Folded into §5.2 (new sub-section).

§5.2 — **Daemon read semantics**

- Daemon NEVER caches `active_context` across ticks. Each tick re-reads (~1ms).
- SQLite in WAL mode (`PRAGMA journal_mode=WAL`) — readers + writers don't block, no `database is locked` errors at this scale (1 writer, 1-2 readers, <100 writes/day).
- Race window: between daemon's tick-start-read and notification-fire is bounded (<100ms typical for a single watcher poll cycle). If operator switches inside that window, the in-flight notification carries the prior context. Acceptable — matches overview §3.5a "mid-job switch never preempts".
- Mechanical: ctxmgr opens DB with `?_journal=WAL&_busy_timeout=5000` DSN suffix.

### R-DAEMON [HIGH] — daemon-fired audit rows have ambiguous context

**Claim**: Daemon ticks fire whether operator is logged in or not. At 3am the daemon may fire a notification — there is no "session-context" for that tick. Tagging it with whatever `active_context` happens to be in `operator_state` is misleading (the operator was asleep, not "in acme context").

**Resolution**: Folded into §5.2. Two-rule policy:

1. Daemon-fired events inherit `operator_state.active_context` at tick-start — matches overview §3.5 bullet 5 ("Cron-fired events inherit operator's active workspace").
2. Audit rows from daemon-origin carry an additional field `source: 'daemon'` (additive to `audit.Entry`). UI can later distinguish daemon-tagged context vs operator-CLI-tagged context. Not built in this spec; flagged as Tier-1 follow-up.

This is good-enough for Phase 1. Auto-infer (e.g. "tick at 3am AND no operator session active in last 6h → tag `null`") deferred to §6.

### R-SWITCH-LOG-UNBOUNDED [MED] — context_switch_log grows forever

**Claim**: If operator switches 50× / day, after a year that's ~18K rows. Not large, but unbounded.

**Resolution**: Acceptable for Phase 1. 18K rows × ~80 bytes = ~1.4 MB / year — invisible to SQLite. `leah ctx history` defaults `LIMIT 20`. Compaction (drop rows older than 90 days) deferred until table crosses 100K rows OR operator notices `ctx history` slowness. Documented as Phase-X cut in §6.

### R-VOICE-INTENT [LOW] — voice "I'm in `<name>` mode" not handled

**Claim**: Tier 3 §4.5 promises voice-intent context switching.

**Resolution**: Explicit Phase-X cut in §6. Voice-intent shortcut is a 3-line plumbing job (parse intent → call `ctxmgr.Switch`) once TTS intake exists. Cheap to add later; nothing in this spec blocks it.

### R-DEFAULT-NAME [LOW] — `default` is reserved without clear escape

**Claim**: What if the operator types `leah ctx new default`?

**Resolution**: §3.1 regex permits the name; `ctx new` errors with `already exists`. Operator can `leah ctx switch default` to revert. No hidden semantics.

### R-AUDIT-TAG [MED] — `ctx show` last-N-actions blocked on audit schema

**Claim**: §3.3 promises "last N actions in this context" but `audit.Entry` has no context field.

**Resolution**: Already deferred in §3.3 ("Phase 1.5"). Follow-up issue files the one-line additive change to `audit.Entry`. This spec's `ctx show` ships the active row only.

---

End spec.

---
slug: macos-integration-plan
status: draft
phase: self-host
owner: leah
covers: W25–W31
---

# macOS integration delivery plan (W25–W31)

Pairs the two specs:
- `docs/engineer/specs/2026-06-10-macos-ecosystem-integration.md`
- `docs/engineer/specs/2026-06-10-knowledge-graph.md`

Each wave is sized to a single PR (~500 LOC budget) with a green test plan
and a concrete `unblocks-what`. Waves ship independently; later waves do not
silently require earlier wave features beyond what is listed.

Trade-offs surface at the head of each wave (priority per `CLAUDE.md`:
UX > performance > long-term). Default simpler. Three similar lines beat a
premature abstraction.

---

## W25 — `internal/macos/` skeleton + Calendar + Contacts + Reminders

**Goal.** Establish the package skeleton, mirror DB schema v1, and the three
lowest-risk read adapters. These are the "obviously safe" Apple stores —
small, well-documented schemas, low sensitivity.

**Files touched (≤500 LOC).**
- `internal/macos/macapp.go` — `MacApp` interface, `Item`, `Query`,
  sentinels.
- `internal/macos/mirror/mirror.go` — schema v1 (entities, items, meta),
  `Open(path)`, `0600` enforcement, schema-version compare.
- `internal/macos/calendar/{reader,reader_test}.go` — RO open of
  `Calendar.sqlitedb`, golden fixture in `testdata/`.
- `internal/macos/contacts/{reader,reader_test}.go` — RO open of
  `AddressBook-v22.abcddb`, golden fixture.
- `internal/macos/reminders/{reader,reader_test}.go` — RO open of
  `Data-*.sqlite`, golden fixture.
- `internal/macos/doc.go` — package overview citing the two specs.

**Risk.** Apple-store schema drift across macOS versions. Mitigated by
golden fixtures captured from the spec author's machine + a `Available()`
probe that fails-closed if the expected tables are missing.

**Size.** ~450 LOC including tests.

**Test plan.** Hermetic tests only, per the macOS spec §8. CI never reads
the real `~/Library`. `make check` green.

**Unblocks.** W26 sync loop has three real adapters to drive.

---

## W26 — Mirror-sync loop wired into daemon

**Goal.** The daemon ticks every 60 s and calls `Sync` on every registered
`MacApp`. Mirror is queryable from the daemon process.

**Trade-off.** Re-using daemonloop's `LastTick` (PR #45) instead of adding
a second timer wheel. Simpler, but skews on sleep/wake — accepted per UX
priority (one catch-up tick is fine, replaying queued ticks is not).

**Files touched.**
- `internal/macos/sync/loop.go` — `Loop.Run(ctx, []MacApp)` driven by
  daemonloop ticks.
- `cmd/leahd/build_macos.go` — wiring constructor.
- `cmd/leahd/run.go` — register loop into the daemon graph.
- `internal/macos/sync/loop_test.go` — fake-clock integration test.

**Risk.** A slow adapter blocks the tick. Mitigation: each `Sync` runs
with a per-adapter 2 s context budget; timeout logs an audit row with
`Kind: "macos_sync_timeout"` and the loop continues.

**Size.** ~250 LOC.

**Test plan.** Integration test with 3 fake adapters; assert each `Sync`
is invoked once per tick; assert timeout does not stall siblings.

**Unblocks.** W27 adapters get a working sync loop to plug into.

---

## W27 — Notes + Mail + Messages read adapters

**Goal.** Add the three Full-Disk-Access-required adapters. These are
higher-sensitivity than W25, so they are gated by the `internal/attestation`
question pool (PR #67) on first connect, and the Messages body is stored
as a 256-char prefix only.

**Trade-off.** Messages body truncation is a UX cost (operator might want
the full text) but per spec §7 the privacy weight wins. Full body requires
a deliberate second toggle; we don't ship the toggle in W27.

**Files touched.**
- `internal/macos/notes/{reader,reader_test}.go`
- `internal/macos/mail/{reader,reader_test}.go` — Mail.app, distinct from
  `internal/adapters/gmail/`. Reads `Envelope Index` SQLite.
- `internal/macos/messages/{reader,reader_test}.go` — `chat.db`, body
  truncation at 256 chars.
- `internal/attestation/macos_scopes.go` — register
  `macos:notes:read`, `macos:mail:read`, `macos:messages:read` (high-sens
  pool slice). Depends on PR #67 landing.

**Risk.** `chat.db` schema is famously gnarly (handles, chats, message
joins). Mitigation: golden fixture covers the four common message shapes
(SMS, iMessage, group, attachment-only); other shapes return empty
`ContactRefs` rather than panic.

**Size.** ~500 LOC (3 readers + scope registration).

**Test plan.** Per-adapter hermetic tests with golden fixtures. Messages
truncation regression test asserts no `Item.Body` exceeds 256 chars even
when the source row is longer.

**Unblocks.** Morning-brief mail enrichment (W30); knowledge-graph
`Person` entity gets Mail + Messages signal sources.

---

## W28 — Ambient signals: Spotlight + Focus + active-app + Bluetooth + Wi-Fi

**Goal.** Add the non-database signal adapters. These are subprocess- or
plist-driven, not SQLite, and they need different debounce + summarization
discipline than the database adapters.

**Trade-off.** Spotlight is a query API, not a corpus — we expose it as
a `Query`-only adapter (no `Sync`, returns `nil`). The `MacApp` interface
already accommodates this via `Sync` returning `nil` for no-op adapters.

**Files touched.**
- `internal/macos/spotlight/{reader,reader_test}.go` — `mdfind` subprocess.
- `internal/macos/focus/{reader,reader_test}.go` — DoNotDisturb plist read.
- `internal/macos/activeapp/{reader,reader_test}.go` — `lsappinfo`
  subprocess + 5 s residency debounce.
- `internal/macos/bluetooth/{reader,reader_test}.go` — `system_profiler`
  subprocess.
- `internal/macos/wifi/{reader,reader_test}.go` — `networksetup` +
  `wdutil info`.
- `internal/macos/foreground/blocklist.go` — read
  `~/.leah-state/macos-foreground-blocklist.txt`, default banking +
  Keychain bundle IDs.

**Risk.** Subprocess output parsing breaks across macOS versions.
Mitigation: each reader has a `parseVN(s string)` helper with versioned
golden output; CI runs every version's parser against the latest golden.

**Size.** ~500 LOC.

**Test plan.** Hermetic tests with captured subprocess output in
`testdata/`. Debounce test for active-app uses a fake clock.

**Unblocks.** Knowledge graph `Location` entity gets Wi-Fi SSID mapping.
Voice-comm (W11+) gains a real `Focus` signal to suppress wake on
"Do Not Disturb".

---

## W29 — Shortcuts.app integration

**Goal.** Operator's existing Shortcuts become callable tools — Leah can
run `shortcuts run "Daily Standup"` after attestation.

**Trade-off.** Shortcuts is a write surface (each shortcut may touch any
Apple service), so we treat invocation like a one-shot write per the
macOS spec §5. AppleScript subprocess via `shortcuts run`.

**Files touched.**
- `internal/macos/shortcuts/{runner,runner_test}.go` — `shortcuts list`
  + `shortcuts run` subprocess wrappers; attestation gate per call.
- `internal/attestation/macos_scopes.go` — add `macos:shortcuts:run`.
- `cmd/leah/macos_shortcuts.go` — `leah shortcuts list`,
  `leah shortcuts run <name>`.

**Risk.** A malicious shortcut name (e.g. shell metachars) passed to
subprocess. Mitigation: shortcut names validated against the list
returned by `shortcuts list` first; reject any name not in the live
list.

**Size.** ~300 LOC.

**Test plan.** Hermetic test with fake `shortcuts` binary on
`PATH`. Attestation-denied test asserts subprocess not invoked.

**Unblocks.** Operator-curated automations callable from Reasoner;
gives Leah a write seam without inventing one.

---

## W30 — Knowledge graph package + first cross-app query

**Goal.** Ship `internal/knowledge/` and wire its first caller —
morning-brief enrichment that asks "who am I meeting today and what have
they said recently?" Returns a `Result` whose scalars feed the brief
template.

**Trade-off.** The package boundary keeps recommend.Engine (W15+, LRA
spec) and voice-comm (W11+) from depending on macos mirror internals.
The cost is one extra layer; the win is the scalar-only Reasoner contract.

**Files touched.**
- `internal/knowledge/graph.go`, `entity.go`, `resolver.go`,
  `query.go`, `persist.go`, `knowledge_test.go` — full package per
  the knowledge-graph spec §2.
- `internal/knowledge/resolvers/{person,project,event,location}.go` —
  per-Kind impls.
- `internal/brief/morning.go` — call `Graph.Query` for each calendar
  attendee in today's events; thread scalars into the brief template.
- `scripts/check-no-raw-knowledge-in-prompts.sh` — lint gate per
  knowledge-graph spec §5.
- `scripts/check-knowledge-no-net.sh` — `grep` gate forbidding `net/http`
  in `internal/knowledge/`.

**Risk.** Resolver person-merge bug fuses two real people. Mitigation:
golden test for entity merge from the spec test plan (§7); merge appends
`audit.jsonl` row so the operator can audit and run `leah forget`.

**Size.** ~500 LOC (package + brief wiring + lint gates).

**Test plan.** Per the knowledge-graph spec §7: synthetic-graph tests,
mirror → graph propagation, `Forget`, entity-merge, scalar-only
projection regression, retention eviction, no-network grep gate.

**Unblocks.** Morning-brief is meaningfully better. Recommendation
engine (LRA spec, W15+) has a context source. Voice-comm session can
ask "who is the operator talking about?"

---

## W31 — `leah init` first-launch wizard

**Goal.** A guided CLI that walks a new operator through the TCC grants
and attestation rows for their chosen integration set. Replaces ad-hoc
`leah connect <macapp>` for the common case.

**Trade-off.** UX over engineering elegance — the wizard is a sequential
script with hardcoded recommended-defaults rather than a generic
plugin-discovery framework. Per `CLAUDE.md`: three similar lines beat a
premature abstraction.

**Files touched.**
- `cmd/leah/init.go` — interactive wizard; uses
  `x-apple.systempreferences:` deep links per macOS spec §3.
- `cmd/leah/init_test.go` — table tests with a fake terminal.
- `docs/operator/macos-first-launch.md` — operator-facing doc.

**Risk.** Wizard fails partway and leaves attestation in a half-written
state. Mitigation: each step is its own `audit.jsonl` row; re-running
`leah init` resumes from the last successful row.

**Size.** ~300 LOC.

**Test plan.** Hermetic tests with a fake `os/exec` for the
`open x-apple.systempreferences:` step; assert each successful TCC verify
appends one audit row, assert resume-from-partial works.

**Unblocks.** Operator onboarding stops being a 9-step README; becomes
a single command. Documentation surface shrinks.

---

## Summary table

| Wave | Goal | Size | New surface | Unblocks |
|---|---|---|---|---|
| W25 | Skeleton + Calendar/Contacts/Reminders | ~450 | `internal/macos/{macapp,mirror,calendar,contacts,reminders}` | W26 |
| W26 | Sync loop wired into daemon | ~250 | `internal/macos/sync` | W27 ambient signals |
| W27 | Notes + Mail + Messages | ~500 | three FDA adapters | brief enrichment, graph |
| W28 | Spotlight, Focus, active-app, BT, Wi-Fi | ~500 | five subprocess adapters | voice-comm Focus |
| W29 | Shortcuts.app integration | ~300 | `internal/macos/shortcuts` | operator-curated tools |
| W30 | Knowledge graph + morning brief | ~500 | `internal/knowledge/` | LRA W15+, voice context |
| W31 | `leah init` wizard | ~300 | `cmd/leah/init.go` | onboarding |

Total: ~2 800 LOC across 7 PRs.

## What got smaller after this plan

- No more "how does Leah understand my Mac stuff" ambiguity — single
  canonical spec pair, single delivery plan.
- Future macOS adapters (FaceTime, Health read, HomeKit read) reuse the
  matrix-row + adapter-interface pattern instead of inventing a new shape.
- Cross-app correlation lives in one package (`internal/knowledge/`)
  instead of being re-implemented inside `internal/brief/`,
  `internal/voice/`, `internal/recommend/`.
- Operator onboarding collapses from a 9-step README to one
  `leah init` invocation by W31.

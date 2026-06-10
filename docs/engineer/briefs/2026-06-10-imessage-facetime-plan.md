---
slug: imessage-facetime-plan
status: draft
phase: self-host
owner: leah
---

# iMessage + FaceTime delivery plan (Waves 20-24)

Combined delivery plan for the macOS-native outbound channels. Specs:

- `docs/engineer/specs/2026-06-10-imessage-adapter.md`
- `docs/engineer/specs/2026-06-10-facetime-adapter.md`

Both adapters break the gmail/gcal OAuth parallel — macOS-local credentials
(operator-signed-in Messages.app + Apple-UI confirmation for FaceTime) replace
the token file. The Attestor + per-RPC scope + `gateAndExec` ordering carries
over unchanged.

## Sequencing principle

One adapter per PR, ≤300 LOC, scaffold-first (no daemon wiring). The CLI
`connect` subcommands land **after** both scaffolds so they share probe
patterns. Dispatcher wiring lands once `connect` is real. Rate-limit
middleware extracts last, when there are two real callers to extract from.

## Wave 20 — iMessage adapter scaffold

**Goal**: Land `internal/adapters/imessage` with the full surface and test
suite from the spec. No daemon, no CLI, no `go.mod` edit.

**Files touched**:

- `internal/adapters/imessage/doc.go` (new)
- `internal/adapters/imessage/imessage.go` (new, ~180 LOC)
- `internal/adapters/imessage/imessage_test.go` (new, ~260 LOC)
- `docs/engineer/specs/2026-06-10-imessage-adapter.md` (status -> `shipped`)

**Test plan**: nine tests from the spec, hermetic. Failing-test-first commit
captures the red output in the PR body per CLAUDE.md TDD rule.

**Risk**: Low. Pure-Go scaffold behind an `OSExec` seam.

**Size**: M (~300 LOC including tests; spec says ≤300 LOC for this wave).

**Unblocks**: Wave 22 (`leah connect imessage`), Wave 23 (`leah ship --imessage`).

## Wave 21 — FaceTime adapter scaffold

**Goal**: Land `internal/adapters/facetime` with the full surface and test
suite. No daemon, no CLI.

**Files touched**:

- `internal/adapters/facetime/doc.go` (new)
- `internal/adapters/facetime/facetime.go` (new, ~120 LOC)
- `internal/adapters/facetime/facetime_test.go` (new, ~180 LOC)
- `docs/engineer/specs/2026-06-10-facetime-adapter.md` (status -> `shipped`)

**Test plan**: nine tests from the spec. Hermetic. Failing-test-first.

**Risk**: Low. Smaller than iMessage because URL-scheme has no AppleScript
template to escape.

**Size**: S (~200 LOC including tests).

**Unblocks**: Wave 22 (`leah connect facetime`), Wave 23 (`leah call`).

## Wave 22 — `leah connect imessage` + `leah connect facetime`

**Goal**: CLI subcommands that probe the macOS environment and write a
`connect_*` audit row so the operator has a record of consent.

**iMessage probe**:

1. Run `osascript -e 'tell application "Messages" to get name of accounts'`
   via the shared OSExec.
2. Non-zero exit -> print "iMessage: not signed in / Automation grant
   missing"; show System Settings -> Privacy & Security -> Automation path;
   exit 1.
3. Zero exit -> Attestor prompt; on consent, write audit row
   `Kind: "connect_imessage"`.

**FaceTime probe**:

1. Confirm `/System/Applications/FaceTime.app` exists (cheap presence
   check; full sign-in state cannot be polled without UI).
2. Attestor prompt; on consent, write audit row `Kind: "connect_facetime"`.

**Files touched**:

- `cmd/leah/connect_imessage.go` (new)
- `cmd/leah/connect_facetime.go` (new)
- `cmd/leah/connect.go` (subcommand registration; expects shape from PR #47)
- `internal/connect/imessage.go` (probe logic + audit row, ~80 LOC)
- `internal/connect/facetime.go` (probe + audit row, ~50 LOC)
- Tests: hermetic via `OSExec` fake.

**Risk**: Med. Depends on PR #47 (`leah connect` shape) being merged.
Re-bases if that shape shifts.

**Size**: M.

**Unblocks**: Wave 23 (the user has now opted in).

## Wave 23 — Dispatcher wiring (`leah ship --imessage`, `leah call`)

**Goal**: Operator-facing CLI surfaces that actually send messages and
initiate calls. The adapters get wired into the daemon's `Attestor` (shared
with gmail/gcal).

**Files touched**:

- `cmd/leah/ship.go` — add `--imessage <to>` flag path.
- `cmd/leah/call.go` (new) — `leah call <callee> [--audio]`.
- `internal/dispatcher/imessage_send.go` (new) — owns the Attestor instance
  and the audit-row sink.
- `internal/dispatcher/facetime_call.go` (new).
- `cmd/leah-daemon/build_imessage.go` (new) — constructs the adapter with
  the real `os/exec`-backed OSExec.
- `cmd/leah-daemon/build_facetime.go` (new).
- Integration tests under `internal/dispatcher/` cover happy path +
  attestation-denied + rate-limited + permission-denied per adapter.

**Risk**: Med. First time the adapters meet the real Attestor; surface
mismatches surface here.

**Size**: L (estimated 400-500 LOC across files; may split into two PRs if
review feedback demands).

**Unblocks**: Wave 24 extraction.

## Wave 24 — Shared rate-limit middleware + audit dashboard widget

**Goal**: Extract the inlined per-adapter rate limiter into shared
middleware now that two callers exist. Add a small status widget to the
audit dashboard showing per-adapter spam stats (`{adapter, sends_1m,
sends_1h, denied_count}`).

**Files touched**:

- `internal/adapters/ratelimit/ratelimit.go` (new) — token-bucket or
  fixed-window; pick simplest after looking at the two real call sites.
- `internal/adapters/imessage/imessage.go` — swap inlined limiter for the
  shared one. Tests update to the shared seam.
- `internal/adapters/facetime/facetime.go` — same.
- `internal/audit/dashboard/spam_widget.go` (new).
- Migration doc note: shared middleware default limits MUST match the
  per-adapter MVP defaults (10/min imessage, 5/min facetime) until an
  operator-config issue lands.

**Risk**: Med. Premature shared-middleware was rejected at MVP per CLAUDE.md
"three similar lines beat a premature abstraction" — this wave only
triggers once iMessage AND FaceTime are in production.

**Size**: M.

**Unblocks**: Future macOS-native adapters (Mail.app, Reminders, Notes) and
non-macOS adapters (slack, sms-via-twilio) that need the same spam fence.

## Cross-cutting decisions

- **No OAuth surface** for either adapter. The OAuth token-file convention
  documented in the gmail / gcal specs does NOT apply. Operator consent is
  carried by (a) the Attestor prompt, (b) Apple's Automation grant
  (iMessage) or UI confirmation (FaceTime).
- **No new Go modules** across all five waves. Subprocess via `os/exec`.
- **No `go.mod` edits** across all five waves.
- **Hashed audit fields** (`recipient_hash`, `callee_hash`) carry through
  unchanged into Wave 24's dashboard widget. Plaintext recipients/callees
  must never appear in any audit-derived UI.
- **Trade-off honesty**: FaceTime's "operator presses dial" UX is worse
  than fully-automatic dialing. We accept that cost; private-API
  workarounds are an explicit non-goal across all five waves.

## Anti-goals (none of these land in W20-W24)

- Inbound iMessage listening / FaceTime answer automation.
- Group chats, attachments, reactions, edit/recall.
- Call recording, screen sharing, hang-up / mute control.
- Phone-tree / DTMF automation.
- Operator-configurable rate limits (issue filed; deferred).
- Recipient / callee allow-listing.
- Signed app / notarization automation (separate distribution track).

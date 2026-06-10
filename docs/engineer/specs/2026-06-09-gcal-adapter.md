---
slug: gcal-adapter
status: draft
phase: self-host
owner: leah
---

# Google Calendar adapter (MVP)

## 1. Goal

Give Leah a typed seam over Google Calendar so the morning-brief flow can
read today's schedule and the meeting-create flow can write a single event.
MVP is read+create-one — recurrence, attendees, reminders, free/busy stay
deferred until a concrete caller asks.

## 2. Dependency (adopt-over-build)

- Module: `google.golang.org/api/calendar/v3`
- Tag: `v0.215.0` (release 2026-03-04)
- License: BSD-3-Clause — `https://github.com/googleapis/google-api-go-client/blob/v0.215.0/LICENSE`
- Why this module: official Google-maintained client, same one
  google.golang.org/api/gmail/v1 ships through; sharing the transport
  surface with the gmail adapter (D1) cuts the OAuth-token plumbing in
  half and avoids two independent HTTP retry profiles.
- `go.mod` edit deferred — D1 (gmail) also requires this module; one
  combined `go mod tidy` lands in the W9 wiring follow-up to avoid a
  rebase storm. Required line:
  `require google.golang.org/api v0.215.0`

## 3. Token storage + attestation flow

The adapter never touches a token file directly. Boot wiring passes
`Config.TokenPath = "$HOME/.leah-state/secrets/gcal-token.json"` and an
`Attestor` implementation; the operator-attestation gate
(`internal/dispatcher/selfbuild.go` `AttestationQuestionsPath` +
`audit.Logger.Append`) is the only writer of the token file and the only
producer of the `Attestor` answer.

Per-call gate. Every public method (`ListToday`, `CreateEvent`) calls
`gateAndToken(ctx, scope)` which runs `Attestor.Attest(ctx, scope)` BEFORE
loading the bearer from `TokenSource`. Ordering is load-bearing — a
denied action must not materialize the token, otherwise the secret would
leak into a logger / panic trace even when the operator said no. Scopes
are per-action so the audit log distinguishes read (`ScopeListToday =
"gcal:list_today"`) from mutate (`ScopeCreateEvent = "gcal:create_event"`).
Mirrors the gmail adapter (D1) so wiring code can share one `Attestor`
implementation across both adapters.

Flow:
1. Operator runs `leah connect gcal` (wiring wave, out of scope here).
2. CLI prompts an attestation question, records the answer in
   `audit.jsonl` with `Kind: "connect_gcal"`, then exchanges the OAuth
   code for a token and writes it to `TokenPath`.
3. Daemon boot calls `gcal.New(Config{TokenPath: ..., Attestor: ...})`;
   missing `TokenPath` or missing `Attestor` fails construction. With a
   valid token-path but no `calendarService` yet, every method returns
   `ErrAuthRequired` until the wiring wave injects one.
4. Each RPC re-enters the attestation gate via `gateAndToken` — silent
   bypass would skip the audit row and the operator-consent UX.
5. Token refresh re-enters the attestation gate — silent refresh would
   bypass the audit row.

## 4. Future wiring (W9 follow-up wave)

- `cmd/leahd/build_gcal.go` constructs the real `calendarService` from
  an `oauth2.TokenSource` + `calendar.Service`, and passes the daemon's
  shared `Attestor` (the same instance the gmail adapter receives) plus a
  `TokenSource` implementation that reads `TokenPath` on demand. Consent
  flow: each `ListToday` / `CreateEvent` call invokes
  `Attestor.Attest(ctx, "gcal:list_today")` /
  `Attestor.Attest(ctx, "gcal:create_event")` — a denied row is appended
  to `audit.jsonl` and the bearer token is never read.
- `internal/brief/morning.go` calls `Adapter.ListToday` and stitches the
  events into the existing brief template — this is the high-value tie-in
  that justifies the adapter existing at all.
- `internal/dispatcher/ship.go` gains a `--calendar-invite` path that
  calls `Adapter.CreateEvent` after a ship completes (operator-opt-in).
- Tests for the wiring wave cover end-to-end via a httptest stand-in for
  the Google API; the adapter's own table tests stay pure-Go.

## 5. Threat model

- **Operator-consent bypass**: every secret-touching RPC routes through
  `gateAndToken`, which calls `Attestor.Attest(ctx, scope)` BEFORE
  reading the bearer from `TokenSource`. A `nil` return advances to the
  token load; any non-nil return aborts the RPC with `ErrAttestationDenied`
  (wrapped) and the `TokenSource.Token` call never executes.
  `TestListTodayTable/attest_denied` and `TestCreateEventTable/attest_denied`
  pin this with a `failingTokenSource` that fails the test if `Token()`
  is invoked. Scopes are per-action (`gcal:list_today` vs
  `gcal:create_event`) so the audit log attributes consent at the grain
  the operator actually sees.
- **Token theft**: file lives under `$HOME/.leah-state/secrets/` with
  0600 mode enforced by the attestation gate (not by this adapter).
  Adapter never logs the token, never echoes `TokenPath` into errors,
  never marshals `Config` to disk.
- **Confused-deputy via CalendarID**: `CalendarID` defaults to `primary`
  and is set once at construction. The adapter does not accept a
  per-call calendar override — a future multi-calendar feature must add
  an explicit method, not a string knob.
- **Rate-limit lockout**: `ErrAuthRequired` short-circuits before any
  network call when `svc` is nil, so a missing token cannot retry-loop
  Google's auth endpoint.
- **400-validation loops**: `ErrInvalidEvent` is a sentinel; callers MUST
  surface the message instead of retrying. Enforced by code review for
  the wiring wave.
- **No background goroutines**: constructor is synchronous; the only
  ambient state is `time.Now`, injectable for tests.

## 6. Out of scope (MVP)

- Recurrence, attendees, reminders, free/busy queries.
- Multi-calendar fan-out.
- Webhook / push-notification subscription for live updates.
- Conflict detection on `CreateEvent` (overlap with existing events).

Each of the above reopens when a real caller files an issue citing the
gap.

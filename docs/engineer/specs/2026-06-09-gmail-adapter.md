# Gmail adapter — MVP scaffold

Status: scaffold (Wave 9, subagent D1)
Owner: trilamsr
Surface: `internal/adapters/gmail/`

## Purpose

Give Leah a narrow, auditable Gmail interface so the morning-brief task can
enrich its summary with unread mail, and so the dispatcher can send operator-
authored replies. MVP intentionally exposes only three RPCs (`ListUnread`,
`MarkRead`, `Send`) — richer MIME, threading, label management, and search
defer to follow-up waves once the operator-attestation flow is observed in
real use.

## Dependency (adopt-over-build)

Per `CLAUDE.md` `feedback_research_design_principles`, the adapter will adopt
Google's first-party Go SDK rather than hand-rolling an HTTP client.

- Module: `google.golang.org/api/gmail/v1`
- Pinned version: `v0.250.0` (latest as of 2026-06-09)
- License: BSD-3-Clause (`https://github.com/googleapis/google-api-go-client/blob/main/LICENSE`)
- Commit sha verification: `git ls-remote https://github.com/googleapis/google-api-go-client refs/tags/v0.250.0`
  — operator runs this when promoting the require line.
- Companion runtime: `golang.org/x/oauth2` (BSD-3-Clause) for token refresh.

This MVP **defers the `go.mod` edit** to the wiring wave. Rationale: the brief
forbids `go.mod` edits while sibling subagents may be running (`ps aux | grep
gmail` was clean at scaffold time, but parallel D2 (`gcal`) may also touch
`go.mod`). The adapter is structured so the SDK lands behind the `Transport`
interface — adding the import is additive and does not change the public
surface.

Require line to add in the wiring wave:

```
require (
    google.golang.org/api v0.250.0
    golang.org/x/oauth2 v0.27.0
)
```

## Surface

```go
type Message      struct { To, Subject, Body string }
type Attestor     interface { Attest(ctx, scope string) error }
type TokenSource  interface { Token(ctx) (string, error) }
type Transport    interface { ListUnread / MarkRead / Send }
type Config       struct { Attestor; TokenSource; Transport }
type Client       struct { /* unexported */ }

func New(Config) (*Client, error)
func (*Client) ListUnread(ctx) ([]string, error)
func (*Client) MarkRead(ctx, id string) error
func (*Client) Send(ctx, Message) error
```

Sentinel errors: `ErrAttestationDenied`, `ErrMessageNotFound`, `ErrSendRejected`.

## Operator-attestation flow

Mirrors `internal/dispatcher/selfbuild.go`'s gating pattern: a secret-touching
RPC is fenced behind a question the operator must affirm in-band. For the
adapter MVP, attestation is modeled as an interface so the dispatcher (which
owns the prompt UI + audit row) can plug in the concrete gate without the
adapter depending on dispatcher internals.

Per-RPC scopes (`ScopeList`, `ScopeMark`, `ScopeSend`) flow to the Attestor so
the audit row records consent at the action grain. Ordering inside
`gateAndToken` is load-bearing: **attestation runs before token load** — a
denied action MUST NOT cause the bearer token to be materialized (and risk
landing in a logger / panic trace).

## Token storage

OAuth token lives at `$HOME/.leah-state/secrets/gmail-token.json` (path-only;
the adapter does NOT create or rotate it — that is the responsibility of a
`leah gmail login` CLI subcommand in the wiring wave). File mode MUST be `0600`
matching the audit log convention (`internal/audit/audit.go` §45).

`TokenSource` is an interface so:

1. Tests inject a fake without touching `$HOME`.
2. The production `fileTokenSource` (added in the wiring wave) can layer
   refresh logic on top of the bare disk read.

## Future wiring sketch (NOT in this MVP)

1. `cmd/leah/gmail.go` — `leah gmail login` subcommand that runs the OAuth
   device-code flow + writes the token file `0600`.
2. `cmd/leah-daemon/brief.go` — morning-brief task instantiates `gmail.New`
   with the dispatcher-backed Attestor and threads unread-count into the
   summary.
3. `internal/dispatcher/gmail_send.go` — `leah ship --gmail` path for
   operator-authored replies, gated by the same attestation question pool
   `selfbuild.go` uses.

## Threat model (token leak surface)

| Surface | Mitigation |
| --- | --- |
| Token in panic traces | `gateAndToken` returns the token only when attestation passes; never logs the token; sentinel `ErrAttestationDenied` wraps the underlying reason without leaking secret material. |
| Token-on-disk | `0600` mode required at write time (out-of-scope for this MVP — enforced by the wiring-wave CLI). |
| Scope creep | Adapter MVP exposes only 3 RPCs. Adding a new RPC requires a new `Scope*` constant + a brief PR; reviewers can audit at the constant grain. |
| Attestor-bypass | `New` refuses to construct a `Client` without an `Attestor`; there is no nil-Attestor fallback. |
| Send-to-attacker | `Send` validates `To != ""` BEFORE attestation, so a typo-trap message that would be rejected anyway does not consume an attestation prompt; recipient allow-listing defers to the wiring wave. |

## Test plan (this MVP)

- `TestListUnread` — happy + auth-denied (attestor rejects).
- `TestMarkRead` — happy + nonexistent message id (`""`).
- `TestSend` — happy + send-rejected (missing recipient).
- `TestNewRejectsMissingDeps` — fail-closed wiring contract.

All tests are hermetic: no disk, no network, no real OAuth.

## Out of scope (MVP)

- CLI wiring (`leah gmail …`)
- Daemon morning-brief enrichment
- `go.mod` edit (deferred — see "Dependency" above)
- MIME beyond plain text To/Subject/Body
- Threading, labels beyond UNREAD, search, attachments
- Recipient allow-listing
- Token refresh / rotation

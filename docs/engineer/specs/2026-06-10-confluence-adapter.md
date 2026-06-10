---
slug: confluence-adapter
status: draft
phase: self-host
owner: leah
---

# Confluence adapter (MVP)

## 1. Goal

Give Leah a narrow, auditable Confluence interface so the morning-brief
task can enrich its summary with pages updated overnight, and so the
operator can pull a page's body for downstream synthesis. MVP exposes
only three RPCs (`ListRecentPages`, `GetPage`, `SearchCQL`) — page
creation, edits, attachments, comments, and space-admin surfaces defer
to follow-up waves once the operator-attestation flow is observed in
real use. Shape mirrors `internal/adapters/gmail` (Attestor + per-RPC
scope + `gateAndToken` ordering) so wiring code reuses one consent
path across the work-tools pack.

## 2. Dependency (adopt-over-build)

Per `CLAUDE.md` adopt-over-build, the adapter will adopt the
community-maintained `go-atlassian` SDK rather than hand-roll an HTTP
client. The same module covers Jira (sibling adapter); choosing it here
amortizes the `go.mod` cost across two adapters.

- Module: `github.com/ctreminiom/go-atlassian/v2`
- Pinned tag: `v2.6.1` (latest active release as of 2026-06-10)
- License: MIT (`https://github.com/ctreminiom/go-atlassian/blob/v2.6.1/LICENSE`)
- Commit sha verification: `git ls-remote https://github.com/ctreminiom/go-atlassian refs/tags/v2.6.1`
  — operator runs this when promoting the require line.
- Companion runtime: `golang.org/x/oauth2` (BSD-3-Clause) for OAuth 3LO
  token refresh; bypassed when API-token mode is selected.

MVP **defers the `go.mod` edit** to the W50 combined tidy (see plan).
Rationale mirrors gmail/gcal: parallel sibling adapters (Jira) also
touch this module, so one combined tidy avoids a rebase storm. The
adapter is structured so the SDK lands behind the `Transport`
interface — adding the import is additive and does not change the
public surface.

Require line for the W50 tidy:

```
require (
    github.com/ctreminiom/go-atlassian/v2 v2.6.1
    golang.org/x/oauth2 v0.27.0
)
```

Fallback if the SDK ever stalls: direct REST against
`https://your-domain.atlassian.net/wiki/rest/api`. The `Transport`
interface makes the swap mechanical.

## 3. Surface

```go
package confluence

const (
    ScopeListRecent = "confluence:list_recent"
    ScopeGetPage    = "confluence:get_page"
    ScopeSearch     = "confluence:search"
)

var (
    ErrAttestationDenied = errors.New("confluence: attestation denied")
    ErrAuthFailed        = errors.New("confluence: auth failed")
    ErrNotFound          = errors.New("confluence: not found")
    ErrRateLimited       = errors.New("confluence: rate limited")
)

type Page struct {
    ID       string
    Title    string
    SpaceKey string
    Body     string // storage-format; empty for List* responses
    URL      string
    Updated  time.Time
}

type Attestor interface {
    Attest(ctx context.Context, scope string) error
}

type TokenSource interface {
    Token(ctx context.Context) (string, error)
}

type Transport interface {
    ListRecent(ctx context.Context, bearer, space string) ([]Page, error)
    GetPage(ctx context.Context, bearer, id string) (Page, error)
    SearchCQL(ctx context.Context, bearer, query string) ([]Page, error)
}

type Config struct {
    Attestor    Attestor
    TokenSource TokenSource
    Transport   Transport
    BaseURL     string // e.g. https://acme.atlassian.net/wiki
}

type Client struct { /* unexported */ }

func New(cfg Config) (*Client, error)
func (*Client) ListRecentPages(ctx context.Context, space string) ([]Page, error)
func (*Client) GetPage(ctx context.Context, id string) (Page, error)
func (*Client) SearchCQL(ctx context.Context, query string) ([]Page, error)
```

`New` fail-closes on nil `Attestor`, nil `TokenSource`, nil `Transport`,
or empty `BaseURL`. There is no silent default.

## 4. Operator-attestation flow

Mirrors `internal/adapters/gmail` `gateAndToken`. Per-call gate: every
public method runs `Attestor.Attest(ctx, scope)` BEFORE loading the
bearer from `TokenSource`. Ordering is load-bearing — a denied action
must not materialize the token, otherwise the secret would leak into a
logger / panic trace even when the operator said no.

Scopes are per-action so the audit log distinguishes read (`confluence:
list_recent`, `confluence:get_page`) from search (`confluence:search`).
Each RPC writes an audit row `Kind: "confluence_<op>"` with `success
bool` so denied / failed / succeeded calls are all observable post-hoc.

Flow:

1. Operator runs `leah connect confluence` (W51 wiring wave).
2. CLI prompts an attestation question from the
   `internal/attestation/pool` (PR #67), records the answer in
   `audit.jsonl` with `Kind: "connect_confluence"`, then writes the
   credential to `TokenPath`.
3. Daemon boot calls `confluence.New(Config{...})`; missing deps fail
   construction.
4. Each RPC re-enters the attestation gate via `gateAndToken` — silent
   bypass would skip the audit row and the operator-consent UX.
5. Token refresh (OAuth 3LO mode) re-enters the attestation gate —
   silent refresh would bypass the audit row.

## 5. Token storage

Credential lives at `$HOME/.leah-state/secrets/confluence-token.json`
(path-only; the adapter does NOT create or rotate it — that is the W51
`leah connect confluence` responsibility). File mode MUST be `0600`
matching the audit log convention (`internal/audit/audit.go` §45).

Two credential modes; MVP picks API-token for simplicity:

- **API token** (MVP default): operator generates at
  `https://id.atlassian.com/manage-profile/security/api-tokens`, paired
  with their Atlassian account email. Token file shape:
  `{"email": "...", "api_token": "...", "workspace_id": "..."}`.
- **OAuth 3LO**: deferred to a follow-up wave once the API-token flow
  is observed in real use. Atlassian's 3LO consent UX is heavier than
  the morning-brief use case warrants.

`TokenSource` is an interface so tests inject a fake without touching
`$HOME` and the production `fileTokenSource` (added in W51) can layer
refresh logic on top of the bare disk read.

## 6. Future wiring (NOT in this MVP)

- `cmd/leah/confluence.go` — `leah connect confluence` subcommand;
  registers in `internal/connect/registry.go`. Walks the operator
  through API-token paste, writes `0600` token file.
- `cmd/leah-daemon/brief.go` — morning-brief task instantiates
  `confluence.New` with the dispatcher-backed Attestor and stitches
  "pages updated overnight in spaces you watch" into the summary.
- `internal/dispatcher/confluence_post.go` — DEFERRED. No write-path
  in MVP; page creation lands once the read flow is stable.
- Recommendation engine (W15+ from learn-recommend-apply, PR #69):
  `Recommendation.Source = "confluence"` ties Confluence-derived
  suggestions back to the consent grain.

## 7. Threat model

| Surface | Mitigation |
| --- | --- |
| Token in panic traces | `gateAndToken` returns the bearer only after attestation passes; never logs the token; sentinel `ErrAttestationDenied` wraps the underlying reason without leaking secret material. |
| Token-on-disk | `0600` mode required at write time (out-of-scope for this MVP — enforced by W51 CLI). |
| Scope creep | Adapter MVP exposes only 3 RPCs. Adding a new RPC requires a new `Scope*` constant + a brief PR; reviewers audit at the constant grain. |
| Attestor-bypass | `New` refuses to construct a `Client` without an `Attestor`; no nil-Attestor fallback. |
| Cross-tenant leakage | `workspace_id` is recorded in the token file at write time; audit rows include the workspace ID; future multi-workspace support adds an explicit method, not a string knob. |
| Page-body leak in audit | Audit row stores `page_id` and `space_key`, NEVER `body` content. `Detail` field excludes page text. |
| CQL injection | The SDK escapes CQL parameters; raw-string concatenation into CQL queries is banned. Reviewer flags any `fmt.Sprintf` into a CQL string. |
| Rate-limit lockout | Atlassian's 5000 req/hr/user limit handled by surfacing `ErrRateLimited` without retry; caller decides cadence. |
| Webhooks (push from Confluence to Leah) | Out of scope MVP. Read-only pull only. |

## 8. Test plan (all hermetic; no real Atlassian, no real network)

- `TestListRecent_HappyPath` — fake `Transport` returns canned pages;
  assert title + space_key + URL come through unchanged.
- `TestListRecent_AttestationDenied_NoTokenLoad` — `failingTokenSource`
  pattern from gmail; `Attestor` denies; `TokenSource.Token` MUST NOT
  be called.
- `TestGetPage_NotFound` — transport returns `ErrNotFound`; surfaces
  the sentinel.
- `TestSearchCQL_HappyPath` — query "type=page AND space=ENG" returns
  expected page set; no body content in audit row.
- `TestSearchCQL_AuditRowExcludesBody` — audit row Detail field has
  `page_id` and `space_key`, never page body text.
- `TestNewRejectsMissingDeps` — nil `Attestor`, nil `TokenSource`, nil
  `Transport`, empty `BaseURL` each fail construction with descriptive
  errors.
- `TestAllRPCs_RateLimited_PassThrough` — transport returns 429;
  surfaces `ErrRateLimited`.

## 9. Trade-offs (per CLAUDE.md UX > performance > long-term)

- **UX**: API-token mode is one-time paste vs OAuth 3LO's redirect
  dance. Operator pays the worse setup once vs. the worse runtime
  forever — UX wins via API-token.
- **Performance**: `ListRecent` paginates server-side; MVP caps at the
  first page (25 items). A caller asking for more reopens the gap.
- **Long-term**: deferring page creation keeps the surface auditable.
  A future write adapter is intentionally NOT scaffolded — three
  similar lines beat a premature abstraction.

## 10. Out of scope (MVP)

- Page create / update / delete.
- Attachments (upload, download).
- Comments (read or write).
- Space admin (membership, permissions, settings).
- Webhooks / push subscriptions.
- Multi-workspace fan-out.
- OAuth 3LO credential mode (API-token only MVP).
- Threading across page-comment trees.
- Full-body audit capture (operator-toggle deferred).

---
slug: msteams-adapter
status: draft
phase: self-host
owner: leah
---

# Microsoft Teams adapter (MVP)

## 1. Goal

Give Leah a narrow, auditable Teams interface so the morning-brief
task can surface mentions in channels the operator watches and so the
dispatcher can post operator-authored messages to a channel. MVP
exposes four RPCs (`ListTeams`, `ListChannels`, `PostMessage`,
`Search`) — replies, reactions, file uploads, meetings, calls, and
the change-notification push surface defer to follow-up waves once
the operator-attestation flow is observed in real use. Shape mirrors
`internal/adapters/gmail` (Attestor + per-RPC scope + `gateAndToken`
ordering) so wiring code reuses one consent path across the work-tools
pack.

## 2. Dependency (adopt-over-build)

Adopts Microsoft's official Microsoft Graph SDK for Go. Heavy SDK,
but it's the only first-party option and Teams is exposed only through
Microsoft Graph (no Teams-specific REST surface).

- Module: `github.com/microsoftgraph/msgraph-sdk-go`
- Pinned tag: `v1.61.0` (latest active release as of 2026-06-10)
- License: MIT (`https://github.com/microsoftgraph/msgraph-sdk-go/blob/v1.61.0/LICENSE`)
- Commit sha verification: `git ls-remote https://github.com/microsoftgraph/msgraph-sdk-go refs/tags/v1.61.0`
- Companion runtime: `golang.org/x/oauth2` (BSD-3-Clause) for token
  refresh; Microsoft Graph requires OAuth 2 — no API-key shortcut.

MVP defers the `go.mod` edit to the W50 combined tidy. The SDK has a
large transitive dep tree (~12 modules including Kiota generated
runtime); W50 absorbs the full cost.

Honest trade-off: the SDK is heavy, but `Transport` interface keeps
the door open for a hand-rolled minimal REST client against
`https://graph.microsoft.com/v1.0` if the dep tree ever becomes
unbearable. Three RPCs against Graph is small enough to hand-roll;
deferred until pain is measured.

## 3. Surface

```go
package msteams

const (
    ScopeListTeams    = "msteams:list_teams"
    ScopeListChannels = "msteams:list_channels"
    ScopePost         = "msteams:post_message"
    ScopeSearch       = "msteams:search"
)

var (
    ErrAttestationDenied = errors.New("msteams: attestation denied")
    ErrAuthFailed        = errors.New("msteams: auth failed")
    ErrNotFound          = errors.New("msteams: not found")
    ErrRateLimited       = errors.New("msteams: rate limited")
    ErrInvalidChannel    = errors.New("msteams: invalid channel")
)

type Team struct {
    ID          string
    DisplayName string
}

type Channel struct {
    ID          string
    TeamID      string
    DisplayName string
}

type Result struct {
    TeamID    string
    ChannelID string
    MessageID string
    Snippet   string
    URL       string
}

type Attestor interface {
    Attest(ctx context.Context, scope string) error
}

type TokenSource interface {
    Token(ctx context.Context) (string, error)
}

type Transport interface {
    ListTeams(ctx context.Context, bearer string) ([]Team, error)
    ListChannels(ctx context.Context, bearer, teamID string) ([]Channel, error)
    PostMessage(ctx context.Context, bearer, teamID, channelID, text string) error
    Search(ctx context.Context, bearer, query string) ([]Result, error)
}

type Config struct {
    Attestor    Attestor
    TokenSource TokenSource
    Transport   Transport
    Now         func() time.Time
}

type Client struct { /* unexported */ }

func New(cfg Config) (*Client, error)
func (*Client) ListTeams(ctx context.Context) ([]Team, error)
func (*Client) ListChannels(ctx context.Context, teamID string) ([]Channel, error)
func (*Client) PostMessage(ctx context.Context, teamID, channelID, text string) error
func (*Client) Search(ctx context.Context, query string) ([]Result, error)
```

`New` fail-closes on nil deps. `Now` defaults to `time.Now` when nil.

## 4. Operator-attestation flow

Mirror of gmail's `gateAndToken`. Per-call gate ordering is
load-bearing:

1. `ListChannels` validates `teamID != ""`. `PostMessage` validates
   `teamID != ""`, `channelID != ""`, `text != ""`. `Search` validates
   `query != ""`. A typo MUST NOT consume an attestation prompt.
2. Rate-limit check (Section 7) — burst-rejected calls MUST NOT
   consume an attestation prompt either.
3. `Attestor.Attest(ctx, scope)` runs. A non-nil return aborts with
   `ErrAttestationDenied` (wrapped). **No bearer token is loaded.**
4. Only on consent does `TokenSource.Token` execute and the transport
   issue the Graph call.
5. Each RPC writes an audit row `Kind: "msteams_<op>"` with `success
   bool`, `team_id`, `channel_hash = sha256(channelID)[:8]`, `text_len`
   (rune count, NEVER text bytes).

Attestation pool reuses `internal/attestation/pool.go` (PR #67).

## 5. Token storage

Credential lives at `$HOME/.leah-state/secrets/msteams-token.json`
(path-only). File mode MUST be `0600`.

Microsoft Graph requires OAuth 2 via Microsoft Identity Platform
(Azure AD). No API-key shortcut. Operator path (W51 wiring wave):

1. Operator registers an application in their Azure AD tenant at
   `https://portal.azure.com` -> App registrations.
2. Grants delegated permissions:
   - `Team.ReadBasic.All` — for `ListTeams`.
   - `Channel.ReadBasic.All` — for `ListChannels`.
   - `ChannelMessage.Send` — for `PostMessage`.
   - `ChannelMessage.Read.All` — for `Search` (search hits message
     content).
3. Runs `leah connect msteams`; CLI opens the device-code OAuth flow;
   on success writes the token + refresh token to the file.

Token file shape:

```
{
  "tenant_id": "...",
  "client_id": "...",
  "access_token": "...",
  "refresh_token": "...",
  "expires_at": "2026-06-10T..."
}
```

Token refresh is non-optional (Microsoft tokens are short-lived).
W51 owns the refresh logic; the adapter `TokenSource` interface
abstracts the refresh.

## 6. Future wiring (NOT in this MVP)

- `cmd/leah/msteams.go` — `leah connect msteams` subcommand; walks
  operator through Azure AD app registration, device-code OAuth.
- `cmd/leah-daemon/brief.go` — morning-brief task: "channel mentions
  since yesterday in teams you watch".
- `internal/dispatcher/msteams_post.go` — `leah ship --msteams
  <team>/<channel>` for operator-authored channel posts.
- Recommendation engine: `Recommendation.Source = "msteams"`.

## 7. Threat model

| Surface | Mitigation |
| --- | --- |
| Token in panic traces | `gateAndToken` returns the bearer only after attestation passes; never logs the token. |
| Token-on-disk | `0600` mode required at write time. Both access AND refresh tokens are sensitive; both stay in the same `0600` file. |
| Scope creep | 4 RPCs MVP. New RPC = new `Scope*` constant + a brief PR. New Graph permission = doubly visible (constant grain AND Azure AD app grain). |
| Attestor-bypass | `New` refuses construction without an `Attestor`. |
| Cross-tenant leakage | `tenant_id` recorded in token file and audit rows; reject if a runtime call's resolved tenant doesn't match the token-file claim. |
| Channel-name leak in audit | Audit row stores `channel_hash = sha256(channelID)[:8]`. `team_id` is recorded plaintext (tenant-scoped GUID, not human-readable). |
| Message-body leak in audit | Audit row stores `text_len` (rune count), NEVER `text` bytes. Operator-toggle for full-body capture deferred. |
| Spam vector | Conservative MVP rate limit: max **4 PostMessage per second** (matches Graph's published Teams throttling envelope of ~30/sec; we run well under). Burst returns `ErrRateLimited`. |
| Change-notification push | Out of scope MVP. Pull only — no Graph subscription surface. |
| Refresh-token rotation failure | Refresh failures surface `ErrAuthFailed`; adapter does NOT silently retry, does NOT log the refresh token. |
| Search content exposure | `Search` returns message snippets — caller must treat them as sensitive. Audit row records `query_hash`, NEVER `query` or `snippet`. |

## 8. Test plan (all hermetic)

- `TestListTeams_HappyPath` — fake transport returns canned teams.
- `TestListTeams_AttestationDenied_NoTokenLoad` — Attestor denies;
  `TokenSource.Token` MUST NOT be called.
- `TestListChannels_TeamIDEmpty_NoAttest` — empty teamID; returns
  `ErrInvalidChannel` BEFORE attestation.
- `TestPostMessage_HappyPath` — valid args; transport invoked; audit
  row has `team_id`, `channel_hash`, `text_len`.
- `TestPostMessage_TextEmpty_NoAttest` — `text == ""`; returns
  `ErrInvalidChannel` BEFORE attestation.
- `TestPostMessage_AuditRowHashed` — `channel_hash` is the 8-char
  sha256 prefix; plaintext channel ID does NOT appear; `text_len`
  matches `utf8.RuneCountInString(text)`.
- `TestSearch_QueryEmpty_NoAttest` — empty query; returns
  `ErrInvalidChannel` BEFORE attestation.
- `TestSearch_AuditRowExcludesContent` — audit row has `query_hash`;
  omits `query` and `snippet`.
- `TestPostMessage_RateLimit_RejectsBurst` — 4 posts in 1s succeed;
  5th returns `ErrRateLimited`; advance `Now`, resume.
- `TestAuthFailed_RefreshDoesNotLogToken` — token refresh failure
  surfaces `ErrAuthFailed`; the test scans the captured error
  rendering and asserts neither access_token nor refresh_token
  appears.
- `TestNewRejectsMissingDeps` — nil deps each fail construction.

## 9. Trade-offs

- **UX**: Azure AD app registration is the worst onboarding step in
  the work-tools pack — heavier than Slack's. W51 mitigates with a
  step-by-step doc + portal link; operator pays once.
- **Performance**: the Microsoft Graph SDK's dep tree is heavy; build
  time grows. Accepted MVP — `Transport` keeps the swap door open.
- **Long-term**: not scaffolding meetings / calls / reactions keeps
  the consent surface narrow. Calls in particular are a separate
  adapter design (think `facetime` not `msteams`).

## 10. Out of scope (MVP)

- Replies in a thread (`PostMessage` is top-level only).
- Reactions.
- File uploads / attachments.
- Meetings (create, join, transcript).
- Calls (Teams audio/video).
- Direct chats (1:1 and group chats — only channel posts MVP).
- Change notifications / Graph subscriptions.
- Multi-tenant fan-out.
- Operator-configurable rate limit.
- Full-body audit capture.

> All internal paths in this doc reflect the pre-2026-07-09 layout; current tree per `git ls-tree -d --name-only HEAD:internal/`.

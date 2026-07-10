---
slug: slack-adapter
status: draft
phase: self-host
owner: leah
---

# Slack adapter (MVP)

## 1. Goal

Give Leah a narrow, auditable Slack interface so the morning-brief
task can surface top DMs and mentions, and so the dispatcher can post
operator-authored messages to a channel without leaving the terminal.
MVP exposes four RPCs (`ListChannels`, `PostMessage`, `GetThread`,
`Search`) — reactions, file uploads, scheduled messages, and the
Events API push surface defer to follow-up waves once the
operator-attestation flow is observed in real use. Shape mirrors
`internal/adapters/gmail` (Attestor + per-RPC scope + `gateAndToken`
ordering) so wiring code reuses one consent path across the work-tools
pack.

## 2. Dependency (adopt-over-build)

Adopts `slack-go/slack`, the most-used and actively maintained Slack
SDK for Go.

- Module: `github.com/slack-go/slack`
- Pinned tag: `v0.15.0` (latest active release as of 2026-06-10)
- License: BSD-2-Clause (`https://github.com/slack-go/slack/blob/v0.15.0/LICENSE`)
- Commit sha verification: `git ls-remote https://github.com/slack-go/slack refs/tags/v0.15.0`
- Companion runtime: `golang.org/x/oauth2` (BSD-3-Clause) for token
  refresh in the Slack-App flow.

MVP defers the `go.mod` edit to the W50 combined tidy.

Slack App vs User Token decision (load-bearing — picked here, not
deferred):

- **Slack App with Bot Token** (MVP default): operator installs a
  Slack App into their workspace; the bot posts as the App. UX cost:
  one App-creation step at `https://api.slack.com/apps`. UX win:
  granular scopes, workspace-admin auditability, never tied to the
  operator's user account.
- **User Token**: rejected for MVP. A compromised Leah with a User
  Token can act as the operator across the entire workspace; the
  blast radius is unacceptable.

Required scopes (recorded in code as constants):
- `chat:write` — for `PostMessage`.
- `channels:read` + `groups:read` + `im:read` — for `ListChannels`.
- `channels:history` + `groups:history` + `im:history` — for
  `GetThread`.
- `search:read` — for `Search` (requires a User Token; Bot Tokens
  cannot search. Documented break: `Search` is the ONLY method that
  requires a paired User Token. If the operator declines the User
  Token, `Search` returns `ErrAuthFailed` — gracefully degraded.).

## 3. Surface

```go
package slack

const (
    ScopeListChannels = "slack:list_channels"
    ScopePost         = "slack:post_message"
    ScopeGetThread    = "slack:get_thread"
    ScopeSearch       = "slack:search"
)

var (
    ErrAttestationDenied = errors.New("slack: attestation denied")
    ErrAuthFailed        = errors.New("slack: auth failed")
    ErrNotFound          = errors.New("slack: not found")
    ErrRateLimited       = errors.New("slack: rate limited")
    ErrInvalidChannel    = errors.New("slack: invalid channel")
)

type Channel struct {
    ID      string
    Name    string // without leading #
    IsIM    bool
    IsGroup bool
}

type Thread struct {
    Channel string
    TS      string // root timestamp
    Replies []Message
}

type Message struct {
    User string // user ID (not username)
    TS   string
    Text string
}

type Result struct {
    Channel string
    TS      string
    Snippet string
    URL     string
}

type Attestor interface {
    Attest(ctx context.Context, scope string) error
}

type TokenSource interface {
    BotToken(ctx context.Context) (string, error)
    UserToken(ctx context.Context) (string, error) // empty string if not configured
}

type Transport interface {
    ListChannels(ctx context.Context, bot string) ([]Channel, error)
    PostMessage(ctx context.Context, bot, channel, text string) error
    GetThread(ctx context.Context, bot, channel, ts string) (Thread, error)
    Search(ctx context.Context, user, query string) ([]Result, error)
}

type Config struct {
    Attestor    Attestor
    TokenSource TokenSource
    Transport   Transport
    Now         func() time.Time
}

type Client struct { /* unexported */ }

func New(cfg Config) (*Client, error)
func (*Client) ListChannels(ctx context.Context) ([]Channel, error)
func (*Client) PostMessage(ctx context.Context, channel, text string) error
func (*Client) GetThread(ctx context.Context, channel, ts string) (Thread, error)
func (*Client) Search(ctx context.Context, query string) ([]Result, error)
```

`New` fail-closes on nil deps. `Now` defaults to `time.Now` when nil.

## 4. Operator-attestation flow

Mirror of gmail's `gateAndToken`. Per-call gate ordering is
load-bearing:

1. `PostMessage` validates `channel != ""` and `text != ""`. `GetThread`
   validates both args non-empty. `Search` validates `query != ""`. A
   typo MUST NOT consume an attestation prompt.
2. Rate-limit check (Section 7) — burst-rejected calls MUST NOT
   consume an attestation prompt either.
3. `Attestor.Attest(ctx, scope)` runs. A non-nil return aborts with
   `ErrAttestationDenied` (wrapped). **No bearer token is loaded.**
4. Only on consent does the relevant `TokenSource` method execute and
   the transport issue the call.
5. Each RPC writes an audit row `Kind: "slack_<op>"` with `success
   bool`, `channel_hash` (sha256(channel)[:8]; channel IDs ARE
   identifiers and we hash them to avoid building a workspace topology
   map in plaintext audit), `text_len` (rune count, not text).

Attestation pool reuses `internal/attestation/pool.go` (PR #67).

## 5. Token storage

Credential lives at `$HOME/.leah-state/secrets/slack-token.json`
(path-only; adapter does NOT create or rotate it — W51 CLI owns that).
File mode MUST be `0600`.

Token file shape:

```
{
  "team_id": "T01234567",
  "team_name": "acme",
  "bot_token": "xoxb-...",
  "user_token": "xoxp-...",   // optional; empty if operator declined
  "bot_scopes": ["chat:write", "channels:read", ...]
}
```

`TokenSource` exposes both `BotToken` and `UserToken` so the adapter
can route per-RPC: `Search` -> `UserToken`, everything else ->
`BotToken`. If `UserToken` returns empty, `Search` short-circuits to
`ErrAuthFailed` without spawning a network call.

Token refresh deferred to W51 (mirror gmail spec).

## 6. Future wiring (NOT in this MVP)

- `cmd/leah/slack.go` — `leah connect slack` subcommand; walks operator
  through Slack App creation, OAuth install URL, paste callback.
- `cmd/leah-daemon/brief.go` — morning-brief task: "top mentions in
  channels you watch, top unread DMs".
- `internal/dispatcher/slack_post.go` — `leah ship --slack #eng` for
  operator-authored channel posts.
- Recommendation engine: `Recommendation.Source = "slack"`.

## 7. Threat model

| Surface | Mitigation |
| --- | --- |
| Token in panic traces | `gateAndToken` returns the bearer only after attestation passes; never logs the token. |
| Token-on-disk | `0600` mode required at write time. |
| Scope creep | 4 RPCs MVP. Adding a new RPC requires a new `Scope*` constant + a brief PR. Slack OAuth scope additions are doubly visible — flagged at adapter constant grain AND at App-config grain. |
| Attestor-bypass | `New` refuses construction without an `Attestor`. |
| Cross-tenant leakage | `team_id` recorded in token file; audit rows include `team_id`; reject if a runtime call's resolved team doesn't match the token-file claim. |
| Channel-name leak in audit | Audit row stores `channel_hash = sha256(channel)[:8]`, NOT plaintext channel ID/name. Workspace topology stays opaque to log readers. |
| Message-body leak in audit | Audit row stores `text_len` (rune count), NEVER `text` bytes. Operator-toggle for full-body capture deferred. |
| Spam vector | Conservative MVP rate limit: max **1 PostMessage per channel per second** (Slack's own published guidance). Burst returns `ErrRateLimited`. Operator-configurable limit deferred. |
| User-Token abuse via Search | `Search` is the only RPC that loads the User Token. If the User Token is absent, `Search` returns `ErrAuthFailed` early — no fallback to Bot Token. Documented and tested. |
| Events API push | Out of scope MVP. Pull only — no incoming webhook surface. |

## 8. Test plan (all hermetic)

- `TestListChannels_HappyPath` — fake transport returns canned
  channels; assert IDs + names + types come through unchanged.
- `TestPostMessage_AttestationDenied_NoTokenLoad` — `failingTokenSource`
  pattern from gmail; Attestor denies; `TokenSource.BotToken` MUST NOT
  be called.
- `TestPostMessage_ChannelEmpty_NoAttest` — `channel == ""`; returns
  `ErrInvalidChannel` BEFORE attestation runs.
- `TestPostMessage_TextEmpty_NoAttest` — `text == ""`; returns
  `ErrInvalidChannel` (validation sentinel) BEFORE attestation.
- `TestGetThread_NotFound` — transport returns `ErrNotFound`; sentinel
  surfaces.
- `TestSearch_NoUserToken_ErrAuthFailed` — `TokenSource.UserToken`
  returns empty; `Search` returns `ErrAuthFailed`; transport NOT
  invoked.
- `TestSearch_HappyPath` — User Token present; transport returns
  results; audit row `Kind: "slack_search"` with `query_hash`, NOT raw
  query.
- `TestPostMessage_RateLimit_RejectsBurst` — same channel: 1st send
  succeeds; 2nd send within 1s returns `ErrRateLimited`; advances
  `Now` past window, resume.
- `TestPostMessage_AuditRowHashed` — `channel_hash` is the 8-char
  sha256 prefix; plaintext channel does NOT appear; `text_len` matches
  `utf8.RuneCountInString(text)`.
- `TestNewRejectsMissingDeps` — nil deps each fail construction.

## 9. Trade-offs

- **UX**: Slack App creation is the worst onboarding step in the
  work-tools pack. W51 mitigates with copy-paste install URL + a
  one-screen wizard. Operator pays once.
- **Performance**: 1-per-sec rate limit caps the dispatcher's burst
  throughput but matches Slack's published quota — avoids a 429 retry
  storm.
- **Long-term**: not scaffolding reactions / file uploads / scheduled
  messages keeps the consent surface narrow. Each future RPC requires
  a new scope constant.

## 10. Out of scope (MVP)

- Threading replies (`PostMessage` is top-level only).
- Reactions (`reactions.add`, tapbacks).
- File uploads / attachments.
- Scheduled messages (`chat.scheduleMessage`).
- Events API / Socket Mode (incoming push).
- Slash commands.
- User profile updates (`users.profile.set`).
- Multi-workspace fan-out.
- DM creation (only existing IM channels listable).
- Operator-configurable rate limit (deferred).
- Full-body audit capture.

> All internal paths in this doc reflect the pre-2026-07-09 layout; current tree per `git ls-tree -d --name-only HEAD:internal/`.

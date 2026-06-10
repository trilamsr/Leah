---
slug: discord-adapter
status: draft
phase: self-host
owner: leah
---

# Discord adapter (MVP)

## 1. Goal

Operator-attested bidirectional Discord messaging. Inbound: subscribe to
specific channels and/or DM threads; outbound: post text messages and voice
clips. This is the live-conversation surface that pairs with WhatsApp's
SMS-like reach — operator chats with Leah from a phone or another desktop
when their primary Mac is asleep, and Leah pushes daily briefs +
accept/reject prompts into a designated channel.

Shape mirrors `internal/adapters/gmail` (per-RPC `Scope*` + `Attestor` +
load-bearing `gateAndExec` ordering) so the dispatcher reuses one consent
path across all comms adapters.

## 2. Dependency (adopt-over-build)

- Module: `github.com/bwmarrin/discordgo`
- Pinned version: `v0.28.1` (latest stable as of 2026-06-10)
- License: BSD-3-Clause
  (`https://github.com/bwmarrin/discordgo/blob/master/LICENSE`)
- Commit sha verification: `git ls-remote https://github.com/bwmarrin/discordgo refs/tags/v0.28.1`
  — operator runs this when promoting the require line.
- Rationale: most-used Go Discord library, MIT-compatible, active
  maintenance, well-documented gateway-session model. The alternative
  (`disgord`) is less active; rolling our own WebSocket client against
  Discord's Gateway v10 would burn weeks for no UX win.

This MVP **defers the `go.mod` edit** to the wiring wave. The adapter is
structured so the SDK lands behind a `Session` interface — adding the
import is additive and does not change the public surface.

Require line to add in the wiring wave:

```
require (
    github.com/bwmarrin/discordgo v0.28.1
)
```

## 3. Surface

```go
package discord

const (
    ScopePost         = "discord:post"
    ScopePostVoice    = "discord:post_voice"
    ScopeSubscribe    = "discord:subscribe"
    ScopeListChannels = "discord:list_channels"
)

var (
    ErrAttestationDenied = errors.New("discord: attestation denied")
    ErrGuildNotAllowed   = errors.New("discord: guild not on allowlist")
    ErrRateLimited       = errors.New("discord: rate limit exceeded")
    ErrAttachmentTooLarge = errors.New("discord: attachment exceeds 8MB limit")
    ErrPostFailed        = errors.New("discord: post failed")
)

type Message struct {
    ChannelID string
    AuthorID  string
    Body      string
    Voice     []byte    // populated when the inbound message is a voice attachment
    Timestamp time.Time
}

type Channel struct {
    ID   string
    Name string
    Type string // "text" | "dm" | "thread"
}

type Attestor interface {
    Attest(ctx context.Context, scope string) error
}

// Session is the discordgo seam. Production wires *discordgo.Session;
// tests inject a fake that records invocations and returns canned errors.
type Session interface {
    ChannelMessageSend(channelID, content string) error
    ChannelFileSend(channelID, name string, body io.Reader) error
    GuildChannels(guildID string) ([]Channel, error)
    Open() error
    Close() error
    AddHandler(handler any) func()
}

type TokenSource interface {
    Token(ctx context.Context) (string, error)
}

type Config struct {
    Attestor       Attestor
    TokenSource    TokenSource
    Session        Session
    GuildAllowlist []string  // empty = reject all inbound; explicit opt-in only
    Now            func() time.Time
}

type Adapter struct { /* unexported */ }

func New(cfg Config) (*Adapter, error)
func (*Adapter) PostMessage(ctx context.Context, channelID, body string) error
func (*Adapter) PostVoice(ctx context.Context, channelID string, audio []byte) error
func (*Adapter) Subscribe(ctx context.Context, channelIDs []string, handler func(Message)) error
func (*Adapter) ListChannels(ctx context.Context, guildID string) ([]Channel, error)
```

`New` fail-closes on nil `Attestor`, nil `TokenSource`, or nil `Session`.
`Now` defaults to `time.Now` when nil. `GuildAllowlist` empty is **not**
a permissive default — it means no inbound message will be dispatched.

## 4. Operator-attestation flow

Per-RPC. Ordering carries from gmail:

1. Argument validation (`channelID != ""`, `body != ""`, audio size ≤ 8MB
   per Discord's bot attachment cap). A bad argument MUST NOT consume an
   attestation prompt.
2. Rate-limit check (Section 7). Burst-rejected outbound MUST NOT consume
   attestation.
3. `Attestor.Attest(ctx, Scope*)` runs. Non-nil return aborts with
   `ErrAttestationDenied` (wrapped). Token is NOT loaded yet.
4. On consent, `TokenSource.Token(ctx)` materializes the bot token; the
   token never appears in errors, logs, or panic traces.
5. Each operation writes an audit row (Section 7 schema) with
   `success bool` so denied / failed / succeeded ops are observable.

Inbound `Subscribe` attests **once** at subscription time, not per
inbound message — the operator consents to the firehose at the channel
grain. Per-message dispatch into the reasoner pipeline is gated by the
guild allowlist + the operator's reasoner-level consent path, not a
re-prompt per chat line (which would be unusable).

## 5. Setup prereqs (operator steps, executed by `leah connect discord` in the wiring wave)

1. Operator opens https://discord.com/developers/applications and creates
   a new Application (e.g. "Leah").
2. Under **Bot**: add a bot user; copy the bot token.
3. Under **Bot -> Privileged Gateway Intents**: enable
   **Message Content Intent** (required to read message bodies, not just
   metadata). Document this as a load-bearing step — without it,
   inbound `Body` is empty.
4. Under **OAuth2 -> URL Generator**: select scopes
   `bot` + `applications.commands`; select permissions
   `Send Messages`, `Read Message History`, `Attach Files`,
   `Read Messages/View Channels`. Visit the generated URL and invite the
   bot to the operator's guild(s).
5. `leah connect discord` reads the bot token via stdin (never argv —
   argv leaks via `ps`) and writes
   `$HOME/.leah-state/secrets/discord-token.json` mode `0600`.
6. The connect command also captures the operator's guild allowlist
   (interactive prompt enumerating the guilds the bot was just invited
   to via `GuildChannels` discovery).

The adapter itself does NOT perform the OAuth invite flow — that is a
one-time browser interaction outside the daemon. The token file is the
only credential surface the adapter touches.

## 6. Voice handling

- **Inbound voice**: Discord voice messages arrive as audio attachments
  on a `Message` event. The adapter fetches the attachment via the
  Session's HTTP transport, populates `Message.Voice`, and hands the
  buffer to the STT pipeline scaffolded in `voice-comm` (W11+). The
  adapter does NOT call STT directly — that decoupling lets the
  reasoner choose its own transcription path.
- **Outbound voice**: TTS via existing `ChainTTS` (PR #49) produces an
  audio buffer. `PostVoice` uploads it as a file attachment named
  `leah-voice.ogg` via `ChannelFileSend`. Discord renders it as a
  playable audio attachment.
- **Voice channels (real-time join) are OUT OF SCOPE MVP.** Joining a
  voice channel requires Opus encoding state, jitter buffer management,
  and PCM streaming through `discordgo.VoiceConnection` — a separate
  design.

## 7. Threat model

| Surface | Mitigation |
| --- | --- |
| Bot token leak in logs / panic traces | `gateAndExec` loads the token only after attestation passes; never logs the token; sentinel errors wrap underlying reasons without leaking secret material. Token file `0600`. |
| Spam vector if Leah is compromised | Hardcoded MVP rate limit: max **10 outbound posts per rolling 60s per channel**, max **50 per rolling 60s globally** (well under Discord's 5 msg/5s/channel and 50/sec global bot limits). 11th send returns `ErrRateLimited`. Operator-configurable limit deferred. |
| Cross-guild leakage / bot scoped too broadly | `GuildAllowlist` MUST be non-empty for `Subscribe` to dispatch any message. Inbound from a non-allowlisted guild is dropped silently (no audit row leaking the guild ID; counter increments instead). |
| Voice-message bytes in audit log | Audit row stores `voice_sha256[:8]` + `voice_duration_ms`, never the bytes. |
| Author-impersonation tracking in audit | Audit row stores `author_hash = sha256(author_id)[:8]` per gmail/imessage convention. Operator can still detect repeat-spam patterns without plaintext IDs. |
| Bot becoming attack vector if Discord account compromised | Out of scope — operator's responsibility. Documented in `leah connect discord` output: "rotate this token if you suspect compromise; revoke via Developer Portal." |
| Message Content Intent not granted | Inbound `Body` is empty. Adapter detects via the first inbound message and emits a one-shot audit row `Kind: "discord_intent_missing"` so the operator sees the cause. |
| Attestor bypass | `New` refuses to construct an `Adapter` without non-nil `Attestor`, `TokenSource`, `Session`. No silent default. |
| Token-on-disk | `0600` mode required at write time (enforced by the wiring-wave CLI, mirroring gmail). |
| Discord API ToS / bot bans | Adapter uses only documented public Gateway + REST surfaces. No selfbot patterns, no user-account tokens. |

Audit row schema (consumed by `internal/audit`):

```
{
  "kind": "discord_post" | "discord_post_voice" | "discord_subscribe" | "discord_list_channels" | "discord_inbound",
  "ts": "2026-06-10T...",
  "success": true,
  "channel_hash": "a1b2c3d4",      // sha256(channel_id)[:8]
  "guild_hash": "e5f6...",          // sha256(guild_id)[:8], "" for DM
  "author_hash": "...",             // inbound only
  "body_len": 42,                   // rune count
  "voice_sha256": "...",            // first 8 hex chars, "" if no voice
  "voice_duration_ms": 0,
  "reason": ""                      // populated on failure
}
```

## 8. Test plan (all hermetic; no real Discord network, no real `discordgo`)

- `TestPostMessage_HappyPath` — fake `Session` captures call; assert
  `channelID` + `body` reach `ChannelMessageSend` after attestation
  + token load (in that order).
- `TestPostMessage_AttestationDenied_NoCall` — fake `Attestor` rejects;
  Session invocation counter MUST stay zero; returns wrapped
  `ErrAttestationDenied`.
- `TestPostMessage_EmptyChannelID_NoAttestation` — validation runs
  first; attestor counter MUST stay zero.
- `TestPostMessage_RateLimit_RejectsBurst` — with injected `Now`, 10
  posts in 30s succeed; 11th returns `ErrRateLimited`; neither
  Session nor Attestor invoked on the 11th.
- `TestPostVoice_AudioBoundedSize` — 9MB buffer returns
  `ErrAttachmentTooLarge` before attestation; 7MB buffer succeeds.
- `TestPostVoice_HappyPath` — fake Session captures the file send;
  assert the filename is `leah-voice.ogg` and the body matches the
  input bytes.
- `TestSubscribe_HandlerRouted` — fake Session emits a synthetic
  message event; assert the handler receives a `Message` with the
  expected fields and `Body` populated.
- `TestSubscribe_GuildAllowlistRejects` — synthetic event from a
  non-allowlisted guild; handler MUST NOT be invoked; reject-counter
  audit row is emitted with hashed guild ID only.
- `TestSubscribe_EmptyAllowlist_DropsAll` — `GuildAllowlist: nil`;
  every synthetic event is dropped; handler invocation counter zero.
- `TestListChannels_HappyPath` — fake Session returns three channels;
  adapter returns them after attestation.
- `TestAuditRowHashed` — inbound + outbound audit rows contain only
  the hashed identifiers; plaintext channel / author / guild IDs do
  NOT appear in any audit-row JSON.
- `TestNewRejectsMissingDeps` — nil `Attestor` / nil `TokenSource` /
  nil `Session` each fail construction with a descriptive error.

## 9. Trade-offs (per CLAUDE.md UX > performance > long-term)

- **UX**: long-lived gateway WebSocket gives sub-second inbound
  latency — strictly better than polling. Accepted.
- **Performance**: a single gateway session shared across all
  channels keeps Discord's connection budget happy. Per-channel
  sessions would multiply state for zero UX gain.
- **Long-term**: a future voice-channel adapter is intentionally NOT
  scaffolded here. Three similar lines beat a premature abstraction.

## 10. Out of scope (MVP)

- Voice channels (real-time join, PCM streaming).
- Threads (parent channels only MVP; thread routing is a follow-up).
- Reactions, embeds-with-buttons, slash commands (operator uses plain
  text replies; richer affordances deferred).
- Multi-bot per operator.
- Stage channels, forums, announcement channels.
- Slash command registration (separate consent surface).
- Operator-configurable rate limit (issue filed alongside this spec).
- Recipient / channel allow-listing for outbound (operator addresses
  channels by ID; channel-level outbound allowlist deferred).

---
slug: text-voice-comms-plan
status: draft
phase: self-host
owner: leah
---

# Text + voice comms delivery plan (Waves 65-72)

Combined delivery plan for Leah's bidirectional text-and-voice channels
with the operator. Specs:

- `docs/engineer/specs/2026-06-10-discord-adapter.md`
- `docs/engineer/specs/2026-06-10-whatsapp-adapter.md`

Both adapters carry the `Attestor` + per-RPC scope + load-bearing
`gateAndExec` ordering established by gmail / imessage. Both deliver
the same operator UX shape: text in/out + voice in/out, with the
operator's phone (WhatsApp) and a Discord client (desktop / mobile /
web) as the two reach surfaces when the operator's primary Mac is
asleep. Both unlock the same downstream wave: daily-brief push +
recommendation accept/reject via reply.

## Sequencing principle

One adapter per PR, scaffold-first (no daemon wiring), then connect-
flows, then voice paths (gated on `voice-comm` W11+), then the
cross-channel daily-brief + recommendation surfaces. Discord scaffold
lands before WhatsApp because Discord's bot model is lighter to test
(WebSocket gateway only, no inbound webhook plumbing) — surfacing the
shared `comms.Adapter` shape early without webhook complications.

## Wave 65 — Discord adapter scaffold

**Goal**: Land `internal/adapters/discord` with `PostMessage`,
`ListChannels`, and the connect-flow scaffolding. No `Subscribe`, no
daemon, no `go.mod` edit.

**Files touched**:

- `internal/adapters/discord/doc.go` (new)
- `internal/adapters/discord/discord.go` (new, ~200 LOC)
- `internal/adapters/discord/discord_test.go` (new, ~280 LOC)
- `cmd/leah/connect_discord.go` (new, ~120 LOC) — reads bot token via
  stdin, writes `~/.leah-state/secrets/discord-token.json` mode `0600`,
  emits `connect_discord` audit row.
- `docs/engineer/specs/2026-06-10-discord-adapter.md`
  (status -> `shipped`).

**Test plan**: `TestPostMessage_*`, `TestListChannels_HappyPath`,
`TestAuditRowHashed`, `TestNewRejectsMissingDeps`,
`TestPostMessage_RateLimit_RejectsBurst`. Failing-test-first commit
captures red output in PR body per CLAUDE.md TDD rule.

**Risk**: Low. Pure-Go scaffold behind a `Session` interface.

**Size**: ≤500 LOC including tests.

**Unblocks**: W66 (Subscribe), W67 (voice).

## Wave 66 — Discord Subscribe + reasoner routing

**Goal**: Long-lived WebSocket gateway session via `discordgo`. Inbound
messages from allowlisted guilds dispatch into the daemon's reasoner
pipeline. `go.mod` edit lands here.

**Files touched**:

- `go.mod` / `go.sum` — add `github.com/bwmarrin/discordgo v0.28.1`.
- `internal/adapters/discord/discord.go` — `Subscribe` implementation
  + guild-allowlist gate + handler dispatch.
- `internal/adapters/discord/discord_test.go` —
  `TestSubscribe_HandlerRouted`,
  `TestSubscribe_GuildAllowlistRejects`,
  `TestSubscribe_EmptyAllowlist_DropsAll`.
- `internal/comms/router.go` (new, ~80 LOC) — shared router that
  funnels adapter `Message` events into the reasoner. First caller;
  WhatsApp will join in W69.
- `cmd/leah-daemon/build_discord.go` (new) — constructs adapter with
  real `*discordgo.Session`.

**Risk**: Med. First long-lived network session in the daemon;
graceful-shutdown + reconnect-on-disconnect surface lives here.

**Size**: M (~400 LOC across files).

**Unblocks**: W71 (multi-channel daily brief), W72 (accept/reject).

## Wave 67 — Discord voice in/out

**Goal**: Inbound voice attachments hand off to the STT pipeline;
outbound TTS posts as a voice attachment via `ChannelFileSend`.

**Depends on**: voice-comm W11+ (STT + ChainTTS surfaces).

**Files touched**:

- `internal/adapters/discord/discord.go` — `PostVoice` + inbound voice
  attachment fetch.
- `internal/adapters/discord/discord_test.go` —
  `TestPostVoice_HappyPath`,
  `TestPostVoice_AudioBoundedSize`,
  inbound-voice handler test.
- `internal/comms/router.go` — voice-bearing `Message` routes to STT
  before reasoner dispatch.

**Risk**: Med. First voice round-trip through the comms layer;
boundary between adapter + STT + reasoner clarifies here.

**Size**: M (~250 LOC).

**Unblocks**: W70 (the voice path mirror for WhatsApp can copy this).

## Wave 68 — WhatsApp adapter scaffold

**Goal**: Land `internal/adapters/whatsapp` with `SendText`,
`SendTemplate`, and the `leah connect whatsapp` setup wizard. No
webhook server, no voice, no `go.mod` edit (stdlib only).

**Files touched**:

- `internal/adapters/whatsapp/doc.go` (new)
- `internal/adapters/whatsapp/whatsapp.go` (new, ~280 LOC) — REST
  client + Attestor + rate-limit + cost-cap + allowlist.
- `internal/adapters/whatsapp/whatsapp_test.go` (new, ~380 LOC).
- `cmd/leah/connect_whatsapp.go` (new, ~140 LOC) — interactive wizard:
  prompts operator for access token + phone-number ID + verify token
  + app secret via stdin, probes `ffmpeg -version`, writes
  `~/.leah-state/secrets/whatsapp-token.json` mode `0600`, emits
  `connect_whatsapp` audit row.
- `docs/engineer/specs/2026-06-10-whatsapp-adapter.md`
  (status -> `shipped`).

**Test plan**: `TestSendText_*`, `TestSendTemplate_NotInApprovedSet`,
`TestSendText_AllowlistRejects`, `TestSendText_CostCapExceeded`,
`TestSendText_RateLimit_RejectsBurst`, `TestAuditRowHashed`,
`TestNewRejectsMissingDeps`. Failing-test-first.

**Risk**: Low. Stdlib-only scaffold; no third-party dependency added.

**Size**: ≤700 LOC including tests (larger than Discord because of
template + cost-cap + allowlist surface).

**Unblocks**: W69 (webhook), W70 (voice).

## Wave 69 — WhatsApp webhook receiver

**Goal**: HTTPS webhook endpoint that handles Meta's verify-handshake
+ HMAC validation + payload dispatch. Inbound text messages route
through the shared `internal/comms/router.go` (introduced in W66).

**Local dev path**: cloudflared tunnel
(`cloudflared tunnel --url http://localhost:8443`).

**Production path**: regatta-Docker (per PR #74) with a stable public
ingress URL.

**Files touched**:

- `internal/adapters/whatsapp/webhook.go` (new, ~140 LOC) —
  `HandleWebhook` http.Handler implementation.
- `internal/adapters/whatsapp/whatsapp_test.go` —
  `TestWebhook_VerifyHandshake_*`, `TestWebhook_HMACInvalid_Rejects`,
  `TestWebhook_HMACValid_DispatchesHandler`,
  `TestWebhook_DuplicateMessageID_Dedup`.
- `cmd/leah-daemon/build_whatsapp.go` (new) — mounts handler on the
  daemon's HTTPS muxer at `/webhook/whatsapp`.
- `internal/comms/router.go` — WhatsApp adapter joins as second
  caller; router now serves two adapters.
- `docs/engineer/specs/2026-06-10-whatsapp-adapter.md` — note ops
  runbook (cloudflared command, regatta-Docker pointer).

**Risk**: Med-high. First webhook receiver in the daemon; HMAC-then-
parse ordering is load-bearing for the threat model.

**Size**: M (~400 LOC).

**Unblocks**: W71 (push), W72 (accept/reject).

## Wave 70 — WhatsApp voice in/out

**Goal**: Inbound `.ogg`/opus voice messages route to STT; outbound
TTS encodes to `.ogg`/opus via ffmpeg subprocess and uploads via the
Graph media endpoint.

**Depends on**: voice-comm W11+, W67 (Discord voice — first voice
round-trip in `internal/comms/router.go` lives there).

**Files touched**:

- `internal/adapters/whatsapp/whatsapp.go` — `SendVoice` + inbound
  media fetch path.
- `internal/adapters/whatsapp/whatsapp_test.go` —
  `TestSendVoice_OpusConversion`,
  `TestSendVoice_OversizedAudio_Rejected`,
  `TestWebhook_InboundVoice_FetchesMedia`.
- `internal/encoding/opus.go` (new, ~50 LOC) — ffmpeg-subprocess
  `OpusEncoder` implementation behind the spec's interface seam.
- `cmd/leah-daemon/build_whatsapp.go` — wires the real `OpusEncoder`.

**Risk**: Med. ffmpeg-subprocess error paths (binary missing,
encoding failure) need clean fall-throughs.

**Size**: M (~300 LOC).

**Unblocks**: W71 (push can include voice clips), W72 (accept/reject
can be spoken).

## Wave 71 — Multi-channel daily-brief push

**Goal**: Daily-brief task emits to a configurable set of channels:
gmail + slack (when shipped) + discord + whatsapp. Operator selects
the brief destinations via config; brief content is rendered per
channel (markdown for slack/discord, plaintext for whatsapp,
HTML+plaintext for gmail).

**Files touched**:

- `internal/brief/dispatcher.go` (new, ~120 LOC) — fans the rendered
  brief to all configured `comms.Adapter` instances; per-channel
  rendering helpers.
- `cmd/leah-daemon/brief.go` — wires Discord + WhatsApp adapters into
  the brief dispatcher alongside gmail.
- `internal/config/comms.go` — adds
  `BriefDestinations []ChannelTarget`; each target = `{adapter,
  channel_or_recipient}`.
- Tests: `TestBriefDispatcher_FansToAllAdapters`,
  `TestBriefDispatcher_PerChannelRendering`,
  `TestBriefDispatcher_PartialFailure_ContinuesOthers`.

**Risk**: Med. Partial-failure semantics (one adapter down MUST NOT
block the others) need explicit test coverage.

**Size**: M (~350 LOC).

**Unblocks**: W72.

## Wave 72 — Recommendation accept/reject via reply

**Goal**: When `learn-recommend-apply` (W15+) emits a recommendation,
the dispatcher posts it to the operator's chosen channel and awaits
a yes/no reply. The reply (text or voice transcribed via STT)
classifies into `accept` / `reject` / `defer` and the recommendation
engine applies or discards accordingly.

**Depends on**: `learn-recommend-apply` W15+, voice-comm W11+, W67,
W70 (so voice replies work on both channels).

**Files touched**:

- `internal/comms/conversation.go` (new, ~180 LOC) — short-lived
  conversation state machine keyed by `(adapter, channel/recipient,
  pending_rec_id)`; classifies inbound replies via a small
  yes/no/defer regex + LLM fallback.
- `internal/recommend/apply.go` — bridges the conversation outcome
  back to the recommendation engine's `Apply` / `Reject` calls.
- `cmd/leah-daemon/recommend.go` — wires the conversation state
  machine into the daemon's reasoner pipeline.
- Tests: `TestAcceptReject_TextYes_Applies`,
  `TestAcceptReject_VoiceNo_Rejects`,
  `TestAcceptReject_Timeout_Defers`,
  `TestAcceptReject_AmbiguousReply_AsksAgain`.

**Risk**: Med-high. Classifier false-positives (interpreting "yeah I
don't think so" as accept) are user-facing; tests must cover the
ambiguity cases.

**Size**: L (~450 LOC).

**Unblocks**: future autonomous-task waves where Leah needs operator
sign-off before mutating state (file edits, calendar sends, paid
actions).

## Cross-cutting decisions

- **One `comms.Adapter` shape across channels.** `internal/comms/
  router.go` introduced in W66 serves Discord first, WhatsApp second
  in W69, slack third (when that adapter lands). Three callers
  justify extracting an interface; until then the router uses
  per-adapter handlers without a premature `Adapter` super-interface.
- **No OAuth token-file convention parallel for Discord.** Discord
  bot tokens are long-lived and operator-rotated; no refresh path
  needed. WhatsApp Cloud API tokens have a refresh path (mirroring
  gmail) deferred to the wiring wave.
- **Hashed audit fields everywhere.** `channel_hash`, `guild_hash`,
  `author_hash`, `recipient_hash` carry the convention from
  imessage / facetime. Plaintext channel / guild / phone / Discord
  IDs MUST never appear in audit-derived UI.
- **Rate-limit middleware extraction deferred.** Each adapter inlines
  its limiter MVP-style. Extraction into shared middleware happens
  when three callers exist (Discord + WhatsApp + slack) — the same
  trigger imessage-facetime-plan W24 set.
- **Webhook ingress is regatta-Docker's problem.** Production
  WhatsApp webhook hosting reuses the regatta-Docker public ingress
  from PR #74; Leah does not own its own TLS-cert + DNS surface.
- **Voice channel join (Discord) + WhatsApp groups (multi-recipient)
  are explicit non-goals across W65-W72.** Both are real product
  asks; both demand their own design pass.

## Anti-goals (none of these land in W65-W72)

- Discord voice-channel real-time join / PCM streaming.
- Discord threads as a first-class routing target (parent channels
  only).
- Discord slash-command registration.
- WhatsApp groups; multi-number per operator; WhatsApp Pay /
  commerce.
- WhatsApp Web automation (`whatsmeow` or similar) — ToS risk;
  explicit non-goal.
- Operator-configurable rate limits (issue filed; deferred).
- Token-rotation automation (deferred; mirrors gmail pattern).
- Slack adapter (separate spec; this plan only consumes it from W71
  if it has shipped by then).

---
title: Comms Notifier — morning brief to Discord/WhatsApp
status: proposed
owner: tri
created: 2026-06-21
---

# Comms Notifier

## 1. Goal

Deliver the proactive morning brief to a remote surface (Discord channel /
WhatsApp thread) so it reaches the operator's phone when the Mac is asleep or
the operator is away. Today `buildBriefNotifier` fans out across desktop +
local-voice + pushover only; Slack/Discord/WhatsApp have no
`contracts.Notifier` (the brief code comments this as "the next wiring step").
This wave wires the comms adapters as notifiers so the *already-shipped* daily
brief cron pushes remotely with zero changes to brief composition.

Outcome: at the `LEAH_BRIEF_HOUR` daily fire, the brief summary lands in a
designated Discord channel and/or WhatsApp number as text; optionally as voice
when F1's `Synthesize` is wired.

## 2. Producer it depends on (verified present on main @ 7621dbb)

- `internal/contracts/notifier.go` — `Notifier` interface
  (`Notify(ctx, title, body) error`). **Present** (desktop/voice/pushover
  satisfy it).
- `internal/notify/fanout.go` — `Fanout{Notifiers []contracts.Notifier}`.
  **Present.**
- `cmd/leah-daemon/brief.go` — `buildBriefNotifier()`, `buildBriefTask`,
  `LEAH_VOICE_ENABLED`/`LEAH_BRIEF_DAILY`/`LEAH_BRIEF_HOUR` gates.
  **Present** — the brief already calls `Fanout.Notify`.
- `internal/adapters/discord` — `PostMessage(ctx, channelID, body)` and
  `PostVoice(ctx, channelID, audio []byte)`. **Present.**
- `internal/adapters/whatsapp` — `SendText(ctx, to, body)`. **Present.**
  (`SendVoice` does NOT exist — see §6/risk.)
- F1 `Synthesizer` seam (`2026-06-21-tts-bytes-seam.md`) — required ONLY for
  the voice path; **text path has no F1 dependency**.

## 3. Interface surface

Thin adapters in `internal/notify/` (keeps adapter packages free of the
notify import; mirrors how desktop/voice live there):

```go
// internal/notify/discord.go
type DiscordNotify struct {
    Poster    interface{ PostMessage(context.Context, string, string) error }
    ChannelID string
}
func (d *DiscordNotify) Notify(ctx context.Context, title, body string) error

// internal/notify/whatsapp.go
type WhatsAppNotify struct {
    Sender interface{ SendText(context.Context, string, string) error }
    To     string
}
func (w *WhatsAppNotify) Notify(ctx context.Context, title, body string) error
```

- Each formats `"<title>: <body>"` (same shape as `VoiceNotify.Notify`).
- Wiring in `cmd/leah-daemon/brief.go::buildBriefNotifier`: append a
  `DiscordNotify` only when discord is connected AND `LEAH_BRIEF_DISCORD_CHANNEL`
  is set; `WhatsAppNotify` only when whatsapp is connected AND
  `LEAH_BRIEF_WHATSAPP_TO` is set — same connected-token-gated, silent-absence
  pattern as `briefOpts` (an unconfigured channel stays silent, never errors).
- **Voice path (after F1):** an optional `LEAH_BRIEF_VOICE_REMOTE=1` makes the
  discord notifier call `Synthesize` then `PostVoice`; falls back to text on
  `ErrSynthesizeUnsupported`. Voice is additive over text, never replaces it
  (text is the durable record).

## 4. Test plan (TDD — failing test first)

- `DiscordNotify.Notify` calls `PostMessage` with `"<title>: <body>"` (fake
  poster captures args).
- `WhatsAppNotify.Notify` calls `SendText` with the formatted body.
- A poster returning an error surfaces a wrapped error (the fan-out must not
  silently drop a failed remote push).
- `buildBriefNotifier` includes the discord notifier ONLY when both the
  connected-token file and `LEAH_BRIEF_DISCORD_CHANNEL` are present; omits it
  otherwise (table test over the 4 present/absent combinations).
- Fan-out independence: a failing remote notifier does not prevent the desktop
  notifier from firing (existing `Fanout` semantics — assert preserved).

## 5. Risk

- **Rate limits** — Discord's per-channel 10/60s cap is enforced inside
  `PostVoice`/`PostMessage`; a once-daily brief is far under. No new limiter.
- **Audit/PII** — the brief summary is operator-facing content already written
  to `~/.leah-state/briefs/`; pushing it to a *connected* channel the operator
  configured is consistent with that trust boundary. Document that the channel
  must be operator-private.
- **Attestation** — `PostMessage`/`PostVoice` already gate on the attestation
  scope; the notifier inherits it. No bypass.

## 6. Out of scope

- WhatsApp **voice**-out: `SendVoice` does not exist on the whatsapp adapter.
  Adding it (media-upload endpoint + ogg transcode) is a separate adapter wave;
  this spec ships whatsapp **text** only and discord **text + voice**.
- Inbound replies / accept-reject — that is roadmap wave F3 (unspec'd; see the
  roadmap spec-gap list).
- Slack/MSTeams notifiers — same pattern, deferred to demand.
- Any change to brief composition (`internal/brief`) — this wave is pure
  delivery wiring.

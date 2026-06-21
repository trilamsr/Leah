---
title: Leah — forward roadmap (remaining waves to a shippable personal Leah)
status: living
owner: tri
created: 2026-06-21
---

# Forward roadmap

Dependency-ordered sequence of the waves still genuinely remaining to a
shippable personal Leah (path-a-full: personal-use product, owner = user, all
UX-audit blockers binding pre-launch). Supersedes the wave tables in
`docs/specs/2026-06-09-roadmap-overview.md` for *sequencing*; that doc remains
the north-star statement. Per-wave Linear tickets live in project Leah
(`https://linear.app/themaydow/project/leah-a8d553e8cc88`).

This roadmap exists to kill four recurring blockers seen in build waves:

1. **Missing producer** — a consumer wired against a producer that does not
   exist (HUD subscribed to unemitted events; brief wired gmail/gcal before the
   transport existed). Every wave below names its **producer prerequisites** —
   what must exist *first* — and whether they are verified-present on main.
2. **Ticket-vs-reality drift** — tickets in Backlog while code shipped. This
   doc is derived from `git`/source, not ticket state. The "Already shipped"
   section records what is done so no wave re-builds it.
3. **Dependency surprises** (e.g. a connect-registry wave blocking four
   adapters; go.mod-touching waves needing single-owner). Each wave flags
   **go.mod?** and its **parallelism group**.
4. **Voice-OUT gap** — remote voice-out (WhatsApp/Discord) blocked because the
   TTS chain only plays locally via `afplay`; no backend returns audio bytes.
   Defused by Wave F1 (TTS-bytes seam) first in the critical path.

## Already shipped (verified on main @ 7621dbb — do NOT re-build)

- **Memory / self-learn / patterns / ctx / self-build dispatcher** — the six
  Wave-1 closed-loop substrate pieces. `internal/dispatcher/selfbuild.go`
  emits `self-build` + `self-build.outcome` audit rows; `ship` row from
  `Ship.Run`. CLI commands wired in `cmd/leah/`.
- **Brief composition + daily cron + live gmail/gcal sections** — `internal/
  brief` (`Gather`/`Render`/`VoiceSummary`), `cmd/leah-daemon/brief.go`
  (`buildBriefTask`, `LEAH_BRIEF_DAILY=1` @ `LEAH_BRIEF_HOUR` default 8,
  `LEAH_VOICE_ENABLED=1` gate). gmail/gcal listers live behind connected-token
  gating (`briefOpts`).
- **TTS chain (local playback)** — `internal/voice` Kokoro→OpenAI→`say`,
  `TTS.Speak(ctx,text) error`. Plays locally only.
- **Comms adapters (transports landed)** — gmail, gcal, slack, discord, jira,
  confluence, notion, whatsapp, msteams, linear, maps, flights, facetime,
  imessage. Discord `PostVoice([]byte)` consumes audio; whatsapp `SendText`
  only (no voice-out yet).
- **Connect registry + OAuth refresh** — `internal/connect` (registry,
  refreshing token source, rotated-token persistence). The W51 dependency that
  blocked downstream adapters is resolved.
- **Daemon degraded-pull tier, audit retention/rotation, obs/SSE, HUD freshness.**

## Critical path

**To closed-loop runnable (M2 exit):** all substrate is shipped. The only
remaining gate is an **end-to-end validation harness** that proves the 9-step
loop fires for one real feature — Wave **M2-V** below. No new producer needed;
it consumes audit kinds that already emit. This is the shortest path and
should run first.

**To proactive morning brief (M3):** brief + daily cron + voice-summary +
local TTS are shipped. The remaining felt-UX gap is **delivering the brief to a
remote surface** (phone) when the Mac is asleep or the operator is away. That
requires (a) Wave **F1** TTS-bytes seam → (b) Wave **F2** comms `Notifier` so
the existing `buildBriefNotifier` fan-out can push to Discord/WhatsApp.

Ordered critical path:

```
M2-V  (closed-loop validation harness)      [no new producer — run first]
  |
F1    (TTS Synthesize->[]byte seam)          [unblocks all remote voice-out]
  |
F2    (comms Notifier: brief->Discord/WhatsApp text+voice)   [voice path depends F1]
  |
F3    (proactive accept/reject round-trip from remote surface)
```

## Remaining waves

Felt-UX value is stated per wave (UX > performance > long-term per CLAUDE.md).

### M2-V — closed-loop validation harness
- **Goal:** prove the 9-step self-build loop end-to-end for one real feature;
  capture each audit kind as it fires; surface a pass/fail receipt.
- **Producer prereqs (verified on main):** `self-build` + `self-build.outcome`
  + `ship` audit kinds (`internal/dispatcher/selfbuild.go`); `daemon.transition`
  rows (`internal/daemonloop`); regatta client. All present.
- **go.mod?** No.
- **Parallelism:** single-owner (touches `internal/dispatcher` + a new
  validation package); spec serializes.
- **Felt-UX:** confidence the north-star loop actually closes; unblocks
  chaining `leah self-build`.
- **Spec:** `docs/engineer/specs/2026-06-21-closed-loop-validation.md` (this wave).

### F1 — TTS Synthesize->[]byte seam
- **Goal:** add `Synthesize(ctx,text) ([]byte, mime, error)` alongside the
  existing `Speak`, so callers that need to *transmit* audio (remote comms) get
  bytes instead of local-only `afplay` playback. Kokoro/OpenAI already write a
  temp wav — return those bytes; `say` declines (no file artifact).
- **Producer prereqs (verified on main):** `internal/voice` chain
  (`tts.go`, `kokoro.go`, `openai.go`); `discord.PostVoice([]byte)` consumer
  exists. All present.
- **go.mod?** No.
- **Parallelism:** single-owner of `internal/voice`.
- **Felt-UX:** the seam that makes Leah's voice reach the operator's phone.
- **Spec:** `docs/engineer/specs/2026-06-21-tts-bytes-seam.md` (this wave).

### F2 — comms Notifier (brief -> Discord/WhatsApp)
- **Goal:** implement `contracts.Notifier` over discord + whatsapp so
  `buildBriefNotifier`'s fan-out (today: desktop + local-voice + pushover) gains
  remote text; remote *voice* rides F1's bytes via `PostVoice` / a new whatsapp
  `SendVoice`.
- **Producer prereqs:** F1 (for voice); discord `PostMessage`/`PostVoice` +
  whatsapp `SendText` (present); `notify.Fanout` (present). Text-only F2 can
  start before F1; voice path waits on F1.
- **go.mod?** No (transports already vendored; discordgo require line is the
  one go.mod edit and was deferred to the discord wiring wave — confirm before
  promoting).
- **Parallelism:** discord-notifier and whatsapp-notifier are file-disjoint ->
  parallelize 2.
- **Felt-UX:** brief lands on the phone when the Mac is asleep.
- **Spec:** `docs/engineer/specs/2026-06-21-comms-notifier.md` (this wave).

### F3 — remote accept/reject round-trip
- **Goal:** operator replies to a pushed prompt from the remote surface (e.g.
  "approve") and the daemon acts (merge nudge / dispatch). Closes the proactive
  loop both directions.
- **Producer prereqs:** F2 (outbound push); discord/whatsapp *inbound*
  subscribe (`discord/subscribe.go` present; whatsapp inbound voice present);
  attestation gate (`internal/attestation`, present).
- **go.mod?** No.
- **Parallelism:** single-owner (cross-cuts daemon + adapters).
- **Felt-UX:** approve a self-build from the couch. Highest leverage, deepest
  dependency — schedule last.
- **Spec:** `docs/engineer/specs/2026-06-21-inbound-reply-consent.md` (MAY-267).
  Discord-gateway inbound (loopback-safe) primary; whatsapp-webhook deferred
  (needs tunnel). Load-bearing piece is the per-action consent gate: a remote
  reply never clears a weaker attestation than the same action faces locally.

### Adapter-deepening (demand-gated, not on critical path)
Per `roadmap-overview.md` Wave 4: each external write-path ships only on
operator felt-pain. These are **parallelism-friendly** (each touches its own
`internal/adapters/<pkg>/`, up to 6) and mostly **no go.mod** now that
transports are hand-rolled. Examples still open: slack DM write, jira/notion
write-paths (W53/54/55 brief-enrichment + dispatch-write contracts), Plaid
finance read, travel research. **Do not detail-design these until demanded** —
deep specs now would be speculative far-future waste.

## Spec-gap list (ranked, highest-risk first)

1. **F3 remote accept/reject round-trip** — **spec'd**
   (`docs/engineer/specs/2026-06-21-inbound-reply-consent.md`, MAY-267). The
   inbound command path that *acts* now has an explicit per-action consent
   contract closing the attestation-boundary hole. Ready to implement (router →
   consent → classifier → discord wiring; whatsapp deferred).
2. **W53/54/55 work-tools dispatch-write contracts** (jira/notion/slack write
   integration) — referenced in the original blocker list, no integration-
   contract spec. Demand-gated, but spec the *contract surface* before the first
   write-path wave so consumers don't wire against an undefined producer.
3. **M3-3/3-4 backlog + smart `leah ship --from-*` flags** — listed in
   roadmap-overview Wave 3, no spec. Low risk (read-only context assembly), low
   dependency. Spec when picked up.

## Single biggest future blocker + how this roadmap defuses it

**Biggest blocker: remote voice-out has no producer.** Every comms voice
consumer (`discord.PostVoice`, future whatsapp `SendVoice`) takes `[]byte`, but
the entire TTS chain only `afplay`s locally — `Speak(ctx,text) error` returns
nothing transmittable. Any future "Leah speaks to my phone" wave wired against
the existing TTS would dead-end exactly like the HUD-vs-unemitted-event and
brief-vs-missing-transport cases.

**Defused by ordering F1 (TTS-bytes seam) before F2/F3 on the critical path**
and by naming it the producer prerequisite of every remote-voice wave. The F1
spec (`2026-06-21-tts-bytes-seam.md`) reuses the temp-wav both backends already
write, so the seam is additive (no rewrite of `Speak`) and right-sized.

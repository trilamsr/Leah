# TTS provider research — Leah voice canon (2026-06-21)

Validation pass on spec §2.7 (ElevenLabs alto, 145 wpm). Question: best UX (warm + distinctive + low-latency streaming) at sensible cost for single-operator personal assistant (~50–100 utterances/day, ~100 char avg).

Sources fetched 2026-06-21: elevenlabs.io/pricing + /docs streaming + cloning; openai pricing + tts-docs + realtime guide; cartesia.ai/pricing + docs + tts API; deepgram.com/product/text-to-speech; ai.google.dev gemini speech-generation. Apple AVSpeechSynthesizer (developer.apple.com — empty SPA, supplemented by prior knowledge).

---

## 1. Latency table (TTFB, streaming)

| Provider / model            | Published TTFB (streaming)        | Real-world (3rd-party 2025 benchmarks) | Streaming protocol     |
|-----------------------------|-----------------------------------|----------------------------------------|------------------------|
| Cartesia Sonic-2 / Sonic-3.5| ~40 ms (Sonic-2), ~90 ms (3.5 HD) | 75–150 ms US-East                      | WebSocket, raw PCM     |
| ElevenLabs Flash v2.5       | ~75 ms                            | 100–200 ms                             | HTTP chunked + WS      |
| ElevenLabs Turbo v2.5       | ~250–400 ms                       | ~300 ms                                | HTTP chunked           |
| ElevenLabs Multilingual v2  | ~600–1200 ms (highest quality)    | ~800 ms                                | HTTP chunked           |
| Deepgram Aura-2             | <200 ms (WS), ~300 ms HTTP        | ~250 ms                                | WebSocket + HTTP       |
| OpenAI gpt-4o-mini-tts      | ~400–600 ms                       | ~500 ms                                | HTTP chunked PCM       |
| OpenAI Realtime (gpt-4o)    | LLM+TTS fused; ~300–500 ms total  | ~400 ms                                | WebRTC / WebSocket     |
| Gemini 2.5 Flash native audio| ~300–500 ms (Live API)            | ~400 ms                                | Live API (WebSocket)   |
| Apple AVSpeechSynthesizer   | <50 ms (local, no network)        | <50 ms                                 | On-device CoreAudio    |

For ambient assistant where reply must start <500 ms after first LLM token: **Cartesia, ElevenLabs Flash, Apple, Deepgram** clear the bar. ElevenLabs Multilingual v2 + Turbo + OpenAI tts-1-hd batch do not.

## 2. Cost table (50–100 utterances/day × 100 chars = ~300k chars/mo upper bound)

| Provider / plan                       | Unit price                                | Cost/mo @ 300k chars  | 6-mo cost  | Notes                                       |
|---------------------------------------|-------------------------------------------|-----------------------|------------|---------------------------------------------|
| ElevenLabs Free                       | 10k credits/mo free                       | over quota            | n/a        | watermark, no commercial                    |
| ElevenLabs Starter $6/mo              | 30k credits/mo                            | over quota            | n/a        | adds Instant Voice Cloning                  |
| **ElevenLabs Creator $11/mo (50% promo) / $22 list** | 121k credits/mo (1 credit = 1 char on Flash/Turbo; 2× on v2) | $22 (need top-up or 2× downgrade) | $132        | Pro Voice Cloning unlocked here             |
| ElevenLabs Pro $99/mo                 | 500k credits/mo                           | $99 (well under cap)  | $594       | overkill                                    |
| Cartesia Free                         | ~33 min/mo (~20k chars)                   | over quota            | n/a        |                                             |
| **Cartesia Pro $4/mo (promo) / $5 list** | ~133 min/mo (~80k chars)               | $5 + overage          | $30–60     | Instant clone included; pro clone NOT       |
| Cartesia Startup $49/mo               | ~500 min/mo                               | $49                   | $294       |                                             |
| Deepgram Aura-2                       | $0.030 per 1k chars (pay-go)              | ~$9                   | $54        | $200 free credit on signup                  |
| OpenAI gpt-4o-mini-tts                | $0.60 / 1M input tokens (~$12 / 1M chars) | ~$3.60                | $22        | 6 stock voices, no clone                    |
| OpenAI tts-1-hd                       | $30 / 1M chars                            | ~$9                   | $54        | 6 stock voices                              |
| OpenAI Realtime gpt-4o-mini           | $10/1M audio input + $20/1M audio output  | ~$5–15 (depends on LLM turns) | $30–90 | bundles LLM; not pure TTS                |
| Gemini 2.5 Flash native audio         | $12 / 1M output audio tokens              | ~$8                   | $48        | 30 prebuilt voices (Kore, Puck, etc.)       |
| **Apple AVSpeechSynthesizer**         | $0 (on-device)                            | **$0**                | **$0**     | Premium voices (Ava, Zoe, Evan) free        |

## 3. Voice quality + distinctiveness

- **ElevenLabs**: still the warmth/expressiveness leader. Pro Voice Clone (Creator tier+) produces a near-indistinguishable alto from ~30 min source audio; library has thousands of community + featured voices. Risk: many SaaS products already use ElevenLabs library voices → "generic AI-assistant" risk if a library voice is picked.
- **Cartesia Sonic-3.5**: state-of-the-art latency + quality is now competitive with ElevenLabs Multilingual v2 in blind A/B (2025 LMArena TTS leaderboard puts Sonic and ElevenLabs within margin of error). Instant clone is included; pro clone is **NOT** offered (gone from current Pro tier — see fetched pricing page). This is a 2026 regression vs earlier coverage.
- **OpenAI**: 6 fixed voices (alloy, echo, fable, onyx, nova, shimmer + coral, ash, sage, ballad on gpt-4o-mini-tts). No cloning. With gpt-4o-mini-tts you can pass `instructions: "Speak in a cheerful and positive tone"` to steer prosody — useful but still a stock voice underneath. Recognizable from ChatGPT app → distinctiveness penalty.
- **Gemini 2.5 Flash native audio**: 30 prebuilt voices, no cloning. Quality good, less expressive than ElevenLabs/Cartesia.
- **Deepgram Aura-2**: 40+ voices, optimized for agent/call-center use. Solid but neutral; not the voice you pick for "warm distinctive companion."
- **Apple AVSpeechSynthesizer**: Premium voices (Ava, Zoe, Evan, Joelle) on macOS 14+ are surprisingly good — Siri-tier neural. Free, offline, zero privacy exposure. Limitations: no cloning, no per-utterance prosody steering, "Siri-adjacent" → high distinctiveness penalty (sounds like Siri to anyone who's used a Mac).

## 4. Voice cloning

| Provider     | Instant clone (minutes audio)  | Pro clone (hours audio)        | Cost                        |
|--------------|--------------------------------|--------------------------------|-----------------------------|
| ElevenLabs   | Yes (Starter+, ~1 min sample)  | Yes (Creator+, ~30 min sample) | Included in plan; no per-clone fee |
| Cartesia     | Yes (Pro tier, included)       | **Removed from current pricing page** | n/a                  |
| OpenAI       | No                             | No                             | n/a                         |
| Gemini       | No                             | No                             | n/a                         |
| Deepgram     | Custom voice (enterprise)      | Custom voice (enterprise)      | quote-only                  |

ElevenLabs is the only provider offering self-serve professional voice cloning at <$25/mo in 2026.

## 5. Privacy

- **Apple AVSpeechSynthesizer**: zero exposure — bytes never leave device. Only winner if privacy is a hard requirement.
- **ElevenLabs**: per ToS, request payloads may be retained for abuse-monitoring 30 days; "Zero Retention" mode on Enterprise only.
- **OpenAI**: Standard API does not train on data, but 30-day retention for abuse review. Zero-retention requires Enterprise / approved use case.
- **Cartesia**: SOC 2 Type 2; does not train on customer data; retention not clearly specified on Pro plan.
- **Gemini**: Free tier *trains on data*; Paid tier does not. Always opt for paid if used.
- **Deepgram**: opt-out training by default; 0-day retention available.

For a personal AI handling sensitive content (calendar, email, finance): all cloud TTS sees the reply text. Mitigation: sensitive prefix routes to Apple local; everything else cloud.

## 6. Realtime API (LLM + TTS fused)

OpenAI Realtime (gpt-4o-realtime) and Gemini Live API both fuse LLM + TTS in one streamed WebSocket call. Latency benefit: ~150–250 ms savings vs LLM→TTS pipeline (no inter-service round-trip). Trade-off: locks Leah to OpenAI/Google as the *brain*, which conflicts with current Anthropic Claude backbone. For Claude-backbone Leah, fused realtime is unavailable — separate TTS is the only path. (Anthropic has not shipped a fused audio API as of 2026-06.)

## 7. Recommendation

**Primary: ElevenLabs Flash v2.5 with a Professional Voice Clone (alto), Creator plan $11–22/mo.**
- Best warmth/expressiveness leader in 2026.
- Flash v2.5 hits ~75–150 ms TTFB — clears the <500 ms budget with margin for LLM-token gap.
- Pro Voice Clone is the only way to escape generic-AI-assistant sound; spec §2.7 was right.
- Cost: $132 / 6mo (or $66 with promo). Acceptable for single-operator product.

**Offline fallback: Apple AVSpeechSynthesizer, voice "Ava (Premium)" at 175 wpm.**
- Triggers when (a) offline, (b) ElevenLabs 429/5xx, (c) sensitivity classifier flags content as private (calendar/email/finance).
- $0, on-device, zero latency, zero exposure.
- UX cost: "Siri-adjacent" voice swap is noticeable — surface a 1-char status glyph so operator knows why voice changed.

**Runner-up worth re-evaluating in 6 months: Cartesia Sonic-3.5.** Latency leader, $4–5/mo at this volume, quality within margin of ElevenLabs. The only blocker is the missing professional-clone tier on current Pro plan — if Cartesia reinstates pro clone, switch.

**Reject: OpenAI tts (no cloning + recognizable as ChatGPT voice), Gemini (no cloning), Deepgram (call-center timbre, not companion), Realtime APIs (lock Leah off Anthropic backbone).**

**Voice canon decision**: keep spec §2.7. Custom-cloned alto via ElevenLabs Pro Voice Clone is correct. Vendor-lock risk is real but mitigated by (a) keeping the source clone-audio file in repo so the same voice can be re-cloned on Cartesia/competitor if ElevenLabs price spikes, (b) Apple fallback already providing graceful degradation, (c) abstracting the TTS call behind `internal/tts/provider.go` interface — switching providers is a 1-day swap, not a rewrite.

**6-month projected spend: $132 (ElevenLabs Creator, no promo) + $0 (Apple fallback) = $132. With promo: $66.**

---
title: TTS Synthesize->bytes seam (remote voice-out producer)
status: proposed
owner: tri
created: 2026-06-21
---

# TTS bytes seam

## 1. Goal

Give the TTS chain a way to *return* synthesized audio as `[]byte` instead of
only playing it locally via `afplay`. Today every backend
(`KokoroTTS`, `OpenAITTS`, `SayTTS`) implements `Speak(ctx,text) error`, which
synthesizes to a temp wav and plays it on the local audio device — the bytes
are never handed back. The comms voice consumers
(`discord.PostVoice(ctx, channelID, audio []byte)` and a future whatsapp
`SendVoice`) need those bytes. This seam is the **producer** every remote
voice-out wave (roadmap F2/F3) depends on; without it those waves dead-end.

Outcome: `Synthesize(ctx,text) ([]byte, string, error)` returns wav bytes +
MIME type, so the daemon can speak the morning brief to the operator's phone,
not just the desk.

## 2. Producer it depends on (verified present on main @ 7621dbb)

- `internal/voice/tts.go` — `TTS` interface, `ChainTTS`, `Executor`,
  `withAudioDevice`. **Present.**
- `internal/voice/kokoro.go` — `KokoroTTS.Speak` writes a temp wav at `path`
  via `kokoro -t <text> -o <path>` *before* `afplay`. The bytes already exist
  on disk. **Present.**
- `internal/voice/openai.go` — `OpenAITTS.Speak` POSTs `response_format: wav`
  and `io.Copy`s the response into a temp wav *before* `afplay`. **Present.**
- `internal/adapters/discord/voice.go` — `PostVoice([]byte)` consumer.
  **Present** (proves the consumer side is real, not stub).

No missing producer. This spec is additive.

## 3. Interface surface

Add a sibling capability — do NOT change `Speak` (callers that want local
playback keep it unchanged):

```go
// internal/voice/tts.go

// Synthesizer returns synthesized audio as bytes plus its MIME type,
// for callers that must transmit audio (remote comms) rather than play it
// locally. Backends that cannot produce a file artifact (macOS `say`)
// return ErrSynthesizeUnsupported.
type Synthesizer interface {
    Synthesize(ctx context.Context, text string) (audio []byte, mime string, err error)
}

var ErrSynthesizeUnsupported = errors.New("voice: backend cannot return audio bytes")
```

- `KokoroTTS` and `OpenAITTS` implement `Synthesizer`: factor the existing
  synth half of `Speak` into a private `synth(ctx,text) (path string, err error)`,
  read the file, return `(bytes, "audio/wav", nil)`. `Speak` calls `synth`
  then `afplay` — behaviour-preserving refactor.
- `SayTTS` returns `ErrSynthesizeUnsupported` (no file artifact path).
- `ChainTTS.Synthesize` walks backends, skipping any returning
  `ErrSynthesizeUnsupported`, returning the first success — mirroring the
  existing `Speak` chain semantics. If every backend is unsupported/failed,
  returns the wrapped last error (same shape as `Speak`'s
  "all backends failed").

Audio-device lock (`withAudioDevice`) is NOT taken on the Synthesize path —
synthesis already runs unlocked today; only `afplay` holds the device.

## 4. Test plan (TDD — failing test first)

- `ChainTTS.Synthesize` returns kokoro bytes when kokoro succeeds (fake
  `Executor` writes known bytes to the `-o` path).
- Falls through to OpenAI when kokoro returns `ErrSynthesizeUnsupported`/error
  (fake HTTP client returns a known wav body).
- `SayTTS.Synthesize` returns `ErrSynthesizeUnsupported` and is skipped by the
  chain (not treated as a hard failure).
- Empty chain / all-unsupported returns a non-nil error (no panic, no empty
  success).
- MIME is `audio/wav` on success.
- `Speak` behaviour unchanged: existing `Speak` tests stay green (the
  refactor must not alter local-playback semantics) — this is the regression
  guard for the `synth` extraction.

## 5. Risk

- **Backend voice drift** (kokoro vs openai produce different voices on
  fallback) — already an accepted shift; Synthesize inherits it, no new risk.
- **Memory** — wav for a 1-sentence brief is small; cap is enforced downstream
  by the consumer (`discord.maxVoiceBytes = 8MB`), not here. Document that
  Synthesize does not cap; the transmit adapter does.
- **`say` gap** — on a host where only `say` is available, remote voice-out is
  unavailable. Acceptable: it degrades to text-only push (F2 handles text
  independently). Surface via the existing voice `SelfChecker` so `/health`
  reports "voice synth unavailable" distinctly from "voice unwired".

## 6. Out of scope

- Streaming / chunked synthesis (the brief is single-shot; defer per
  tts-kokoro §8 #4).
- Opus/ogg transcoding — Discord accepts `audio/*`; whatsapp transcode (if
  required by its media endpoint) belongs in the whatsapp adapter, not here.
- Any change to `Speak` semantics or the backend selection order in
  `pickBackends`.
- Wiring Synthesize into a notifier — that is roadmap wave F2
  (`2026-06-21-comms-notifier.md`).

> All internal paths in this doc reflect the pre-2026-07-09 layout; current tree per `git ls-tree -d --name-only HEAD:internal/`.

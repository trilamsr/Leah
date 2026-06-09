# TTS Swap: OpenAI tts-1-hd → Kokoro-82M Local

**Status:** draft
**Date:** 2026-06-09
**Reference:** https://github.com/hexgrad/kokoro (Apache-2.0)

## 1. Goal

Swap default text-to-speech backend from metered OpenAI `tts-1-hd` to local
Kokoro-82M (82M-param TTS, Apache-2.0, runs on Apple MPS). Keep OpenAI as a
quality fallback and macOS `say` as a last-resort offline backend.

Outcome: cost-zero predictable TTS surface for the personal-Mac operator; no
per-character billing; no network hop on the hot path.

## 2. Architecture — 3-tier chain

```
NewTTS() → ChainTTS
            ├─ 1. KokoroTTS    (default: local kokoro CLI → afplay)
            ├─ 2. OpenAITTS    (fallback: HTTP /v1/audio/speech → afplay)
            └─ 3. SayTTS       (last-resort: macOS `say` binary)
```

Selection at construction time:
- Probe Kokoro: `kokoro --version` exits 0 within 200ms → use.
- Else probe `OPENAI_API_KEY` env: present → use OpenAITTS.
- Else: SayTTS (always available on macOS).

Runtime per-call fallback: if the active backend returns a non-nil error
on `Speak`, the chain tries the next backend in order. A backend that init
succeeded but errors at speak time does NOT get demoted permanently — every
call re-tries from the top of the chain. (Rationale: transient errors are
common; permanent demotion hides recovery.)

## 3. Interface

```go
// internal/voice/tts.go
package voice

import "context"

// TTS speaks the given text aloud. Implementations may block until playback
// completes or return immediately and play in background — caller must not
// assume either. Cancel via ctx.
type TTS interface {
    Speak(ctx context.Context, text string) error
}
```

## 4. Selection — `NewTTS()`

```go
func NewTTS() TTS {
    return &ChainTTS{
        backends: pickBackends(),
    }
}
```

`pickBackends()` returns an ordered slice. The chain's `Speak` walks the
slice, returning on first success.

## 5. Cost & quality matrix

| Backend       | Cost                         | Quality        | Latency (start)  | Offline | License        |
|---------------|------------------------------|----------------|------------------|---------|----------------|
| Kokoro-82M    | $0 (local)                   | Natural        | ~500ms cold-start, ~100ms warm | Yes (after model d/l) | Apache-2.0 |
| OpenAI tts-1-hd | $30 / 1M chars             | High           | ~700ms (network) | No      | Proprietary    |
| macOS `say`   | $0                           | Robotic        | ~50ms            | Yes     | macOS bundled  |

Default workload (~50 briefs/day × 30 words ≈ 9000 chars/day) on OpenAI =
~$0.27/day = $98/year. Kokoro = $0 ongoing after one-time model download
(~330MB).

## 6. Install instructions (operator)

```bash
# One-time
pip install kokoro-tts

# Verify
kokoro --version  # should print version string
which kokoro      # should resolve on $PATH

# Optional: pre-warm model cache
kokoro -t "hello" -o /tmp/warm.wav
```

If Kokoro is unavailable the chain transparently falls back; the operator
loses cost-zero but the system keeps working.

## 7. Build order

1. **Spec** (this doc).
2. **Scaffold `internal/voice/`** — interface + 3 backends + ChainTTS +
   tests. No wiring; pkg compiles green; tests pass without requiring
   `kokoro` / `say` / network to be present.
3. **Wire to existing `internal/notify/`** (FOLLOW-UP, separate PR) —
   `Desktop.Notify` can optionally call `TTS.Speak` for audible alerts.

## 8. Adversarial review (self-critique)

| # | Concern                                              | Severity | Disposition                                                  |
|---|------------------------------------------------------|----------|--------------------------------------------------------------|
| 1 | Kokoro subprocess cold-start ~500ms                  | LOW      | Acceptable for TTS; tts-1-hd network is ~700ms. Documented. |
| 2 | afplay piping vs Kokoro built-in playback            | MED      | Decision: write wav to temp file → afplay. Single audio path across all 3 backends → easier to reason about + cancel via ctx. Kokoro built-in playback would diverge from OpenAI path. |
| 3 | Voice ID consistency across backends                 | MED      | Accepted shift: persona drift when chain falls back. Document expected voice per backend. Phase 1 picks Kokoro `af_bella` (closest to OpenAI `nova`). |
| 4 | 30-word brief = ~5s wait before audio starts (no streaming) | MED | Accepted Phase 1. Future: chunk-on-sentence for >50-word inputs. Personal-Mac use-case: 30-word brief acceptable single-shot. |
| 5 | pip install adds Python runtime dep                  | LOW      | Operator is on personal Mac; Python already required by other tools. Documented in §6. |
| 6 | M-series MPS requirement                             | LOW      | Operator confirmed Mac primary; no CI playback. |
| 7 | Apache-2.0 license                                   | NONE     | Clean for personal + commercial. |
| 8 | Kokoro CLI surface drift (upstream API change)       | LOW      | Single shell-out point in `kokoro.go::Speak`; flag changes are 1-line fixes. |
| 9 | Tests cannot rely on `kokoro` / `say` being installed | HIGH    | Use Executor interface (mirrors `internal/notify/`); inject fake in tests. Production wires real `exec.Command`. |
| 10 | Concurrent Speak calls collide on afplay             | LOW     | Phase 1: serialize via `sync.Mutex` inside ChainTTS. Document. |

No CRITICAL findings. HIGH (#9) and MED (#2, #3, #4) addressed inline in
scaffold below. Spec amended: ChainTTS holds a `sync.Mutex` (concern #10).

## 9. Cuts (Phase 1)

- No voice cloning (Kokoro supports — defer until use-case).
- No SSML / prosody control (Kokoro flat-tone Phase 1).
- No streaming chunked playback (defer; see concern #4).
- No multi-language / language auto-detect (en-US only Phase 1).
- No wiring into `notify/` (separate follow-up).
- No CLI flag exposure (`NewTTS()` decides; flag override Phase 2 only if
  operator hits a case where auto-selection picks wrong).

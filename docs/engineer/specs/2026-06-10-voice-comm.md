# Voice communication — Leah operator+AI voice loop

Status: spec (W11–W14 design lock)
Owner: trilamsr
Surface: `internal/voice/{listener,session,wake}/`, `cmd/leah/voice.go`

## 1. Goal

Voice unlocks one capability text cannot: **hands-busy operation**. The
operator at a soldering bench, driving, cooking, or holding an infant cannot
type, but can speak — and crucially, can speak in fragments while doing
something else with their eyes. The existing `leah listen` command (push-to-
talk, single utterance, sox+whisper-cli) proves the STT/TTS path works;
voice-comm replaces the push-to-talk button with a **continuous always-on
loop**: wake-word arms the session, the operator speaks, Leah replies via TTS,
the operator interrupts or follows up without re-arming. Concretely: the
operator says "Leah, what's on my calendar?" → Leah speaks the brief → mid-
sentence the operator says "actually skip the standup" → Leah cancels TTS
and acknowledges. That single round-trip is the value; everything else in
this spec exists to make it boring and auditable.

## 2. Mental model — barge-in, single-active turn

Three viable models exist. We pick one:

| Model | Description | Fit for Leah |
| --- | --- | --- |
| Turn-based PTT | Operator holds key; Leah replies; release for next | Already implemented as `leah listen`. Hands-busy fails. |
| Streaming half-duplex | Wake-word arms; one utterance; Leah replies; loop | Simpler, but no interruption of Leah's TTS. |
| **Barge-in full-duplex** | Mic always hot; wake-word OR mid-TTS speech triggers a new turn; TTS cancels on detected speech | The hands-busy story. Chosen. |

**Decision: barge-in with a single-active-turn invariant.** Only one
Reasoner call is in flight at a time. New operator speech during TTS cancels
the in-flight TTS and the in-flight Reasoner call (if any), then starts a
fresh turn. Per `README.md` § House rules priority UX > performance > long-term: barge-in
is the UX win; the implementation cost (streaming refactor of the Reasoner
pipeline) is the price.

**Reasoner remains synchronous.** We do NOT stream Reasoner output into TTS
token-by-token in MVP. Reasoner returns one string; TTS speaks it. Streaming
the LLM into TTS chunks defers to W15+. Justification: the existing Reasoner
interface (`internal/dispatcher/ask.go` §20) returns `(text, error)`;
preserving it means W11–W14 is a strict additive layer, not a pipeline
rewrite. The latency cost (first-audio waits for last-token) is real but
bounded — Reasoner replies are typically <2s for `KindAsk`, which fits the
budget below.

## 3. Surface

### 3.1 Go packages

```
internal/voice/listener/   STT: streaming audio frames → text segments
internal/voice/wake/       Wake-word detector (energy threshold first)
internal/voice/session/    State machine + barge-in arbitration
internal/voice/             (existing) TTS chain, push-to-talk Listen
```

```go
// internal/voice/listener
type Segment struct {
    Text  string
    Final bool   // true on end-of-utterance VAD trigger
    StartedAt, FinishedAt time.Time
}

type Listener interface {
    // Start opens the mic and emits Segments on the returned channel.
    // Closing the returned cancel func releases the mic. Segments stream
    // partial transcripts (Final=false) and one final (Final=true) per
    // utterance. The channel closes when ctx is cancelled.
    Start(ctx context.Context) (<-chan Segment, error)
}
```

```go
// internal/voice/wake
type Detector interface {
    // Detect blocks until a wake-word or energy-threshold event fires, or
    // ctx is cancelled. Returns nil on detection; ctx.Err() on cancel.
    Detect(ctx context.Context) error
}
```

```go
// internal/voice/session
type Session struct {
    Wake      wake.Detector
    Listen    listener.Listener
    Speak     voice.TTS
    Reason    Reasoner          // same interface as dispatcher.Reasoner
    Audit     *audit.Logger
    Attest    Attestor          // gates first turn
    IdleAfter time.Duration     // default 60s; back to wake-required
}

func (s *Session) Run(ctx context.Context) error
```

### 3.2 CLI

```
leah voice start   # enter the loop; blocks until Ctrl-C or idle-exit
leah voice stop    # signal a running daemon-mode instance to exit
leah voice status  # one-shot: print state, last turn, transcript-log toggle
```

For MVP only `leah voice start` is required. `stop` and `status` are W14
polish — `start`'s Ctrl-C is the supported exit.

### 3.3 STT backend — Whisper-local (chosen) vs OpenAI Realtime (deferred)

| Axis | Whisper-local (`whisper.cpp` streaming) | OpenAI Realtime |
| --- | --- | --- |
| Cost | $0 | ~$0.06/min |
| Network | None | Required |
| Latency to partial | ~300ms on M-series | ~150ms |
| Privacy | Audio never leaves device | Audio over WSS |
| Dependency | Already wired (`whisper-cli`) | New WSS client + auth |
| Failure modes | Model-not-found, CPU saturation | Net loss, auth, rate-limit |

**Chosen: Whisper-local via `whisper-stream` (the streaming binary from
`whisper.cpp`).** Rationale: `internal/voice/listen.go` already shells out to
`whisper-cli`; the streaming variant is the same install. Privacy default
matters more than 150ms — see §11 threat model. OpenAI Realtime stays on the
roadmap behind a `LEAH_VOICE_ALLOW_OPENAI_STT=1` opt-in mirroring the existing
TTS opt-in (`tts.go` §118).

### 3.4 Wake-word backend — energy threshold (chosen) vs Porcupine (deferred)

`whisper-stream` can run continuously, but transcribing every ambient utterance
wastes CPU and produces false positives ("Leah" in a podcast). Three options:

1. **Energy threshold + magic-phrase post-filter** (chosen): mic frames whose
   RMS exceeds a calibrated floor wake the STT; we then check the first 500ms
   of decoded text for "leah" / "hey leah" / "okay leah". One binary
   (`sox`), no extra model. False-positive cost: one wasted STT pass.
2. Porcupine: dedicated wake-word model, ~50ms detection, free for personal
   use but requires an API key + binary install.
3. Always-on STT: no wake-word at all — every utterance is reasoned about.
   Rejected — privacy nightmare, see §11.

Per UX > performance: the energy-threshold path is simpler to install and
debug. Porcupine defers to W15+.

## 4. Barge-in semantics

Single invariant: **mic is always hot; TTS yields to operator speech.**

When the operator speaks during TTS:

1. `listener.Start` is running for the entire session (one goroutine for
   wake AND barge-in detection — same audio stream).
2. The session's audio-frame consumer notices RMS-over-threshold while state
   is `speaking`.
3. Session cancels the TTS context. `voice.TTS` already accepts ctx; this is
   the existing path.
4. Audio-device lock (`tts.go` §59 `audioDeviceMu`) releases at TTS exit.
5. Session transitions `speaking → interrupted → listening` and waits for the
   barge-in utterance's `Final=true` segment.
6. Reasoner is called with the new utterance (the prior in-flight Reasoner
   call, if still running, is also cancelled via the same ctx).

The audio-device lock from PR #49 already serializes the single macOS device.
Barge-in cancellation MUST release the lock — verified by a test that asserts
a second TTS call after barge-in does not deadlock.

**False-positive cost:** a cough cancels TTS. Mitigation: the energy
threshold is paired with a 200ms sustain — single spikes don't trigger.
Acceptable: an operator who coughed can say "continue" and Reasoner will
re-issue. Adding speaker-identification ("is that the operator's voice or
the dog?") is out of scope (§12).

## 5. Hot-path latency budget

Target: **wake → first-TTS-audio in under 1.5s 95p.**

| Stage | Budget (95p) | Notes |
| --- | --- | --- |
| Mic frame → energy trigger | 50ms | sox callback latency |
| Energy trigger → STT partial | 300ms | whisper-stream warm-start |
| STT partial → final (VAD) | 400ms | end-of-utterance silence |
| Final → Reasoner.Ask invoked | 10ms | dispatch goroutine handoff |
| Reasoner.Ask → text returned | 600ms | depends on model; tracked, not gated |
| Text → TTS first audio byte | 140ms | Kokoro warm |
| **Total** | **1.5s** | |
| Barge-in detected → TTS halted | 200ms | A8: cancel-signal → Speak unwound; `leah_voice_barge_in_cancel_seconds` |

If `Reasoner.Ask` exceeds 600ms (likely with a 70B local model), we file
**tracking issue: streaming-reasoner-into-tts (W15+)** rather than relaxing
the spec target. Per README.md root-cause discipline: relaxing the budget
because a stage misses it is a symptom-fix.

Stages 2 + 3 require `whisper-stream`, not the current one-shot
`whisper-cli`. W11 owns the binary swap.

## 6. Audit + privacy

One audit row per turn. Schema (uses existing `audit.Entry`, `Kind`
namespace):

| Field | Value |
| --- | --- |
| `Kind` | `"voice_turn"` |
| `BlastRadius` | 0 for ask-class; inherits routed action's BR for ship/review |
| `Outcome` | `"success" \| "interrupted" \| "stt_error" \| "reasoner_error" \| "tts_error"` |
| `Detail` | `"<intent>: <truncated transcript>"` — transcript clipped to 200 runes (mirrors `cmd/leah/listen.go` §133) |

**Transcripts default OFF in `Detail`.** A two-mode toggle lives in
`$HOME/.leah-state/voice.json`:

```json
{ "log_transcripts": false }
```

When false, `Detail` records only `<intent>: [transcript suppressed]`. When
true, the truncated transcript appears verbatim. Audit rows themselves are
always written — the toggle gates content, not existence. Rationale: an
operator who wants their voice history private still needs the row for
"what did Leah do at 3:14pm" forensics.

**Audio bytes are NEVER persisted past the current turn.** The wav file
`whisper-stream` writes lives at `/tmp/leah-voice-<pid>.wav` and is unlinked
on session exit OR on next-turn-start (whichever comes first). No archive,
no replay. Crash leaves the file; an OS reboot clears `/tmp`.

## 7. Operator-attestation

**First voice turn per session requires attestation.** Identical pattern to
gmail/gcal (`docs/engineer/specs/2026-06-09-gmail-adapter.md` §62): an
`Attestor` interface gates the first Reasoner call. The prompt is spoken via
TTS then answered by voice:

```
Leah: "Voice session starting. Confirm out loud: 'yes Leah'."
Operator: "yes Leah"
```

The session matches the literal phrase "yes leah" (case-insensitive, trimmed)
on the FIRST `Final=true` segment after the prompt. Any other reply terminates
the session with `outcome=attestation_denied`. Subsequent turns within the
same session reuse the attestation — exiting via Ctrl-C or `IdleAfter`
revokes it.

**Per-action BR>0 escalation.** When a voice turn routes to a `ship` or
ops-class action (BR>0), the dispatcher's existing in-band attestation pool
(W10-4, PR #65) runs ON TOP of the session attestation. Session attestation
arms the loop; per-action attestation arms each destructive turn. Same
phrase-match mechanism: TTS asks "ship now? say 'yes ship'", operator says
"yes ship", action proceeds. A mismatch records
`outcome=attestation_denied` and the loop stays armed for the next turn.

Mic OS permission is a separate concern — handled by the system permission
prompt on first mic open. `leah voice start` returns an actionable error
when the mic open fails:

```
voice: microphone access denied — grant Terminal.app mic access in
System Settings → Privacy & Security → Microphone
```

No retry loop; the operator re-runs after granting.

## 8. Fallback modes

| Failure | Behavior |
| --- | --- |
| TTS chain empty (no kokoro / say / openai-allowed) | Print Reasoner output to stdout; session continues. Existing `ChainTTS` error `"chain has no backends"` is the trigger. |
| `whisper-stream` missing | `leah voice start` exits with install hint: `brew install whisper-cpp` (already the convention in `voice/listen.go` §41). |
| `sox` missing | Same — `brew install sox`. |
| Reasoner error | TTS speaks "I couldn't reason that through — try again." Audit row outcome `reasoner_error`. Session stays armed. |
| Wake-word "backend" failure | Not applicable — energy threshold is part of the listener; `leah voice start` always works as a manual arming. |
| Audio device contention | Existing `audioDeviceMu` already serializes; barge-in cancel releases. |

## 9. State machine

| From state | Event | To state | Side effect |
| --- | --- | --- | --- |
| `idle` | `start` | `armed_waiting_attest` | TTS speaks attestation prompt |
| `armed_waiting_attest` | `speech-final == "yes leah"` | `armed` | none |
| `armed_waiting_attest` | `speech-final != "yes leah"` | `terminated` | audit `attestation_denied` |
| `armed` | `energy-trigger` (no wake-phrase needed in same session) | `listening` | none |
| `armed` | `idle-timeout` (`IdleAfter` elapsed) | `requires_wake` | TTS speaks "going to sleep" |
| `requires_wake` | `wake-phrase` ("hey leah" / "leah") | `listening` | none |
| `requires_wake` | `idle-timeout` x2 | `terminated` | session exits |
| `listening` | `speech-final` | `thinking` | Reasoner.Ask invoked |
| `listening` | `silence-timeout` (5s of no speech) | `armed` | none |
| `thinking` | `reasoner-done` | `speaking` | TTS.Speak invoked |
| `thinking` | `reasoner-error` | `armed` | TTS speaks generic error |
| `thinking` | `energy-trigger` (barge-in pre-tts) | `listening` | cancel Reasoner ctx |
| `speaking` | `tts-done` | `armed` | audit `voice_turn` outcome=success |
| `speaking` | `energy-trigger` (barge-in) | `interrupted` | cancel TTS ctx |
| `speaking` | `tts-error` | `armed` | audit outcome=tts_error |
| `interrupted` | `speech-final` | `thinking` | audit prior turn outcome=interrupted |
| `interrupted` | `silence-timeout` | `armed` | barge-in was spurious (cough) |
| `*` | `ctx-cancelled` (Ctrl-C) | `terminated` | cleanup mic + audio lock |

Self-loops: continuous partial segments in `listening` do not transition.

## 10. Test plan

**All tests hermetic. No real audio in CI.** Per the `internal/voice/tts_test.go`
+ `Executor` pattern: dependencies inject via interfaces.

Doubles:

- `fakeListener` — a channel-backed `Listener` the test drives directly.
- `fakeWake` — `Detect` returns when the test calls `.Trigger()`.
- `fakeTTS` — records `Speak` calls; honors ctx-cancel.
- `fakeReasoner` — returns canned text after configurable delay; honors ctx.
- `fakeAttestor` — configurable accept/deny.

Required unit tests:

- `TestSession_HappyPath` — attest → utterance → reasoner → TTS → audit row written.
- `TestSession_AttestationDenied` — first non-"yes leah" reply terminates session.
- `TestSession_BargeInDuringTTS` — second utterance mid-TTS cancels first TTS ctx + starts new turn; both audit rows written.
- `TestSession_BargeInDuringReasoner` — utterance arrives before Reasoner returns; Reasoner ctx cancelled.
- `TestSession_IdleTimeout` — `IdleAfter` elapsed → `requires_wake` state.
- `TestSession_ReasonerError` — outcome=reasoner_error; session stays armed.
- `TestSession_TranscriptSuppression` — toggle off → Detail says "[transcript suppressed]"; toggle on → contains text.
- `TestSession_NoTTSBackends` — falls back to stdout; session continues.
- `TestSession_AudioDeviceReleaseAfterBargeIn` — second TTS does not deadlock.

Integration test (in `internal/voice/session/`):

- `TestSession_FullLoop_Doubled` — five turns including one barge-in, asserting state-machine trace matches expected sequence.

**No real-audio CI lane.** Recording fixtures is fragile (sample-rate drift,
ambient noise) and CI runners lack audio devices. Manual operator
verification is a release gate, tracked as a checklist in
`docs/engineer/runbooks/voice-comm.md` (deferred to W14).

## 11. Threat model

| Surface | Risk | Mitigation |
| --- | --- | --- |
| Always-listening mic | Ambient conversation transcribed and stored | Audio bytes deleted on next-turn-start; transcript logging defaults OFF; `leah voice stop` releases mic immediately |
| Wake-word false positive | Random utterance triggers a Reasoner call (cost + noise) | Energy threshold + 200ms sustain + first-500ms phrase post-filter; in `requires_wake` state the phrase MUST be "leah" — bare energy doesn't suffice |
| Transcript in audit log | Sensitive utterance ("my SSN is …") persisted | Default OFF; opt-in toggle in `voice.json`; 200-rune cap clips long transcripts; audit file mode 0600 |
| OAuth-secret-via-voice | Operator inadvertently dictates a token; Reasoner echoes it; TTS speaks it back over speaker | Secrets-in-utterance heuristic: any decoded segment matching `^(sk-|gh[ps]_|xox[bp]-)\w{20,}$` patterns triggers an immediate session pause with TTS "secret-shaped string detected — discarded". Audit row records `outcome=secret_discarded`, transcript replaced with `[REDACTED]` even when toggle is on. |
| Barge-in DoS (TV in background) | TV audio keeps cancelling Leah's TTS | Energy threshold is calibrated on the first 5s of session silence; if RMS floor is elevated (e.g. TV), threshold auto-raises. Manual override via `voice.json` `energy_floor_db`. |
| Spoofed-operator (second person in room) | Anyone in earshot can issue commands | Out of scope MVP — speaker-identification deferred (§12). The risk is bounded by per-action attestation for BR>0 actions (§7) — a stranger can ask Leah's calendar but cannot ship a PR. |
| Mic-permission revoked mid-session | Subsequent reads silently return empty | Listener emits a sentinel error; session terminates with `outcome=mic_revoked` rather than spinning. |

## 12. Out of scope (MVP)

- Voice cloning / custom voice synthesis
- Multi-speaker recognition / speaker identification
- Sentiment analysis on transcripts
- Language switching mid-session (English only)
- Prosody / SSML control
- Push-to-talk hardware (foot pedal, headset button)
- Reasoner-token streaming into TTS chunks (deferred to W15+ tracking issue)
- OpenAI Realtime STT (deferred behind opt-in env var)
- Porcupine wake-word (deferred until energy threshold proves insufficient)
- Multi-session (two `leah voice start` processes; mic contention undefined)

## 13. Grade rubric

**B:** W11 lands; `leah voice start` runs a single turn end-to-end with
doubles in test, real Kokoro+whisper-stream on operator machine. No barge-in.
Audit row schema present. Attestation gate present. `TestSession_HappyPath`
and `TestSession_AttestationDenied` pass.

**A:** W12 + W13 land; barge-in works in manual operator verification; full
state-machine implementation matches §9 table; 9 unit tests + 1 integration
test green; latency budget §5 measured on operator machine and recorded in
runbook.

**A+:** W14 lands; secret-shape redaction (§11) implemented and tested;
transcript-toggle round-trip verified; idle-timeout escalation to
`requires_wake` proven; runbook checklist signed off; tracking issue filed
for streaming-reasoner-into-TTS with measured baseline numbers.

```release-notes
[DOCS] voice-comm spec + W11–W14 delivery plan
```

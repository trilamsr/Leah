# Voice frontier upgrade — cascaded-pipeline → 2026-frontier

Status: spec (W109–W115 design lock)
Wave: 8 — S6 (per `docs/engineer/briefs/2026-06-10-wave-8-aiml-upgrade.md` §S6)
Surface: `internal/voice/{listener,session,wake,loop}/`, `internal/voice/tts.go`, `internal/reasoner/`

## 1. Goal + non-goals

### Goal
Cascaded STT → Reasoner → TTS (`2026-06-10-voice-comm.md` §5) burns 600–1500ms between final-segment and first-audio that a frontier path does not. V1 (#162) instrumented every stage — this spec turns those histograms into targets they gate. Concretely:

- Cut first-audio latency to ≤200ms by streaming Reasoner deltas into TTS at sentence boundaries.
- Replace energy-RMS wake detection with neural KWS (Porcupine) behind a flag; energy path stays as fallback.
- Offer single-stage S2S (OpenAI Realtime-2) as opt-in escape hatch.
- Cut barge-in DoS (cough/TV) ~80% via Voice Isolation + 30s rolling energy floor.
- Eliminate 30s TTS-chain wedge when Kokoro hangs.
- Stay reachable when network drops — local Ollama Reasoner tier behind cloud.

### Non-goals (v1)
- Full S2S as default — Realtime-2 stays opt-in for cost + privacy.
- Hardware form-factor changes — rejected in wave-8 P2.
- Multi-speaker diarization — Eagle is 1-of-N enrolled-operator detection only.
- Language switching mid-session — English only.
- Replacing voice-comm barge-in semantics — this spec is additive.

## 2. Streaming Reasoner→TTS sentence-boundary chunking (primary unlock)

**Wave-9 V10 gates this on V1 baseline being live in main for ≥3 voice turns.** V1 #162 merged 2026-06-10 11:14 PDT; gate reachable.

### 2.1 Surface

```go
type StreamingClient interface {
    Client  // existing Complete(ctx, system, user) (string, float64, error)
    Stream(ctx context.Context, system, user string) (<-chan Delta, error)
}

type Delta struct {
    Text      string         // text delta; empty for non-text events
    ToolUse   *ToolUseEvent  // non-nil when SDK reports a tool-use block
    Final     bool           // last delta in the message
    InputTok  int            // cumulative; only set on Final
    OutputTok int            // cumulative; only set on Final
    Err       error          // stream aborted; channel still drains
}
```

`Complete` callers (engine, brief, recommend) keep their non-streaming path. Only `internal/voice/session/` consumes `Stream`.

### 2.2 Sentence-boundary chunker

```go
// SentenceChunker emits a chunk when buf ends in [.!?]\s AND ≥MinChars
// speakable chars (ignoring leading ws). On Final the residual flushes.
type SentenceChunker struct {
    MinChars int  // default 12; below this "." is Mr./Dr. abbreviation candidate
    Buf      strings.Builder
}
func (c *SentenceChunker) Push(delta string) (chunk string, ok bool)
func (c *SentenceChunker) Flush() string
```

12-char minimum prevents `"OK."` flushing on three bytes (TTS spin-up cost dominates short utterances).

### 2.3 Tool-call vs text delta discrimination

Anthropic SDK interleaves `content_block_start` with `type=tool_use`:

- `Text != ""` → SentenceChunker.
- `ToolUse != nil` → bypass chunker; drain in-flight TTS chunk, then run tool-use handler (rare in voice today; reserved for W17+ skills).
- `Err != nil` → cancel reply ctx; Flush() goes to fallback "couldn't reason that through".

### 2.4 Session.Run integration

```go
deltaCh, err := s.StreamReason.Stream(replyCtx, system, user)
chunker := &SentenceChunker{MinChars: 12}
for d := range deltaCh {
    if d.Text != "" {
        if chunk, ok := chunker.Push(d.Text); ok {
            if err := s.Speak.Speak(replyCtx, chunk); err != nil { break }
        }
    }
    if d.Final {
        if tail := chunker.Flush(); tail != "" {
            _ = s.Speak.Speak(replyCtx, tail)
        }
    }
}
```

Per-chunk Speak serializes via existing `audioDeviceMu` (`tts.go` §59). Barge-in cancels `replyCtx` which propagates into both stream consumer and in-flight Speak.

### 2.5 Measurable target (V1-baselined)

- `leah_voice_stage_seconds{stage="reasoner_first_token"}` — NEW series this spec adds (deltas need first-byte timestamp; existing `reasoner_seconds` measures whole-response).
- `leah_voice_stage_seconds{stage="tts_first_audio"}` — already present.
- Target p95 first-audio: ≤300ms (reasoner_first_token) + ≤140ms (tts_first_audio) = **≤440ms**. Cascaded baseline locked in W109 PR body from first 50 production turns. Spec target = "≤50% of cascaded baseline".

## 3. Porcupine wake-word backend

Energy-RMS wake (`voice-comm.md` §3.4) is a 2015-era false-positive factory. Porcupine: 2026-vintage on-device neural KWS at ~50ms. Free for personal use; license pins to operator's AccessKey.

```go
type PorcupineDetector struct {
    AccessKey   string  // PICOVOICE_ACCESS_KEY env
    Keyword     string  // path to .ppn; default $HOME/.leah-state/wake/leah.ppn
    Sensitivity float64 // 0.0-1.0; default 0.5
}
func (d *PorcupineDetector) Detect(ctx context.Context) error
```

Selection: `LEAH_VOICE_WAKE=porcupine|energy` (default energy). Missing key under porcupine fail-fast:

```
voice: PICOVOICE_ACCESS_KEY not set — get a free personal key at
console.picovoice.ai, then re-run. LEAH_VOICE_WAKE=energy keeps legacy.
```

No silent fallback per voice-comm §8 convention.

Tests: `TestPorcupineDetector_NoAccessKey`, `TestPorcupineDetector_DetectsKeyword` (prerecorded .wav), `TestPorcupineDetector_HonorsCtxCancel`, `TestEnergyDetector_StillWorks` regression. Real binary gated `//go:build porcupine` + `make test-porcupine`.

## 4. OpenAI Realtime-2 opt-in path

Cascaded floor even with §2 streaming is ~250ms (STT VAD-final + handoff + first-token + first-audio). Realtime-2 collapses to 250–500ms end-to-end via audio→audio with no text intermediate. For bench work the gap is operationally identical; for phone-call work cascaded's "could you repeat that" recoverability beats Realtime-2's opaque failures. Opt-in.

```
LEAH_VOICE_BACKEND=cascaded   # default
LEAH_VOICE_BACKEND=realtime   # OpenAI Realtime-2 single-stage S2S
```

`Session.Path` field (PathCascaded | PathRealtime). `PathRealtime` swaps the turn engine — bypasses `Wake`, `listener.Listener`, `Reason`, `Speak`. WSS streams operator audio up, assistant audio down on one socket; the session loop only enforces attestation, barge-in arbitration, audit. The upstream-audio half (`internal/voice/listener/openai_realtime_decoder.go`) lands in #146; this spec adds downstream-audio (`internal/voice/realtime/wss.go`) + path arbiter.

### Cost vs latency (audit row Detail)

Realtime-2 pricing 2026-06: ~$0.06/min in + ~$0.24/min out = ~$0.30/min round-trip vs cascaded $0 (Kokoro+whisper local) or $0.06/min (Whisper-cloud STT). 10-turn evening session @ 5s/turn ≈ $0.25 vs $0.

`leah_voice_turn_cost_dollars{path}` histogram joins V1's series.

### Path target

`leah_voice_turn_seconds{path="realtime"}` p95 ≤500ms vs cascaded ≤1500ms (voice-comm §5). Missing the 500ms p95 over a rolling 50-turn window emits `voice_realtime_degraded` + auto-falls-back cascaded for the session. No silent perf regression.

## 5. Picovoice Eagle speaker-ID

Per-turn "yes Leah" attestation (voice-comm §7) friction is fine for shipping, tedious for the rightful operator. Eagle: N-shot enrollment from ~30s sample; subsequent turns voiceprint-match. When Eagle reports operator-match with confidence ≥0.85, the in-band phrase skips for BR=0 turns. BR>0 turns still require per-action attestation (voice-comm §7 unchanged).

```
leah voice enroll   # records 30s; writes $HOME/.leah-state/voice/eagle.profile
leah voice forget   # deletes the profile
```

```go
type EagleIdentifier struct {
    AccessKey   string  // PICOVOICE_ACCESS_KEY (same key as Porcupine)
    ProfilePath string  // default $HOME/.leah-state/voice/eagle.profile
    Threshold   float64 // default 0.85
}
func (e *EagleIdentifier) Identify(ctx context.Context, audio []byte) (operator bool, confidence float64, err error)
```

### Operator-trust: NO cloud egress

Eagle runs entirely on-device. Audit row includes `eagle_score`; profile is operator-readable; `--purge` removes it. Verifiability:

- Eagle Go binding's HTTP transport is wrapped in a `net.Dialer` pinned to `127.0.0.1:0` — any outbound dial fails. `TestEagle_NoNetworkEgress` runs with iptables deny-all, asserts Identify succeeds.
- Eagle ships an offline model file; SBOM-emit (S12 reproducible build) catches upstream changes.

Tests: `TestEagle_NotEnrolled_FallsBackToPhrase`, `TestEagle_OperatorMatch_SkipsPhrase`, `TestEagle_OperatorMismatch_StillRequiresPhrase`, `TestEagle_BR_GreaterThan_Zero_StillAttests`.

## 6. Voice Isolation mic mode (macOS)

Sonoma+ ships a system-level mic mode that strips background TV/typing/cough at the CoreAudio layer — strictly upstream of every Leah component. Apple claims ~80% bg-DoS cut; field-verify on operator machine.

```go
// internal/voice/macos/voiceisolation.go
func Available() bool                    // wraps defaults read com.apple.coreaudio + sysctl
func Prefer() (mode string, err error)   // no-op on non-Darwin / Apple Silicon < 2021
```

Called from `Session.Run` boot:

```go
if vi.Available() {
    mode, _ := vi.Prefer()
    audit.Write(ctx, "voice_mic_mode", "info", "set="+mode)
}
```

Tests: `TestVoiceIsolation_NoOpOnNonDarwin` (build-tag gated). Real CoreAudio integration test is operator-machine manual; runbook checklist.

## 7. Phoneme-distance wake-phrase fuzzy match

§3.4 first-500ms phrase filter today uses byte-equality lowercase compare. Decoder noise turns "Leah" into "leak", "leeah", "Leah,", "leer" — all rejected. G2P + Levenshtein on phoneme sequence catches all four. Source critique claimed "3x true-positive rate"; concrete number lands in W113 test fixture.

```go
// PhonemeMatch reports whether decoded is within editDistance phonemes of
// any keyword. G2P uses a static CMU dict subset (~50 phonemes reachable
// from wake keywords; full CMU dict is overkill).
func PhonemeMatch(decoded string, keywords []string, editDistance int) bool
```

Keywords: `"leah"`, `"hey leah"`, `"okay leah"`. Default distance: 1 phoneme.

Static `internal/voice/wake/cmu_subset.txt` (~40 entries hand-maintained; full CMU expansion W117+):

```
LEAH  L IY AH
HEY   HH EY
OKAY  OW K EY
LEAK  L IY K
LEER  L IY R
```

Table tests: `{"leah",true}`, `{"Leah,",true}`, `{"leak",true}` (1 phoneme delta), `{"leeah",true}`, `{"leer",true}`, `{"hey leah",true}`, `{"lima",false}` (distance 2), `{"library",false}`.

## 8. Rolling 30s energy-floor

voice-comm §11 calibrates `energy_floor_db` on first 5s of session silence. Operator who opens quiet then turns on a TV mid-session gets a barge-in storm: every laugh-track segment fires the threshold. 30s rolling window keeps floor current.

```go
type RollingFloor struct {
    Window   time.Duration  // default 30s
    Quantile float64        // default 0.20; floor = 20th-percentile RMS over window
    OffsetDB float64        // default +6dB; threshold = floor + offset
}
func (r *RollingFloor) Push(rms float64, t time.Time)
func (r *RollingFloor) Threshold() float64
```

Quantile (not mean) so a single loud spike doesn't pull the floor up. 0.20 = "quietest 20% of recent ambient" — robust to brief loud events.

Tests: `TestRollingFloor_QuietRoom`, `TestRollingFloor_TVTurnedOn` (floor migrates upward within Window), `TestRollingFloor_BriefSpikeIgnored`.

## 9. ChainTTS per-backend 2s timeout

`tts.go` §87 walks backends sequentially. A wedged Kokoro hangs ~30s (afplay waiting on Kokoro stdout). Fix: per-backend 2s ctx-deadline; failure walks to next backend immediately.

```go
func (c *ChainTTS) Speak(ctx context.Context, text string) error {
    if len(c.backends) == 0 { return errors.New("voice: chain has no backends") }
    var lastErr error
    for _, b := range c.backends {
        backendCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
        err := b.Speak(backendCtx, text)
        cancel()
        if err != nil { lastErr = err; continue }
        if c.OnSpeak != nil { c.OnSpeak(backendName(b)) }
        return nil
    }
    return fmt.Errorf("voice: all backends failed: %w", lastErr)
}
```

2s = §2 streaming chunks are ≤1 sentence each; 2s matches natural ceiling of one sentence on Kokoro warm. Configurable: `LEAH_VOICE_BACKEND_TIMEOUT_MS=2000`.

Tests: `TestChainTTS_BackendTimeout_WalksToNext` (A sleeps 5s, B instant, wall-clock ≤2.5s), `TestChainTTS_AllBackendsTimeout_ReturnsAllFailed`.

## 10. Local Ollama Reasoner fallback

Cascaded path hard-depends on `anthropic.com`. Flaky hotel wifi or `LEAH_LOCAL_ONLY=1` (S10) drops Leah to silence. Ollama running `llama3.1:8b` or `qwen2.5:7b` on-device closes the gap.

```go
type TieredClient struct {
    Cloud     Client  // AnthropicClient
    Local     Client  // OllamaClient
    Canned    Client  // returns "I can't reach the network right now."
    LocalOnly bool    // LEAH_LOCAL_ONLY=1
}
func (t *TieredClient) Complete(ctx context.Context, system, user string) (string, float64, error) {
    if !t.LocalOnly {
        if text, cost, err := t.Cloud.Complete(ctx, system, user); err == nil { return text, cost, nil }
    }
    if text, cost, err := t.Local.Complete(ctx, system, user); err == nil { return text, cost, nil }
    return t.Canned.Complete(ctx, system, user)
}
```

`Stream` uses the same ladder — Ollama `/api/chat` streams the same delta shape.

LocalOnly verification: `TestTieredClient_LocalOnly_NoCloudDial` wraps Cloud's transport with a `RoundTripper` that fails any request whose host isn't `127.0.0.1`/`localhost`. Backs the S10 "0 egress bytes" audit claim.

Tests: `TestTieredClient_CloudOK_UsesCloud`, `TestTieredClient_CloudFails_FallsBackLocal`, `TestTieredClient_BothFail_UsesCanned`, `TestTieredClient_LocalOnly_SkipsCloud`.

## 11. Wave plan W109–W115 (file-disjoint)

Each = single PR, single owning package(s), no shared roots. Fan-out cap-6 → W109+W110+W111+W112+W113+W114 dispatch parallel; W115 sequences after W109.

| Wave | Title | Owner pkg | ~LOC |
|------|-------|-----------|------|
| W109 | Streaming Reasoner.Client + SentenceChunker | `internal/reasoner/`, `internal/voice/session/` | 250–350 |
| W110 | Porcupine wake + LEAH_VOICE_WAKE flag | `internal/voice/wake/` | 120 |
| W111 | OpenAI Realtime-2 single-stage path | `internal/voice/realtime/`, `internal/voice/session/` | 300–400 |
| W112 | Eagle speaker-ID + `leah voice enroll` | `internal/voice/speakerid/`, `cmd/leah/voice.go` | 200 |
| W113 | Voice Isolation + phoneme-fuzzy + rolling floor + ChainTTS 2s timeout | `internal/voice/macos/`, `internal/voice/wake/`, `internal/voice/listener/`, `internal/voice/tts.go` | 200 |
| W114 | Tiered Reasoner (cloud → local → canned) + LOCAL_ONLY | `internal/reasoner/` | 150 |
| W115 | Wire §2 chunker into Session.Run on cascaded path | `internal/voice/session/` | 80 |

W115 sequences after W109 (consumes StreamingClient). W113 groups four ≤50-LOC pieces touching different sub-packages — single PR keeps diff comprehensible.

## 12. Test plan per wave (TDD)

Per CLAUDE.md: failing test first; failing output in PR body; then impl. Per-wave fixtures:

- **W109**: `TestSentenceChunker_BoundaryFlush`, `_MinCharsGuard`, `_AbbreviationNotASentence`, `TestSession_StreamingReply_HappyPath`, `_BargeInMidChunk`, `_StreamErrorFallsBackToCanned`.
- **W110**: §3.
- **W111**: `TestRealtimePath_{ConnectsAndAudits,BargeIn,DegradedFallsBackCascaded,AttestationGate}`.
- **W112**: §5.
- **W113**: §6 + §7 + §8 + §9.
- **W114**: §10.
- **W115**: `TestSession_StreamingPathLatency` — fake StreamingClient + fakeTTS, asserts `tts_first_audio_seconds` < 0.5s.

CI hermeticity: no real network, no real audio (voice-comm §10). Porcupine/Eagle/Realtime gate behind build tags + `make test-frontier` operator-machine lane.

## 13. Cost analysis

| Capability | Annual | Per-use |
|---|---|---|
| Porcupine wake | $0 personal | $0 |
| Eagle speaker-ID | $0 personal / $99/yr commercial | $0 |
| OpenAI Realtime-2 | $0 | ~$0.30/min round-trip |
| Ollama local Reasoner | $0 | $0 (CPU/RAM) |
| Voice Isolation | $0 macOS-native | $0 |
| Phoneme G2P | $0 static file | $0 |

Realtime monthly budget = per-process counter `leah_voice_realtime_dollars` joining S2 cost circuit breaker. 80% cap → next Realtime turn auto-degrades cascaded with audit row `voice_realtime_budget_cap`.

## 14. Operator override

```
LEAH_VOICE_FRONTIER=0                  # kill-switch; falls back full to cascaded
LEAH_VOICE_BACKEND=cascaded|realtime   # §4 (default cascaded)
LEAH_VOICE_WAKE=energy|porcupine       # §3 (default energy)
LEAH_VOICE_ALLOW_OPENAI=1              # cascaded TTS opt-in (unchanged)
LEAH_LOCAL_ONLY=1                      # §10 (S10 trust moat)
LEAH_VOICE_BACKEND_TIMEOUT_MS=2000     # §9 per-backend timeout
PICOVOICE_ACCESS_KEY=...               # required for §3 + §5
```

Each new path also gates on its individual flag — `LEAH_VOICE_FRONTIER=0` flips them all off in one shot for the operator who suspects a regression.

## 15. Grade rubric

**B:** W109 + W113's ChainTTS-timeout land; SentenceChunker unit tests green; cascaded streaming Reasoner→TTS measurably faster than V1 baseline on operator machine (number in PR body).

**A:** W109 + W110 + W113 + W114 land; Porcupine when flag set; Ollama fallback proven network-down; rolling floor + phoneme match measurably reduce false-positive wake rate vs V1 baseline.

**A+:** W109–W115 land; Realtime-2 path runs end-to-end in operator demo; Eagle skips phrase for enrolled operator; `LEAH_LOCAL_ONLY=1` audit verifies 0 cloud egress; `voice_realtime_budget_cap` triggers cleanly.

```release-notes
[DOCS] voice frontier spec — streaming Reasoner→TTS, Porcupine, Realtime-2, Eagle, Voice Isolation, phoneme-fuzzy wake, rolling floor, 2s ChainTTS timeout, Ollama fallback
```

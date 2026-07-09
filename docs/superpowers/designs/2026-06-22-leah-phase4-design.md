# Leah Phase 4 — Multi-modal + multi-agent layer

**Version:** v1.0 (2026-06-22). Authoritative for Phase 4 dispatch.
**Predecessor:** `docs/superpowers/designs/2026-06-21-leah-macos-native-ui-design.md` v3.2.2 (§19 sequenced Phases 1–3 + named Phase 4 as the multi-modal + multi-agent layer; this doc is the full Phase 4 build).
**Thin sketches superseded:** `docs/engineer/specs/2026-06-10-voice-frontier.md` · `docs/engineer/specs/2026-06-10-learn-recommend-apply.md` · `docs/engineer/specs/2026-06-10-mcp-a2a-publish.md`. Those files remain for archival rationale; this spec is the canonical Phase 4 contract.
**Phase boundary:** Phase 4 PRs do not start until Phase 3 (v1.0 public launch per §19) ships and runs on the operator's machine for ≥ 7 days. Phase 4 ends with v1.1 public launch.

> **Spec parity:** this file is checked by `scripts/check-spec-parity.sh`. Forbidden phrases (renamed terms, killed cosmetics) are not used in normative body; legacy citations live only in the historical-anchor table at the end.

---

## 0. Executive summary

Phase 4 takes Leah from a single-Mac, hotkey-summoned, single-process answer engine to a multi-modal (voice + vision), multi-device (peer Mac sync), multi-agent (inbound MCP + A2A peering) operator companion. It also adds the supervisor + plugin + privacy-budget substrate that v1.1 third-party adapters require.

Nine deliverables. Five build waves. Sixteen implementer tasks previewed.

| # | Deliverable | Wave | Ship gate |
|---|---|---|---|
| 1 | Voice frontier runtime | W1 | Full-duplex voice convo with barge-in, no GUI required |
| 2 | Multi-device sync | W3 | Two Macs on same iCloud share memory + HUD state CRDT-merge |
| 3 | Learn-recommend-apply pass-2 | W2 | Ranked recommendation queue surfaced in HUD with A/B + decay |
| 4 | Camera + vision | W1 | Live frame + screenshot routed to Sonnet vision with consent gate |
| 5 | Multi-agent A2A | W4 | Inbound MCP server live + peer-Leah handshake works |
| 6 | Continuous attestation | W2 | About pane shows live attestation; unsigned plugin load blocks |
| 7 | Plugin SDK | W4 | One first-party plugin shipped via SDK; doc-site reproducible |
| 8 | Privacy budget runtime | W2 | Per-feature meter visible; degradation path observable in HUD |
| 9 | Watchdog supervisor | W5 | Kill-9 of HUD/voice/camera process recovers in < 2 s |

Total: ~10 weeks solo (parallel-cap of 6 per `CLAUDE.md` Dispatch parallelism). Each wave = independently mergeable; v1.1 ship after W5.

### 0.1 What Phase 4 is not

- Not a redesign of the v1 visual identity (Phase 1–3 lock; §3 of predecessor stays normative).
- Not iOS/iPad — that is v2 horizon per §18 of predecessor.
- Not a new LLM backbone — Anthropic Sonnet 4.6 + Haiku 4.5 + Opus 4.8 remains canonical (predecessor §17.14).
- Not a new vector store — `sqlite-vec` via `modernc.org/sqlite/vec` stays single-file (predecessor §17.10 + §17.16).
- Not a paid tier — BYOK Anthropic remains the cost model (predecessor §17.18, decision #128). Plugin SDK is free; sandbox runs on operator's machine.

### 0.2 Cross-cutting invariants (binding)

1. **Daemon owns LLM + key** (predecessor §17.14). HUD/voice/camera/plugin processes never see the Anthropic API key. Every Phase 4 surface routes through the daemon proxy.
2. **Single SQLite file** (predecessor §17.10). All Phase 4 tables live in `~/Library/Application Support/Leah/leah.db`. Multi-device sync writes CRDT deltas into existing tables, never a parallel store.
3. **Default-OFF for ambient capture** — voice continuous mode, camera live mode, multi-device sync all default OFF in the wizard. Operator opts in per-feature in Settings.
4. **Privacy budget is enforced in the daemon**, not by callers (§8). A subsystem cannot exceed its per-feature budget regardless of how it was invoked.
5. **Attestation is a load-bearing gate** (§6) — plugin load + auto-update install both block on attestation verdict.
6. **No new AI signatures** anywhere — per `CLAUDE.md` identity/output rules.

---

## 1. Deliverable 1 — Voice frontier runtime

### 1.1 Goal

Make Leah respond to voice while you keep both hands on the keyboard — full-duplex, low-latency, with barge-in and a voice-only mode for the operator who wants to walk away from the screen. Predecessor §6.7 + §17.17 + §2.7 give the canon; Phase 4 builds the runtime.

### 1.2 Capability matrix

| Capability | Phase 3 baseline | Phase 4 addition |
|---|---|---|
| Wake-word | Bundled `wake-leah.mlmodel`, opt-in, push-to-talk modifier | Continuous mode + per-app suppression learning loop |
| STT | Single-shot dictation via `SFSpeechRecognizer` | Streaming Whisper-large-v3 ONNX local primary; OpenAI Whisper API fallback |
| TTS | ElevenLabs Flash v2.5 + Apple Ava Premium | + Barge-in interrupt + side-channel duck of system audio |
| Conversation | Text-first, voice-shaped | Voice-only mode (no panel summoned); HUD shows live transcript |
| VAD | Single-utterance gate | Continuous VAD with adaptive noise floor |

### 1.3 Interfaces

#### 1.3.1 Daemon Go interfaces

```go
// internal/voice/stt.go
type STT interface {
    // Stream transcribes a 16 kHz mono PCM stream.
    // Each partial emits with isFinal=false; final emits once with isFinal=true.
    Stream(ctx context.Context, audio <-chan AudioFrame) (<-chan Partial, error)
    Info() ProviderInfo // {name, isLocal, modelID, ramMB}
}

// internal/voice/duplex.go
type DuplexSession interface {
    // Start opens mic, runs STT + reasoner + TTS concurrently with barge-in.
    Start(ctx context.Context, opts DuplexOpts) (<-chan DuplexEvent, error)
    Interrupt() // stops current TTS within 80 ms (barge-in)
    End()       // graceful close + commit transcript to memory
}

type DuplexOpts struct {
    VoiceOnly      bool          // true = no HUD summoned
    SuppressApps   []BundleID    // skip wake-word while these are frontmost
    NoiseFloorDBFS float64       // adaptive; -55 default
    MaxTurnSeconds time.Duration // 90-second soft cap; warn at 75
}

type DuplexEvent struct {
    Kind      DuplexEventKind // WakeDetected | PartialIn | FinalIn | TTSStart | TTSChunk | BargeIn | TTSEnd | Error
    Text      string
    LatencyMS int
    Err       error
}
```

#### 1.3.2 IPC frame kinds (additions to predecessor §17.2)

| Frame kind | Direction | Payload | Notes |
|---|---|---|---|
| `voice.start` | HUD → daemon | `{voiceOnly bool, source: wake | hotkey | menubar}` | Starts a `DuplexSession` |
| `voice.partial` | daemon → HUD | `{text, isFinal}` | Renders live transcript in panel |
| `voice.tts.chunk` | daemon → HUD | `{audioBase64, seqNo}` | PCM16LE 24 kHz; HUD plays via `AVAudioEngine` |
| `voice.barge` | HUD → daemon | `{}` | Mic VAD detected speech-during-TTS |
| `voice.end` | both | `{reason: user | timeout | error, commitID?}` | `commitID` references memory row |

#### 1.3.3 Swift protocol (HUD process)

```swift
// LeahHUD/Voice/VoiceCoordinator.swift
protocol VoiceCoordinator {
    func startSession(voiceOnly: Bool) async throws
    func endSession() async
    var transcriptStream: AsyncStream<TranscriptUpdate> { get }
    var levelStream: AsyncStream<Float> { get } // 0…1 for waveform
}
```

### 1.4 Data model

New tables in `leah.db` (migration `2026-06-22-001-voice.sql`):

```sql
CREATE TABLE voice_session (
    id            INTEGER PRIMARY KEY,
    started_at    INTEGER NOT NULL,
    ended_at      INTEGER,
    voice_only    INTEGER NOT NULL DEFAULT 0,
    source        TEXT NOT NULL CHECK(source IN ('wake','hotkey','menubar','ptt')),
    end_reason    TEXT CHECK(end_reason IN ('user','timeout','error','barge_exhausted')),
    stt_provider  TEXT NOT NULL, -- 'whisper-large-v3-onnx' | 'openai-whisper-api'
    tts_provider  TEXT NOT NULL, -- 'eleven-flash-25' | 'apple-ava-premium'
    ram_peak_mb   INTEGER,
    bytes_uploaded INTEGER NOT NULL DEFAULT 0  -- 0 when fully local
);

CREATE TABLE voice_turn (
    id            INTEGER PRIMARY KEY,
    session_id    INTEGER NOT NULL REFERENCES voice_session(id) ON DELETE CASCADE,
    ord           INTEGER NOT NULL,
    role          TEXT NOT NULL CHECK(role IN ('user','assistant')),
    text          TEXT NOT NULL,
    stt_ms        INTEGER, -- null on assistant turns
    ttfb_ms       INTEGER, -- null on user turns; first-byte of TTS
    tts_ms        INTEGER, -- total TTS playback duration
    barge_in      INTEGER NOT NULL DEFAULT 0,
    UNIQUE(session_id, ord)
);

CREATE TABLE voice_suppression (
    bundle_id     TEXT PRIMARY KEY,
    learned       INTEGER NOT NULL DEFAULT 0, -- 1 = auto-learned vs operator-set
    last_seen_at  INTEGER NOT NULL,
    confidence    REAL NOT NULL DEFAULT 0.0
);
```

Migration path: append-only; Phase 3 rows untouched. `voice_session.bytes_uploaded` exists for privacy budget (§8).

### 1.5 Security model

- **Mic permission** — `NSMicrophoneUsageDescription` already requested in wizard step 4 (predecessor §8.4). Phase 4 continuous mode requires a second explicit toggle in Settings → Voice → "Continuous listening" with copy: "Leah will listen while running in the background. Wake-word detection stays on-device."
- **Whisper-large-v3 ONNX is the default STT** — audio never leaves the Mac in this path. OpenAI Whisper API fallback is opt-in (Settings → Voice → "Cloud transcription fallback when on AC + offline-quality drops") and writes to `voice_session.bytes_uploaded` for privacy budget.
- **TTS routing** — the existing privacy classifier (predecessor §17.17) gates calendar/email/finance/memory blockword strings to Apple Ava. Phase 4 extends the classifier with a `voice_only_mode` flag that forces local TTS regardless of content class when no screen is visible (assume strangers may be in earshot).
- **Wake-word model integrity** — `wake-leah.mlmodel` is signed; daemon refuses to load on hash mismatch.
- **Threat model** — adversary with mic access to a compromised app cannot exfiltrate audio: STT runs in daemon process, audio buffers never reach HUD/plugin processes.

### 1.6 Failure modes

| Failure | Detection | Degraded behavior |
|---|---|---|
| ONNX runtime crash | Subprocess exit code | Restart via watchdog (§9); during gap, fall back to `SFSpeechRecognizer` single-shot |
| Whisper local OOM | `os.PSI` memory pressure → 100 ms | Switch to OpenAI Whisper API for this session if cloud fallback enabled; else end session with "Listening is too heavy right now — try again in a moment" |
| TTS network slow (TTFB > 600 ms) | Per-chunk timer | Local Apple Ava continues; reasoner unaffected |
| Mic device hot-unplug (AirPods, USB) | `AVAudioSessionRouteChange` | Re-route to default input; if none, end session with audible tone + panel message |
| Wake-word false trigger flood (≥ 3 in 60 s without speech-follow) | Pattern detector | Suspend wake-word for 10 min; surface notification widget "Suppressed listening — too many false wakes" |
| Continuous mode held > 4 h | Soft timer | Auto-end with operator-visible nudge; require explicit re-arm |

### 1.7 Performance budget

| Surface | RAM target | CPU target | Latency target |
|---|---|---|---|
| Whisper-large-v3 ONNX cold load | 850 MB | 20% × 800 ms | First partial ≤ 350 ms post-VAD-stop |
| Whisper streaming, idle | 850 MB resident | < 3% steady | n/a |
| Wake-word continuous | 35 MB | < 1.5% steady (CoreML ANE) | Wake → ack tone ≤ 120 ms |
| TTS Flash v2.5 stream | 8 MB | < 1% | TTFB ≤ 150 ms |
| Apple Ava synth | 25 MB | 4% × duration | TTFB ≤ 250 ms |
| Full duplex turn (operator stop → assistant first audio) | combined | combined | ≤ 1.1 s p95 |
| Barge-in | n/a | n/a | TTS halt ≤ 80 ms |

### 1.8 UI surfaces

- **HUD focus panel** — live transcript renders inline; assistant TTS chunks animate the gold-edge waveform. Existing panel; no new chrome.
- **HUD ambient** — when `voiceOnly=true`, ambient HUD shows a pulse glyph + truncated current-utterance text. No panel summoned.
- **Menubar item** — new state `voice.session` shows a small dot during active duplex; click → end session.
- **Settings → Voice** — adds "Continuous listening" + "Cloud transcription fallback" + per-app suppression list (operator-editable + auto-learned rows distinguished by `voice_suppression.learned`).
- **Wizard step 4** — copy updated to mention voice-only mode (informational; opt-in toggle stays in Settings, NOT wizard, to keep first-launch shape unchanged).

### 1.9 What ships in v1.1 vs deferred

**Ships:**
- Whisper-large-v3 ONNX local + OpenAI Whisper API fallback (opt-in).
- Continuous mode (opt-in) with 4 h soft cap.
- Barge-in.
- Voice-only mode (no HUD).
- Auto-learned per-app suppression.

**Deferred to v1.2:**
- Multi-speaker diarization (single-operator scope this round).
- Localization of wake-word (en-US only).
- Custom hotword training UI (model file replacement still requires app reinstall).
- Streaming TTS over Bluetooth-LE direct to AirPods bypass macOS audio graph (latency win is small; engineering cost is large).

---

## 2. Deliverable 2 — Multi-device sync

### 2.1 Goal

Operator with two Macs (desktop + laptop, or work + home) sees the same memory, the same HUD state, the same pinned widgets — without an account, without a server, and without a sync conflict that loses an answer. Predecessor §17.7 marks iCloud sync "out of scope v1"; Phase 4 lifts that limit using a peer-to-peer design that keeps the no-server invariant.

### 2.2 Topology

```
[Mac A daemon] <—Bonjour discovery—> [Mac B daemon]
       |                                    |
       |  CRDT delta stream (mTLS over TCP) |
       |  Shared keychain item via iCloud   |
       \— iCloud Keychain (API keys only) —/
```

- Bonjour service: `_leah-sync._tcp` on port range 51820–51829 (first-available bind).
- Trust root: shared symmetric secret (Curve25519 key pair) stored in iCloud Keychain with `kSecAttrSynchronizable = true`. Pairing handshake mints + writes the key on the first device; second device reads it when the operator signs in.
- Transport: mTLS over TCP; mutual cert pinning against the shared key fingerprint.
- API keys (Anthropic, Voyage, ElevenLabs, OpenAI Whisper) sync via the existing Keychain rows being marked `kSecAttrSynchronizable = true` when the operator opts in (Settings → Sync → "Share API keys via iCloud Keychain"). No keys ever transit the peer channel.

### 2.3 CRDT model

Two replicated data classes:

1. **Last-writer-wins register** — Settings rows, pin set, HUD geometry. Vector-clock per `(device_id, key)`; conflict resolves on `(timestamp, device_id)` lexicographic order.
2. **Add-only log + tombstones** — memory rows, conversation history, widget pin events. Each row gets a `lww` triple `(node_uuid, lamport, op)` and a `deleted_at` tombstone column. Sync streams replays the log; tombstones are honored idempotently. Garbage collection prunes tombstones older than 90 days after both peers have ack'd consumption.

### 2.4 Interfaces

#### 2.4.1 Go daemon

```go
// internal/sync/peer.go
type Peer interface {
    ID() DeviceID            // stable UUIDv7 per Mac, generated at first launch
    Endpoint() netip.AddrPort
    LastSeenAt() time.Time
    Status() PeerStatus      // Online | Idle | Paused | Unreachable
}

// internal/sync/coord.go
type SyncCoordinator interface {
    Pair(ctx context.Context, otp string) (Peer, error)            // 6-digit OTP shown on existing peer
    Unpair(ctx context.Context, p Peer) error
    Pause(ctx context.Context, p Peer) error
    Resume(ctx context.Context, p Peer) error
    Snapshot(ctx context.Context) (SyncSnapshot, error)            // diagnostics
    Subscribe() <-chan SyncEvent
}

type SyncEvent struct {
    Kind    SyncEventKind   // Discovered | Paired | DeltaApplied | Conflict | Disconnected
    Peer    Peer
    Stats   DeltaStats
}
```

#### 2.4.2 IPC frame kinds

| Frame kind | Direction | Payload |
|---|---|---|
| `sync.peer.list` | HUD ↔ daemon | `[{id, name, status, lastSeenAt}]` |
| `sync.pair.start` | HUD → daemon | `{otp}` |
| `sync.pair.ack` | daemon → HUD | `{peerID, name}` |
| `sync.toast` | daemon → HUD | `{kind: "applied" | "paused" | "conflict", peerID, summary}` |

#### 2.4.3 Bonjour service record

```
type: _leah-sync._tcp
TXT:  v=1; node=<uuidv7>; name=<sanitized hostname>; cap=<bitfield>
```

`cap` bitfield enumerates supported CRDT classes; older peers ignore unknown bits.

### 2.5 Data model

```sql
CREATE TABLE sync_peer (
    id            TEXT PRIMARY KEY,           -- uuidv7
    name          TEXT NOT NULL,
    paired_at     INTEGER NOT NULL,
    paused        INTEGER NOT NULL DEFAULT 0,
    last_seen_at  INTEGER,
    fingerprint   BLOB NOT NULL               -- Curve25519 pubkey
);

CREATE TABLE sync_clock (
    table_name    TEXT NOT NULL,
    row_id        INTEGER NOT NULL,
    node_uuid     TEXT NOT NULL,
    lamport       INTEGER NOT NULL,
    PRIMARY KEY(table_name, row_id, node_uuid)
);

CREATE TABLE sync_tombstone (
    table_name    TEXT NOT NULL,
    row_id        INTEGER NOT NULL,
    deleted_at    INTEGER NOT NULL,
    deleted_by    TEXT NOT NULL,              -- node_uuid
    PRIMARY KEY(table_name, row_id)
);

CREATE TABLE sync_outbox (
    id            INTEGER PRIMARY KEY,
    peer_id       TEXT NOT NULL REFERENCES sync_peer(id) ON DELETE CASCADE,
    payload       BLOB NOT NULL,              -- gzipped CRDT delta batch
    enqueued_at   INTEGER NOT NULL,
    sent_at       INTEGER
);
```

Phase 3 tables that participate in sync (memory, conversation, pins, settings) gain a `node_uuid TEXT NOT NULL DEFAULT '<self>'` column via additive migration; existing rows backfill to the local node.

### 2.6 Security model

- **No server, no account.** Trust originates in iCloud Keychain shared secret + operator-visible OTP at pairing.
- **mTLS pinned to the shared secret fingerprint** — MITM on the local network cannot impersonate a peer even if Bonjour records are spoofed.
- **No raw API keys on the wire.** Keys ride iCloud Keychain only; the peer channel carries CRDT deltas.
- **Pause as kill-switch** — `Pause` halts inbound + outbound replication for a peer immediately; resume re-syncs from last lamport.
- **Threat model boundaries** — trusts iCloud Keychain (Apple platform trust). Does not defend against a compromise of one paired Mac; an attacker with root on either machine can read all memory rows. Mitigation = "Unpair" wipes the peer + nukes the shared secret.

### 2.7 Failure modes

| Failure | Detection | Degraded behavior |
|---|---|---|
| Peer offline | Heartbeat miss × 3 (15 s × 3) | Mark `Unreachable`; queue deltas in `sync_outbox`; resume on reconnect |
| Clock skew > 5 min between peers | NTP sample at pair-time + delta header | LWW resolves to declared lamport, not wall clock; warn in Settings |
| iCloud Keychain not signed in | Pairing OTP step fails | Settings → Sync displays "Sign into iCloud and enable Keychain" with deep-link to System Settings |
| Conflict on LWW register | Vector clock divergence detected | Apply newer lamport; emit `sync.toast` with revert affordance (24 h undo) |
| Outbox > 50 MB | Disk watcher | Compress + truncate oldest deltas already ack'd; warn if still > 50 MB after compression |
| Schema version mismatch | Capability handshake | Suspend sync with that peer; surface "Update both Macs to the same Leah version" in Settings |

### 2.8 Performance budget

| Operation | Target |
|---|---|
| Bonjour discovery (LAN) | Peer visible ≤ 3 s after both daemons live |
| Pair handshake | ≤ 1.5 s end-to-end (Curve25519 + mTLS up) |
| Delta apply (1 row) | ≤ 8 ms p95 |
| Delta apply (1 000-row catch-up) | ≤ 800 ms |
| Steady-state RAM overhead | ≤ 25 MB |
| Steady-state network | ≤ 4 KB / minute when idle (heartbeat only) |
| HUD-mirror keystroke (typed on Mac A, visible on Mac B HUD) | NOT a v1.1 surface — see §2.9 |

### 2.9 UI surfaces

- **Settings → Sync (new pane)** — peer list, pair button, OTP display, per-peer pause/unpair, "Share API keys via iCloud Keychain" toggle, last-sync timestamp, conflict ledger.
- **Menubar item** — adds a tiny dot indicator when ≥ 1 peer online.
- **Notification widget** — `sync.toast` events surface as the standard widget toast (predecessor §10.1 — "notification" widget). Coalesced under the existing 2-cap.
- **HUD ambient** — no first-class surface; sync status only visible in menubar dot + Settings.
- **Wizard** — NOT changed. Pairing happens post-wizard from Settings; first-launch shape stays fixed.

### 2.10 What ships in v1.1 vs deferred

**Ships:**
- Bonjour discovery + OTP pairing for ≤ 2 peer Macs.
- iCloud Keychain API-key share (opt-in).
- CRDT sync of: memory rows, conversation history, pin set, settings registers, widget catalog state.
- Per-peer pause + unpair.
- Conflict toast + 24 h undo.

**Deferred to v1.2+:**
- **Live HUD mirroring** (typed-on-A-visible-on-B) — interesting but tangential to the core "same brain on every Mac" thesis; ship after operator validates 2-Mac flow.
- **iPhone/iPad peer** — v2 horizon per predecessor §18.
- **> 2 peer fan-out** — code scales but OTP UX is two-Mac shaped; revisit when operator owns 3+ Macs.
- **End-to-end encrypted sync via Apple Identity Service relay when both peers are offline-LAN-different** — requires APNs relay infra. v1.1 stays LAN-only; cross-network sync waits.

---

## 3. Deliverable 3 — Learn-recommend-apply pass-2

### 3.1 Goal

Phase 1 ships the loop sketch (predecessor decision-log + `docs/engineer/specs/2026-06-10-learn-recommend-apply.md`); Phase 4 pass-2 makes it ranked, observable, and degradable. The goal is not "more recommendations" — it is "fewer recommendations, better timed, with a hard floor on disruption."

### 3.2 Recommendation lifecycle

```
observation —> candidate —> ranked —> surfaced —> outcome
                  |           |          |           |
                  |           |          |           +— accepted | dismissed | ignored | applied
                  |           |          +— HUD notification widget OR voice nudge OR silent
                  |           +— bandit rank (predecessor §17.x bandit-recommender spec)
                  +— hard-stop check vs. anti-recommend list
```

### 3.3 Interfaces

```go
// internal/learn/recommend.go
type Recommender interface {
    Observe(ctx context.Context, ev Observation) error
    NextBatch(ctx context.Context, surface Surface, maxN int) ([]Recommendation, error)
    Record(ctx context.Context, id RecommendationID, outcome Outcome) error
    AntiAdd(ctx context.Context, kind RecommendKind, reason string) error
    AntiList(ctx context.Context) ([]AntiRule, error)
}

type Recommendation struct {
    ID         RecommendationID
    Kind       RecommendKind        // e.g. "pin-widget", "voice-on", "integration-connect", "memory-purge", "wake-on"
    Score      float64              // bandit posterior mean
    Confidence float64              // [0,1]; below floor → silent surface
    Body       string               // operator-visible copy
    Action     ActionRef            // deep link or IPC action
    Decay      DecaySchedule
    SurfacedAt time.Time
    ExpiresAt  time.Time
}

type Outcome struct {
    Kind      OutcomeKind   // Accepted | Dismissed | Ignored | Applied | ABBaseline | ABTreatment
    LatencyMS int
    Note      string
}
```

### 3.4 Ranking

- **Bandit base** — predecessor `2026-06-10-bandit-recommender.md` Thompson-sampling kernel kept. Phase 4 adds context features: `time_of_day`, `day_of_week`, `recent_app_focus`, `active_voice_session`, `pinned_widget_set_hash`.
- **Floor** — recommendations with `confidence < 0.35` are silent (kept in queue but not surfaced).
- **Pacing** — ≤ 3 surfaced recommendations per rolling 24 h, ≤ 1 surfaced per hour. Voice nudges count double against pacing.
- **Position priors** — same-class re-suggestion after Dismissed gets a × 0.5 score penalty for 7 days; after Ignored × 0.7 for 3 days; after Accepted+Applied gets × 0.0 (capped) for 30 days then full reset.

### 3.5 Decay schedules

| Kind | Half-life | Hard expire |
|---|---|---|
| Integration-connect (Slack/Linear/etc) | 14 d | 60 d |
| Pin-widget | 7 d | 21 d |
| Voice-on | 30 d | 180 d |
| Wake-on | 90 d | never (operator must explicitly enable per decision #2) |
| Memory-purge | 1 d | 7 d |
| Plugin-install | 30 d | 90 d |
| Multi-device-pair | 30 d | 90 d |

Decay multiplies score each tick; ranked queue is `score × exp(-age/halfLife)`.

### 3.6 Anti-recommend hard-stop

Stored in `anti_recommend` table; checked before any candidate enters ranking. Sources:

- Operator-added (Settings → Recommendations → "Never suggest…")
- Auto-added on 3 consecutive Dismissed of the same kind within 30 days.
- Spec-pinned: `wake-word-on` cannot be auto-recommended (per decision #2); the operator must opt in from Settings.

### 3.7 A/B infrastructure

- **Pair-arm flag** — recommendation kinds with two phrasing variants enter an experiment; recommender flips a coin per impression, records outcome arm in `Outcome.Kind`.
- **Window** — minimum 50 impressions per arm before a winner is locked; ties stay 50/50.
- **No external service** — experiment state in `learn_experiment` table; nothing leaves the Mac.

### 3.8 Data model

```sql
CREATE TABLE learn_observation (
    id          INTEGER PRIMARY KEY,
    at          INTEGER NOT NULL,
    kind        TEXT NOT NULL,
    payload     BLOB,
    ctx_hash    BLOB        -- 64-bit context feature hash
);

CREATE TABLE learn_recommendation (
    id          INTEGER PRIMARY KEY,
    kind        TEXT NOT NULL,
    body        TEXT NOT NULL,
    action_ref  TEXT NOT NULL,
    score       REAL NOT NULL,
    confidence  REAL NOT NULL,
    decay_id    INTEGER NOT NULL REFERENCES learn_decay(id),
    surfaced_at INTEGER,
    expires_at  INTEGER NOT NULL,
    state       TEXT NOT NULL  -- queued | surfaced | accepted | dismissed | ignored | applied | expired
);

CREATE TABLE learn_decay (
    id            INTEGER PRIMARY KEY,
    kind          TEXT NOT NULL,
    half_life_s   INTEGER NOT NULL,
    hard_expire_s INTEGER NOT NULL
);

CREATE TABLE learn_experiment (
    id          INTEGER PRIMARY KEY,
    kind        TEXT NOT NULL,
    arm_a       TEXT NOT NULL,
    arm_b       TEXT NOT NULL,
    impressions_a INTEGER NOT NULL DEFAULT 0,
    impressions_b INTEGER NOT NULL DEFAULT 0,
    wins_a      INTEGER NOT NULL DEFAULT 0,
    wins_b      INTEGER NOT NULL DEFAULT 0,
    locked      INTEGER NOT NULL DEFAULT 0,
    locked_arm  TEXT
);

CREATE TABLE anti_recommend (
    kind        TEXT NOT NULL,
    reason      TEXT NOT NULL,
    added_at    INTEGER NOT NULL,
    source      TEXT NOT NULL CHECK(source IN ('operator','auto','spec')),
    PRIMARY KEY(kind, source)
);
```

### 3.9 Security model

- Observations carry `ctx_hash` not raw context features — adversary reading the SQLite file cannot reconstruct operator behavior verbatim.
- Anti-recommend rules with `source = 'spec'` cannot be removed by operator UI (only by app update); guard against accidental wake-word auto-enable.
- A/B experiment state never leaves the Mac.

### 3.10 Failure modes

| Failure | Behavior |
|---|---|
| Recommender empty queue | Silent. No surfaced recommendation. Never fabricate. |
| Bandit posterior NaN | Drop candidate, log to OSLog, do not surface. |
| All candidates below confidence floor | Silent. |
| Action-ref dead (e.g. integration removed) | Drop candidate at surface time, log. |
| Pacing-cap hit | Hold in queue; re-eligible next cap window. |
| A/B impressions skewed > 70/30 | Lock skewed arm as winner if statistical significance reached; otherwise force re-balance. |

### 3.11 Performance budget

| Surface | Target |
|---|---|
| `Observe()` | ≤ 1 ms; async batch insert |
| `NextBatch(N=5)` | ≤ 8 ms p95 |
| Surface check (every 60 s) | ≤ 12 ms |
| RAM steady | ≤ 18 MB |
| Total queue size | bounded at 500 candidates; oldest expired drops first |

### 3.12 UI surfaces

- **HUD notification widget** — primary surface. Recommendation renders as toast with action button + dismiss; outcome wired back via IPC.
- **Settings → Recommendations (new)** — toggle per kind; anti-recommend list (operator-editable, spec-rules read-only with lock glyph); A/B experiment ledger; recent surfaced list (last 30).
- **Dashboard** — surfaced/dismissed/applied counters in a small "Coach" card. Reuses Dashboard chrome (predecessor §4.7).
- **Voice** — recommendations marked `surface: voice` deliver as a one-sentence nudge during the idle close of a voice session ("Want me to pin the agenda widget? Just say yes."). Default off until operator opts in.

### 3.13 What ships in v1.1 vs deferred

**Ships:** ranked queue, decay, anti-recommend (operator + auto + spec sources), pacing caps, basic A/B (50/50 split with manual lock), Coach card.

**Deferred:** contextual-bandit feature crosses (interaction terms only), counterfactual outcome estimation, recommender explanations ("we suggested this because…") — all defer to v1.2 once operator has 60 d of outcome history.

---

## 4. Deliverable 4 — Camera + vision

### 4.1 Goal

Operator points at the screen (or the world) and asks. Phase 1 had ad-hoc screenshot interrogation; Phase 4 makes it first-class: live frame analysis, OCR pipeline, and visual-question-answer routing — all consent-gated and recorded against privacy budget.

### 4.2 Modes

| Mode | Source | Trigger | Default |
|---|---|---|---|
| Screenshot ask | screen capture | `⌥⇧Space` or `/look` | enabled |
| Selection ask | screen + drag rect | `⌥⇧Space` then drag | enabled |
| Live screen | continuous `CGDisplayStream` | Settings opt-in | OFF |
| Live camera | `AVCaptureDevice` front cam | Settings opt-in | OFF |
| OCR-only | screenshot → text | `/read` | enabled |

### 4.3 Interfaces

```go
// internal/vision/capture.go
type Capture interface {
    Screenshot(ctx context.Context, rect image.Rectangle) (Image, error)
    StartLiveScreen(ctx context.Context, fps int) (<-chan Frame, CancelFunc, error)
    StartLiveCamera(ctx context.Context, fps int) (<-chan Frame, CancelFunc, error)
}

// internal/vision/router.go
type VisionRouter interface {
    Ask(ctx context.Context, frame Image, prompt string, mode VisionMode) (<-chan ReasonerEvent, error)
    OCR(ctx context.Context, frame Image) ([]TextBlock, error)
}

type VisionMode int
const (
    VisionLocal VisionMode = iota   // Vision framework only (OCR, classifier)
    VisionSonnet                     // routes to Sonnet 4.6 vision API
    VisionAuto                       // local first; escalate to Sonnet if low confidence
)
```

IPC frame additions:

| Frame kind | Direction | Payload |
|---|---|---|
| `vision.snap` | HUD → daemon | `{rect?, prompt, mode}` |
| `vision.stream.start` | HUD → daemon | `{source: screen | camera, fps}` |
| `vision.stream.frame` | daemon → HUD | `{thumbBase64, summary?}` |
| `vision.consent.required` | daemon → HUD | `{reason}` |

### 4.4 OCR pipeline

- Primary: `Vision.framework` `VNRecognizeTextRequest` (revision 3, `accurate` level, language correction ON).
- Daemon caches OCR results keyed by frame perceptual hash; identical frame within 5 s reuses last OCR.
- Output bound: text blocks with bounding box + confidence; daemon assembles into prompt context.

### 4.5 Data model

```sql
CREATE TABLE vision_event (
    id          INTEGER PRIMARY KEY,
    at          INTEGER NOT NULL,
    mode        TEXT NOT NULL CHECK(mode IN ('screenshot','selection','live_screen','live_camera','ocr')),
    sent_to_cloud INTEGER NOT NULL DEFAULT 0,
    bytes       INTEGER NOT NULL,
    prompt      TEXT,
    thumb_path  TEXT,                 -- 256-px JPEG; auto-pruned after 7 d
    consent_ref TEXT
);

CREATE TABLE vision_consent (
    id          INTEGER PRIMARY KEY,
    mode        TEXT NOT NULL,
    granted_at  INTEGER NOT NULL,
    expires_at  INTEGER,              -- null = persistent until revoked
    scope       TEXT NOT NULL         -- 'this_session' | 'until_quit' | 'persistent'
);
```

### 4.6 Security model

- **Screen recording permission** — `CGPreflightScreenCaptureAccess()`; if denied, HUD shows "Grant screen recording in System Settings" with deep link.
- **Camera permission** — `AVCaptureDevice.requestAccess(.video)`; mirror copy.
- **Consent on every cloud upload** — first time per session per mode the daemon would send a frame to Sonnet vision, HUD shows a one-line confirm: "Send this screenshot to Claude?" with [Send] [Keep local] [Always allow this session]. Choice persisted in `vision_consent` with appropriate `scope`.
- **Live modes require explicit Settings opt-in** + per-session re-consent. Live screen mode shows a persistent menubar dot.
- **Capture-detection awareness** — if `CGScreenIsCaptured()` returns true (another app is recording), Leah's live mode pauses with a notification widget; resumes on `CGDisplayStreamUpdate` settle.
- **OCR is local** — `Vision.framework` runs on-device; OCR alone never uploads.
- **Thumbnail retention** — 7 d default, configurable in Settings → Privacy → "Vision history retention" (1 d / 7 d / 30 d / never).

### 4.7 Failure modes

| Failure | Behavior |
|---|---|
| Sonnet vision API 429 | Retry with backoff; if persistent, render `vision.consent.required` with "Cloud is busy — try local mode?" |
| OCR returns zero blocks | Surface "I couldn't read anything in this image" inline; do NOT silently upload to cloud unless `VisionSonnet` was explicitly chosen |
| Live screen FPS missed budget (target × 0.5 for 5 s) | Auto-downshift fps; warn after second downshift |
| Camera disconnected mid-stream | End mode; surface notification |
| Screenshot of restricted app (e.g. some DRM windows) | macOS already returns black frame; daemon detects all-black + surfaces "This window can't be captured" |

### 4.8 Performance budget

| Surface | RAM | CPU | Latency |
|---|---|---|---|
| Screenshot + OCR | +30 MB transient | 25% × 200 ms | ≤ 400 ms end-to-end |
| Sonnet vision call | +5 MB | per LLM stream | TTFB ≤ 1 s |
| Live screen 5 fps + thumbnail summarize | +85 MB resident | 6% steady | per-frame ≤ 180 ms |
| Live camera 2 fps + face-presence | +45 MB resident | 4% steady | per-frame ≤ 250 ms |

### 4.9 UI surfaces

- **HUD focus panel** — vision asks render the thumbnail inline (small, top-left of streaming response) with a "View source" affordance opening Quick Look. Citations carry over from predecessor §10.1 citation widget.
- **`⌥⇧Space`** — new global chord. Bound in Settings → General; check for conflicts at wizard time (no UX change in wizard — bind during runtime).
- **Selection drag** — translucent dark overlay (matches panel material) + 1-px gold rect + sub-pixel HUD anchor.
- **Menubar item** — adds a tiny eye glyph when live screen or live camera is on.
- **Settings → Privacy** — vision history retention + live mode toggles + per-mode consent revoke.
- **Dashboard** — vision events appear in the activity feed with mode + cloud-sent flag.

### 4.10 What ships in v1.1 vs deferred

**Ships:** screenshot ask + selection ask + OCR + live screen (opt-in) + live camera (opt-in) + consent ledger + retention controls.

**Deferred:** multi-monitor selection wand polish (works but seams across displays), face-recognition ("is this person in my contacts?") — explicit non-goal; differential-privacy histogram of camera activity (privacy-budget §8 carries this); on-device VLM that replaces Sonnet vision (waits for Apple Foundation Models multimodal API or sufficient open-weight ≤ 4 B model).

---

## 5. Deliverable 5 — Multi-agent A2A (inbound MCP + Leah-to-Leah)

### 5.1 Goal

Phase 3 ships outbound MCP (Leah calls external MCP tools). Phase 4 reverses the arrow: third-party agents call Leah as a peer. The same code path also lets two Leah instances (one on a friend's Mac, with explicit pairing) negotiate capability and exchange answers. Predecessor §17.16 trust moats stay binding.

### 5.2 Surfaces

| Surface | Protocol | Transport | Authn |
|---|---|---|---|
| Inbound MCP server | MCP/1 | stdio (CLI) + local HTTP/SSE | shared-secret token in header |
| Inbound A2A peer | Leah/A2A v1 (this spec) | mTLS over TCP, peer-pinned | Curve25519 pubkey exchange |
| Outbound A2A client | Leah/A2A v1 | same | same |

### 5.3 Inbound MCP server

Exposes a curated, allowlisted toolset:

| Tool | Body | Permission |
|---|---|---|
| `leah.memory.search` | `{query, k}` → matches | Token-scoped: `memory:read` |
| `leah.calendar.next` | `{within: duration}` → events | `calendar:read` (if integration on) |
| `leah.repo.cite` | `{repo, question}` → cite list | `repo:read` (if integration on) |
| `leah.ask` | `{prompt}` → stream | `ask:run` (rate-limited; counts against operator's Anthropic budget!) |
| `leah.widget.render` | `{widgetID, params}` → schema-conformant render | `widget:render` |

Inbound is OFF by default. Operator enables in Settings → Connections → "Allow inbound MCP." Token is issued per client with a name + scope set; revocable.

### 5.4 A2A protocol (Leah/A2A v1)

```
Frame := { v: 1, id: uuid, kind: string, payload: cbor }
Kinds:
  hello.{offer, ack}          # capability negotiation
  identity.{prove, verify}    # ed25519 sig over nonce
  ask.{request, partial, end} # delegated reasoning
  memory.{search, result}     # bounded memory query
  task.{offer, accept, reject}# work hand-off
  consent.{require, grant, deny}
  bye.{}
```

Capability negotiation declares feature bits — newer peers degrade gracefully against older.

#### 5.4.1 Identity verification

- Each Leah instance has a per-device Ed25519 key generated at first launch, stored in Keychain (`com.maydow.leah.identity`).
- Peering exchanges signed nonces; both sides commit `(pubkey, fingerprint, declared name)` to `a2a_peer` table.
- Operator confirms pairing with a 6-digit OTP shown on the inviting Mac (same UX as multi-device sync §2; different table because A2A peers ≠ owned Macs).

### 5.5 Interfaces

```go
// internal/a2a/server.go
type A2AServer interface {
    Listen(ctx context.Context, addr netip.AddrPort) error
    Stop(ctx context.Context) error
    Peers() []A2APeer
    Revoke(ctx context.Context, peerID PeerID) error
}

// internal/a2a/client.go
type A2AClient interface {
    Dial(ctx context.Context, addr netip.AddrPort, pubkey ed25519.PublicKey) (A2ASession, error)
}

type A2ASession interface {
    Negotiate(ctx context.Context) (CapabilitySet, error)
    Ask(ctx context.Context, prompt string) (<-chan ReasonerEvent, error)
    SearchMemory(ctx context.Context, q string, k int) ([]MemoryHit, error)
    Bye(ctx context.Context)
}

// internal/mcp/inbound.go
type InboundMCP interface {
    Register(tool MCPTool) error
    Serve(ctx context.Context, transport MCPTransport) error
    IssueToken(ctx context.Context, name string, scopes []Scope) (Token, error)
    RevokeToken(ctx context.Context, t Token) error
}
```

### 5.6 Data model

```sql
CREATE TABLE a2a_peer (
    id           TEXT PRIMARY KEY,    -- ed25519 pubkey fingerprint (hex)
    name         TEXT NOT NULL,
    paired_at    INTEGER NOT NULL,
    last_seen_at INTEGER,
    paused       INTEGER NOT NULL DEFAULT 0,
    scopes       TEXT NOT NULL        -- json array
);

CREATE TABLE a2a_call (
    id           INTEGER PRIMARY KEY,
    peer_id      TEXT NOT NULL REFERENCES a2a_peer(id) ON DELETE CASCADE,
    at           INTEGER NOT NULL,
    kind         TEXT NOT NULL,       -- ask | memory.search | etc
    bytes_in     INTEGER NOT NULL,
    bytes_out    INTEGER NOT NULL,
    consent_ref  INTEGER REFERENCES a2a_consent(id),
    outcome      TEXT NOT NULL CHECK(outcome IN ('ok','denied','error'))
);

CREATE TABLE mcp_token (
    id           INTEGER PRIMARY KEY,
    name         TEXT NOT NULL,
    hash         BLOB NOT NULL,       -- token hash, never the token
    scopes       TEXT NOT NULL,
    issued_at    INTEGER NOT NULL,
    revoked_at   INTEGER
);
```

### 5.7 Security model

- **Inbound off by default.** No port bound, no token issued, no peer accepted until operator opts in.
- **Per-call consent** for `leah.ask` (which spends Anthropic tokens). First call from a peer surfaces a HUD prompt: "Allow [peer] to ask Leah questions? You pay for the tokens." [Once] [This session] [Always]. Decision persisted in `a2a_consent`.
- **Memory scopes are namespaced** — `leah.memory.search` returns only rows tagged `share: external` OR explicitly marked by operator. Default is private.
- **Tokens hashed at rest** (SHA-256 with per-row salt). Token strings shown once at issue.
- **Listening port** binds to `127.0.0.1` by default; LAN bind requires an additional toggle ("Allow connections from local network").
- **Threat model** — trusts the operator-paired peer's pubkey. Does NOT trust the network. Does NOT defend against operator pairing with a malicious peer; UI copy at pair time names this risk and asks for the OTP via out-of-band channel.

### 5.8 Failure modes

| Failure | Behavior |
|---|---|
| Token mismatch | 401 + log; rate-limit after 5 fails / minute per source IP |
| Peer signature invalid | Drop session, log to OSLog, do not retry |
| Operator denies per-call consent | Return MCP error `consent.denied`; do not bill tokens |
| Anthropic budget exceeded for `leah.ask` from peer | Return `budget.exhausted`; surface widget toast |
| Peer attempts unknown tool | Return `tool.unknown` per MCP spec; log |
| Loop-protection: A asks B asks A asks B | Caller depth header; reject at depth > 2 |

### 5.9 Performance budget

| Operation | Target |
|---|---|
| Inbound MCP `memory.search` | ≤ 25 ms p95 |
| Inbound MCP `widget.render` | ≤ 50 ms p95 (matches widget mount budget) |
| A2A handshake | ≤ 1.2 s |
| A2A `Ask` proxy overhead | ≤ 30 ms above direct reasoner |
| Steady-state RAM | ≤ 22 MB |
| Listening sockets max | 64 concurrent (token-scoped + peer connections combined) |

### 5.10 UI surfaces

- **Settings → Connections (new pane, replaces v1 Integrations renamed?)** — actually leaves Integrations as-is and adds a peer to it:
  - **Outbound integrations** (Slack/Calendar/etc) — existing.
  - **Inbound MCP** — token list with name + scope + last-used + revoke button.
  - **A2A peers** — paired peer list with status + pause + unpair.
- **HUD notification widget** — surfaces inbound calls when consent required.
- **Dashboard** — A2A call log + token usage attributed to peers.
- **Menubar item** — small dot when ≥ 1 peer connected.

### 5.11 What ships in v1.1 vs deferred

**Ships:** inbound MCP server (stdio + local HTTP), token issue + revoke, A2A handshake, capability negotiation, memory search + ask delegation, per-call consent for billing.

**Deferred:** federated learning across peers, encrypted memory-share with selective disclosure, A2A "compete" mode (multiple peer agents propose answers + operator picks), peer-reputation scoring. All v1.2+.

---

## 6. Deliverable 6 — Continuous attestation

### 6.1 Goal

Predecessor §17.5 (selfbuild attestation) defines the build-time signature. Phase 4 wires runtime verification — every launch verifies the binary against its declared attestation, the About pane shows the verdict live, and unsigned plugin loads block.

### 6.2 What gets attested

| Artifact | Mechanism | When |
|---|---|---|
| `Leah.app` main binary | Developer ID + Apple notarization + Sparkle EdDSA | Every launch + every Sparkle update install |
| `wake-leah.mlmodel` | SHA-256 manifest in `Resources/manifest.json` signed with EdDSA | Voice subsystem start |
| `bge-small-en-v1.5.onnx` (Phase 2 fallback model) | same | Embedding subsystem start |
| Plugin bundles (§7) | Developer ID + plugin-key EdDSA | Plugin load |
| Sparkle appcast | HTTPS pin + EdDSA sig on each item | Update check |

### 6.3 Interfaces

```go
// internal/attest/verifier.go
type Verifier interface {
    VerifySelf(ctx context.Context) (Attestation, error)
    VerifyArtifact(ctx context.Context, path string, manifest ManifestRef) (Attestation, error)
    VerifyPlugin(ctx context.Context, bundlePath string) (Attestation, error)
    LastVerdict() Attestation
    Subscribe() <-chan AttestationChange
}

type Attestation struct {
    Subject     string                // path or identifier
    State       AttestState           // Verified | Stale | Failed | Unknown
    SignedBy    []SignerRef           // Developer ID, EdDSA pubkey fingerprint, notarization ticket id
    VerifiedAt  time.Time
    NextRecheck time.Time
    Reason      string                // populated on Failed/Stale
}
```

### 6.4 Recheck policy

- **Self attest:** at launch + every 24 h while running + on `NSWorkspaceDidWake`.
- **Plugin:** at load + every 60 min while loaded.
- **Sparkle appcast:** every fetch.
- **Model files:** at subsystem start + on file-change (fsnotify).

### 6.5 Data model

```sql
CREATE TABLE attest_record (
    id          INTEGER PRIMARY KEY,
    subject     TEXT NOT NULL,
    state       TEXT NOT NULL,
    verified_at INTEGER NOT NULL,
    next_recheck INTEGER NOT NULL,
    signed_by   TEXT NOT NULL,        -- json
    reason      TEXT
);

CREATE INDEX idx_attest_subject_at ON attest_record(subject, verified_at DESC);
```

### 6.6 Security model

- **Trust roots** — Apple Developer ID CA, Apple notarization ticket, Leah EdDSA pubkey (compiled into binary).
- **Plugin pubkeys** — registered at install (operator confirms); revocation list synced from `https://maydow.github.io/leah/revoked-plugins.json` daily.
- **Verdict drives behavior** — `Failed` on self blocks restart loop (watchdog §9 holds last-known-good and surfaces a HUD alert); `Failed` on plugin blocks plugin load; `Stale` warns but does not block.
- **Threat model** — assumes Apple notarization is trusted; defends against on-disk tamper of Resources/ + plugin bundles; does NOT defend against an exploit that gains code-execution inside the already-verified daemon.

### 6.7 Failure modes

| Failure | Behavior |
|---|---|
| Self attest fails at launch | Watchdog blocks startup; show macOS Notification "Leah failed integrity check — reinstall from leah.app" with [Reinstall] [Show log] |
| Plugin attest fails at load | Plugin disabled; surface "Plugin X failed attestation. View details" widget toast |
| Plugin attest goes stale (revocation list update) | Plugin disabled at next 60-min recheck; toast |
| Sparkle appcast signature invalid | Update silently dropped; OSLog entry; recheck next day |
| Network unreachable for revocation list | Tolerated for 7 days; after 7 d offline, plugins flagged Stale + load-block becomes a load-warn ("Couldn't reach revocation list for 8 days — proceed?") |

### 6.8 Performance budget

| Surface | Target |
|---|---|
| Self attest at launch | ≤ 350 ms |
| Plugin attest | ≤ 120 ms per bundle |
| Background recheck | ≤ 200 ms per artifact |
| RAM | ≤ 6 MB |

### 6.9 UI surfaces

- **Settings → About (existing)** — adds a Verification panel: live state chip ([Verified] / [Stale] / [Failed]) + last-checked timestamp + signed-by list + "Recheck now" button + link to log file.
- **HUD notification widget** — failure surfaces with severity styling.
- **Wizard** — NOT changed; verification happens silently on first launch.
- **CLI** — `leah doctor verify` prints full attestation tree to terminal.

### 6.10 What ships in v1.1 vs deferred

**Ships:** self verify at launch + every 24 h, plugin verify at load + every 60 min, About pane verdict, watchdog hard-block on self-failed.

**Deferred:** TPM/T2-attested attestation chain (requires Apple platform API not yet stable), reproducible-build verification ("rebuild from source matches shipped binary"), social-attestation ("3 of 5 community signers vouch"). All v1.2+.

---

## 7. Deliverable 7 — Plugin SDK

### 7.1 Goal

Third-party adapters as a first-class extension surface. Plugins ship as signed bundles, sandboxed, declare capabilities in a manifest, run in subprocess isolation, talk MCP to the daemon. The goal is "a plugin can replicate any first-party integration" — Slack, Linear, Gmail were built this way by Phase 4.

### 7.2 Plugin shape

```
MyPlugin.leahplugin/
  Contents/
    Info.plist           # bundle id, version, min-leah-version
    manifest.json        # capability + permission declarations
    binary               # ARM64 binary; macOS x86_64 path retained for Rosetta compat
    Resources/           # icons, locale strings
    Signature/
      developer-id.cms   # Apple Developer ID codesign blob
      plugin.eddsa       # EdDSA sig of manifest + binary
```

### 7.3 Manifest schema (v1)

```json
{
  "schema_version": 1,
  "id": "com.example.weather",
  "name": "Weather Pro",
  "version": "1.0.0",
  "min_leah": "1.1.0",
  "author": { "name": "...", "url": "..." },
  "capabilities": [
    { "kind": "widget", "type": "weather", "renderer": "schema-only" },
    { "kind": "mcp.tool", "name": "weather.now", "scopes": ["network:weather-api"] }
  ],
  "permissions": {
    "network": ["api.weatherprovider.example"],
    "fs.read": [],
    "fs.write": [],
    "keychain": ["com.example.weather.apikey"]
  },
  "ipc_quota": { "rpc_per_minute": 60, "stream_bytes_per_minute": 524288 },
  "ui": { "icon": "Resources/icon.svg", "settings_pane": "settings.html" }
}
```

### 7.4 Sandbox model

- **Process** — each plugin runs in its own subprocess; daemon launches via `posix_spawn` with `posix_spawnattr_setbinpref_np` for arch + a profile that denies all FS by default.
- **Filesystem** — read-only on plugin bundle + per-plugin scratch dir `~/Library/Application Support/Leah/plugins/<id>/`; manifest declares additional FS reads/writes that operator approves at install.
- **Network** — declared hostnames only; daemon enforces via a localhost HTTP proxy plugins must use (no direct socket).
- **Keychain** — declared service strings only; daemon proxies access.
- **No mic / camera / screen capture** — plugin SDK v1 does not expose AV capture (operator's media stays inside daemon-owned subsystems).
- **IPC quota** — `ipc_quota` enforced by daemon; over-quota plugin paused for a minute.

### 7.5 Interfaces

```go
// internal/plugin/host.go
type Host interface {
    Install(ctx context.Context, bundlePath string) (PluginID, error)
    Uninstall(ctx context.Context, id PluginID) error
    Enable(ctx context.Context, id PluginID) error
    Disable(ctx context.Context, id PluginID) error
    Reload(ctx context.Context, id PluginID) error
    List() []PluginInfo
    Logs(ctx context.Context, id PluginID, tail int) ([]LogLine, error)
}

// Plugin-side SDK (Go module github.com/maydow/leah-plugin-sdk-go)
type Plugin interface {
    Manifest() Manifest
    Init(ctx context.Context, h PluginHost) error
    Shutdown(ctx context.Context) error
}

type PluginHost interface {
    Log(level LogLevel, msg string, kv ...any)
    Keychain() KeychainAccessor
    HTTP() *http.Client          // pre-configured to use daemon's network proxy
    EmitMCPTool(t MCPTool) error
    EmitWidget(w WidgetSchema) error
    Bus() <-chan HostEvent
}
```

A Swift SDK ships as a Swift Package for plugin authors building widget renderers in SwiftUI.

### 7.6 Data model

```sql
CREATE TABLE plugin (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    version     TEXT NOT NULL,
    installed_at INTEGER NOT NULL,
    enabled     INTEGER NOT NULL DEFAULT 1,
    manifest    BLOB NOT NULL,        -- raw manifest.json
    bundle_path TEXT NOT NULL,
    attest_state TEXT NOT NULL
);

CREATE TABLE plugin_log (
    id          INTEGER PRIMARY KEY,
    plugin_id   TEXT NOT NULL REFERENCES plugin(id) ON DELETE CASCADE,
    at          INTEGER NOT NULL,
    level       INTEGER NOT NULL,
    msg         TEXT NOT NULL,
    kv          BLOB
);

CREATE TABLE plugin_quota (
    plugin_id   TEXT NOT NULL,
    window_at   INTEGER NOT NULL,
    rpcs        INTEGER NOT NULL DEFAULT 0,
    bytes       INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY(plugin_id, window_at)
);
```

### 7.7 Security model

- **Signing required** — daemon refuses to install or load an unsigned bundle. Operator override available only via `LEAH_PLUGIN_ALLOW_UNSIGNED=1` env var (developer escape hatch; logs warning + flags every call in event timeline).
- **Two signatures** — Apple Developer ID codesign + Leah plugin-EdDSA. The Leah signature is the SDK key (author-managed); Apple Developer ID provides macOS-level trust.
- **Manifest is the contract** — declared `permissions` are the daemon-enforced ceiling. Plugin attempting an undeclared action gets denied + a one-time toast to the operator.
- **No daemon API key access** — plugin uses its own Keychain entries; never the Anthropic key.
- **Inbound MCP for plugins** — a plugin that wants to call `leah.ask` goes through the same per-call consent flow as A2A (§5.7).

### 7.8 Failure modes

| Failure | Behavior |
|---|---|
| Plugin crash | Daemon restarts subprocess with exponential backoff (max 3 crashes / 10 min before disable) |
| Plugin OOM | Sandbox RSS cap (256 MB) terminates; treated as crash |
| Plugin manifest invalid | Install refused with line-numbered error |
| Plugin requests undeclared scope | Denied + toast + log |
| Plugin sig invalid at runtime recheck | Disabled; widget toast |
| Plugin abuses IPC quota | Paused 60 s; repeat offender flagged in Settings |

### 7.9 Performance budget

| Surface | Target |
|---|---|
| Plugin cold start | ≤ 200 ms |
| Plugin per-RPC overhead | ≤ 6 ms vs in-process |
| Plugin steady-state RAM ceiling | 256 MB hard cap (sandbox kills on breach) |
| Plugin manifest parse | ≤ 25 ms |
| Daemon RAM overhead per plugin | ≤ 12 MB (subprocess control struct + log buffer) |

### 7.10 UI surfaces

- **Settings → Plugins (new pane)** — installed list with status chip, version, author, enable toggle, "Show logs," "Reveal in Finder," uninstall.
- **HUD notification widget** — plugin install confirm + revocation/failure toasts.
- **Plugin gallery** — out of scope for v1.1; plugins install via drag-onto-Leah or `leah plugin install <path>` CLI; first-party doc site (`docs/superpowers/plugin-sdk/`) seeds discovery.
- **Dashboard** — per-plugin RPC + bytes histograms.

### 7.11 What ships in v1.1 vs deferred

**Ships:** Go SDK + Swift SDK, signing toolchain, manifest schema v1, sandbox model, install/enable/disable/uninstall flow, log tail, sample first-party "weather-pro" plugin shipping via this same SDK to prove the contract.

**Deferred:** plugin marketplace UI, in-app billing for plugins (out-of-scope per BYOK doctrine), shared widget render across plugins, multi-process IPC between plugins, plugin-to-plugin RPC. All v1.2+.

### 7.12 Open question

The Sparkle EdDSA key for Leah's own binary lives in the operator's 1Password (predecessor §17.19). The plugin-EdDSA key (used to sign first-party plugins) — does it share the same custody chain, or get its own vault item? Spec the option set; defer the lock until plugin shipping is imminent.

| Option | Pros | Cons |
|---|---|---|
| A. Same 1Password vault, separate item | Simple custody story | Compromise of one item leaks the other if MFA shared |
| B. Separate 1Password vault | Compartmentalization | More vaults to manage; cross-vault sharing UX clunkier |
| C. HSM (YubiKey) for plugin signing | Strongest | Adds hardware dependency for shipping |

Lock at plugin-publish W4 task.

---

## 8. Deliverable 8 — Privacy budget runtime

### 8.1 Goal

Phase 1–3 enforce static rules ("voice TTS for finance words goes to Apple," "embeddings default to local when `LEAH_EMBED_LOCAL=1`"). Phase 4 adds runtime meters — operator can see "I've spent X MB of cloud upload from vision this week" and "live screen mode is at 60% of its day quota." Budgets are caps, not advisories; the daemon refuses operations beyond the cap with a documented degradation.

### 8.2 Buckets

| Bucket | Unit | Default cap | Surface |
|---|---|---|---|
| `cloud.llm.tokens` | tokens / day | unlimited (BYOK) | Dashboard ledger only |
| `cloud.embed.bytes` | bytes / day | 50 MB | Dashboard + Settings → Privacy |
| `cloud.stt.seconds` | audio seconds / day | 900 s (15 min) | Settings (defaults assume voice mostly local) |
| `cloud.tts.chars` | chars / day | 25 000 | Settings (operator can raise) |
| `cloud.vision.bytes` | bytes / day | 30 MB | Settings + per-event consent §4 |
| `peer.a2a.tokens` | tokens / day | 5 000 (per peer) | Per-peer Settings row |
| `plugin.network.bytes` | bytes / day (per plugin) | 50 MB | Settings → Plugins per-row |

### 8.3 Interfaces

```go
// internal/budget/budget.go
type Budget interface {
    Charge(ctx context.Context, bucket Bucket, n int64) error  // err == ErrOverBudget
    Peek(ctx context.Context, bucket Bucket) (Balance, error)
    Set(ctx context.Context, bucket Bucket, cap int64, window Window) error
    Reset(ctx context.Context, bucket Bucket) error
    Subscribe() <-chan BudgetEvent
}

type Balance struct {
    Bucket    Bucket
    Spent     int64
    Cap       int64
    Window    Window      // Hour | Day | Week | Month
    ResetsAt  time.Time
    Trend     []Sample    // last 24 samples
}
```

### 8.4 Degradation paths

| Bucket | At 80% | At 100% | At cap exceeded |
|---|---|---|---|
| `cloud.embed.bytes` | Soft warn in Dashboard | Switch to local BGE | Block until reset (already local) |
| `cloud.stt.seconds` | Soft warn | Switch to local Whisper-large-v3 | If local unavailable: end voice session with copy |
| `cloud.tts.chars` | Soft warn | Switch to Apple Ava | Block (Apple Ava always available) |
| `cloud.vision.bytes` | Soft warn | Force `VisionLocal` mode | Block cloud calls |
| `peer.a2a.tokens` | Notify peer via MCP `budget.warning` | Reject `leah.ask` with `budget.warning` | Reject with `budget.exhausted` |
| `plugin.network.bytes` | Soft warn to operator | Pause plugin 60 s | Disable plugin until next reset |

### 8.5 Data model

```sql
CREATE TABLE budget_bucket (
    name        TEXT PRIMARY KEY,
    cap         INTEGER NOT NULL,
    window      TEXT NOT NULL,         -- 'hour'|'day'|'week'|'month'
    enabled     INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE budget_sample (
    bucket      TEXT NOT NULL,
    at          INTEGER NOT NULL,      -- truncated to bucket window
    spent       INTEGER NOT NULL,
    PRIMARY KEY(bucket, at)
);

CREATE INDEX idx_budget_sample_bucket_at ON budget_sample(bucket, at DESC);
```

### 8.6 Security model

- Budgets are an integrity boundary — a caller cannot bypass `Charge()`. Code review gate: every cloud call site must charge before issuing.
- Budget data is local — never leaves the Mac except as an aggregate count in the optional telemetry channel (predecessor §17.6, default off).
- Operator can raise or lower any cap from Settings; spec-pinned floors (e.g. `cloud.embed.bytes` minimum cap 0, max ∞) prevent footguns.

### 8.7 Failure modes

| Failure | Behavior |
|---|---|
| Budget DB write fail | Charge proceeds optimistically; reconcile on next sample tick |
| Charge during clock skew (NTP step backward) | Sample collision: take max(current, observed) |
| Bucket cap reduced mid-window below spent | Block new charges; do not retroactively refund |
| Subscriber lag | Drop oldest events; never block the charge path |

### 8.8 Performance budget

| Operation | Target |
|---|---|
| `Charge()` | ≤ 0.4 ms p95 |
| `Peek()` | ≤ 0.6 ms p95 |
| Sample-tick aggregation | ≤ 10 ms / minute |
| RAM | ≤ 4 MB |
| Disk per day | ≤ 200 KB |

### 8.9 UI surfaces

- **Settings → Privacy → Budgets (new)** — meter per bucket; raise/lower controls (with spec-pinned floors); resets-at timestamp.
- **Dashboard "Privacy" card** — week trend per bucket.
- **HUD notification widget** — fires at 100% (degradation kick-in) and at 100% + can't-degrade.
- **CLI** — `leah budget show` prints meter table.

### 8.10 What ships in v1.1 vs deferred

**Ships:** all listed buckets, soft warn + degradation + block ladder, operator-editable caps, Dashboard card.

**Deferred:** per-app budget attribution (which frontmost app drove the spend), shared budgets across multi-device peers (each Mac keeps its own ledger in v1.1), budget alerts via voice. All v1.2+.

---

## 9. Deliverable 9 — Watchdog supervisor

### 9.1 Goal

Phase 4 introduces 4 new long-lived subsystems (voice continuous, vision live, plugin subprocesses, sync) on top of Phase 1–3's daemon + HUD. Each is a failure surface. A real watchdog supervises them all with one policy, one log, one recovery path.

### 9.2 Supervised processes

| Process | Role | Crash policy |
|---|---|---|
| daemon | LLM + memory + sync + IPC bus | crash-only restart; backoff 200 ms × 2^n, cap 30 s; circuit-breaker at 5 / 60 s |
| HUD | UI | predecessor §17.8 KeepAlive policy retained; watchdog inherits |
| voice-stt | Whisper ONNX runner | restart; if 3 crashes / 5 min → disable continuous mode + surface |
| voice-tts | TTS prefetch worker | restart; degrade to Apple Ava on persistent fail |
| vision-live | Live screen / camera frame pump | restart; if 3 / 5 min → disable live mode |
| plugin-N | Each plugin subprocess | per-plugin policy from §7.8 |
| sync-peer-N | Per-peer sync worker | restart; flap-detect 5/60 s → pause peer |

### 9.3 Interfaces

```go
// internal/supervisor/supervisor.go
type Supervisor interface {
    Register(p ProcessSpec) ProcessHandle
    Stop(ctx context.Context, h ProcessHandle) error
    Restart(ctx context.Context, h ProcessHandle) error
    Status() []ProcessStatus
    Subscribe() <-chan SupervisorEvent
}

type ProcessSpec struct {
    Name        string
    Args        []string
    Env         []string
    StartTimeout time.Duration
    StopTimeout  time.Duration
    Restart      RestartPolicy   // CrashOnly | Always | Never
    BackoffMin   time.Duration
    BackoffMax   time.Duration
    Circuit      CircuitPolicy   // max crashes per window
    Health       HealthCheck     // optional liveness probe over Unix socket
    Limits       ResourceLimits  // RSS, FD, CPU cap
}
```

### 9.4 Leak detection

- Sample RSS per process every 30 s; rolling 10-min slope.
- Slope > +5 MB/min sustained 10 min → log + emit `supervisor.leak.suspected`.
- Slope > +20 MB/min for 5 min → preemptive restart with reason `leak`.
- Plugin processes get tighter caps (per-plugin RSS hard cap §7.9); daemon process gets `leak.suspected` event but no automatic restart (operator decides).

### 9.5 Eviction strategy

When system memory pressure hits `os.PSI` warning:

1. Drop fsnotify-watched adapter cache.
2. Pause live vision mode (preserve session; resume on pressure clear).
3. Shed plugin subprocesses in LRU order (one at a time, 10 s gap).
4. Reduce Whisper continuous to single-shot mode.
5. If pressure persists ≥ 60 s: end voice session with operator-visible nudge.

### 9.6 Data model

```sql
CREATE TABLE supervisor_event (
    id          INTEGER PRIMARY KEY,
    at          INTEGER NOT NULL,
    process     TEXT NOT NULL,
    kind        TEXT NOT NULL,        -- start|stop|crash|restart|leak|circuit_open|circuit_close
    pid         INTEGER,
    code        INTEGER,
    reason      TEXT
);

CREATE TABLE supervisor_rss (
    process     TEXT NOT NULL,
    at          INTEGER NOT NULL,
    rss_mb      INTEGER NOT NULL,
    PRIMARY KEY(process, at)
);
```

### 9.7 Security model

- Supervisor runs in-process inside the daemon (not a separate root-owned process). launchd remains the supervisor-of-the-supervisor for the daemon itself (predecessor §17.8).
- Resource limits enforced via `setrlimit` per child.
- Event log redacts plugin process Env (may contain operator-set keys).

### 9.8 Failure modes

| Failure | Behavior |
|---|---|
| Supervisor itself fails | launchd respawns daemon; HUD shows daemon-down ghost-panel until reconnect |
| Circuit-breaker open | Process stays down; operator-visible widget toast + Settings → About → Logs link |
| Restart loop wedged | After 10 failed restarts: open circuit, mark process disabled, require operator action |
| HealthCheck false-negative | Restart anyway (defensive); log; operator notified if repeats |

### 9.9 Performance budget

| Surface | Target |
|---|---|
| Supervisor overhead | ≤ 0.5% CPU steady |
| RSS sampling | ≤ 2 ms / sample |
| RAM | ≤ 8 MB |
| Restart action | ≤ 80 ms from crash detected to spawn issued |

### 9.10 UI surfaces

- **Settings → About → Diagnostics (existing About pane gains a Diagnostics row)** — process table: name, state, RSS, restarts (24 h), last crash reason. "Restart all" button (admin operator escape hatch).
- **HUD notification widget** — fires on circuit-open + leak preemptive restart.
- **CLI** — `leah doctor processes` for table dump; `leah doctor logs <process>` for tail.
- **Dashboard** — small "Health" card with green/yellow/red per process.

### 9.11 What ships in v1.1 vs deferred

**Ships:** supervisor for all listed processes, restart + backoff + circuit, RSS leak detection + preemptive restart, eviction ladder, diagnostics surface.

**Deferred:** distributed supervision across paired Macs (each Mac supervises its own subsystems), automatic memory dump on crash (privacy concerns — operator opts in for individual processes), historical crash heat-map UI. All v1.2+.

---

## 10. Cross-cutting matrices

### 10.1 Capability × macOS version

| Feature | macOS 14 Sonoma | macOS 15 Sequoia | macOS 26 |
|---|---|---|---|
| Voice frontier runtime | full | full | full |
| Multi-device sync | full | full | full |
| Recommend pass-2 | full | full | full |
| Vision OCR | full | full | full |
| Vision Sonnet route | full | full | full |
| Live screen mode | full | full | full |
| Live camera mode | full | full | full |
| Inbound MCP | full | full | full |
| A2A peer | full | full | full |
| Continuous attestation | full | full | full |
| Plugin SDK | full | full | full |
| Privacy budgets | full | full | full |
| Watchdog supervisor | full | full | full |
| Apple Foundation Models multimodal (future swap) | n/a | partial | full |

All Phase 4 features hold the macOS 14 floor from predecessor §17.5. No new floor introduced.

### 10.2 Model × latency × cost

| Workload | Model | Latency target | Cost note |
|---|---|---|---|
| Vision-anchored answer | Sonnet 4.6 vision | TTFB ≤ 1 s | Counts vs `cloud.llm.tokens` |
| OCR | Apple Vision local | ≤ 250 ms | Free |
| STT continuous | Whisper-large-v3 ONNX local | First partial ≤ 350 ms | Free |
| STT cloud fallback | OpenAI Whisper API | First partial ≤ 800 ms | Counts vs `cloud.stt.seconds` + dollar cost |
| TTS streaming | Eleven Flash v2.5 | TTFB ≤ 150 ms | Counts vs `cloud.tts.chars` |
| TTS private | Apple Ava Premium | TTFB ≤ 250 ms | Free |
| Recommend rank | Local Thompson sampler | ≤ 8 ms | Free |
| Plugin tool call | Subprocess RPC | ≤ 6 ms overhead | Free |

### 10.3 Subsystem × privacy bucket

| Subsystem | Local default | Cloud bucket charged |
|---|---|---|
| Voice STT | Whisper-large-v3 ONNX | `cloud.stt.seconds` only if fallback enabled + used |
| Voice TTS | Apple Ava for sensitive class | `cloud.tts.chars` otherwise |
| Vision OCR | Apple Vision | none |
| Vision VQA | Sonnet vision | `cloud.vision.bytes` (image bytes) + `cloud.llm.tokens` |
| Embedding | Voyage 3.5-lite OR BGE local | `cloud.embed.bytes` if Voyage |
| Recommend | Local Thompson | none |
| Sync | LAN mTLS | none |
| Plugin network | Plugin-declared hosts | `plugin.network.bytes` |

---

## 11. Phase 4 task index — preview

Sixteen implementer tasks across five waves. Each ticket is bench-disjoint at the file level (per `CLAUDE.md` parallel-cap = 6 on file-disjoint code PRs). Spec PRs serialize per `CLAUDE.md`.

### Wave 1 — perception substrate (parallel ≤ 6)

| ID | Title | Scope (paths) | Depends on |
|---|---|---|---|
| W1-T01 | Voice STT — Whisper-large-v3 ONNX runner | `internal/voice/stt/whisper/` + `internal/voice/stt.go` | Phase 3 voice TTS lands |
| W1-T02 | Voice duplex coordinator + barge-in | `internal/voice/duplex.go` + HUD `LeahHUD/Voice/` | W1-T01 |
| W1-T03 | Vision capture + OCR pipeline | `internal/vision/capture/`, `internal/vision/router.go` | none |
| W1-T04 | Vision Sonnet route + consent gate | `internal/vision/sonnet.go` + HUD consent flow | W1-T03 |
| W1-T05 | Voice + vision migrations | `internal/sqlstore/migrations/2026-06-22-*.sql` | none |

### Wave 2 — control plane (parallel ≤ 4)

| ID | Title | Scope (paths) | Depends on |
|---|---|---|---|
| W2-T06 | Recommend pass-2 (ranking, decay, anti-list) | `internal/learn/recommend.go`, `internal/learn/bandit.go` | predecessor bandit-recommender |
| W2-T07 | Recommend A/B + UI surfaces | `internal/learn/ab.go` + Settings/Dashboard | W2-T06 |
| W2-T08 | Privacy budget runtime | `internal/budget/` | none |
| W2-T09 | Continuous attestation | `internal/attest/` + Settings → About | none |

### Wave 3 — multi-device (parallel ≤ 3)

| ID | Title | Scope (paths) | Depends on |
|---|---|---|---|
| W3-T10 | Bonjour discovery + OTP pair | `internal/sync/discovery/`, `internal/sync/pair/` | W2-T09 |
| W3-T11 | CRDT model + sync coordinator | `internal/sync/crdt/`, `internal/sync/coord.go` | W3-T10 |
| W3-T12 | iCloud Keychain key share | `internal/keystore/icloud.go` + Settings → Sync | W3-T10 |

### Wave 4 — multi-agent + plugins (parallel ≤ 3)

| ID | Title | Scope (paths) | Depends on |
|---|---|---|---|
| W4-T13 | Inbound MCP server + tokens | `internal/mcp/inbound/` + Settings → Connections | W2-T08 |
| W4-T14 | A2A protocol + peer handshake | `internal/a2a/` + Settings → Connections | W2-T09 |
| W4-T15 | Plugin SDK + host + sample plugin | `pkg/leahplugin/`, `internal/plugin/`, `plugins/weather-pro/` | W2-T09 + W4-T13 |

### Wave 5 — supervision + ship (single owner)

| ID | Title | Scope (paths) | Depends on |
|---|---|---|---|
| W5-T16 | Watchdog supervisor + diagnostics + Dashboard cards + Phase 4 ship checklist | `internal/supervisor/`, Settings → About → Diagnostics, Dashboard cards, `docs/superpowers/phase4-ship-checklist.md` | W1-W4 land |

Total: 16 tickets. Phase 4 plan-author may split W2-T06 + W4-T15 + W5-T16 (each spans multiple files / packages), bringing the final count toward the 12–18 target band.

---

## 12. Open questions

Tracked here so future plan-authors do not re-derive.

| # | Question | Section | Resolution gate |
|---|---|---|---|
| Q1 | Plugin-EdDSA key custody — same vault as Sparkle, separate vault, or HSM? | §7.12 | Before W4-T15 ships |
| Q2 | A2A peer identity display name — operator-chosen at pair, or peer-declared? Spec assumes peer-declared with operator override at pair-time; needs UX review against §5 wireframe TBD. | §5.4.1 | Before W4-T14 lands |
| Q3 | Live screen FPS default — 5 fps vs 2 fps? Spec sets 5; operator-tested band should narrow. | §4.8 | After 2 weeks live use |
| Q4 | Multi-device LWW conflict UX — toast + 24 h undo vs inline merge editor? Spec ships toast; consider merge editor if conflict rate > 1/day in operator testing. | §2.7 | After W3 lands |
| Q5 | Sparkle revocation-list URL — `revoked-plugins.json` vs in-appcast — appcast saves a round-trip; spec uses separate URL for cache TTL flexibility. | §6.6 | Before W2-T09 ships |
| Q6 | Voice "voice-only mode" wake — does it require wake-word, push-to-talk, or both? Spec ships both; operator may want either-or in Settings. | §1.9 | After W1 lands |

---

## 13. Cross-references to predecessor

| Phase 4 § | Predecessor anchor |
|---|---|
| §1 Voice frontier | §2.7 (voice canon), §6.7 (wake-word), §8.4 (mic permission), §17.17 (TTS) |
| §2 Multi-device sync | §17.7 (iCloud sync — was deferred), §17.10 (SQLite invariants), §17.18 (Keychain) |
| §3 Recommend pass-2 | §10 widgets (notification surface), `2026-06-10-bandit-recommender.md` (base) |
| §4 Camera + vision | §10.1 citation widget, §17.13 (Touch ID for sensitive ops) |
| §5 Multi-agent A2A | §17.14 (LLM provider), §17.16 (vector store), `2026-06-10-mcp-a2a-publish.md` (sketch superseded) |
| §6 Continuous attestation | §17.1 (distribution + entitlements), §17.5 (compat matrix), §17.19 (Sparkle EdDSA custody), §17.20 (distribution channel) |
| §7 Plugin SDK | §10.6 (adapter registry), §10.8 (widget security model), §17.2 (framework primitives) |
| §8 Privacy budgets | §17.6 (telemetry opt-in), §17.15 (embeddings), §17.17 (TTS classifier) |
| §9 Watchdog supervisor | §17.8 (crash recovery — extends launchd policy) |

---

## 14. Spec discipline notes

- No "TBD" — each undecided point is enumerated in §12 with an option set + a resolution gate tied to a wave or task.
- Comment-density: this spec is comment-free in code blocks except where a contract phrase needs the WHY (e.g. CRDT tombstone column).
- Deletion default: Phase 4 deletes the three thin engineer/specs sketches by superseding them. Plan-author for Phase 4 should remove those three files as part of W5-T16 (with `git mv` to `docs/engineer/specs/archive/` if reviewers prefer audit-trail retention).
- No AI signatures per `CLAUDE.md` identity/output rule.
- Header depth: ≤ H4 throughout. Tables used for every matrix.

— end —

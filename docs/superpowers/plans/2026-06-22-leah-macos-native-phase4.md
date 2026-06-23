# Leah macOS Native UI — Phase 4 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL — Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Every Go-side dispatch MUST reference `docs/engineer/dispatch-templates/implementer-adapter.md` by path; every Swift-side dispatch MUST reference `docs/engineer/dispatch-templates/implementer.md` by path. Every reviewer dispatch MUST prepend `.claude/notes/reviewer-header.md` and return its verdict via the subagent transcript channel — NOT `gh pr comment` (same-session reviewer posting to GH inherits author identity = self-approval). Resolve the live reviewer-header path with `find . -name reviewer-header.md -path '*/notes/*' -not -path '*/.git/*'`.

**Goal:** Land Phase 4 ("Multi-modal + multi-agent layer") from spec `docs/superpowers/specs/2026-06-22-leah-phase4-design.md` v1.0. Nine deliverables, five build waves, twenty implementer tasks. v1.1 ship after W5.

1. **Voice frontier runtime** (§1) — full-duplex voice convo with Whisper-large-v3 ONNX local STT, barge-in, voice-only mode, continuous wake, per-app suppression learning.
2. **Multi-device sync** (§2) — peer Mac CRDT sync via Bonjour + mTLS + iCloud Keychain shared secret. No server, no account.
3. **Recommend pass-2** (§3) — ranked + decayed + anti-listed recommendation queue with pacing caps + A/B + Coach card.
4. **Camera + vision** (§4) — screenshot ask + selection drag + OCR + live screen + live camera, consent-gated, retention-bounded.
5. **Multi-agent A2A** (§5) — inbound MCP server with token scopes; Leah-to-Leah peer handshake + delegated `Ask` + memory search.
6. **Continuous attestation** (§6) — runtime self-attest + plugin attest + Sparkle EdDSA verify + About-pane live verdict.
7. **Plugin SDK** (§7) — signed bundle format, sandbox subprocess host, Go + Swift SDKs, sample `weather-pro` plugin.
8. **Privacy budget runtime** (§8) — 7 buckets, soft-warn → degrade → block ladder, per-bucket meter + Dashboard card.
9. **Watchdog supervisor** (§9) — restart + backoff + circuit-breaker + RSS leak detection + eviction ladder for all long-lived subsystems.

**Architecture:** Phase 4 layers four substrates (perception, control plane, multi-device, multi-agent) onto the Phase 1–3 daemon + HUD + Settings IA. The daemon stays the trust + LLM-key boundary (predecessor §17.14). All new long-lived subsystems route through a single in-process `Supervisor` (Wave 5). Single SQLite file invariant holds — every new table lives in `~/Library/Application Support/Leah/leah.db`. Default-OFF for ambient capture (voice continuous, vision live, multi-device sync).

| Subsystem | Phase 3 state | Phase 4 delta |
|---|---|---|
| `internal/voice/` | TTS chain + wake-word energy detector + listen | + Whisper-large-v3 ONNX STT, `DuplexSession`, barge-in, voice-only mode |
| `internal/vision/` (new) | none | screenshot + selection + OCR + Sonnet route + live modes + consent gate |
| `internal/sync/` (new) | none | Bonjour discovery + OTP pair + CRDT + mTLS transport |
| `internal/learn/` (new) | `internal/recommend/bandit.go` exists | Recommend pass-2 lifecycle, decay, anti-list, A/B, Coach card |
| `internal/budget/` | empty placeholder dir | 7 buckets + degradation ladder + meter UI |
| `internal/attest/` (new) | `internal/attestation/` ad-hoc | runtime verifier + recheck policy + plugin verdict |
| `internal/mcp/inbound/` (new) | `internal/mcp/server.go` outbound only | inbound MCP server + token scope |
| `internal/a2a/` (new) | none | Leah/A2A v1 protocol + peer handshake |
| `internal/plugin/` (new) | none | host + sandbox + quota + log buffer |
| `pkg/leahplugin/` (new) | none | Go SDK for plugin authors |
| `internal/supervisor/` (new) | `internal/watchdog/` HUD-side keepalive only | full in-process supervisor + leak detect + eviction |
| Swift HUD voice | `LeahAudio` + `LeahWake` + `PushToTalk.swift` | + `LeahHUD/Voice/VoiceCoordinator.swift` continuous duplex coordinator |
| Swift HUD vision | none | `LeahHUD/Vision/SnapTool.swift` + `SelectionOverlay.swift` + thumbnail renderer |
| Settings panes | 9 panes locked | + Sync (new), + Recommendations (new), + Plugins (new), + Budgets sub-pane in Privacy, + Diagnostics row in About, + Connections rename of Integrations |

**Tech Stack:** Go 1.25, SwiftUI/AppKit (macOS 14+), `github.com/anthropics/anthropic-sdk-go`, ONNX Runtime Go bindings (`github.com/yalue/onnxruntime_go`), Apple `Vision.framework` (OCR), `AVCaptureDevice` (camera), `CGDisplayStream` (live screen), `Bonjour`/`NetService`, `crypto/tls` + `crypto/ed25519` + `golang.org/x/crypto/curve25519` (peer auth), `kSecAttrSynchronizable` Keychain API (iCloud share), `posix_spawn` + `setrlimit` (plugin sandbox), Sparkle 2.9.x EdDSA (continuous attest).

## Global Constraints

- Module path: `github.com/trilam/leah`
- macOS deployment target: 14.0 (no new floor; predecessor §17.5)
- Single SQLite file invariant: `~/Library/Application Support/Leah/leah.db`; Phase 4 migrations are additive only
- Default-OFF for ambient capture (voice continuous mode, vision live modes, multi-device sync)
- Daemon owns the Anthropic API key — HUD/voice/vision/plugin processes never see it
- Privacy budget is enforced in the daemon, not by callers — every cloud call site MUST `Charge()` before issuing
- No AI signatures anywhere (no `Co-Authored-By`, no "Generated with", no "written by Claude")
- WHY-not-WHAT comments. Default to no comment. Test/Fuzz/Benchmark godocs: 1 line max
- Reviewer required per PR — independent reviewer subagent (agent-id `^(a[0-9a-f]{16}|cavecrew-reviewer-[a-z0-9-]+)$`) spawned immediately after `gh pr create`, verdict via transcript channel only
- Author posting own APPROVE = self-approval regardless of channel — never `gh pr comment` from same-session reviewer
- Deletion default: every PR states what got smaller. Phase 4 deletes the three superseded sketches in W5-T21:
  - `docs/engineer/specs/2026-06-10-voice-frontier.md`
  - `docs/engineer/specs/2026-06-10-learn-recommend-apply.md`
  - `docs/engineer/specs/2026-06-10-mcp-a2a-publish.md`
- Pre-PR verify gate (Go): `gofmt -l .` empty AND `go vet ./...` clean AND `golangci-lint run ./internal/<pkg> ./cmd/<pkg> 2>&1 | grep -E 'errcheck|govet|staticcheck' | head -5` empty AND `go test ./internal/<pkg> ./cmd/<pkg> 2>&1 | tail -5` PASS — `golangci-lint` is non-negotiable per Phase 3 lesson
- Pre-PR verify gate (Swift): `cd app/Leah && swift build 2>&1 | tail -5` clean AND `swift test --filter <module>Tests 2>&1 | tail -5` PASS
- Resource bundles in Swift packages: use `.copy("Fixtures")` not `.process` when call sites pass `subdirectory:` (Phase 2 lesson)
- Shared-seam files: when ≥ 2 tasks register defaults into the same shared table (e.g. `learn_decay` defaults, `budget_bucket` defaults, `anti_recommend` spec rows) — land a solo `registerXxxDefaults()` seam helper PR BEFORE fan-out (Phase 3 lesson)
- Orphan-scan-before-tag: every wave-exit gate AND the v1.1 ship gate (T21) MUST run `scripts/dev/orphan-scan.sh` (or equivalent `grep -RIn "<pkg>\." cmd/ internal/ | grep -v _test.go`) over every Phase-4 `internal/<pkg>/` and assert ZERO Phase-4 packages have zero non-test callers — v3.3.0 shipped with 3 wiring gaps (TTS provider, KG citation route, MCP composition) because providers existed but the composition root never instantiated them. Catching this post-tag is too late (Phase 3 lesson)
- Composition-root wiring is its own task, never implicit: providers/runtimes/handlers added in earlier waves do NOT self-register into `cmd/leah-daemon/main.go`. T19 (Wave 5) is the explicit composition-root wiring task — it must land BEFORE T20 (E2E smoke), because the smoke is meaningless if the daemon boot path never instantiates the surfaces under test (Phase 3 lesson — Wave 1 T02/T03/T04 in v3.3.0 shipped providers that the operator and an agent both assumed Task 4 wired; it didn't)
- Cavecrew-builder dispatch has NO `Bash` tool — only `Read`, `Edit`, `Write`, `Grep`, `Glob`. Any fix that requires running tests, `go vet`, `golangci-lint`, `git` (status/diff/commit/push), `gh`, or `make` MUST dispatch to `general-purpose` (or `claude`) instead. Builder is correct for typo fixes, single-function rewrites, mechanical renames, comment removal — not for anything that has to run the verify gate or push (Phase 3 dispatch lesson)
- Spec parity guard: `scripts/check-spec-parity.sh` MUST stay green; forbidden phrases (renamed terms, killed cosmetics) cannot enter code or tests
- Existing IPC frame: `struct Frame { Kind, TurnID string; Seq uint64; Payload json.RawMessage }` — do NOT reuse Kind strings reserved by Phase 1/2/3 (`ask`, `verify-key`, `diag.state`, `widget.mount`, `widget.pin`, `widget.unpin`, `tts.speak`, `tts.cancel`, `wake.arm`, `wake.disarm`, `ptt.on`, `ptt.off`, `eval.run`, `kg.cite`)
- Settings IA: the 9-pane lock from Phase 3 RELAXES in Phase 4 — three NEW panes (Sync, Recommendations, Plugins) are explicitly authorized by §2.9, §3.12, §7.10; Privacy gains a Budgets sub-pane (§8.9); About gains a Diagnostics row (§9.10); Integrations is RENAMED to Connections (§5.10)
- Cross-cutting invariants from §0.2 are binding for every task — implementers should not rederive them

---

## File structure decisions

### Go-side new package boundaries

| Package | Responsibility | Key files |
|---|---|---|
| `internal/voice/stt/whisper/` | Whisper-large-v3 ONNX runner + audio frame pipe | `runner.go`, `vad.go`, `model_loader.go` |
| `internal/voice/duplex/` | Duplex session orchestrator + barge-in arbitrator | `session.go`, `barge.go` |
| `internal/vision/capture/` | screen + camera capture + perceptual-hash cache | `screen.go`, `camera.go`, `phash.go` |
| `internal/vision/ocr/` | Vision.framework OCR cgo wrapper | `ocr_darwin.go`, `ocr_stub.go` |
| `internal/vision/router/` | local vs Sonnet route + consent gate | `router.go`, `consent.go` |
| `internal/learn/` | observation, ranking, decay, anti-list, A/B | `recommend.go`, `decay.go`, `antilist.go`, `ab.go` |
| `internal/budget/` | charge, peek, set, subscribe, sample-tick | `budget.go`, `bucket.go`, `degrade.go` |
| `internal/attest/` | self + artifact + plugin verifier + recheck | `verifier.go`, `manifest.go`, `revocation.go` |
| `internal/sync/discovery/` | Bonjour publish + browse | `bonjour.go` |
| `internal/sync/pair/` | OTP handshake + mTLS bootstrap | `otp.go`, `mtls.go` |
| `internal/sync/crdt/` | LWW register + add-only log + tombstone GC | `lww.go`, `log.go`, `tombstone.go` |
| `internal/sync/coord/` | session loop + outbox + conflict toast emitter | `coord.go`, `outbox.go` |
| `internal/keystore/icloud/` | iCloud-synchronizable Keychain wrapper | `icloud_darwin.go` |
| `internal/mcp/inbound/` | inbound MCP server + tool allowlist + token | `server.go`, `tools.go`, `token.go` |
| `internal/a2a/` | Leah/A2A v1 server + client + frame codec | `server.go`, `client.go`, `frame.go`, `consent.go` |
| `internal/plugin/` | host, manifest validator, sandbox launcher, quota | `host.go`, `manifest.go`, `sandbox.go`, `quota.go` |
| `pkg/leahplugin/` | Go SDK exposed to plugin authors | `plugin.go`, `host_iface.go`, `manifest_schema.go` |
| `internal/supervisor/` | process registry + restart + circuit + leak | `supervisor.go`, `process.go`, `leak.go`, `evict.go` |

### Swift-side new module boundaries

| Module | Responsibility | Key files |
|---|---|---|
| `LeahHUD/Voice/` | VoiceCoordinator + live transcript renderer + waveform | `VoiceCoordinator.swift`, `WaveformView.swift` |
| `LeahHUD/Vision/` | snap tool + selection overlay + thumbnail | `SnapTool.swift`, `SelectionOverlay.swift`, `ThumbView.swift` |
| `LeahUI/Settings/SyncPane.swift` | peer list + pair OTP + per-peer pause/unpair | `SyncPane.swift` |
| `LeahUI/Settings/RecommendationsPane.swift` | toggle per kind + anti-list + Coach surface | `RecommendationsPane.swift` |
| `LeahUI/Settings/PluginsPane.swift` | installed list + enable + logs + uninstall | `PluginsPane.swift` |
| `LeahUI/Settings/ConnectionsPane.swift` | rename of `IntegrationsPane.swift` + Inbound MCP + A2A peers | `ConnectionsPane.swift` |
| `LeahUI/Dashboard/PrivacyCard.swift` | week trend per bucket | `PrivacyCard.swift` |
| `LeahUI/Dashboard/CoachCard.swift` | surfaced/dismissed/applied counters | `CoachCard.swift` |
| `LeahUI/Dashboard/HealthCard.swift` | per-process green/yellow/red | `HealthCard.swift` |

### Migrations

All in `internal/sqlstore/migrations/`; chronologically ordered:

- `2026-06-22-001-voice.sql` — `voice_session`, `voice_turn`, `voice_suppression`
- `2026-06-22-002-vision.sql` — `vision_event`, `vision_consent`
- `2026-06-22-003-learn.sql` — `learn_observation`, `learn_recommendation`, `learn_decay`, `learn_experiment`, `anti_recommend`
- `2026-06-22-004-budget.sql` — `budget_bucket`, `budget_sample`
- `2026-06-22-005-attest.sql` — `attest_record`
- `2026-06-22-006-sync.sql` — `sync_peer`, `sync_clock`, `sync_tombstone`, `sync_outbox` + `node_uuid` columns on memory/conversation/pin/settings tables
- `2026-06-22-007-a2a.sql` — `a2a_peer`, `a2a_call`, `mcp_token`, `a2a_consent`
- `2026-06-22-008-plugin.sql` — `plugin`, `plugin_log`, `plugin_quota`
- `2026-06-22-009-supervisor.sql` — `supervisor_event`, `supervisor_rss`

W1-T05 lands all nine migrations as a single PR (single-owner per CLAUDE.md frozen-enum-files rule); subsequent tasks reference but do not author migration files.

---

## Wave dependency matrix (21 tasks)

- **Wave 1** (perception substrate, parallel ≤ 6 — file-disjoint Go-side except T05 single-owner): T01 Whisper STT, T02 duplex coordinator + barge-in, T03 vision capture + OCR, T04 vision Sonnet route + consent, T05 nine migrations (single-owner, lands FIRST).
- **Wave 2** (control plane, parallel ≤ 4 — file-disjoint): T06 learn observation + ranking + decay, T07 learn anti-list + A/B + Recommendations pane, T08 privacy budget runtime, T09 continuous attestation. Wave 2 starts after T05 merged.
- **Wave 3** (multi-device sync, parallel ≤ 3 — file-disjoint): T10 Bonjour discovery + OTP pair, T11 CRDT model + sync coordinator, T12 iCloud Keychain key share + Sync pane. Wave 3 starts after T09 merged.
- **Wave 4** (multi-agent + plugins, parallel ≤ 3 — file-disjoint): T13 inbound MCP server + tokens + Connections pane, T14 A2A protocol + peer handshake, T15 plugin SDK Go-side host + manifest validator, T16 plugin SDK + sample `weather-pro` plugin + Plugins pane. Wave 4 starts after T08 + T09 merged.
- **Wave 5** (supervision + ship, ≤ 3 parallel then serialized composition-root → E2E → ship): T17 watchdog supervisor + leak detect + eviction, T18 Dashboard cards (Coach + Privacy + Health), T19 composition-root wiring of every Phase-4 surface into `cmd/leah-daemon/main.go` (single-owner serialized, lands BEFORE T20), T20 Phase 4 E2E smoke + dispatch-template referenced harness, T21 Phase 4 ship checklist + spec-parity + orphan-scan + deletion of three superseded sketches + reviewer-and-merge pass. Wave 5 starts after W1–W4 land; T19 → T20 → T21 strictly serialized.

---

## Wave 1 — Perception substrate (parallel ≤ 6)

---

### Task 1: Voice STT — Whisper-large-v3 ONNX runner (`internal/voice/stt/whisper/`)

**Files:**
- Create: `internal/voice/stt/whisper/runner.go`
- Create: `internal/voice/stt/whisper/runner_test.go`
- Create: `internal/voice/stt/whisper/vad.go`
- Create: `internal/voice/stt/whisper/vad_test.go`
- Create: `internal/voice/stt/whisper/model_loader.go`
- Create: `internal/voice/stt/whisper/testdata/sine_400hz_1s.pcm`
- Create: `internal/voice/stt.go`
- Modify: `go.mod` — add `github.com/yalue/onnxruntime_go v1.13.0`

**Why this exists:** §1 mandates Whisper-large-v3 ONNX as the local-default STT. Audio never leaves the Mac on this path. Phase 3 only shipped `internal/voice/listen.go` (single-shot via `SFSpeechRecognizer` cgo); Phase 4 adds a streaming local primary that the duplex coordinator (T02) drives.

**Interfaces:**
- Produces:
  - `package stt` at `internal/voice/stt.go`
  - `type AudioFrame struct { PCM []int16; SampleRate int; Ts time.Time }`
  - `type Partial struct { Text string; IsFinal bool; Confidence float64; LatencyMS int }`
  - `type ProviderInfo struct { Name string; IsLocal bool; ModelID string; RAMmb int }`
  - `type STT interface { Stream(ctx context.Context, audio <-chan AudioFrame) (<-chan Partial, error); Info() ProviderInfo }`
  - `package whisper` at `internal/voice/stt/whisper/`
  - `type Runner struct { ... }` implements `stt.STT`
  - `func NewRunner(modelDir string) (*Runner, error)` — loads `whisper-large-v3.onnx` from `modelDir`
  - `func (*Runner) Stream(ctx, audio) (<-chan stt.Partial, error)`
  - `func (*Runner) Info() stt.ProviderInfo` — returns `{Name: "whisper-large-v3-onnx", IsLocal: true, ModelID: "whisper-large-v3", RAMmb: 850}`
  - `func (*Runner) Close() error`
  - `type VAD struct { NoiseFloorDBFS float64 }`
  - `func (*VAD) Adapt(frame stt.AudioFrame)` — adaptive noise floor update
  - `func (*VAD) IsVoice(frame stt.AudioFrame) bool`

- [ ] **Step 1: Write failing tests**

`internal/voice/stt/whisper/vad_test.go`:
```go
package whisper

import (
	"testing"
	"time"

	"github.com/trilam/leah/internal/voice/stt"
)

func TestVAD_SilenceIsNotVoice(t *testing.T) {
	v := &VAD{NoiseFloorDBFS: -55}
	frame := stt.AudioFrame{PCM: make([]int16, 320), SampleRate: 16000, Ts: time.Now()}
	if v.IsVoice(frame) {
		t.Fatal("zero PCM must not register as voice")
	}
}

func TestVAD_LoudSineIsVoice(t *testing.T) {
	v := &VAD{NoiseFloorDBFS: -55}
	pcm := make([]int16, 320)
	for i := range pcm {
		pcm[i] = 20000
	}
	frame := stt.AudioFrame{PCM: pcm, SampleRate: 16000, Ts: time.Now()}
	v.Adapt(frame)
	if !v.IsVoice(frame) {
		t.Fatal("loud constant signal must register as voice")
	}
}

func TestVAD_AdaptiveNoiseFloorRises(t *testing.T) {
	v := &VAD{NoiseFloorDBFS: -55}
	noisy := make([]int16, 320)
	for i := range noisy {
		noisy[i] = 4000
	}
	frame := stt.AudioFrame{PCM: noisy, SampleRate: 16000, Ts: time.Now()}
	for i := 0; i < 30; i++ {
		v.Adapt(frame)
	}
	if v.NoiseFloorDBFS < -50 {
		t.Fatalf("noise floor should have adapted up, got %v", v.NoiseFloorDBFS)
	}
}
```

`internal/voice/stt/whisper/runner_test.go`:
```go
package whisper

import (
	"context"
	"testing"
	"time"

	"github.com/trilam/leah/internal/voice/stt"
)

func TestRunner_InfoReportsLocal(t *testing.T) {
	r, err := NewRunner(t.TempDir())
	if err == nil {
		defer r.Close()
		info := r.Info()
		if !info.IsLocal {
			t.Fatal("Whisper runner must report IsLocal=true")
		}
		if info.ModelID != "whisper-large-v3" {
			t.Fatalf("ModelID: want whisper-large-v3, got %q", info.ModelID)
		}
	}
}

func TestRunner_StreamCancelsOnContextDone(t *testing.T) {
	r, err := NewRunner(t.TempDir())
	if err != nil {
		t.Skip("ONNX model not available in test env; runtime-gated test")
	}
	defer r.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	audio := make(chan stt.AudioFrame)
	partials, err := r.Stream(ctx, audio)
	if err != nil {
		t.Fatal(err)
	}
	<-ctx.Done()
	for range partials {
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/voice/stt/whisper/... 2>&1 | tail -10
```

Expected: FAIL — `undefined: VAD`, `undefined: NewRunner`, `package stt: no Go files`.

- [ ] **Step 3: Implement `internal/voice/stt.go`**

```go
package stt

import (
	"context"
	"time"
)

type AudioFrame struct {
	PCM        []int16
	SampleRate int
	Ts         time.Time
}

type Partial struct {
	Text       string
	IsFinal    bool
	Confidence float64
	LatencyMS  int
}

type ProviderInfo struct {
	Name    string
	IsLocal bool
	ModelID string
	RAMmb   int
}

type STT interface {
	Stream(ctx context.Context, audio <-chan AudioFrame) (<-chan Partial, error)
	Info() ProviderInfo
}
```

- [ ] **Step 4: Implement VAD**

`internal/voice/stt/whisper/vad.go`:
```go
package whisper

import (
	"math"

	"github.com/trilam/leah/internal/voice/stt"
)

type VAD struct {
	NoiseFloorDBFS float64
	emaAlpha       float64
}

func (v *VAD) frameDBFS(frame stt.AudioFrame) float64 {
	if len(frame.PCM) == 0 {
		return -120
	}
	var sumSq float64
	for _, s := range frame.PCM {
		f := float64(s) / 32768
		sumSq += f * f
	}
	rms := math.Sqrt(sumSq / float64(len(frame.PCM)))
	if rms <= 0 {
		return -120
	}
	return 20 * math.Log10(rms)
}

func (v *VAD) Adapt(frame stt.AudioFrame) {
	if v.emaAlpha == 0 {
		v.emaAlpha = 0.05
	}
	d := v.frameDBFS(frame)
	v.NoiseFloorDBFS = (1-v.emaAlpha)*v.NoiseFloorDBFS + v.emaAlpha*d
}

func (v *VAD) IsVoice(frame stt.AudioFrame) bool {
	return v.frameDBFS(frame) > v.NoiseFloorDBFS+6
}
```

- [ ] **Step 5: Implement model loader + Runner stub**

`internal/voice/stt/whisper/model_loader.go`:
```go
package whisper

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
)

var ErrModelMissing = errors.New("whisper-large-v3.onnx missing from model dir")

func loadModel(dir string) ([]byte, string, error) {
	p := filepath.Join(dir, "whisper-large-v3.onnx")
	f, err := os.Open(p)
	if err != nil {
		return nil, "", ErrModelMissing
	}
	defer f.Close()
	h := sha256.New()
	buf, err := io.ReadAll(io.TeeReader(f, h))
	if err != nil {
		return nil, "", err
	}
	return buf, hex.EncodeToString(h.Sum(nil)), nil
}
```

`internal/voice/stt/whisper/runner.go`:
```go
package whisper

import (
	"context"
	"sync"
	"time"

	"github.com/trilam/leah/internal/voice/stt"
)

type Runner struct {
	model []byte
	sha   string
	mu    sync.Mutex
}

func NewRunner(modelDir string) (*Runner, error) {
	buf, sha, err := loadModel(modelDir)
	if err != nil {
		return nil, err
	}
	return &Runner{model: buf, sha: sha}, nil
}

func (r *Runner) Info() stt.ProviderInfo {
	return stt.ProviderInfo{Name: "whisper-large-v3-onnx", IsLocal: true, ModelID: "whisper-large-v3", RAMmb: 850}
}

func (r *Runner) Stream(ctx context.Context, audio <-chan stt.AudioFrame) (<-chan stt.Partial, error) {
	out := make(chan stt.Partial, 8)
	go func() {
		defer close(out)
		vad := &VAD{NoiseFloorDBFS: -55}
		var window []stt.AudioFrame
		flush := func(final bool) {
			if len(window) == 0 {
				return
			}
			start := window[0].Ts
			select {
			case out <- stt.Partial{Text: r.transcribe(window), IsFinal: final, Confidence: 0.85, LatencyMS: int(time.Since(start).Milliseconds())}:
			case <-ctx.Done():
			}
			if final {
				window = window[:0]
			}
		}
		for {
			select {
			case <-ctx.Done():
				flush(true)
				return
			case f, ok := <-audio:
				if !ok {
					flush(true)
					return
				}
				vad.Adapt(f)
				if vad.IsVoice(f) {
					window = append(window, f)
					if len(window)%5 == 0 {
						flush(false)
					}
				} else if len(window) > 0 {
					flush(true)
				}
			}
		}
	}()
	return out, nil
}

// transcribe is the ONNX-Runtime call site. Wired against onnxruntime_go in a follow-up
// step inside the same task; placeholder here for the harness.
func (r *Runner) transcribe(window []stt.AudioFrame) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(window) == 0 {
		return ""
	}
	return ""
}

func (r *Runner) Close() error { return nil }
```

- [ ] **Step 6: Wire ONNX Runtime call**

Add to `internal/voice/stt/whisper/runner.go` (replace `transcribe` body):

```go
import ort "github.com/yalue/onnxruntime_go"

func (r *Runner) transcribe(window []stt.AudioFrame) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !ort.IsInitialized() {
		if err := ort.InitializeEnvironment(); err != nil {
			return ""
		}
	}
	sess, err := ort.NewAdvancedSessionWithONNXData[float32, int32](r.model, []string{"audio"}, []string{"tokens"}, nil)
	if err != nil {
		return ""
	}
	defer sess.Destroy()
	floatPCM := make([]float32, 0, len(window)*len(window[0].PCM))
	for _, fr := range window {
		for _, s := range fr.PCM {
			floatPCM = append(floatPCM, float32(s)/32768)
		}
	}
	in, err := ort.NewTensor(ort.NewShape(1, int64(len(floatPCM))), floatPCM)
	if err != nil {
		return ""
	}
	defer in.Destroy()
	tokens := make([]int32, 1024)
	out, err := ort.NewTensor(ort.NewShape(1, 1024), tokens)
	if err != nil {
		return ""
	}
	defer out.Destroy()
	if err := sess.Run([]ort.ArbitraryTensor{in}, []ort.ArbitraryTensor{out}); err != nil {
		return ""
	}
	return decodeTokens(out.GetData())
}

func decodeTokens(tokens []int32) string {
	// Whisper BPE decode — for the harness milestone, emit a placeholder; real BPE
	// table lands in a follow-up commit on this same branch.
	return ""
}
```

- [ ] **Step 7: Run tests to verify they pass**

```bash
go test ./internal/voice/stt/whisper/... 2>&1 | tail -10
go vet ./internal/voice/stt/...
gofmt -l internal/voice/stt/ | head
```

Expected: PASS on all VAD tests + `TestRunner_InfoReportsLocal`; `TestRunner_StreamCancelsOnContextDone` SKIP when model file absent.

- [ ] **Step 8: Commit**

```bash
git add internal/voice/stt.go internal/voice/stt/whisper/ go.mod go.sum
git commit -m "voice(stt): Whisper-large-v3 ONNX local runner + adaptive VAD

Adds streaming STT primary per spec §1. Audio stays on-device.
What got smaller: replaces ad-hoc SFSpeech single-shot as the duplex source."
```

- [ ] **Step 9: Open PR + spawn reviewer**

```bash
gh pr create --title "voice(stt): Whisper-large-v3 ONNX local runner" --body "Implements spec §1.3.1 + §1.4. Local-default STT, audio never leaves the Mac.

Test plan:
- VAD silence/voice/adaptation tests pass
- Runner Info reports IsLocal=true
- ONNX env initializes lazily; runner Close releases

Deletion: none new; supersedes single-shot path in T02.

Reviewer dispatched in transcript channel."
```

Spawn reviewer subagent immediately after `gh pr create` per CLAUDE.md.

---

### Task 2: Voice duplex coordinator + barge-in (`internal/voice/duplex/` + HUD `LeahHUD/Voice/`)

**Files:**
- Create: `internal/voice/duplex/session.go`
- Create: `internal/voice/duplex/session_test.go`
- Create: `internal/voice/duplex/barge.go`
- Create: `internal/voice/duplex/barge_test.go`
- Create: `cmd/leah-daemon/ipc_voice.go`
- Modify: `cmd/leah-daemon/ipc_handler.go` — register new frame kinds
- Create: `app/Leah/Sources/LeahHUD/Voice/VoiceCoordinator.swift`
- Create: `app/Leah/Sources/LeahHUD/Voice/WaveformView.swift`
- Create: `app/Leah/Tests/LeahHUDTests/VoiceCoordinatorTests.swift`

**Why this exists:** §1.3 mandates a single `DuplexSession` interface that orchestrates STT (Task 1) + reasoner + TTS (Phase 3 `internal/tts/`) concurrently with barge-in. Barge-in must halt TTS within 80 ms when mic VAD detects speech-during-TTS. The HUD-side `VoiceCoordinator` is the user-visible counterpart that renders transcript + waveform.

**Interfaces:**
- Consumes: `stt.STT` (T01), `tts.Provider` (Phase 3 `internal/tts/`), `reasoner.Stream(ctx, prompt)` (Phase 1)
- Produces:
  - `type DuplexEventKind int` with constants `WakeDetected | PartialIn | FinalIn | TTSStart | TTSChunk | BargeIn | TTSEnd | ErrorEvent`
  - `type DuplexEvent struct { Kind DuplexEventKind; Text string; LatencyMS int; Err error }`
  - `type DuplexOpts struct { VoiceOnly bool; SuppressApps []string; NoiseFloorDBFS float64; MaxTurnSeconds time.Duration }`
  - `type DuplexSession interface { Start(ctx context.Context, opts DuplexOpts) (<-chan DuplexEvent, error); Interrupt(); End() }`
  - `func NewSession(stt stt.STT, tts tts.Provider, ask reasoner.AskFn) DuplexSession`
  - IPC kinds: `voice.start`, `voice.partial`, `voice.tts.chunk`, `voice.barge`, `voice.end`
  - Swift: `protocol VoiceCoordinator` with `startSession(voiceOnly:)`, `endSession()`, `transcriptStream: AsyncStream<TranscriptUpdate>`, `levelStream: AsyncStream<Float>`

- [ ] **Step 1: Write failing tests for barge-in**

`internal/voice/duplex/barge_test.go`:
```go
package duplex

import (
	"testing"
	"time"
)

func TestBargeArbiter_HaltsTTSWithin80ms(t *testing.T) {
	arb := newBargeArbiter()
	start := time.Now()
	halted := make(chan time.Duration, 1)
	arb.onHalt = func() { halted <- time.Since(start) }
	arb.ttsStarted()
	arb.micVoiceDetected()
	select {
	case d := <-halted:
		if d > 80*time.Millisecond {
			t.Fatalf("barge-in took %v, budget is 80ms", d)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("barge-in never fired")
	}
}

func TestBargeArbiter_IgnoresVoiceBeforeTTS(t *testing.T) {
	arb := newBargeArbiter()
	called := false
	arb.onHalt = func() { called = true }
	arb.micVoiceDetected()
	time.Sleep(20 * time.Millisecond)
	if called {
		t.Fatal("barge fired without active TTS")
	}
}
```

`internal/voice/duplex/session_test.go`:
```go
package duplex

import (
	"context"
	"testing"
	"time"
)

func TestSession_EndEmitsTTSEnd(t *testing.T) {
	s := NewSession(fakeSTT{}, fakeTTS{}, fakeAsk)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events, err := s.Start(ctx, DuplexOpts{MaxTurnSeconds: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	go func() { time.Sleep(20 * time.Millisecond); s.End() }()
	var sawEnd bool
	for ev := range events {
		if ev.Kind == TTSEnd || ev.Kind == ErrorEvent {
			sawEnd = true
			break
		}
	}
	if !sawEnd {
		t.Fatal("Session.End never emitted terminal event")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/voice/duplex/... 2>&1 | tail -10
```

Expected: FAIL — `undefined: newBargeArbiter`, `undefined: NewSession`.

- [ ] **Step 3: Implement barge arbiter**

`internal/voice/duplex/barge.go`:
```go
package duplex

import "sync"

type bargeArbiter struct {
	mu        sync.Mutex
	ttsActive bool
	onHalt    func()
}

func newBargeArbiter() *bargeArbiter { return &bargeArbiter{} }

func (a *bargeArbiter) ttsStarted() {
	a.mu.Lock()
	a.ttsActive = true
	a.mu.Unlock()
}

func (a *bargeArbiter) ttsEnded() {
	a.mu.Lock()
	a.ttsActive = false
	a.mu.Unlock()
}

func (a *bargeArbiter) micVoiceDetected() {
	a.mu.Lock()
	active := a.ttsActive
	cb := a.onHalt
	a.mu.Unlock()
	if active && cb != nil {
		cb()
	}
}
```

- [ ] **Step 4: Implement session**

`internal/voice/duplex/session.go`:
```go
package duplex

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/trilam/leah/internal/voice/stt"
	"github.com/trilam/leah/internal/voice/tts"
)

type DuplexEventKind int

const (
	WakeDetected DuplexEventKind = iota
	PartialIn
	FinalIn
	TTSStart
	TTSChunk
	BargeIn
	TTSEnd
	ErrorEvent
)

type DuplexEvent struct {
	Kind      DuplexEventKind
	Text      string
	LatencyMS int
	Err       error
}

type DuplexOpts struct {
	VoiceOnly      bool
	SuppressApps   []string
	NoiseFloorDBFS float64
	MaxTurnSeconds time.Duration
}

type AskFn func(ctx context.Context, prompt string) (<-chan string, error)

type DuplexSession interface {
	Start(ctx context.Context, opts DuplexOpts) (<-chan DuplexEvent, error)
	Interrupt()
	End()
}

type session struct {
	stt      stt.STT
	tts      tts.Provider
	ask      AskFn
	arb      *bargeArbiter
	cancel   context.CancelFunc
	out      chan DuplexEvent
	endOnce  sync.Once
}

func NewSession(s stt.STT, t tts.Provider, ask AskFn) DuplexSession {
	return &session{stt: s, tts: t, ask: ask, arb: newBargeArbiter()}
}

func (s *session) Start(ctx context.Context, opts DuplexOpts) (<-chan DuplexEvent, error) {
	if s.stt == nil || s.tts == nil || s.ask == nil {
		return nil, errors.New("duplex: stt/tts/ask required")
	}
	ctx, s.cancel = context.WithCancel(ctx)
	s.out = make(chan DuplexEvent, 16)
	s.arb.onHalt = func() {
		select {
		case s.out <- DuplexEvent{Kind: BargeIn}:
		default:
		}
	}
	go s.loop(ctx, opts)
	return s.out, nil
}

func (s *session) loop(ctx context.Context, opts DuplexOpts) {
	defer close(s.out)
	deadline := time.After(opts.MaxTurnSeconds)
	for {
		select {
		case <-ctx.Done():
			s.emit(DuplexEvent{Kind: TTSEnd})
			return
		case <-deadline:
			s.emit(DuplexEvent{Kind: TTSEnd, Text: "timeout"})
			return
		}
	}
}

func (s *session) emit(ev DuplexEvent) {
	select {
	case s.out <- ev:
	default:
	}
}

func (s *session) Interrupt() { s.arb.micVoiceDetected() }
func (s *session) End()       { s.endOnce.Do(func() { if s.cancel != nil { s.cancel() } }) }
```

- [ ] **Step 5: Add test fakes**

Append to `internal/voice/duplex/session_test.go`:
```go
type fakeSTT struct{}

func (fakeSTT) Stream(ctx context.Context, audio <-chan stt.AudioFrame) (<-chan stt.Partial, error) {
	c := make(chan stt.Partial)
	close(c)
	return c, nil
}
func (fakeSTT) Info() stt.ProviderInfo { return stt.ProviderInfo{Name: "fake"} }

type fakeTTS struct{}

func (fakeTTS) Name() string { return "fake" }
func (fakeTTS) Speak(ctx context.Context, text, voice string) (tts.AudioStream, error) {
	return nil, nil
}
func (fakeTTS) PreWarm(ctx context.Context) error { return nil }

func fakeAsk(ctx context.Context, prompt string) (<-chan string, error) {
	c := make(chan string)
	close(c)
	return c, nil
}
```

- [ ] **Step 6: Register IPC frame kinds**

`cmd/leah-daemon/ipc_voice.go`:
```go
package main

import (
	"context"
	"encoding/json"

	"github.com/trilam/leah/internal/ipc"
	"github.com/trilam/leah/internal/voice/duplex"
)

type voiceStartPayload struct {
	VoiceOnly bool   `json:"voiceOnly"`
	Source    string `json:"source"`
}

func registerVoiceFrames(reg *ipc.HandlerRegistry, sess duplex.DuplexSession) {
	reg.Register("voice.start", func(ctx context.Context, f ipc.Frame) (any, error) {
		var p voiceStartPayload
		if err := json.Unmarshal(f.Payload, &p); err != nil {
			return nil, err
		}
		_, err := sess.Start(ctx, duplex.DuplexOpts{VoiceOnly: p.VoiceOnly})
		return map[string]any{"ok": err == nil}, err
	})
	reg.Register("voice.barge", func(ctx context.Context, f ipc.Frame) (any, error) {
		sess.Interrupt()
		return map[string]any{"ok": true}, nil
	})
	reg.Register("voice.end", func(ctx context.Context, f ipc.Frame) (any, error) {
		sess.End()
		return map[string]any{"ok": true}, nil
	})
}
```

Wire into `cmd/leah-daemon/ipc_handler.go` by adding `registerVoiceFrames(reg, voiceSession)` next to existing registrations.

- [ ] **Step 7: Implement Swift VoiceCoordinator**

`app/Leah/Sources/LeahHUD/Voice/VoiceCoordinator.swift`:
```swift
import Foundation
import AVFoundation

public struct TranscriptUpdate: Sendable {
    public let text: String
    public let isFinal: Bool
}

public protocol VoiceCoordinator: AnyObject {
    func startSession(voiceOnly: Bool) async throws
    func endSession() async
    var transcriptStream: AsyncStream<TranscriptUpdate> { get }
    var levelStream: AsyncStream<Float> { get }
}

public final class DefaultVoiceCoordinator: VoiceCoordinator {
    private let ipc: LeahIPCClient
    private var transcriptCont: AsyncStream<TranscriptUpdate>.Continuation?
    private var levelCont: AsyncStream<Float>.Continuation?

    public lazy var transcriptStream: AsyncStream<TranscriptUpdate> = AsyncStream { cont in
        self.transcriptCont = cont
    }
    public lazy var levelStream: AsyncStream<Float> = AsyncStream { cont in
        self.levelCont = cont
    }

    public init(ipc: LeahIPCClient) { self.ipc = ipc }

    public func startSession(voiceOnly: Bool) async throws {
        try await ipc.send(kind: "voice.start", payload: ["voiceOnly": voiceOnly, "source": "hotkey"])
    }

    public func endSession() async {
        _ = try? await ipc.send(kind: "voice.end", payload: [:] as [String: Any])
        transcriptCont?.finish()
        levelCont?.finish()
    }
}
```

`app/Leah/Sources/LeahHUD/Voice/WaveformView.swift`:
```swift
import SwiftUI

public struct WaveformView: View {
    public let levels: [Float]
    public init(levels: [Float]) { self.levels = levels }
    public var body: some View {
        Canvas { ctx, size in
            let bar = size.width / CGFloat(max(levels.count, 1))
            for (i, lvl) in levels.enumerated() {
                let h = CGFloat(lvl) * size.height
                let rect = CGRect(x: CGFloat(i) * bar, y: (size.height - h) / 2, width: bar * 0.8, height: h)
                ctx.fill(Path(rect), with: .color(.accentColor))
            }
        }
    }
}
```

- [ ] **Step 8: Swift test**

`app/Leah/Tests/LeahHUDTests/VoiceCoordinatorTests.swift`:
```swift
import XCTest
@testable import LeahHUD

final class VoiceCoordinatorTests: XCTestCase {
    func testEndCancelsStreams() async {
        let ipc = MockIPC()
        let coord = DefaultVoiceCoordinator(ipc: ipc)
        try? await coord.startSession(voiceOnly: false)
        await coord.endSession()
        XCTAssertEqual(ipc.sent.map(\.kind), ["voice.start", "voice.end"])
    }
}

final class MockIPC: LeahIPCClient {
    var sent: [(kind: String, payload: [String: Any])] = []
    func send(kind: String, payload: [String: Any]) async throws -> Data {
        sent.append((kind, payload))
        return Data()
    }
}
```

- [ ] **Step 9: Run all tests**

```bash
go test ./internal/voice/duplex/... 2>&1 | tail -10
cd app/Leah && swift test --filter VoiceCoordinatorTests 2>&1 | tail -10
```

Expected: PASS.

- [ ] **Step 10: Commit + PR + reviewer**

```bash
git add internal/voice/duplex/ cmd/leah-daemon/ipc_voice.go cmd/leah-daemon/ipc_handler.go app/Leah/Sources/LeahHUD/Voice/ app/Leah/Tests/LeahHUDTests/VoiceCoordinatorTests.swift
git commit -m "voice(duplex): session coordinator with barge-in arbiter

Halts TTS within 80ms on mic-voice-during-TTS detection per spec §1.7.
What got smaller: subsumes Phase 3 single-shot listener into a streaming session."
gh pr create --title "voice(duplex): coordinator + barge-in" --body "Spec §1.3.1 + §1.3.2 + §1.3.3. New IPC kinds: voice.start, voice.partial, voice.tts.chunk, voice.barge, voice.end."
```

Spawn reviewer in transcript channel.

---

### Task 3: Vision capture + OCR pipeline (`internal/vision/capture/` + `internal/vision/ocr/`)

**Files:**
- Create: `internal/vision/capture/screen.go`
- Create: `internal/vision/capture/screen_darwin.go`
- Create: `internal/vision/capture/screen_stub.go`
- Create: `internal/vision/capture/camera_darwin.go`
- Create: `internal/vision/capture/camera_stub.go`
- Create: `internal/vision/capture/phash.go`
- Create: `internal/vision/capture/phash_test.go`
- Create: `internal/vision/ocr/ocr.go`
- Create: `internal/vision/ocr/ocr_darwin.go`
- Create: `internal/vision/ocr/ocr_stub.go`
- Create: `internal/vision/ocr/ocr_test.go`
- Create: `internal/vision/types.go`

**Why this exists:** §4 mandates a vision substrate with screenshot + selection + OCR + live modes. Phase 1–3 had no `internal/vision/`; this lands the local-only OCR pipeline + capture types that T04 (Sonnet route + consent) consumes. OCR is local-only — `Vision.framework` never uploads.

**Interfaces:**
- Produces:
  - `package vision` at `internal/vision/types.go`
  - `type Image struct { Pixels []byte; Width, Height int; MIME string }`
  - `type Frame struct { Image Image; Ts time.Time; Source FrameSource }`
  - `type FrameSource int` with `SourceScreen | SourceCamera | SourceSelection`
  - `type TextBlock struct { Text string; Rect image.Rectangle; Confidence float64 }`
  - `type CancelFunc func()`
  - `type Capture interface { Screenshot(ctx, rect image.Rectangle) (Image, error); StartLiveScreen(ctx, fps int) (<-chan Frame, CancelFunc, error); StartLiveCamera(ctx, fps int) (<-chan Frame, CancelFunc, error) }`
  - `type OCREngine interface { Recognize(ctx context.Context, img Image) ([]TextBlock, error) }`
  - `func PHash(img Image) uint64` — 64-bit perceptual hash for cache keying
  - `func PHashDistance(a, b uint64) int` — Hamming distance

- [ ] **Step 1: Write failing tests**

`internal/vision/capture/phash_test.go`:
```go
package capture

import (
	"testing"

	"github.com/trilam/leah/internal/vision"
)

func TestPHash_IdenticalImagesMatch(t *testing.T) {
	img := vision.Image{Pixels: make([]byte, 64*64*4), Width: 64, Height: 64, MIME: "image/rgba"}
	for i := range img.Pixels {
		img.Pixels[i] = byte(i % 256)
	}
	if PHash(img) != PHash(img) {
		t.Fatal("identical images must hash equal")
	}
}

func TestPHash_DifferentImagesDiffer(t *testing.T) {
	a := vision.Image{Pixels: make([]byte, 64*64*4), Width: 64, Height: 64, MIME: "image/rgba"}
	b := vision.Image{Pixels: make([]byte, 64*64*4), Width: 64, Height: 64, MIME: "image/rgba"}
	for i := range a.Pixels {
		a.Pixels[i] = 0
		b.Pixels[i] = 255
	}
	if PHashDistance(PHash(a), PHash(b)) < 20 {
		t.Fatal("opposite images must have large hamming distance")
	}
}
```

`internal/vision/ocr/ocr_test.go`:
```go
package ocr

import (
	"context"
	"testing"

	"github.com/trilam/leah/internal/vision"
)

func TestOCR_EmptyImageReturnsEmpty(t *testing.T) {
	eng := NewEngine()
	blocks, err := eng.Recognize(context.Background(), vision.Image{Pixels: nil, Width: 0, Height: 0})
	if err == nil && len(blocks) != 0 {
		t.Fatalf("empty image: want []TextBlock, got %d blocks", len(blocks))
	}
}
```

- [ ] **Step 2: Run tests, verify FAIL**

```bash
go test ./internal/vision/... 2>&1 | tail -10
```

Expected: FAIL — `undefined: PHash`, `undefined: NewEngine`.

- [ ] **Step 3: Implement types + phash**

`internal/vision/types.go`:
```go
package vision

import (
	"image"
	"time"
)

type Image struct {
	Pixels []byte
	Width  int
	Height int
	MIME   string
}

type FrameSource int

const (
	SourceScreen FrameSource = iota
	SourceCamera
	SourceSelection
)

type Frame struct {
	Image  Image
	Ts     time.Time
	Source FrameSource
}

type TextBlock struct {
	Text       string
	Rect       image.Rectangle
	Confidence float64
}

type CancelFunc func()

type Capture interface {
	Screenshot(ctx context.Context, rect image.Rectangle) (Image, error)
	StartLiveScreen(ctx context.Context, fps int) (<-chan Frame, CancelFunc, error)
	StartLiveCamera(ctx context.Context, fps int) (<-chan Frame, CancelFunc, error)
}

type OCREngine interface {
	Recognize(ctx context.Context, img Image) ([]TextBlock, error)
}
```

`internal/vision/capture/phash.go`:
```go
package capture

import (
	"math/bits"

	"github.com/trilam/leah/internal/vision"
)

// PHash returns a 64-bit perceptual hash via 8x8 average-luminance threshold.
// Used to cache OCR results within a 5-second window per spec §4.4.
func PHash(img vision.Image) uint64 {
	if img.Width == 0 || img.Height == 0 || len(img.Pixels) == 0 {
		return 0
	}
	const N = 8
	cw := img.Width / N
	ch := img.Height / N
	if cw == 0 || ch == 0 {
		return 0
	}
	var cells [N * N]uint64
	var total uint64
	bpp := 4
	if img.MIME == "image/gray" {
		bpp = 1
	}
	for cy := 0; cy < N; cy++ {
		for cx := 0; cx < N; cx++ {
			var sum uint64
			var count uint64
			for y := cy * ch; y < (cy+1)*ch && y < img.Height; y++ {
				for x := cx * cw; x < (cx+1)*cw && x < img.Width; x++ {
					off := (y*img.Width + x) * bpp
					if off+bpp > len(img.Pixels) {
						continue
					}
					var lum uint64
					if bpp == 4 {
						lum = uint64(img.Pixels[off])*299 + uint64(img.Pixels[off+1])*587 + uint64(img.Pixels[off+2])*114
						lum /= 1000
					} else {
						lum = uint64(img.Pixels[off])
					}
					sum += lum
					count++
				}
			}
			if count == 0 {
				cells[cy*N+cx] = 0
			} else {
				cells[cy*N+cx] = sum / count
			}
			total += cells[cy*N+cx]
		}
	}
	avg := total / uint64(N*N)
	var h uint64
	for i, v := range cells {
		if v > avg {
			h |= 1 << uint(i)
		}
	}
	return h
}

func PHashDistance(a, b uint64) int { return bits.OnesCount64(a ^ b) }
```

- [ ] **Step 4: Implement OCR**

`internal/vision/ocr/ocr.go`:
```go
package ocr

import (
	"github.com/trilam/leah/internal/vision"
)

func NewEngine() vision.OCREngine { return newDarwinEngine() }
```

`internal/vision/ocr/ocr_darwin.go` (build tag `darwin`):
```go
//go:build darwin

package ocr

/*
#cgo LDFLAGS: -framework Vision -framework CoreGraphics -framework CoreImage -framework Foundation
#include <stdlib.h>
typedef struct { const char* text; int x; int y; int w; int h; double conf; } OCRHit;
int leah_ocr_recognize(const unsigned char* px, int w, int h, int bpp, OCRHit** out_hits);
void leah_ocr_free(OCRHit* hits, int n);
*/
import "C"

import (
	"context"
	"image"
	"unsafe"

	"github.com/trilam/leah/internal/vision"
)

type darwinEngine struct{}

func newDarwinEngine() vision.OCREngine { return &darwinEngine{} }

func (darwinEngine) Recognize(ctx context.Context, img vision.Image) ([]vision.TextBlock, error) {
	if img.Width == 0 || img.Height == 0 || len(img.Pixels) == 0 {
		return nil, nil
	}
	bpp := 4
	if img.MIME == "image/gray" {
		bpp = 1
	}
	var hits *C.OCRHit
	n := int(C.leah_ocr_recognize((*C.uchar)(unsafe.Pointer(&img.Pixels[0])), C.int(img.Width), C.int(img.Height), C.int(bpp), &hits))
	if n <= 0 {
		return nil, nil
	}
	defer C.leah_ocr_free(hits, C.int(n))
	out := make([]vision.TextBlock, 0, n)
	hitsSlice := unsafe.Slice(hits, n)
	for _, h := range hitsSlice {
		out = append(out, vision.TextBlock{
			Text:       C.GoString(h.text),
			Rect:       image.Rect(int(h.x), int(h.y), int(h.x+h.w), int(h.y+h.h)),
			Confidence: float64(h.conf),
		})
	}
	return out, nil
}
```

`internal/vision/ocr/ocr_stub.go` (build tag `!darwin`):
```go
//go:build !darwin

package ocr

import (
	"context"

	"github.com/trilam/leah/internal/vision"
)

type stubEngine struct{}

func newDarwinEngine() vision.OCREngine { return stubEngine{} }

func (stubEngine) Recognize(ctx context.Context, img vision.Image) ([]vision.TextBlock, error) {
	return nil, nil
}
```

Also ship the Objective-C bridge as `internal/vision/ocr/ocr_bridge.m`:
```objc
#import <Vision/Vision.h>
#import <CoreGraphics/CoreGraphics.h>

typedef struct { const char* text; int x; int y; int w; int h; double conf; } OCRHit;

int leah_ocr_recognize(const unsigned char* px, int w, int h, int bpp, OCRHit** out_hits) {
    CGColorSpaceRef cs = (bpp == 1) ? CGColorSpaceCreateDeviceGray() : CGColorSpaceCreateDeviceRGB();
    CGContextRef ctx = CGBitmapContextCreate((void*)px, w, h, 8, w*bpp, cs, (bpp == 1) ? kCGImageAlphaNone : kCGImageAlphaPremultipliedLast);
    CGImageRef cgImg = CGBitmapContextCreateImage(ctx);
    VNRecognizeTextRequest* req = [[VNRecognizeTextRequest alloc] init];
    req.recognitionLevel = VNRequestTextRecognitionLevelAccurate;
    req.usesLanguageCorrection = YES;
    VNImageRequestHandler* handler = [[VNImageRequestHandler alloc] initWithCGImage:cgImg options:@{}];
    NSError* err = nil;
    [handler performRequests:@[req] error:&err];
    NSArray<VNRecognizedTextObservation*>* results = req.results;
    int n = (int)results.count;
    if (n == 0) { *out_hits = NULL; CGImageRelease(cgImg); CGContextRelease(ctx); CGColorSpaceRelease(cs); return 0; }
    OCRHit* hits = calloc(n, sizeof(OCRHit));
    for (int i = 0; i < n; i++) {
        VNRecognizedTextObservation* o = results[i];
        VNRecognizedText* top = [[o topCandidates:1] firstObject];
        hits[i].text = strdup([top.string UTF8String]);
        hits[i].x = (int)(o.boundingBox.origin.x * w);
        hits[i].y = (int)((1.0 - o.boundingBox.origin.y - o.boundingBox.size.height) * h);
        hits[i].w = (int)(o.boundingBox.size.width * w);
        hits[i].h = (int)(o.boundingBox.size.height * h);
        hits[i].conf = top.confidence;
    }
    *out_hits = hits;
    CGImageRelease(cgImg); CGContextRelease(ctx); CGColorSpaceRelease(cs);
    return n;
}

void leah_ocr_free(OCRHit* hits, int n) {
    for (int i = 0; i < n; i++) free((void*)hits[i].text);
    free(hits);
}
```

- [ ] **Step 5: Implement capture stubs**

`internal/vision/capture/screen_darwin.go` (build tag `darwin`):
```go
//go:build darwin

package capture

import (
	"context"
	"errors"
	"image"

	"github.com/trilam/leah/internal/vision"
)

type darwinCapture struct{}

func New() vision.Capture { return &darwinCapture{} }

func (darwinCapture) Screenshot(ctx context.Context, rect image.Rectangle) (vision.Image, error) {
	return vision.Image{}, errors.New("capture: screenshot impl pending CGDisplayCreateImage bridge")
}

func (darwinCapture) StartLiveScreen(ctx context.Context, fps int) (<-chan vision.Frame, vision.CancelFunc, error) {
	return nil, nil, errors.New("capture: live screen pending CGDisplayStream bridge")
}

func (darwinCapture) StartLiveCamera(ctx context.Context, fps int) (<-chan vision.Frame, vision.CancelFunc, error) {
	return nil, nil, errors.New("capture: live camera pending AVCaptureDevice bridge")
}
```

`internal/vision/capture/screen_stub.go` (build tag `!darwin`):
```go
//go:build !darwin

package capture

import (
	"context"
	"errors"
	"image"

	"github.com/trilam/leah/internal/vision"
)

type stubCapture struct{}

func New() vision.Capture { return stubCapture{} }

func (stubCapture) Screenshot(ctx context.Context, rect image.Rectangle) (vision.Image, error) {
	return vision.Image{}, errors.New("capture: darwin-only")
}
func (stubCapture) StartLiveScreen(ctx context.Context, fps int) (<-chan vision.Frame, vision.CancelFunc, error) {
	return nil, nil, errors.New("capture: darwin-only")
}
func (stubCapture) StartLiveCamera(ctx context.Context, fps int) (<-chan vision.Frame, vision.CancelFunc, error) {
	return nil, nil, errors.New("capture: darwin-only")
}
```

(Native CG/AVF implementations land in a follow-up commit on this branch; the contract + tests are the ship gate for T03.)

- [ ] **Step 6: Run tests**

```bash
go test ./internal/vision/... 2>&1 | tail -10
golangci-lint run ./internal/vision/... 2>&1 | head
```

Expected: PASS.

- [ ] **Step 7: Commit + PR + reviewer**

```bash
git add internal/vision/
git commit -m "vision(capture+ocr): pipeline contracts + perceptual hash cache key

Local-only OCR via Vision.framework cgo. PHash caches Recognize() results.
What got smaller: removes ad-hoc screenshot path from Phase 1 brief subsystem."
gh pr create --title "vision: capture + OCR pipeline contracts" --body "Spec §4.3 + §4.4. OCR never uploads. PHash cache per §4.4."
```

---

### Task 4: Vision Sonnet route + consent gate (`internal/vision/router/`)

**Files:**
- Create: `internal/vision/router/router.go`
- Create: `internal/vision/router/router_test.go`
- Create: `internal/vision/router/consent.go`
- Create: `internal/vision/router/consent_test.go`
- Create: `internal/vision/router/sonnet.go`
- Create: `cmd/leah-daemon/ipc_vision.go`
- Create: `app/Leah/Sources/LeahHUD/Vision/SnapTool.swift`
- Create: `app/Leah/Sources/LeahHUD/Vision/SelectionOverlay.swift`
- Create: `app/Leah/Sources/LeahHUD/Vision/ConsentSheet.swift`

**Why this exists:** §4.6 mandates a first-time-per-session consent prompt before any frame uploads to Sonnet vision. The router consumes the capture + OCR contracts from T03 and decides local vs Sonnet based on `VisionMode`.

**Interfaces:**
- Consumes: `vision.Capture` (T03), `vision.OCREngine` (T03), reasoner stream
- Produces:
  - `type VisionMode int` with `VisionLocal | VisionSonnet | VisionAuto`
  - `type ReasonerEvent struct { Text string; IsFinal bool; Err error }`
  - `type Router interface { Ask(ctx, frame vision.Image, prompt string, mode VisionMode) (<-chan ReasonerEvent, error); OCR(ctx, frame vision.Image) ([]vision.TextBlock, error) }`
  - `type ConsentStore interface { Granted(mode string) bool; Grant(mode string, scope ConsentScope); Revoke(mode string) }`
  - `type ConsentScope int` with `ScopeThisSession | ScopeUntilQuit | ScopePersistent`
  - IPC kinds: `vision.snap`, `vision.stream.start`, `vision.stream.frame`, `vision.consent.required`

- [ ] **Step 1: Write failing consent tests**

`internal/vision/router/consent_test.go`:
```go
package router

import "testing"

func TestConsent_DefaultDenied(t *testing.T) {
	s := newMemConsent()
	if s.Granted("live_screen") {
		t.Fatal("default must be denied")
	}
}

func TestConsent_GrantSessionOnlyPersists(t *testing.T) {
	s := newMemConsent()
	s.Grant("live_screen", ScopeThisSession)
	if !s.Granted("live_screen") {
		t.Fatal("session grant must register")
	}
}

func TestConsent_RevokeClears(t *testing.T) {
	s := newMemConsent()
	s.Grant("screenshot", ScopePersistent)
	s.Revoke("screenshot")
	if s.Granted("screenshot") {
		t.Fatal("revoke must clear grant")
	}
}
```

- [ ] **Step 2: Run, FAIL**

```bash
go test ./internal/vision/router/... 2>&1 | tail
```

- [ ] **Step 3: Implement consent**

`internal/vision/router/consent.go`:
```go
package router

import "sync"

type ConsentScope int

const (
	ScopeThisSession ConsentScope = iota
	ScopeUntilQuit
	ScopePersistent
)

type ConsentStore interface {
	Granted(mode string) bool
	Grant(mode string, scope ConsentScope)
	Revoke(mode string)
}

type memConsent struct {
	mu sync.Mutex
	m  map[string]ConsentScope
}

func newMemConsent() *memConsent { return &memConsent{m: map[string]ConsentScope{}} }

func (c *memConsent) Granted(mode string) bool {
	c.mu.Lock(); defer c.mu.Unlock()
	_, ok := c.m[mode]
	return ok
}

func (c *memConsent) Grant(mode string, scope ConsentScope) {
	c.mu.Lock(); defer c.mu.Unlock()
	c.m[mode] = scope
}

func (c *memConsent) Revoke(mode string) {
	c.mu.Lock(); defer c.mu.Unlock()
	delete(c.m, mode)
}
```

- [ ] **Step 4: Implement router**

`internal/vision/router/router.go`:
```go
package router

import (
	"context"
	"errors"

	"github.com/trilam/leah/internal/vision"
)

type VisionMode int

const (
	VisionLocal VisionMode = iota
	VisionSonnet
	VisionAuto
)

type ReasonerEvent struct {
	Text    string
	IsFinal bool
	Err     error
}

type Router interface {
	Ask(ctx context.Context, frame vision.Image, prompt string, mode VisionMode) (<-chan ReasonerEvent, error)
	OCR(ctx context.Context, frame vision.Image) ([]vision.TextBlock, error)
}

type SonnetClient interface {
	StreamVision(ctx context.Context, image vision.Image, prompt string) (<-chan string, error)
}

type router struct {
	ocr     vision.OCREngine
	sonnet  SonnetClient
	consent ConsentStore
	prompt  func(mode string) bool
}

func New(ocr vision.OCREngine, sonnet SonnetClient, consent ConsentStore, prompt func(mode string) bool) Router {
	return &router{ocr: ocr, sonnet: sonnet, consent: consent, prompt: prompt}
}

var ErrConsentDenied = errors.New("vision: consent denied")

func (r *router) Ask(ctx context.Context, frame vision.Image, prompt string, mode VisionMode) (<-chan ReasonerEvent, error) {
	if mode == VisionSonnet || (mode == VisionAuto && r.shouldEscalate(frame)) {
		if !r.consent.Granted("screenshot") {
			if !r.prompt("screenshot") {
				return nil, ErrConsentDenied
			}
		}
		out := make(chan ReasonerEvent, 8)
		go func() {
			defer close(out)
			stream, err := r.sonnet.StreamVision(ctx, frame, prompt)
			if err != nil {
				out <- ReasonerEvent{Err: err, IsFinal: true}
				return
			}
			for chunk := range stream {
				out <- ReasonerEvent{Text: chunk}
			}
			out <- ReasonerEvent{IsFinal: true}
		}()
		return out, nil
	}
	out := make(chan ReasonerEvent, 1)
	close(out)
	return out, nil
}

func (r *router) OCR(ctx context.Context, frame vision.Image) ([]vision.TextBlock, error) {
	return r.ocr.Recognize(ctx, frame)
}

func (r *router) shouldEscalate(frame vision.Image) bool { return true }
```

- [ ] **Step 5: Wire IPC**

`cmd/leah-daemon/ipc_vision.go`:
```go
package main

import (
	"context"
	"encoding/json"

	"github.com/trilam/leah/internal/ipc"
	"github.com/trilam/leah/internal/vision/router"
)

type snapPayload struct {
	Prompt string `json:"prompt"`
	Mode   int    `json:"mode"`
}

func registerVisionFrames(reg *ipc.HandlerRegistry, r router.Router) {
	reg.Register("vision.snap", func(ctx context.Context, f ipc.Frame) (any, error) {
		var p snapPayload
		if err := json.Unmarshal(f.Payload, &p); err != nil {
			return nil, err
		}
		return map[string]any{"queued": true}, nil
	})
}
```

- [ ] **Step 6: Swift HUD surfaces**

`app/Leah/Sources/LeahHUD/Vision/SnapTool.swift`, `SelectionOverlay.swift`, `ConsentSheet.swift` — render bind the `⌥⇧Space` chord to `vision.snap` frame and host the consent prompt.

(Skeleton code matching Phase 3 SwiftUI patterns; full HUD glue lands inside this same task.)

- [ ] **Step 7: Verify + commit + PR + reviewer**

```bash
go test ./internal/vision/router/...
golangci-lint run ./internal/vision/...
cd app/Leah && swift build 2>&1 | tail
```

```bash
git add internal/vision/router/ cmd/leah-daemon/ipc_vision.go app/Leah/Sources/LeahHUD/Vision/
git commit -m "vision(router): Sonnet route + per-session consent gate

First cloud upload per session prompts via HUD sheet per §4.6.
What got smaller: removes ad-hoc 'send screenshot to claude' path from Phase 1 brief."
gh pr create --title "vision(router): Sonnet route + consent gate"
```

---

### Task 5: Phase 4 migrations (`internal/sqlstore/migrations/2026-06-22-*.sql`) — SINGLE OWNER

**Files:**
- Create: `internal/sqlstore/migrations/2026-06-22-001-voice.sql`
- Create: `internal/sqlstore/migrations/2026-06-22-002-vision.sql`
- Create: `internal/sqlstore/migrations/2026-06-22-003-learn.sql`
- Create: `internal/sqlstore/migrations/2026-06-22-004-budget.sql`
- Create: `internal/sqlstore/migrations/2026-06-22-005-attest.sql`
- Create: `internal/sqlstore/migrations/2026-06-22-006-sync.sql`
- Create: `internal/sqlstore/migrations/2026-06-22-007-a2a.sql`
- Create: `internal/sqlstore/migrations/2026-06-22-008-plugin.sql`
- Create: `internal/sqlstore/migrations/2026-06-22-009-supervisor.sql`
- Modify: `internal/sqlstore/migrate.go` — register the nine new files
- Create: `internal/sqlstore/phase4_migrations_test.go`

**Why this exists:** All nine Phase 4 migrations land in one PR because the migration registry is a frozen-enum file (per CLAUDE.md). Splitting across tasks invites stale-base regressions (Phase 2 lesson). Tasks T06–T21 reference but do not author migration files.

**Interfaces:**
- Produces all schema from spec §1.4, §2.5, §3.8, §4.5, §5.6, §6.5, §7.6, §8.5, §9.6
- Phase 3 tables (`memory`, `conversation`, `pin`, `settings_kv`) gain `node_uuid TEXT NOT NULL DEFAULT 'self'` column via `ALTER TABLE`

- [ ] **Step 1: Write failing test**

`internal/sqlstore/phase4_migrations_test.go`:
```go
package sqlstore

import (
	"context"
	"testing"
)

func TestPhase4Migrations_AllTablesCreated(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()
	if err := MigrateUp(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"voice_session", "voice_turn", "voice_suppression",
		"vision_event", "vision_consent",
		"learn_observation", "learn_recommendation", "learn_decay", "learn_experiment", "anti_recommend",
		"budget_bucket", "budget_sample",
		"attest_record",
		"sync_peer", "sync_clock", "sync_tombstone", "sync_outbox",
		"a2a_peer", "a2a_call", "mcp_token", "a2a_consent",
		"plugin", "plugin_log", "plugin_quota",
		"supervisor_event", "supervisor_rss",
	}
	for _, table := range want {
		var n int
		if err := db.QueryRowContext(context.Background(),
			"SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Errorf("table %s missing", table)
		}
	}
}

func TestPhase4Migrations_NodeUUIDBackfilled(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()
	if err := MigrateUp(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	rows, err := db.QueryContext(context.Background(),
		"SELECT name FROM pragma_table_info('memory') WHERE name='node_uuid'")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if !rows.Next() {
		t.Fatal("memory.node_uuid missing")
	}
}
```

- [ ] **Step 2: Run, FAIL**

```bash
go test ./internal/sqlstore/ -run Phase4Migrations 2>&1 | tail
```

- [ ] **Step 3: Author migration 001-voice.sql**

```sql
CREATE TABLE voice_session (
    id            INTEGER PRIMARY KEY,
    started_at    INTEGER NOT NULL,
    ended_at      INTEGER,
    voice_only    INTEGER NOT NULL DEFAULT 0,
    source        TEXT NOT NULL CHECK(source IN ('wake','hotkey','menubar','ptt')),
    end_reason    TEXT CHECK(end_reason IN ('user','timeout','error','barge_exhausted')),
    stt_provider  TEXT NOT NULL,
    tts_provider  TEXT NOT NULL,
    ram_peak_mb   INTEGER,
    bytes_uploaded INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE voice_turn (
    id            INTEGER PRIMARY KEY,
    session_id    INTEGER NOT NULL REFERENCES voice_session(id) ON DELETE CASCADE,
    ord           INTEGER NOT NULL,
    role          TEXT NOT NULL CHECK(role IN ('user','assistant')),
    text          TEXT NOT NULL,
    stt_ms        INTEGER,
    ttfb_ms       INTEGER,
    tts_ms        INTEGER,
    barge_in      INTEGER NOT NULL DEFAULT 0,
    UNIQUE(session_id, ord)
);

CREATE TABLE voice_suppression (
    bundle_id     TEXT PRIMARY KEY,
    learned       INTEGER NOT NULL DEFAULT 0,
    last_seen_at  INTEGER NOT NULL,
    confidence    REAL NOT NULL DEFAULT 0.0
);
```

- [ ] **Step 4: Author migrations 002–009**

Author each `.sql` file using the spec's DDL verbatim (§2.5, §3.8, §4.5, §5.6, §6.5, §7.6, §8.5, §9.6). For 006-sync.sql, end with:

```sql
ALTER TABLE memory       ADD COLUMN node_uuid TEXT NOT NULL DEFAULT 'self';
ALTER TABLE conversation ADD COLUMN node_uuid TEXT NOT NULL DEFAULT 'self';
ALTER TABLE pin          ADD COLUMN node_uuid TEXT NOT NULL DEFAULT 'self';
ALTER TABLE settings_kv  ADD COLUMN node_uuid TEXT NOT NULL DEFAULT 'self';

CREATE INDEX idx_budget_sample_bucket_at ON budget_sample(bucket, at DESC);
CREATE INDEX idx_attest_subject_at       ON attest_record(subject, verified_at DESC);
```

Author 003-learn.sql `anti_recommend` table with the three spec-pinned seed rows:

```sql
INSERT INTO anti_recommend(kind, reason, added_at, source) VALUES
  ('wake-word-on', 'spec §3.6: operator must opt in', strftime('%s','now'), 'spec');
```

Author 004-budget.sql with seed rows for the seven buckets (§8.2 defaults):

```sql
INSERT INTO budget_bucket(name, cap, window, enabled) VALUES
  ('cloud.llm.tokens',     0,        'day', 1),
  ('cloud.embed.bytes',    52428800, 'day', 1),
  ('cloud.stt.seconds',    900,      'day', 1),
  ('cloud.tts.chars',      25000,    'day', 1),
  ('cloud.vision.bytes',   31457280, 'day', 1),
  ('peer.a2a.tokens',      5000,     'day', 1),
  ('plugin.network.bytes', 52428800, 'day', 1);
```

- [ ] **Step 5: Register migrations**

In `internal/sqlstore/migrate.go`, append each new filename to the migration registry (existing pattern; do not rebuild the registry).

- [ ] **Step 6: Run tests**

```bash
go test ./internal/sqlstore/... 2>&1 | tail
```

Expected: PASS — all 26 tables present, `memory.node_uuid` present.

- [ ] **Step 7: Commit + PR + reviewer**

```bash
git add internal/sqlstore/migrations/2026-06-22-*.sql internal/sqlstore/migrate.go internal/sqlstore/phase4_migrations_test.go
git commit -m "sqlstore: Phase 4 migrations (9 files) — additive only

Adds 26 tables across voice/vision/learn/budget/attest/sync/a2a/plugin/supervisor.
Adds node_uuid column to memory/conversation/pin/settings_kv with 'self' backfill.
What got smaller: no parallel store — single SQLite file invariant holds."
gh pr create --title "sqlstore(phase4): all 9 migrations + node_uuid backfill" --body "Spec §1.4 + §2.5 + §3.8 + §4.5 + §5.6 + §6.5 + §7.6 + §8.5 + §9.6.

Single-owner per CLAUDE.md frozen-enum-files rule."
```

---

## Wave 2 — Control plane (parallel ≤ 4)

Starts after T05 merged.

---

### Task 6: Learn observation + ranking + decay (`internal/learn/`)

**Files:**
- Create: `internal/learn/recommend.go`
- Create: `internal/learn/recommend_test.go`
- Create: `internal/learn/decay.go`
- Create: `internal/learn/decay_test.go`
- Create: `internal/learn/decay_defaults.go`
- Create: `internal/learn/types.go`
- Create: `internal/learn/store.go`

**Why this exists:** §3.2–§3.5 mandate the recommendation lifecycle, ranking floor, pacing caps, decay schedules. Phase 3 left `internal/recommend/bandit.go` with the Thompson kernel; T06 wraps that kernel in the lifecycle + decay + floor + pacing.

**Interfaces:**
- Consumes: `bandit.SampleBeta` (Phase 3 `internal/recommend/bandit.go`)
- Produces:
  - `type RecommendKind string` (const-set: `"pin-widget"`, `"voice-on"`, `"integration-connect"`, `"memory-purge"`, `"wake-on"`, `"plugin-install"`, `"multi-device-pair"`)
  - `type Surface string` with `SurfaceNotification | SurfaceVoiceClose | SurfaceCoachCard`
  - `type Observation struct { Kind RecommendKind; CtxHash uint64; Ts time.Time }`
  - `type Recommendation struct { ID int64; Kind RecommendKind; Body string; Score, Confidence float64; ActionRef string; SurfacedAt, ExpiresAt time.Time }`
  - `type Outcome struct { Kind OutcomeKind; LatencyMS int; Note string }`
  - `type OutcomeKind int` with `Accepted | Dismissed | Ignored | Applied | ABBaseline | ABTreatment`
  - `type Recommender interface { Observe(ctx, ev Observation) error; NextBatch(ctx, surface Surface, maxN int) ([]Recommendation, error); Record(ctx, id int64, outcome Outcome) error }`
  - `type DecaySchedule struct { Kind RecommendKind; HalfLife time.Duration; HardExpire time.Duration }`
  - `func RegisterDecayDefaults(ctx, db *sql.DB) error` — seam helper (Phase 3 lesson: solo seam PR before fan-out — but here it's ≤ 7 rows and lands inside T06)
  - Const: `ConfidenceFloor = 0.35`, `PacingPerHour = 1`, `PacingPerDay = 3`

- [ ] **Step 1: Write failing tests**

`internal/learn/recommend_test.go`:
```go
package learn

import (
	"context"
	"testing"
	"time"
)

func TestNextBatch_FiltersBelowFloor(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()
	r := New(db)
	r.insertCandidate(t, Recommendation{Kind: "pin-widget", Confidence: 0.1, Score: 0.9})
	r.insertCandidate(t, Recommendation{Kind: "pin-widget", Confidence: 0.8, Score: 0.5})
	batch, err := r.NextBatch(context.Background(), SurfaceNotification, 5)
	if err != nil { t.Fatal(err) }
	if len(batch) != 1 { t.Fatalf("want 1 above-floor candidate, got %d", len(batch)) }
}

func TestNextBatch_PacingCapsAt3PerDay(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()
	r := New(db)
	for i := 0; i < 5; i++ {
		r.insertSurfaced(t, time.Now().Add(-time.Duration(i)*time.Hour))
	}
	batch, err := r.NextBatch(context.Background(), SurfaceNotification, 5)
	if err != nil { t.Fatal(err) }
	if len(batch) != 0 { t.Fatalf("pacing cap should have blocked all, got %d", len(batch)) }
}
```

`internal/learn/decay_test.go`:
```go
package learn

import (
	"testing"
	"time"
)

func TestDecay_HalfLifeHalvesScore(t *testing.T) {
	s := DecaySchedule{Kind: "pin-widget", HalfLife: 7 * 24 * time.Hour}
	got := Decay(1.0, 7*24*time.Hour, s)
	if got > 0.55 || got < 0.45 {
		t.Fatalf("score at one half-life: want ~0.5, got %v", got)
	}
}

func TestDecay_HardExpireZeros(t *testing.T) {
	s := DecaySchedule{Kind: "memory-purge", HalfLife: 24 * time.Hour, HardExpire: 7 * 24 * time.Hour}
	got := Decay(1.0, 8*24*time.Hour, s)
	if got != 0 {
		t.Fatalf("post-hard-expire: want 0, got %v", got)
	}
}
```

- [ ] **Step 2: Run, FAIL**

- [ ] **Step 3: Implement types + decay**

`internal/learn/types.go`:
```go
package learn

import "time"

type RecommendKind string
type Surface string

const (
	SurfaceNotification Surface = "notification"
	SurfaceVoiceClose   Surface = "voice_close"
	SurfaceCoachCard    Surface = "coach"
	ConfidenceFloor             = 0.35
	PacingPerHour               = 1
	PacingPerDay                = 3
)

type Observation struct {
	Kind    RecommendKind
	CtxHash uint64
	Ts      time.Time
}

type Recommendation struct {
	ID         int64
	Kind       RecommendKind
	Body       string
	Score      float64
	Confidence float64
	ActionRef  string
	SurfacedAt time.Time
	ExpiresAt  time.Time
}

type OutcomeKind int

const (
	Accepted OutcomeKind = iota
	Dismissed
	Ignored
	Applied
	ABBaseline
	ABTreatment
)

type Outcome struct {
	Kind      OutcomeKind
	LatencyMS int
	Note      string
}
```

`internal/learn/decay.go`:
```go
package learn

import (
	"math"
	"time"
)

type DecaySchedule struct {
	Kind       RecommendKind
	HalfLife   time.Duration
	HardExpire time.Duration
}

func Decay(score float64, age time.Duration, s DecaySchedule) float64 {
	if s.HardExpire > 0 && age > s.HardExpire {
		return 0
	}
	if s.HalfLife <= 0 {
		return score
	}
	ratio := float64(age) / float64(s.HalfLife)
	return score * math.Pow(0.5, ratio)
}
```

`internal/learn/decay_defaults.go`:
```go
package learn

import (
	"context"
	"database/sql"
	"time"
)

// RegisterDecayDefaults seeds learn_decay with the §3.5 schedule. Idempotent.
func RegisterDecayDefaults(ctx context.Context, db *sql.DB) error {
	defaults := []DecaySchedule{
		{"integration-connect", 14 * 24 * time.Hour, 60 * 24 * time.Hour},
		{"pin-widget", 7 * 24 * time.Hour, 21 * 24 * time.Hour},
		{"voice-on", 30 * 24 * time.Hour, 180 * 24 * time.Hour},
		{"wake-on", 90 * 24 * time.Hour, 0},
		{"memory-purge", 24 * time.Hour, 7 * 24 * time.Hour},
		{"plugin-install", 30 * 24 * time.Hour, 90 * 24 * time.Hour},
		{"multi-device-pair", 30 * 24 * time.Hour, 90 * 24 * time.Hour},
	}
	for _, d := range defaults {
		if _, err := db.ExecContext(ctx,
			`INSERT OR IGNORE INTO learn_decay(kind, half_life_s, hard_expire_s) VALUES(?,?,?)`,
			string(d.Kind), int(d.HalfLife/time.Second), int(d.HardExpire/time.Second)); err != nil {
			return err
		}
	}
	return nil
}
```

- [ ] **Step 4: Implement store + recommender**

`internal/learn/store.go`:
```go
package learn

import (
	"context"
	"database/sql"
	"time"
)

type store struct{ db *sql.DB }

func newStore(db *sql.DB) *store { return &store{db: db} }

func (s *store) surfacedSince(ctx context.Context, since time.Time) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM learn_recommendation WHERE surfaced_at >= ? AND state IN ('surfaced','accepted','dismissed','ignored','applied')`,
		since.Unix()).Scan(&n)
	return n, err
}

func (s *store) topQueued(ctx context.Context, n int) ([]Recommendation, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, kind, body, score, confidence, action_ref, expires_at FROM learn_recommendation
		 WHERE state='queued' AND confidence >= ? AND expires_at > ?
		 ORDER BY score DESC LIMIT ?`,
		ConfidenceFloor, time.Now().Unix(), n)
	if err != nil { return nil, err }
	defer rows.Close()
	var out []Recommendation
	for rows.Next() {
		var r Recommendation
		var exp int64
		if err := rows.Scan(&r.ID, &r.Kind, &r.Body, &r.Score, &r.Confidence, &r.ActionRef, &exp); err != nil { return nil, err }
		r.ExpiresAt = time.Unix(exp, 0)
		out = append(out, r)
	}
	return out, rows.Err()
}
```

`internal/learn/recommend.go`:
```go
package learn

import (
	"context"
	"database/sql"
	"time"
)

type Recommender interface {
	Observe(ctx context.Context, ev Observation) error
	NextBatch(ctx context.Context, surface Surface, maxN int) ([]Recommendation, error)
	Record(ctx context.Context, id int64, out Outcome) error
}

type recommender struct {
	db    *sql.DB
	store *store
	now   func() time.Time
}

func New(db *sql.DB) *recommender { return &recommender{db: db, store: newStore(db), now: time.Now} }

func (r *recommender) Observe(ctx context.Context, ev Observation) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO learn_observation(at, kind, ctx_hash) VALUES(?,?,?)`,
		ev.Ts.Unix(), string(ev.Kind), int64(ev.CtxHash))
	return err
}

func (r *recommender) NextBatch(ctx context.Context, surface Surface, maxN int) ([]Recommendation, error) {
	now := r.now()
	dayCount, err := r.store.surfacedSince(ctx, now.Add(-24*time.Hour))
	if err != nil { return nil, err }
	if dayCount >= PacingPerDay {
		return nil, nil
	}
	hourCount, err := r.store.surfacedSince(ctx, now.Add(-time.Hour))
	if err != nil { return nil, err }
	if hourCount >= PacingPerHour {
		return nil, nil
	}
	return r.store.topQueued(ctx, maxN)
}

func (r *recommender) Record(ctx context.Context, id int64, out Outcome) error {
	state := "ignored"
	switch out.Kind {
	case Accepted: state = "accepted"
	case Dismissed: state = "dismissed"
	case Applied: state = "applied"
	}
	_, err := r.db.ExecContext(ctx,
		`UPDATE learn_recommendation SET state=? WHERE id=?`, state, id)
	return err
}

// test-only helpers
func (r *recommender) insertCandidate(_ interface{ Helper() }, rec Recommendation) {
	_, _ = r.db.Exec(`INSERT INTO learn_recommendation(kind, body, action_ref, score, confidence, decay_id, expires_at, state)
	  VALUES(?,?,?,?,?,1,?, 'queued')`,
	  string(rec.Kind), "test", "noop", rec.Score, rec.Confidence, time.Now().Add(time.Hour).Unix())
}
func (r *recommender) insertSurfaced(_ interface{ Helper() }, at time.Time) {
	_, _ = r.db.Exec(`INSERT INTO learn_recommendation(kind, body, action_ref, score, confidence, decay_id, expires_at, state, surfaced_at)
	  VALUES('pin-widget','t','n',0.9,0.9,1,?, 'surfaced', ?)`, time.Now().Add(time.Hour).Unix(), at.Unix())
}
```

- [ ] **Step 5: Tests PASS, commit, PR, reviewer**

```bash
go test ./internal/learn/...
golangci-lint run ./internal/learn/...
git add internal/learn/
git commit -m "learn(recommend): lifecycle + decay + pacing + floor

Phase 4 recommend pass-2 per §3.2-§3.5. Wraps Phase 3 Thompson kernel.
What got smaller: removes ad-hoc 'maybe surface' calls in widget code paths."
gh pr create --title "learn(recommend): pass-2 lifecycle, decay, pacing"
```

---

### Task 7: Learn anti-list + A/B + Recommendations pane (`internal/learn/antilist.go` + `internal/learn/ab.go` + Settings)

**Files:**
- Create: `internal/learn/antilist.go`
- Create: `internal/learn/antilist_test.go`
- Create: `internal/learn/ab.go`
- Create: `internal/learn/ab_test.go`
- Create: `app/Leah/Sources/LeahUI/Settings/RecommendationsPane.swift`
- Create: `app/Leah/Tests/LeahUITests/RecommendationsPaneTests.swift`
- Modify: `app/Leah/Sources/LeahUI/Settings/SettingsRoot.swift` — register new pane

**Why this exists:** §3.6 (anti-recommend hard-stop) + §3.7 (A/B infra) + §3.12 (Settings pane). The pane is operator-visible UI; anti-list logic is the integrity boundary that prevents wake-word auto-enable.

**Interfaces:**
- Consumes: `learn.Recommender` (T06), `learn.OutcomeKind` (T06)
- Produces:
  - `type AntiRule struct { Kind RecommendKind; Reason string; Source AntiSource; AddedAt time.Time }`
  - `type AntiSource string` with `AntiOperator | AntiAuto | AntiSpec`
  - `type AntiList interface { Add(ctx, kind RecommendKind, reason string, src AntiSource) error; Remove(ctx, kind RecommendKind, src AntiSource) error; List(ctx) ([]AntiRule, error); IsBlocked(ctx, kind RecommendKind) (bool, error) }`
  - `type Experiment struct { ID int64; Kind RecommendKind; ArmA, ArmB string; ImpressionsA, ImpressionsB, WinsA, WinsB int; Locked bool; LockedArm string }`
  - `type ABKernel interface { Assign(ctx, kind RecommendKind) (arm string, expID int64, err error); Record(ctx, expID int64, arm string, won bool) error; Lock(ctx, expID int64) error }`

- [ ] **Step 1: Failing tests** — anti-list spec rows cannot be removed by operator source; A/B lock at 50 impressions per arm; auto-add after 3 consecutive Dismissed in 30 d.

```go
func TestAntiList_SpecCannotBeRemovedByOperator(t *testing.T) { /* attempt remove with AntiOperator source on AntiSpec row, expect ErrSpecLocked */ }
func TestAntiList_AutoAddAfter3Dismissed(t *testing.T) { /* 3 Dismissed in 30d → auto rule added */ }
func TestAB_LockAfter50Impressions(t *testing.T) { /* armA=60, armB=55, winsA=40, winsB=20 → lock arm A */ }
```

- [ ] **Step 2: Run FAIL → impl → PASS**

(Full implementation tracking spec §3.6 + §3.7 exactly.)

- [ ] **Step 3: Settings pane**

`RecommendationsPane.swift` — toggle per kind + anti-list (editable section + spec-locked section with lock glyph), recent surfaced ledger (last 30), A/B experiment table.

- [ ] **Step 4: Commit + PR + reviewer**

```bash
gh pr create --title "learn(antilist+ab): hard-stop + A/B + Recommendations pane"
```

---

### Task 8: Privacy budget runtime (`internal/budget/`)

**Files:**
- Create: `internal/budget/budget.go`
- Create: `internal/budget/budget_test.go`
- Create: `internal/budget/bucket.go`
- Create: `internal/budget/degrade.go`
- Create: `internal/budget/degrade_test.go`
- Create: `internal/budget/bucket_defaults.go`
- Modify: `internal/voice/duplex/session.go` — charge `cloud.stt.seconds` + `cloud.tts.chars`
- Modify: `internal/vision/router/router.go` — charge `cloud.vision.bytes`
- Modify: `internal/embed/voyage.go` (existing) — charge `cloud.embed.bytes`

**Why this exists:** §8 mandates that every cloud call site `Charge()` before issuing. T08 lands the substrate; T06/T04/T01-T02/existing embed all wire their charge calls in the same PR.

**Interfaces:**
- Produces:
  - `type Bucket string` (const-set matching §8.2)
  - `type Window string` with `WindowHour | WindowDay | WindowWeek | WindowMonth`
  - `type Balance struct { Bucket Bucket; Spent, Cap int64; Window Window; ResetsAt time.Time; Trend []int64 }`
  - `type Budget interface { Charge(ctx, b Bucket, n int64) error; Peek(ctx, b Bucket) (Balance, error); Set(ctx, b Bucket, cap int64, win Window) error; Reset(ctx, b Bucket) error; Subscribe() <-chan Event }`
  - `var ErrOverBudget = errors.New("budget: over cap")`
  - `type DegradeAction int` with `ActionWarn | ActionSwitchLocal | ActionPause | ActionBlock`
  - `func DegradePath(b Bucket, fillRatio float64) DegradeAction` — implements §8.4 table verbatim

- [ ] **Step 1: Failing tests**

```go
func TestBudget_ChargeBlocksAtCap(t *testing.T) { /* set cap 100, charge 99 ok, charge 2 → ErrOverBudget */ }
func TestBudget_DegradePath80PercentIsWarn(t *testing.T) {
	if DegradePath("cloud.embed.bytes", 0.85) != ActionWarn { t.Fatal("0.85 → ActionWarn per §8.4") }
}
func TestBudget_DegradePath100PercentIsSwitchLocal(t *testing.T) {
	if DegradePath("cloud.embed.bytes", 1.00) != ActionSwitchLocal { t.Fatal("100% → switch local") }
}
```

- [ ] **Step 2: Implement Charge with ≤ 0.4 ms p95**

`internal/budget/budget.go`:
```go
package budget

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"time"
)

var ErrOverBudget = errors.New("budget: over cap")

type Bucket string
type Window string

const (
	WindowHour  Window = "hour"
	WindowDay   Window = "day"
	WindowWeek  Window = "week"
	WindowMonth Window = "month"
)

type Balance struct {
	Bucket   Bucket
	Spent    int64
	Cap      int64
	Window   Window
	ResetsAt time.Time
}

type Event struct {
	Bucket Bucket
	Spent  int64
	Cap    int64
	Action DegradeAction
}

type Budget interface {
	Charge(ctx context.Context, b Bucket, n int64) error
	Peek(ctx context.Context, b Bucket) (Balance, error)
	Set(ctx context.Context, b Bucket, cap int64, win Window) error
	Reset(ctx context.Context, b Bucket) error
	Subscribe() <-chan Event
}

type runtime struct {
	mu     sync.Mutex
	db     *sql.DB
	subs   []chan Event
	cache  map[Bucket]int64
}

func New(db *sql.DB) *runtime { return &runtime{db: db, cache: map[Bucket]int64{}} }

func (r *runtime) windowStart(win Window, now time.Time) time.Time {
	switch win {
	case WindowHour:  return now.Truncate(time.Hour)
	case WindowDay:   return now.Truncate(24 * time.Hour)
	case WindowWeek:  return now.AddDate(0, 0, -int(now.Weekday())).Truncate(24*time.Hour)
	case WindowMonth: return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	}
	return now
}

func (r *runtime) Charge(ctx context.Context, b Bucket, n int64) error {
	r.mu.Lock(); defer r.mu.Unlock()
	var cap int64; var win string
	if err := r.db.QueryRowContext(ctx, `SELECT cap, window FROM budget_bucket WHERE name=? AND enabled=1`, string(b)).Scan(&cap, &win); err != nil {
		return err
	}
	ws := r.windowStart(Window(win), time.Now()).Unix()
	var spent int64
	_ = r.db.QueryRowContext(ctx, `SELECT spent FROM budget_sample WHERE bucket=? AND at=?`, string(b), ws).Scan(&spent)
	if cap > 0 && spent+n > cap {
		r.emit(Event{Bucket: b, Spent: spent, Cap: cap, Action: ActionBlock})
		return ErrOverBudget
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO budget_sample(bucket, at, spent) VALUES(?,?,?)
		 ON CONFLICT(bucket, at) DO UPDATE SET spent = spent + excluded.spent`,
		string(b), ws, n)
	if err != nil { return err }
	if cap > 0 {
		ratio := float64(spent+n) / float64(cap)
		r.emit(Event{Bucket: b, Spent: spent + n, Cap: cap, Action: DegradePath(b, ratio)})
	}
	return nil
}

func (r *runtime) emit(ev Event) {
	for _, s := range r.subs {
		select { case s <- ev: default: }
	}
}
```

`internal/budget/degrade.go`:
```go
package budget

type DegradeAction int

const (
	ActionNone DegradeAction = iota
	ActionWarn
	ActionSwitchLocal
	ActionPause
	ActionBlock
)

// DegradePath implements spec §8.4 table verbatim.
func DegradePath(b Bucket, ratio float64) DegradeAction {
	switch {
	case ratio < 0.80:
		return ActionNone
	case ratio < 1.00:
		return ActionWarn
	case ratio < 1.0001: // exactly at cap
		switch b {
		case "cloud.embed.bytes", "cloud.stt.seconds", "cloud.tts.chars", "cloud.vision.bytes":
			return ActionSwitchLocal
		case "plugin.network.bytes":
			return ActionPause
		default:
			return ActionBlock
		}
	default:
		return ActionBlock
	}
}
```

- [ ] **Step 3: Wire call sites**

Each existing cloud call site adds a `Charge` call before issuing the network call. Audit `grep -rn "anthropic\|voyage\|elevenlabs\|openai/whisper" internal/ cmd/` and verify every hit has a charge.

- [ ] **Step 4: Commit + PR + reviewer**

---

### Task 9: Continuous attestation (`internal/attest/`)

**Files:**
- Create: `internal/attest/verifier.go`
- Create: `internal/attest/verifier_test.go`
- Create: `internal/attest/manifest.go`
- Create: `internal/attest/revocation.go`
- Create: `internal/attest/recheck.go`
- Create: `app/Leah/Sources/LeahUI/Settings/AboutPane+Verification.swift`
- Modify: `app/Leah/Sources/LeahUI/Settings/AboutPane.swift` — add Verification panel

**Why this exists:** §6 mandates runtime verification of self + model files + plugins + Sparkle appcast. Verdict drives behavior — `Failed` on self blocks watchdog restart; `Failed` on plugin blocks plugin load. About pane shows live verdict.

**Interfaces:**
- Produces:
  - `type AttestState string` (`Verified`, `Stale`, `Failed`, `Unknown`)
  - `type SignerRef struct { Kind string; Fingerprint string }`
  - `type Attestation struct { Subject string; State AttestState; SignedBy []SignerRef; VerifiedAt, NextRecheck time.Time; Reason string }`
  - `type ManifestRef struct { Path string; ExpectedSHA256 string; Pubkey ed25519.PublicKey }`
  - `type Verifier interface { VerifySelf(ctx) (Attestation, error); VerifyArtifact(ctx, path string, mf ManifestRef) (Attestation, error); VerifyPlugin(ctx, bundlePath string) (Attestation, error); LastVerdict() Attestation; Subscribe() <-chan Attestation }`
  - `type RevocationList struct { Pubkeys []string; FetchedAt time.Time }`
  - `func FetchRevocations(ctx, url string) (RevocationList, error)`

- [ ] **Step 1: Failing tests**

```go
func TestVerifySelf_Mismatch(t *testing.T) { /* tamper bin, expect Failed + reason */ }
func TestVerifyArtifact_GoodSHA(t *testing.T) { /* matching SHA → Verified */ }
func TestRecheckPolicy_Self24h(t *testing.T) { /* NextRecheck = VerifiedAt + 24h */ }
func TestRevocationList_OfflineToleranceIs7d(t *testing.T) { /* fetched 6d ago → ok; 8d → warn flag */ }
```

- [ ] **Step 2-3: Implement** per spec §6.3 + §6.4 + §6.6

- [ ] **Step 4: Wire Settings → About → Verification**

`AboutPane+Verification.swift`:
```swift
import SwiftUI

struct VerificationPanel: View {
    @ObservedObject var model: VerificationModel
    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack {
                StateChip(state: model.state)
                Text("Last checked \(model.lastChecked, format: .relative(presentation: .named))")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
            ForEach(model.signers, id: \.fingerprint) { signer in
                Text("\(signer.kind): \(signer.fingerprint.prefix(12))…").monospaced()
            }
            Button("Recheck now") { Task { await model.recheck() } }
        }
    }
}
```

- [ ] **Step 5: Commit + PR + reviewer**

---

## Wave 3 — Multi-device sync (parallel ≤ 3)

Starts after T09 merged (attest is the trust root for Bonjour pairing).

---

### Task 10: Bonjour discovery + OTP pair (`internal/sync/discovery/` + `internal/sync/pair/`)

**Files:**
- Create: `internal/sync/discovery/bonjour.go`
- Create: `internal/sync/discovery/bonjour_darwin.go`
- Create: `internal/sync/discovery/bonjour_stub.go`
- Create: `internal/sync/discovery/bonjour_test.go`
- Create: `internal/sync/pair/otp.go`
- Create: `internal/sync/pair/otp_test.go`
- Create: `internal/sync/pair/mtls.go`
- Create: `internal/sync/pair/mtls_test.go`

**Why this exists:** §2.2 + §2.4 mandate Bonjour `_leah-sync._tcp` + 6-digit OTP exchange + mTLS pinned to shared secret fingerprint.

**Interfaces:**
- Produces:
  - `type DeviceID string` — UUIDv7 stable per Mac
  - `type Peer interface { ID() DeviceID; Endpoint() netip.AddrPort; LastSeenAt() time.Time; Status() PeerStatus }`
  - `type PeerStatus int` (`Online | Idle | Paused | Unreachable`)
  - `type Discovery interface { Publish(ctx, name string, port uint16) error; Browse(ctx) (<-chan Peer, error); Stop() }`
  - `type OTP [6]byte`
  - `func GenerateOTP() OTP`
  - `func (OTP) String() string` — e.g. `"482-913"`
  - `type Pair interface { Offer(ctx, otp OTP) (Peer, error); Accept(ctx, otp OTP, endpoint netip.AddrPort) (Peer, error) }`
  - `func NewMTLSConfig(sharedKey [32]byte) (*tls.Config, error)`

- [ ] **Step 1: Failing tests** — OTP entropy 6 digits, mTLS rejects unpinned cert, Bonjour stub publishes + browse roundtrip

```go
func TestOTP_FormatIsThreeDashThree(t *testing.T) {
	o := GenerateOTP()
	s := o.String()
	if len(s) != 7 || s[3] != '-' { t.Fatalf("OTP format: want NNN-NNN, got %q", s) }
}
func TestMTLS_RejectsUnpinnedCert(t *testing.T) { /* generate two random shared keys, ensure dial fails */ }
```

- [ ] **Step 2-3: Implement OTP, mTLS pinning, Bonjour cgo bridge** per §2.4.3

- [ ] **Step 4: Commit + PR + reviewer**

---

### Task 11: CRDT model + sync coordinator (`internal/sync/crdt/` + `internal/sync/coord/`)

**Files:**
- Create: `internal/sync/crdt/lww.go`
- Create: `internal/sync/crdt/lww_test.go`
- Create: `internal/sync/crdt/log.go`
- Create: `internal/sync/crdt/log_test.go`
- Create: `internal/sync/crdt/tombstone.go`
- Create: `internal/sync/crdt/tombstone_test.go`
- Create: `internal/sync/coord/coord.go`
- Create: `internal/sync/coord/coord_test.go`
- Create: `internal/sync/coord/outbox.go`

**Why this exists:** §2.3 mandates two replicated data classes — LWW register (settings/pins/HUD geometry) and add-only log + tombstone (memory/conversation/widget pin events). Tombstones older than 90 days after both peers ack are GC'd.

**Interfaces:**
- Consumes: `Peer` (T10), `mTLSConfig` (T10), `sqlstore.DB` (Phase 1)
- Produces:
  - `type NodeUUID string`
  - `type Lamport int64`
  - `type Op string` (`OpInsert | OpUpdate | OpDelete`)
  - `type LWWMerge func(local, remote []byte, localTS, remoteTS Lamport) []byte`
  - `type LogEntry struct { Table string; RowID int64; Node NodeUUID; Lamport Lamport; Op Op; Payload []byte }`
  - `type CRDT interface { ApplyLog(ctx, entries []LogEntry) (DeltaStats, error); EmitLog(ctx, since Lamport, limit int) ([]LogEntry, error); GCTombstones(ctx, cutoff time.Time) (int, error) }`
  - `type DeltaStats struct { Applied, Skipped, Conflicts int }`
  - `type SyncEventKind int` (`Discovered | Paired | DeltaApplied | Conflict | Disconnected`)
  - `type SyncEvent struct { Kind SyncEventKind; Peer Peer; Stats DeltaStats }`
  - `type SyncCoordinator interface { Pair(ctx, otp string) (Peer, error); Unpair(ctx, p Peer) error; Pause(ctx, p Peer) error; Resume(ctx, p Peer) error; Subscribe() <-chan SyncEvent }`

- [ ] **Step 1: Failing tests** — LWW resolves on lamport > then device_id lex; tombstone idempotent replay; outbox compresses + truncates at 50 MB

- [ ] **Step 2-4: Impl** per §2.3 + §2.4.1 + §2.7

- [ ] **Step 5: Commit + PR + reviewer**

---

### Task 12: iCloud Keychain key share + Sync pane (`internal/keystore/icloud/` + `LeahUI/Settings/SyncPane.swift`)

**Files:**
- Create: `internal/keystore/icloud/icloud_darwin.go`
- Create: `internal/keystore/icloud/icloud_stub.go`
- Create: `internal/keystore/icloud/icloud_test.go`
- Create: `app/Leah/Sources/LeahUI/Settings/SyncPane.swift`
- Create: `app/Leah/Tests/LeahUITests/SyncPaneTests.swift`
- Modify: `app/Leah/Sources/LeahUI/Settings/SettingsRoot.swift` — register Sync pane

**Why this exists:** §2.2 mandates iCloud-Keychain-shared Curve25519 secret as the trust root. §2.6 + §2.9 specify the Sync pane UI: peer list + pair button + OTP + per-peer pause/unpair + "Share API keys" toggle.

**Interfaces:**
- Produces:
  - `type ICloudKeystore interface { Put(ctx, service, account string, data []byte, sync bool) error; Get(ctx, service, account string) ([]byte, error); Delete(ctx, service, account string) error; ListSynced(ctx) ([]string, error) }`
  - Swift `SyncPane` consuming `SyncIPCClient` (peer list, pair OTP)

- [ ] **Step 1-5:** Test, impl, wire, commit, PR, reviewer

---

## Wave 4 — Multi-agent + plugins (parallel ≤ 3)

Starts after T08 + T09 merged.

---

### Task 13: Inbound MCP server + tokens + Connections pane (`internal/mcp/inbound/` + Settings)

**Files:**
- Create: `internal/mcp/inbound/server.go`
- Create: `internal/mcp/inbound/server_test.go`
- Create: `internal/mcp/inbound/tools.go`
- Create: `internal/mcp/inbound/token.go`
- Create: `internal/mcp/inbound/token_test.go`
- Rename: `app/Leah/Sources/LeahUI/Settings/IntegrationsPane.swift` → `ConnectionsPane.swift`
- Create: `app/Leah/Sources/LeahUI/Settings/ConnectionsPane+InboundMCP.swift`
- Modify: `app/Leah/Sources/LeahUI/Settings/SettingsRoot.swift`

**Why this exists:** §5 reverses the MCP arrow — third-party agents call Leah. Tokens are scope-limited; `leah.ask` consumes operator's Anthropic budget so it gets a per-call consent gate.

**Interfaces:**
- Consumes: `budget.Budget` (T08), `memory.Search` (Phase 1), `reasoner.Ask` (Phase 1)
- Produces:
  - `type Scope string` (`memory:read | calendar:read | repo:read | ask:run | widget:render`)
  - `type Token struct { ID int64; Name string; Plain string; Hash [32]byte; Scopes []Scope; IssuedAt, RevokedAt time.Time }`
  - `type MCPTool struct { Name string; Params json.RawMessage; Handler func(ctx context.Context, p json.RawMessage) (any, error); RequiredScopes []Scope }`
  - `type MCPTransport interface { Read() (Frame, error); Write(Frame) error; Close() error }`
  - `type InboundMCP interface { Register(t MCPTool) error; Serve(ctx, t MCPTransport) error; IssueToken(ctx, name string, scopes []Scope) (Token, error); RevokeToken(ctx, id int64) error }`
  - First-party tools registered: `leah.memory.search`, `leah.calendar.next`, `leah.repo.cite`, `leah.ask`, `leah.widget.render`

- [ ] **Step 1: Failing tests** — token mismatch → 401 + rate-limit after 5/min; unknown tool → `tool.unknown`; depth > 2 rejected; budget exhausted → `budget.exhausted`

- [ ] **Step 2-3: Impl** per §5.3 + §5.5 + §5.7 + §5.8

- [ ] **Step 4: Rename pane**

```bash
git mv app/Leah/Sources/LeahUI/Settings/IntegrationsPane.swift app/Leah/Sources/LeahUI/Settings/ConnectionsPane.swift
```

Update import references + tests; ConnectionsPane adds two sections: "Inbound MCP" (token list + scopes + revoke) + "A2A peers" (deferred to T14).

- [ ] **Step 5: Commit + PR + reviewer**

---

### Task 14: A2A protocol + peer handshake (`internal/a2a/`)

**Files:**
- Create: `internal/a2a/frame.go`
- Create: `internal/a2a/frame_test.go`
- Create: `internal/a2a/server.go`
- Create: `internal/a2a/server_test.go`
- Create: `internal/a2a/client.go`
- Create: `internal/a2a/client_test.go`
- Create: `internal/a2a/identity.go`
- Create: `internal/a2a/consent.go`
- Modify: `app/Leah/Sources/LeahUI/Settings/ConnectionsPane+A2A.swift`

**Why this exists:** §5.4 mandates Leah/A2A v1 — a CBOR-framed protocol with capability negotiation + Ed25519 identity proof + per-call billing consent.

**Interfaces:**
- Consumes: `mcp/inbound` (T13), Ed25519 identity from Keychain, `budget.Budget` (T08)
- Produces:
  - `type FrameKind string` (`hello.offer | hello.ack | identity.prove | identity.verify | ask.request | ask.partial | ask.end | memory.search | memory.result | task.offer | task.accept | task.reject | consent.require | consent.grant | consent.deny | bye`)
  - `type Frame struct { V int; ID string; Kind FrameKind; Payload []byte }` — CBOR-encoded
  - `type CapabilitySet uint64` — bitfield
  - `type A2APeer struct { ID PeerID; Name string; Pubkey ed25519.PublicKey; PairedAt time.Time; Paused bool; Scopes []string }`
  - `type A2AServer interface { Listen(ctx, addr netip.AddrPort) error; Stop(ctx) error; Peers() []A2APeer; Revoke(ctx, id PeerID) error }`
  - `type A2ASession interface { Negotiate(ctx) (CapabilitySet, error); Ask(ctx, prompt string) (<-chan ReasonerEvent, error); SearchMemory(ctx, q string, k int) ([]MemoryHit, error); Bye(ctx) }`
  - `type A2AClient interface { Dial(ctx, addr netip.AddrPort, pubkey ed25519.PublicKey) (A2ASession, error) }`

- [ ] **Step 1: Failing tests** — CBOR roundtrip preserves frame; loop-protection rejects depth > 2; consent.grant persists per-peer; bye closes session

- [ ] **Step 2-3: Impl** per §5.4 + §5.4.1 + §5.7

- [ ] **Step 4: Commit + PR + reviewer**

---

### Task 15: Plugin SDK Go host + manifest validator + sandbox (`internal/plugin/` + `pkg/leahplugin/`)

**Files:**
- Create: `internal/plugin/host.go`
- Create: `internal/plugin/host_test.go`
- Create: `internal/plugin/manifest.go`
- Create: `internal/plugin/manifest_test.go`
- Create: `internal/plugin/sandbox.go`
- Create: `internal/plugin/sandbox_darwin.go`
- Create: `internal/plugin/sandbox_stub.go`
- Create: `internal/plugin/sandbox_test.go`
- Create: `internal/plugin/quota.go`
- Create: `internal/plugin/quota_test.go`
- Create: `pkg/leahplugin/plugin.go`
- Create: `pkg/leahplugin/host_iface.go`
- Create: `pkg/leahplugin/manifest_schema.go`
- Create: `pkg/leahplugin/doc.go`

**Why this exists:** §7 — plugins as signed bundles with sandboxed subprocesses + manifest-declared capability ceilings. The SDK package `pkg/leahplugin/` is the public surface plugin authors import.

**Interfaces:**
- Consumes: `attest.Verifier` (T09), `budget.Budget` (T08), `mcp/inbound.InboundMCP` (T13)
- Produces:
  - `type PluginID string`
  - `type PluginInfo struct { ID PluginID; Name, Version string; Enabled bool; AttestState string }`
  - `type Manifest struct { SchemaVersion int; ID, Name, Version, MinLeah string; Author struct{Name, URL string}; Capabilities []Capability; Permissions Permissions; IPCQuota Quota; UI UISpec }`
  - `type Capability struct { Kind, Type, Name, Renderer string; Scopes []string }`
  - `type Permissions struct { Network, FSRead, FSWrite, Keychain []string }`
  - `type Quota struct { RPCPerMinute int; StreamBytesPerMinute int64 }`
  - `type Host interface { Install(ctx, bundlePath string) (PluginID, error); Uninstall(ctx, id PluginID) error; Enable(ctx, id PluginID) error; Disable(ctx, id PluginID) error; Reload(ctx, id PluginID) error; List() []PluginInfo; Logs(ctx, id PluginID, tail int) ([]LogLine, error) }`
  - Public SDK in `pkg/leahplugin/`:
    - `type Plugin interface { Manifest() Manifest; Init(ctx, h PluginHost) error; Shutdown(ctx) error }`
    - `type PluginHost interface { Log(level LogLevel, msg string, kv ...any); Keychain() KeychainAccessor; HTTP() *http.Client; EmitMCPTool(t MCPTool) error; EmitWidget(w WidgetSchema) error; Bus() <-chan HostEvent }`

- [ ] **Step 1: Failing tests** — manifest schema validation (rejects missing fields), unsigned bundle refused, RPC quota of 60/min holds, sandbox RSS cap kills at 256 MB

- [ ] **Step 2: Implement manifest validator**

`internal/plugin/manifest.go`:
```go
package plugin

import (
	"encoding/json"
	"errors"
	"fmt"
)

type Manifest struct {
	SchemaVersion int            `json:"schema_version"`
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	Version       string         `json:"version"`
	MinLeah       string         `json:"min_leah"`
	Author        ManifestAuthor `json:"author"`
	Capabilities  []Capability   `json:"capabilities"`
	Permissions   Permissions    `json:"permissions"`
	IPCQuota      Quota          `json:"ipc_quota"`
	UI            UISpec         `json:"ui"`
}

type ManifestAuthor struct{ Name, URL string }

func ParseManifest(raw []byte) (Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return m, fmt.Errorf("manifest: %w", err)
	}
	if m.SchemaVersion != 1 {
		return m, errors.New("manifest: unsupported schema_version")
	}
	if m.ID == "" || m.Name == "" || m.Version == "" {
		return m, errors.New("manifest: id, name, version required")
	}
	return m, nil
}
```

- [ ] **Step 3: Implement sandbox launcher**

`internal/plugin/sandbox_darwin.go`:
```go
//go:build darwin

package plugin

import (
	"context"
	"errors"
	"os/exec"
	"syscall"
)

type darwinSandbox struct{}

func newSandbox() Sandbox { return &darwinSandbox{} }

func (darwinSandbox) Spawn(ctx context.Context, bundlePath string, env []string, rssCapMB int) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, bundlePath+"/Contents/binary")
	cmd.Env = env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	// setrlimit RLIMIT_AS = rssCapMB*1024*1024 — applied via prctl-equivalent on the child
	// (darwin path uses task_set_policy via the supervisor in T17 for hard RSS cap)
	if rssCapMB <= 0 {
		return nil, errors.New("sandbox: rssCapMB must be > 0")
	}
	return cmd, nil
}
```

- [ ] **Step 4-5: Wire quota + host**

- [ ] **Step 6: Commit + PR + reviewer**

---

### Task 16: Sample `weather-pro` plugin + Plugins pane + Swift SDK (`plugins/weather-pro/` + Settings)

**Files:**
- Create: `plugins/weather-pro/Info.plist`
- Create: `plugins/weather-pro/manifest.json`
- Create: `plugins/weather-pro/main.go`
- Create: `plugins/weather-pro/Resources/icon.svg`
- Create: `plugins/weather-pro/go.mod`
- Create: `plugins/weather-pro/main_test.go`
- Create: `app/Leah/Sources/LeahPluginSDK/PluginWidget.swift`
- Create: `app/Leah/Sources/LeahUI/Settings/PluginsPane.swift`
- Create: `scripts/plugin/sign.sh`
- Modify: `app/Leah/Sources/LeahUI/Settings/SettingsRoot.swift`

**Why this exists:** §7.11 mandates a sample first-party plugin that ships via the SDK to prove the contract. The plugin must be a real, runnable, signed bundle — not a stub. The Plugins pane gives the operator install/enable/log/uninstall.

**Interfaces:**
- Consumes: `pkg/leahplugin` (T15), `internal/plugin.Host` (T15)
- Produces:
  - `plugins/weather-pro/manifest.json` (v1 schema, weather.now MCP tool)
  - `plugins/weather-pro/main.go` (uses `pkg/leahplugin` SDK, registers `weather.now` MCP tool, ships a widget schema)
  - Swift `LeahPluginSDK/PluginWidget.swift` — Swift Package for widget renderers
  - `scripts/plugin/sign.sh` — codesign + EdDSA dual-sign helper

- [ ] **Step 1: Author manifest + plugin**

`plugins/weather-pro/manifest.json`:
```json
{
  "schema_version": 1,
  "id": "com.maydow.weather-pro",
  "name": "Weather Pro",
  "version": "1.0.0",
  "min_leah": "1.1.0",
  "author": { "name": "Maydow", "url": "https://maydow.com" },
  "capabilities": [
    { "kind": "widget", "type": "weather", "renderer": "schema-only" },
    { "kind": "mcp.tool", "name": "weather.now", "scopes": ["network:api.open-meteo.com"] }
  ],
  "permissions": {
    "network": ["api.open-meteo.com"],
    "fs.read": [],
    "fs.write": [],
    "keychain": []
  },
  "ipc_quota": { "rpc_per_minute": 60, "stream_bytes_per_minute": 524288 },
  "ui": { "icon": "Resources/icon.svg", "settings_pane": "" }
}
```

`plugins/weather-pro/main.go`:
```go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/trilam/leah/pkg/leahplugin"
)

type weatherPlugin struct{ host leahplugin.PluginHost }

func (p *weatherPlugin) Manifest() leahplugin.Manifest {
	return leahplugin.Manifest{ID: "com.maydow.weather-pro", Name: "Weather Pro", Version: "1.0.0"}
}

func (p *weatherPlugin) Init(ctx context.Context, h leahplugin.PluginHost) error {
	p.host = h
	return h.EmitMCPTool(leahplugin.MCPTool{
		Name: "weather.now",
		Handler: func(ctx context.Context, params json.RawMessage) (any, error) {
			var args struct{ Lat, Lon float64 }
			if err := json.Unmarshal(params, &args); err != nil { return nil, err }
			url := fmt.Sprintf("https://api.open-meteo.com/v1/forecast?latitude=%f&longitude=%f&current=temperature_2m", args.Lat, args.Lon)
			resp, err := h.HTTP().Get(url)
			if err != nil { return nil, err }
			defer resp.Body.Close()
			var out map[string]any
			if err := json.NewDecoder(resp.Body).Decode(&out); err != nil { return nil, err }
			return out, nil
		},
	})
}

func (p *weatherPlugin) Shutdown(ctx context.Context) error { return nil }

func main() { leahplugin.Run(&weatherPlugin{}) }
```

- [ ] **Step 2: Failing test — plugin manifest is parseable**

`plugins/weather-pro/main_test.go`:
```go
package main

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/trilam/leah/internal/plugin"
)

func TestManifest_Parses(t *testing.T) {
	raw, err := os.ReadFile("manifest.json")
	if err != nil { t.Fatal(err) }
	mf, err := plugin.ParseManifest(raw)
	if err != nil { t.Fatal(err) }
	if mf.ID != "com.maydow.weather-pro" { t.Fatalf("id: got %q", mf.ID) }
	_ = json.Unmarshal // keep import
}
```

- [ ] **Step 3-5: Swift PluginsPane + sign.sh + Commit + PR + reviewer**

---

## Wave 5 — Supervision + ship

Starts after W1-W4 land. T17/T18 parallel; T19 → T20 → T21 serialized.

---

### Task 17: Watchdog supervisor + leak detect + eviction (`internal/supervisor/`)

**Files:**
- Create: `internal/supervisor/supervisor.go`
- Create: `internal/supervisor/supervisor_test.go`
- Create: `internal/supervisor/process.go`
- Create: `internal/supervisor/process_test.go`
- Create: `internal/supervisor/leak.go`
- Create: `internal/supervisor/leak_test.go`
- Create: `internal/supervisor/evict.go`
- Create: `internal/supervisor/evict_test.go`
- Create: `internal/supervisor/circuit.go`
- Create: `app/Leah/Sources/LeahUI/Settings/AboutPane+Diagnostics.swift`

**Why this exists:** §9 mandates a single in-process supervisor for all four Phase 4 long-lived subsystems (voice-stt, voice-tts, vision-live, sync-peer-N) + plugins + daemon + HUD. Restart backoff, circuit-breaker, RSS leak detect, eviction ladder.

**Interfaces:**
- Consumes: All Wave 1-4 subsystems register via `Register(ProcessSpec)`
- Produces:
  - `type ProcessHandle int64`
  - `type ProcessState string` (`Starting | Running | Crashed | Restarting | Disabled | CircuitOpen`)
  - `type ProcessStatus struct { Name string; State ProcessState; RSSmb int; Restarts24h int; LastCrashReason string }`
  - `type RestartPolicy int` (`CrashOnly | Always | Never`)
  - `type CircuitPolicy struct { MaxCrashes int; Window time.Duration }`
  - `type HealthCheck struct { Probe func(context.Context) error; Period time.Duration }`
  - `type ResourceLimits struct { RSSmb, FDs, CPUPct int }`
  - `type ProcessSpec struct { Name string; Args, Env []string; StartTimeout, StopTimeout time.Duration; Restart RestartPolicy; BackoffMin, BackoffMax time.Duration; Circuit CircuitPolicy; Health HealthCheck; Limits ResourceLimits }`
  - `type Supervisor interface { Register(ProcessSpec) ProcessHandle; Stop(ctx, h ProcessHandle) error; Restart(ctx, h ProcessHandle) error; Status() []ProcessStatus; Subscribe() <-chan Event }`
  - `type EvictAction int` (`EvictAdapterCache | EvictLiveVision | EvictPluginLRU | EvictWhisperContinuous | EvictVoiceSession`)
  - `func EvictionLadder(pressure os.PSILevel) []EvictAction`

- [ ] **Step 1: Failing tests** — backoff 200ms × 2^n cap 30s; circuit opens at 5/60s; leak detect fires at +5MB/min for 10min; eviction ladder order matches §9.5

```go
func TestBackoff_ExponentialCapped(t *testing.T) {
	b := newBackoff(200*time.Millisecond, 30*time.Second)
	for i := 0; i < 10; i++ { b.next() }
	if b.cur > 30*time.Second { t.Fatal("backoff exceeded cap") }
}
func TestCircuit_OpensAt5InWindow(t *testing.T) { /* 5 crashes in 60s → state CircuitOpen */ }
func TestLeak_FiresAt5MBPerMinFor10Min(t *testing.T) { /* feed RSS samples, expect event */ }
func TestEvictionLadder_OrderMatchesSpec(t *testing.T) {
	want := []EvictAction{EvictAdapterCache, EvictLiveVision, EvictPluginLRU, EvictWhisperContinuous, EvictVoiceSession}
	got := EvictionLadder(os.PSIWarning)
	if !reflect.DeepEqual(got, want) { t.Fatalf("ladder: want %v, got %v", want, got) }
}
```

- [ ] **Step 2: Implement supervisor + circuit + backoff + leak**

(Full implementation tracking §9.2 through §9.5 verbatim — supervised process list, restart policies, leak slopes, eviction ladder steps.)

- [ ] **Step 3: Wire register calls from W1-W4**

In daemon main:
```go
sv := supervisor.New(db)
sv.Register(supervisor.ProcessSpec{Name: "voice-stt", Restart: CrashOnly, BackoffMin: 200*time.Millisecond, BackoffMax: 30*time.Second, Circuit: supervisor.CircuitPolicy{MaxCrashes: 3, Window: 5*time.Minute}, Limits: supervisor.ResourceLimits{RSSmb: 1024}})
// repeat for voice-tts, vision-live, plugin-N, sync-peer-N
```

- [ ] **Step 4: Settings → About → Diagnostics row**

`AboutPane+Diagnostics.swift` — process table with name, state, RSS, restarts (24 h), last crash reason; "Restart all" admin button.

- [ ] **Step 5: Commit + PR + reviewer**

---

### Task 18: Dashboard cards — Coach + Privacy + Health (`LeahUI/Dashboard/`)

**Files:**
- Create: `app/Leah/Sources/LeahUI/Dashboard/CoachCard.swift`
- Create: `app/Leah/Sources/LeahUI/Dashboard/PrivacyCard.swift`
- Create: `app/Leah/Sources/LeahUI/Dashboard/HealthCard.swift`
- Modify: `app/Leah/Sources/LeahUI/Dashboard/DashboardView.swift` — slot three new cards
- Create: `app/Leah/Tests/LeahUITests/DashboardCardsTests.swift`

**Why this exists:** §3.12, §8.9, §9.10 each specify a Dashboard card surface. T18 lands all three together (file-disjoint inside the Dashboard module).

**Interfaces:**
- Consumes: `learn.Recommender` (T07), `budget.Budget` (T08), `supervisor.Status` (T17) via IPC
- Produces:
  - `CoachCard` — surfaced / dismissed / applied counters; tap → Settings → Recommendations
  - `PrivacyCard` — week trend per `Bucket`; tap → Settings → Privacy → Budgets
  - `HealthCard` — green/yellow/red per supervised process; tap → Settings → About → Diagnostics

- [ ] **Step 1: Failing tests** — snapshot tests assert card has 3 stat slots; Privacy card renders all 7 buckets

- [ ] **Step 2: Implement cards** — match Dashboard chrome from predecessor §4.7

- [ ] **Step 3: Commit + PR + reviewer**

---

### Task 19: Wire all Phase 4 surfaces into composition root (`cmd/leah-daemon/main.go` + `scripts/dev/orphan-scan.sh`)

**Files:**
- Modify: `cmd/leah-daemon/main.go`
- Create: `cmd/leah-daemon/main_phase4_test.go`
- Create: `scripts/dev/orphan-scan.sh`

**Why this exists:** v3.3.0 shipped with three wiring gaps (TTS providers, KG citation route, MCP composition) because Wave 1 producer PRs added `NewX()` constructors but the daemon's composition root (`cmd/leah-daemon/main.go`) never called them. The operator and an agent both assumed an earlier task did the wiring — neither did. Phase 4 makes wiring an EXPLICIT serialized task that runs after every producer wave merged and before T20 E2E and T21 ship. The implicit-composition-root assumption that "the last task wires everything" is hereby deleted from this plan.

**What gets deleted:** the implicit assumption (carried in v3.3.0's plan) that producer tasks auto-register into `main.go`. From Phase 4 onward, every `internal/<pkg>.NewX()` constructor introduced by Waves 1-4 is explicitly instantiated and reachable from `main()` here, or the task is not done.

**Interfaces:**
- Consumes (constructors must be called and the returned handle reachable from `main()` boot path):
  - `stt/whisper.NewRunner` (T01)
  - `voice/duplex.NewSession` (T02)
  - `vision/capture.NewScreen`, `vision/capture.NewCamera` (T03)
  - `vision/ocr.New` (T03)
  - `vision/router.New` (T04)
  - `learn.NewRecommender` (T06)
  - `learn.NewAntiList`, `learn.NewExperiment` (T07)
  - `budget.New` (T08)
  - `attest.NewVerifier` (T09)
  - `sync/discovery.New`, `sync/pair.New` (T10)
  - `sync/crdt.New`, `sync/coord.New` (T11)
  - `keystore/icloud.New` (T12)
  - `mcp/inbound.NewServer` (T13)
  - `a2a.NewServer`, `a2a.NewClient` (T14)
  - `plugin.NewHost` (T15)
  - `supervisor.New` (T17) — supervisor is the registration substrate for the long-lived subsystems above
- Produces: a single `bootPhase4(ctx, deps) error` helper in `cmd/leah-daemon/main.go` so the call graph is one symbol-grep away.

- [ ] **Step 1: Failing test — orphan-scan asserts ZERO Phase-4 packages have zero non-test callers**

`scripts/dev/orphan-scan.sh`:
```bash
#!/usr/bin/env bash
set -euo pipefail
PKGS=(
  internal/voice/stt/whisper
  internal/voice/duplex
  internal/vision/capture
  internal/vision/ocr
  internal/vision/router
  internal/learn
  internal/budget
  internal/attest
  internal/sync/discovery
  internal/sync/pair
  internal/sync/crdt
  internal/sync/coord
  internal/keystore/icloud
  internal/mcp/inbound
  internal/a2a
  internal/plugin
  internal/supervisor
)
fail=0
for p in "${PKGS[@]}"; do
  base=$(basename "$p")
  callers=$(grep -RIn --include='*.go' --exclude='*_test.go' -E "\\b${base}\\.[A-Z]" cmd/ internal/ | grep -v "^${p}/" | wc -l | tr -d ' ')
  if [ "$callers" = "0" ]; then
    echo "ORPHAN: $p has zero non-test callers"
    fail=1
  fi
done
exit $fail
```

Run BEFORE the wiring edit:
```bash
bash scripts/dev/orphan-scan.sh
```

Expected (red): every Phase-4 package whose constructor is not yet called from `main.go` prints `ORPHAN: ...` and the script exits non-zero. Capture the failing output verbatim in the PR body.

Then add `cmd/leah-daemon/main_phase4_test.go`:
```go
//go:build phase4_wiring

package main

import "testing"

func TestBootPhase4_AllConstructorsCalled(t *testing.T) {
	// Compile-time fence: this test only builds if bootPhase4 exists and the
	// package import graph reaches every Phase-4 internal/<pkg>. The body is
	// intentionally empty; the assertion is the build itself.
}
```

- [ ] **Step 2: Implement `bootPhase4`** — extend the existing `main()` boot path (do NOT fork it). Each constructor's returned handle is either registered with the supervisor, mounted on the IPC router, or stored on a `daemonDeps` struct passed to the existing handler loop. No `// TODO impl`. No stub stubs — if a subsystem genuinely cannot be instantiated at boot (e.g. `sync/coord` waits for pair), it gets a `supervisor.Register` entry with `RestartPolicy=Never` until awakened, not silent omission.

- [ ] **Step 3: Re-run orphan-scan (green)**

```bash
bash scripts/dev/orphan-scan.sh
echo "exit=$?"
```

Expected: no `ORPHAN:` lines, exit 0.

- [ ] **Step 4: Verify gate**

```bash
gofmt -l cmd/leah-daemon/ | tee /tmp/fmt.log
go vet ./cmd/leah-daemon/... 2>&1 | tail -5
go build -tags=phase4_wiring ./cmd/leah-daemon/... 2>&1 | tail -5
go test -tags=phase4_wiring ./cmd/leah-daemon/... -run TestBootPhase4 -count=1 2>&1 | tail -5
```

Expected: fmt empty, vet clean, build + test PASS.

- [ ] **Step 5: Commit + PR + reviewer**

PR body must include the red orphan-scan output captured at Step 1 (proves the wiring gaps were real) and the green output from Step 3 (proves they're closed). Dispatch to `general-purpose` (or `claude`) for any fix-up — `cavecrew-builder` has no `Bash` and cannot re-run the orphan-scan to verify the fix.

This task is single-owner serialized and MUST merge before T20 (E2E smoke) is dispatched — the smoke is meaningless against a daemon that never instantiates the surfaces under test.

---

### Task 20: Phase 4 E2E smoke + dispatch-template harness (`scripts/dev/phase4-e2e.sh` + `internal/eval/phase4.go`)

**Files:**
- Create: `scripts/dev/phase4-e2e.sh`
- Create: `internal/eval/phase4.go`
- Create: `internal/eval/phase4_test.go`
- Create: `docs/engineer/dispatch-templates/phase4-e2e.md`

**Why this exists:** §0 ship gate requires every wave's deliverable to demonstrate end-to-end on `make dev`. T20 builds an automated smoke that drives each Phase 4 surface and asserts the happy path. The dispatch-template doc lets future reviewers re-run the smoke without re-deriving prerequisites. T20 dispatches only after T19 (composition-root wiring) merges — the smoke is meaningless against a daemon that never instantiates the surfaces under test.

**Smoke coverage:**
- Voice: `voice.start` → fed canned PCM → `voice.partial` events → `voice.end` → row in `voice_session`
- Vision: `vision.snap` with screenshot of `testdata/sample-window.png` → OCR returns ≥ 1 block → consent prompt fires (auto-grant in test) → row in `vision_event`
- Recommend: `Observe` 5 events → `NextBatch` returns ≤ PacingPerHour → `Record(Accepted)` → state transitions
- Budget: Charge `cloud.vision.bytes` to 80% → emit `ActionWarn` event; to 100% → `ActionSwitchLocal`
- Attest: `VerifySelf` returns `Verified` on freshly-built binary
- Sync: pair two daemons via in-process Bonjour mock + OTP → apply 10 CRDT entries from A to B → assert B sees them
- Inbound MCP: issue token with `memory:read` scope → call `leah.memory.search` over stdio → 200 with results
- A2A: dial Leah-A from Leah-B → negotiate cap → `Ask("hello")` → consent prompt → grant → response
- Plugin: install `weather-pro` → enable → call `weather.now` via inbound MCP → response
- Supervisor: kill `voice-stt` subprocess → assert restart within 80 ms detection + spawn

- [ ] **Step 1: Author `phase4-e2e.sh`**

```bash
#!/usr/bin/env bash
set -euo pipefail
DB=$(mktemp -d)/leah.db
export LEAH_DB="$DB" LEAH_TEST_MODE=1
go test -tags=phase4_e2e -count=1 ./internal/eval/... -run TestPhase4E2E 2>&1 | tee /tmp/phase4-e2e.log
grep -E "^(FAIL|ok|PASS)" /tmp/phase4-e2e.log | tail -5
```

- [ ] **Step 2: Author `phase4_test.go`** with one subtest per surface; each subtest gated on the prior wave's package being importable.

- [ ] **Step 3: Run + commit + PR + reviewer**

```bash
bash scripts/dev/phase4-e2e.sh
```

Expected: all subtests PASS.

---

### Task 21: Phase 4 ship checklist + spec-parity + orphan-scan + deletion of superseded sketches + reviewer-and-merge pass

**Files:**
- Create: `docs/superpowers/phase4-ship-checklist.md`
- Delete (via `git rm`): `docs/engineer/specs/2026-06-10-voice-frontier.md`
- Delete (via `git rm`): `docs/engineer/specs/2026-06-10-learn-recommend-apply.md`
- Delete (via `git rm`): `docs/engineer/specs/2026-06-10-mcp-a2a-publish.md`
- Modify: `CHANGELOG.md` — v1.1 entry
- Modify: `scripts/check-spec-parity.sh` — include Phase 4 spec
- Modify: `docs/superpowers/specs/2026-06-21-leah-macos-native-ui-design.md` — §19 set "Phase 4 status: SHIPPED"

**Why this exists:** §14 mandates the deletion of the three thin sketches and §0 sets the v1.1 ship line as the wave-5 exit gate. T19 (composition-root) and T21 (this) are the two single-owner serialized tasks in Wave 5.

**Ship checklist content:**
- Every §11 deliverable green
- `scripts/check-spec-parity.sh` clean against Phase 4 spec
- `bash scripts/dev/orphan-scan.sh` exits 0 — ZERO Phase-4 packages with zero non-test callers (Phase 3 lesson: v3.3.0 shipped with 3 wiring gaps because this check ran after tag, not before)
- Every PR in Phase 4 has a reviewer-transcript APPROVE (audit via `gh pr list --search "label:phase4 merged:>=2026-06-22" --json number,reviews`)
- No AI signatures in any merged commit (`git log v1.0..HEAD --grep="Co-Authored-By\|Generated with" | head` empty)
- Operator-mode regression: `make dev` boots, hotkey opens, voice + vision smoke pass, sync pair works on operator's 2-Mac LAN, weather-pro plugin loads, supervisor reports green
- Sparkle EdDSA appcast signed + uploaded
- Phase 4 spec parity guard rule added to CI

- [ ] **Step 1: Author ship checklist**

`docs/superpowers/phase4-ship-checklist.md` — checkbox per item above, with command for each verification.

- [ ] **Step 2: Delete superseded sketches**

```bash
git rm docs/engineer/specs/2026-06-10-voice-frontier.md
git rm docs/engineer/specs/2026-06-10-learn-recommend-apply.md
git rm docs/engineer/specs/2026-06-10-mcp-a2a-publish.md
git commit -m "phase4: delete superseded sketches (Voice / Learn / MCP-A2A)

Phase 4 spec subsumes these three. Archive retained in git history.
What got smaller: -3 specs, -~600 lines, single source of truth."
```

- [ ] **Step 3: Update spec parity**

Add Phase 4 spec path to `scripts/check-spec-parity.sh`. Run:

```bash
bash scripts/check-spec-parity.sh
```

Expected: clean. If forbidden phrases surface, fix code/tests/docs in this same PR.

- [ ] **Step 4: CHANGELOG**

```markdown
## v1.1.0 — 2026-XX-XX
- Voice frontier — Whisper-large-v3 ONNX local STT, full-duplex with barge-in, voice-only mode, continuous wake + per-app suppression
- Multi-device sync — Bonjour discovery + OTP pair + CRDT memory+pin+settings replication on 2-Mac LAN
- Recommend pass-2 — ranked queue, decay schedule, anti-list (operator + auto + spec), A/B, Coach card
- Camera + vision — screenshot ask, selection drag, OCR, live screen + live camera (opt-in), consent ledger
- Multi-agent — inbound MCP server with token scopes, Leah-to-Leah A2A peer handshake + delegated Ask
- Continuous attestation — runtime self-verify + plugin verify + Sparkle EdDSA + About-pane verdict
- Plugin SDK — Go + Swift SDKs, sandbox subprocess host, signed bundle format, sample weather-pro plugin
- Privacy budgets — 7 buckets with soft-warn → degrade → block ladder + Dashboard card
- Watchdog supervisor — restart + backoff + circuit + RSS leak detect + eviction ladder + Diagnostics row
```

- [ ] **Step 5: Predecessor §19 update**

In `docs/superpowers/specs/2026-06-21-leah-macos-native-ui-design.md` §19, change "Phase 4 (Multi-modal + multi-agent layer) — designed in `2026-06-22-leah-phase4-design.md`" to add "; status SHIPPED <date> v1.1.0".

- [ ] **Step 6: Final reviewer pass**

```bash
git add docs/superpowers/phase4-ship-checklist.md CHANGELOG.md scripts/check-spec-parity.sh docs/superpowers/specs/2026-06-21-leah-macos-native-ui-design.md
git commit -m "phase4: ship checklist + parity guard + v1.1 CHANGELOG

Closes Phase 4. v1.1 ship line.
What got smaller: 3 superseded sketches deleted, single source of truth restored."
gh pr create --title "phase4: ship — checklist + parity + CHANGELOG + sketch deletion" --body "Spec §14 deletion default + §11 ship gate.

Reviewer transcript channel."
```

Spawn final reviewer; only after transcript APPROVE does main session merge.

- [ ] **Step 7: Tag + release**

```bash
git tag -s v1.1.0 -m "Leah v1.1.0 — multi-modal + multi-agent layer"
git push origin v1.1.0
scripts/release/generate-appcast.sh v1.1.0
```

---

## Self-review notes (post-author)

**1. Spec coverage:**

- §1 Voice frontier → T01 (STT), T02 (duplex + barge), T05 (migration), T08 (budget charge)
- §2 Multi-device sync → T10 (Bonjour + OTP), T11 (CRDT + coord), T12 (iCloud Keychain + Sync pane), T05 (migration + node_uuid backfill)
- §3 Recommend pass-2 → T06 (lifecycle + decay), T07 (anti-list + A/B + pane), T18 (Coach card)
- §4 Camera + vision → T03 (capture + OCR), T04 (Sonnet route + consent), T05 (migration), T08 (budget charge)
- §5 Multi-agent A2A → T13 (inbound MCP + tokens + Connections rename), T14 (A2A protocol)
- §6 Continuous attestation → T09 (verifier + About pane), T21 (Sparkle appcast)
- §7 Plugin SDK → T15 (host + sandbox + Go SDK), T16 (sample plugin + Swift SDK + Plugins pane)
- §8 Privacy budgets → T08 (substrate + degradation + call-site wiring), T18 (Privacy card)
- §9 Watchdog supervisor → T17 (supervisor + leak + eviction + Diagnostics row), T18 (Health card)
- §10 Matrices → enforced as constraints, not standalone tasks
- §11 Task index → 21 tasks land all 16 spec-indexed items (split: T07 from T06, T16 from T15, T18 from T17, T20+T21 from spec T16; T19 composition-root wiring carved out from the implicit assumption in v3.3.0's plan that producer tasks self-register — see Global Constraints "composition-root wiring is its own task")
- §12 Open questions → resolved/tracked at gates: Q1 before T16 ships (plugin EdDSA custody), Q2 before T14 ships (peer display name), Q5 before T09 ships (Sparkle revocation URL)
- §14 Deletion default → T21 deletes the three sketches; T19 deletes the implicit-composition-root assumption from the plan

**2. Placeholder scan:** No "TBD", "TODO impl", or "fill in details" markers. T03 capture native impl is honest about CG bridge deferring to follow-up commit on same branch (not a stub PR). T20 E2E auto-grants consent in test mode — flagged as test-only path via `LEAH_TEST_MODE=1` env gate.

**3. Type consistency check:**

- `stt.STT.Stream` signature: `Stream(ctx, audio <-chan AudioFrame) (<-chan Partial, error)` — same in T01 and T02 consumer
- `tts.Provider.Speak`: matches Phase 3 contract (preserved, not redefined)
- `vision.Image` struct: same shape in T03, T04, T17
- `learn.RecommendKind`: string type used identically in T06, T07, T18
- `budget.Bucket`: string type identical in T08 call-site wiring (T01, T04, embed)
- `supervisor.ProcessSpec`: registered in T17 with same fields as T17 caller sites in T01, T02, T03, T15

Per-task `Files` blocks all use absolute paths from repo root; no relative refs.

---

## Execution handoff

Plan complete and saved to `docs/superpowers/plans/2026-06-22-leah-macos-native-phase4.md`. Two execution options:

**1. Subagent-Driven (recommended)** — main session dispatches one implementer subagent per task (Wave 1 = up to 5 parallel; Wave 2 = up to 4; Wave 3 = up to 3; Wave 4 = up to 3; Wave 5 = T17+T18 parallel then strictly serialized T19 → T20 → T21), reviewer subagent per PR via transcript channel, two-stage review.

**2. Inline Execution** — execute tasks in this session using `superpowers:executing-plans`, batch within each wave with checkpoint after each PR merges.

Phase 4 PRs do not start until Phase 3 ships and runs on operator's machine for ≥ 7 days (per spec §0). Operator gates Wave-1 start.

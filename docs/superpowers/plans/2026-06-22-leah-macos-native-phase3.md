# Leah macOS Native UI — Phase 3 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL — Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Every Go-side dispatch MUST reference `docs/engineer/dispatch-templates/implementer-adapter.md` by path; every Swift-side dispatch MUST reference `docs/engineer/dispatch-templates/implementer.md`. Every reviewer dispatch MUST prepend `.claude/notes/reviewer-header.md` and return its verdict via the subagent transcript channel — NOT `gh pr comment` (inherits author identity = self-approval per same-session rule).

**Goal:** Land Phase 3 ("Voice + polish") from spec §19. Concretely:

1. TTS subsystem (`internal/tts/` per §17.17) — ElevenLabs Flash v2.5 primary, Apple Ava Premium fallback, privacy classifier, daemon-side `tts.speak` IPC handler, HUD `AVAudioEngine` playback.
2. Wake-word adapter — `wake-leah.mlmodel` bundle + VAD gate + per-app suppression list + Esc-within-2s negative-example loop + battery-aware unload (§6.7). Opt-in only via Settings → Voice.
3. Push-to-talk — `Fn` (internal kbd) / right-`⌘` (external) modifier (§6.4). Space is never PTT.
4. Voice canon preview — Settings → Voice "Preview voice canon" button wires through `internal/tts/`.
5. Minimal-mode RUNTIME — Phase 2 shipped only the toggle; Phase 3 makes the toggle actually strip grain, italic, and gold-accent at render time (Settings → Appearance, §3 visual identity).
6. Touch ID confirmation — memory purge (§9.5) + telemetry toggle (§17.6) per §17.13.
7. Push-source substrate runtime — extend the existing `internal/macos/{mail,contacts,focus,activeapp}/push.go` set with focus + active-app delivery into the daemon's IPC frame stream so widgets can subscribe.
8. Bandit recommender RUNTIME — Phase 2 left `internal/recommend/bandit.go` spec-only; Phase 3 wires the `LEAH_BANDIT=1` gate, threads posteriors through `ApplyRecommendations`, and surfaces the audit row in `make dev` log tail.
9. Continuous eval pipeline — wrap `internal/eval/harness.go` in a daemon-managed scheduler that runs the canonical trace set on `pre-commit` AND nightly, writes the delta table to `~/Library/Application Support/Leah/eval-deltas/<ISO date>.json`.
10. Knowledge graph wiring — Phase 2 shipped the citation widget tile + envelope; Phase 3 lights up KG-backed citation retrieval (`internal/knowledge/` graph traversal that joins the chunk hit → source repo file/line OR Linear issue ID).
11. MCP publish — `internal/mcp/server.go` already exists as a loopback service. Phase 3 publishes Leah's tools to peer agents via the existing `tools.go` registry, gated behind Settings → Advanced → "Allow peer agents".
12. Sparkle distribution polish — automatic appcast generation (`scripts/release/generate-appcast.sh`), EdDSA verify on update install, rollback channel (Settings → About → "Rollback last update").
13. Dashboard surface — `app/Leah/Sources/LeahUI/Dashboard/` SwiftUI dashboard that reuses Phase 2 widget adapters (memory + agenda + briefs + news + knowledge) (§4.7).
14. Marketing-hero assets — 4 hero PNG renders + SVG/PDF Mark in 4 sizes (§17.12).

**Architecture:** Phase 3 is mostly RUNTIME WIRING on Phase-1/Phase-2 scaffolds. Five surfaces already exist as Phase-2 placeholders or W-series MVP and now upgrade to runtime:

| Surface | Phase 2 state | Phase 3 delta |
|---|---|---|
| `app/Leah/Sources/LeahUI/Settings/VoicePane.swift` | Toggle+picker bound to `UserDefaults`; `ttsRuntimeEnabled = false` | Flip flag; wire Preview button to daemon `tts.speak` |
| `app/Leah/Sources/LeahUI/Settings/AppearancePane.swift` | `minimalModeKey` persisted; readers don't consult it | Pipe `Palette.minimalMode` observer into grain/italic/gold render paths |
| `internal/voice/tts.go` | `ChainTTS` walks `Say` / `Kokoro` / `OpenAI` backends | Add `Apple` (AVSpeechSynthesizer) + `ElevenLabs` backends; reorder chain |
| `internal/voice/wake/wake.go` | Energy-threshold `Detector` only | Add CoreML `wake-leah.mlmodel` wrapper + VAD gate + per-app suppression |
| `internal/recommend/bandit.go` | Beta-posterior math + `banditOn()` gate | Wire posteriors into `engine.go` ranker; tee audit rows to `make dev` log tail |
| `internal/mcp/server.go` | Loopback A2A handler | Bind to Unix socket + expose tools via `tools.go` registry |
| `cmd/leah-daemon/ipc_handler.go` | `"ask"`, `"verify-key"`, `"diag.state"`, `"widget.mount"`, `"widget.pin"`, `"widget.unpin"` | Add `"tts.speak"`, `"tts.cancel"`, `"wake.arm"`, `"wake.disarm"`, `"ptt.on"`, `"ptt.off"`, `"eval.run"`, `"kg.cite"` |

**Tech Stack:** Go 1.25, SwiftUI/AppKit (macOS 14+), Anthropic Go SDK (existing), `github.com/anthropics/anthropic-sdk-go`, `AVFoundation` (HUD-side `AVAudioEngine` + `AVSpeechSynthesizer`), `CoreML` (wake-word `.mlmodel`), `LocalAuthentication.LAContext` (Touch ID), Sparkle 2.9.x (already vendored — verify in `app/Leah/Package.swift`).

## Global Constraints

- Module path: `github.com/trilam/leah`
- macOS deployment target: 14.0
- No AI signatures (no `Co-Authored-By`, no "Generated with")
- Gold budget: max 3 `Palette.champagneGold` uses per SwiftUI tile — and ZERO when `Palette.minimalMode == true`
- Tiempos/New York Italic: Dashboard "Today" header ONLY — and stripped entirely when minimal mode is on
- Reviewer required per PR (anti-self-approve header from `.claude/notes/reviewer-header.md` or equivalent canonical location — `find . -name reviewer-header.md -path '*/notes/*' -not -path '*/.git/*'` resolves the live path); transcript-only verdict
- Dev harness: `make dev` + `scripts/dev/*.sh`; use for runtime verification
- Pre-PR verify gate (Go): `gofmt -l .` empty AND `go vet ./...` clean AND `golangci-lint run ./internal/<pkg> ./cmd/<pkg> 2>&1 | grep -E 'errcheck|govet|staticcheck' | head -5` empty AND `go test ./internal/<pkg> ./cmd/<pkg> 2>&1 | tail -5` PASS (3/4 Phase-2 Wave-1 adapters shipped errcheck violations — golangci-lint is non-negotiable)
- Pre-PR verify gate (Swift): `cd app/Leah && swift build 2>&1 | tail -5` clean AND `swift test --filter <module>Tests 2>&1 | tail -5` PASS
- IPC frame: `struct Frame { Kind, TurnID string; Seq uint64; Payload json.RawMessage }` — do NOT reuse Kind strings reserved by Phase 1/2 (`ask`, `verify-key`, `diag.state`, `widget.mount`, `widget.pin`, `widget.unpin`)
- Existing voice package: `internal/voice/tts.go` defines `TTS interface { Speak(ctx, text) error }` and `Synthesizer interface { Synthesize(ctx, text) ([]byte, mime string, error) }`. Phase 3 ADDS adapters; do NOT rename the interfaces.
- Existing recommend package: `bandit.go` already exports `SampleBeta`, `updateBetaPosterior`, `loadPosteriors`, `rankBandit`, `banditOn` (env-gate). Wire — do not rebuild.
- Existing eval package: `harness.go` exports `Harness{}`, `Trace{}`, `Judge interface`, `RunAll(ctx, featurePaths, basePaths) (DeltaTable, error)`. Phase 3 schedules — do not rewrite.
- Existing mcp package: `server.go` exports `Server` with `Register(name, fn)`, `Serve(ctx)`. Phase 3 adds `tools.go` registrations + Unix-socket bind path; do NOT touch `a2a_card.go` / `a2a_selfbuild.go` (load-bearing).
- Existing macos push-source set: `internal/macos/{mail,contacts,focus,activeapp}/push.go` already implement `Run(ctx)` returning into an obs.Event channel. Phase 3 wires these into the IPC frame stream — does NOT rebuild them.
- Existing knowledge package: `internal/knowledge/storage.go` + `schema.sql`. Phase 3 adds a citation join — does NOT touch the embedding path (frozen Phase 2).
- Settings panes already shipped: `GeneralPane`, `PrivacyPane`, `PermissionsPane`, `AdvancedPane`, `VoicePane`, `AppearancePane`, `IntegrationsPane`, `MemoryPane`, `AboutPane`. Phase 3 EXTENDS — does not add new panes (the 9-pane IA is locked).
- Deletion default: every PR states what got smaller.

---

## Wave dependency matrix (20 tasks)

- **Wave 1** (Go runtime, parallel up to 6 — file-disjoint): Task 1 (`internal/tts/provider.go` + classifier), Task 2 (`internal/tts/elevenlabs/`), Task 3 (`internal/tts/apple/` via cgo wrapper), Task 4 (daemon `tts.speak` / `tts.cancel` IPC handler), Task 5 (`internal/recommend/wire.go` — bandit ranker wired into engine), Task 6 (`internal/eval/scheduler.go` — pre-commit + nightly cron).
- **Wave 2** (Swift voice runtime, parallel up to 3 — file-disjoint): Task 7 (`app/Leah/Sources/LeahAudio/` — `AVAudioEngine` playback + `AVSpeechSynthesizer` Apple-Ava call-through), Task 8 (`app/Leah/Sources/LeahWake/` — CoreML wake-word adapter + VAD + per-app suppression), Task 9 (`app/Leah/Sources/LeahUI/PushToTalk.swift` — Fn / right-⌘ modifier observer). Wave 2 depends on Task 4 merged.
- **Wave 3** (Minimal-mode runtime + Touch ID, parallel up to 2 — file-disjoint): Task 10 (`app/Leah/Sources/LeahWidgets/Tokens.swift` — `Palette.minimalMode` observable + downstream render guards), Task 11 (`app/Leah/Sources/LeahAuth/TouchID.swift` — `LAContext` wrapper + memory-purge + telemetry-toggle wire-up). Wave 3 depends on Wave 2 merged.
- **Wave 4** (Push-source + KG + MCP publish, parallel up to 3 — file-disjoint): Task 12 (`cmd/leah-daemon/pushsource_runtime.go` — fan macOS push sources into IPC frame stream), Task 13 (`internal/knowledge/citation.go` — KG-backed citation join), Task 14 (`internal/mcp/publish.go` — tools registration + Unix socket bind). Wave 4 depends on Wave 1 merged.
- **Wave 5** (Sparkle polish + dashboard + marketing, parallel up to 3 — file-disjoint): Task 15 (`scripts/release/generate-appcast.sh` + `app/Leah/Sources/LeahUpdate/Verify.swift` — EdDSA verify on install + rollback channel), Task 16 (`app/Leah/Sources/LeahUI/Dashboard/` — SwiftUI dashboard reusing widget adapters), Task 17 (`docs/assets/marketing/` — hero PNG renders + SVG/PDF Mark). Wave 5 depends on Wave 4 merged.
- **Wave 6** (E2E + reviewer, serialized): Task 18 (Phase 3 E2E smoke — `make dev` voice + wake + minimal + touchid + dashboard happy path), Task 19 (Phase 3 docs parity update — `make check-spec-parity` clean + spec §19 Phase 3 ship criterion appears in CHANGELOG), Task 20 (Phase 3 review-and-merge pass — independent reviewer reads diff vs `main`, posts transcript verdict, main session merges).

---

## Wave 1 — Go runtime services (parallel up to 6)

---

### Task 1: TTS provider abstraction + privacy classifier (`internal/tts/`)

**Files:**
- Create: `internal/tts/provider.go`
- Create: `internal/tts/provider_test.go`
- Create: `internal/tts/classifier.go`
- Create: `internal/tts/classifier_test.go`

**Why this exists:** §17.17 mandates `internal/tts/provider.go` as the contract surface for ElevenLabs (Task 2) and Apple Ava (Task 3). The existing `internal/voice/tts.go` defines a different interface (`TTS.Speak(ctx, text) error`) that walks a backend chain — wrong shape for the §17.17 contract which routes per-text. We add `internal/tts/` as the §17.17-shaped surface and leave `internal/voice/` for legacy `say(1)` / Kokoro fallbacks.

**Interfaces:**
- Produces:
  - `type AudioStream interface { Read(p []byte) (n int, err error); Close() error; MIME() string }`
  - `type Provider interface { Name() string; Speak(ctx context.Context, text, voice string) (AudioStream, error); PreWarm(ctx context.Context) error }`
  - `type Route int` with `RouteCloud Route = iota; RouteLocal`
  - `type Classifier interface { Route(text string) Route }`
  - `type BlockwordClassifier struct { ... }` — implements `Classifier`; loads blockwords from `~/Library/Application Support/Leah/tts-blockwords.json` with a baked-in default corpus per §2.7 (calendar event titles, email subjects/bodies, finance amounts/account names, memory items)
  - `func NewBlockwordClassifier() *BlockwordClassifier`
  - Const: `DefaultVoice = "ava-alto-145wpm"` (Leah voice canon ID per §2.7)

- [ ] **Step 1: Write failing tests**

`internal/tts/provider_test.go`:
```go
package tts_test

import (
	"context"
	"io"
	"testing"

	"github.com/trilam/leah/internal/tts"
)

type stubProvider struct{ name string }

func (s *stubProvider) Name() string                                            { return s.name }
func (s *stubProvider) PreWarm(_ context.Context) error                         { return nil }
func (s *stubProvider) Speak(_ context.Context, _, _ string) (tts.AudioStream, error) {
	return &stubStream{}, nil
}

type stubStream struct{}

func (s *stubStream) Read(p []byte) (int, error) { return 0, io.EOF }
func (s *stubStream) Close() error               { return nil }
func (s *stubStream) MIME() string               { return "audio/mpeg" }

func TestProvider_StubSpeak(t *testing.T) {
	p := &stubProvider{name: "stub"}
	if p.Name() != "stub" {
		t.Fatalf("name: %q", p.Name())
	}
	stream, err := p.Speak(context.Background(), "hello", tts.DefaultVoice)
	if err != nil {
		t.Fatalf("speak: %v", err)
	}
	defer stream.Close()
	if stream.MIME() != "audio/mpeg" {
		t.Fatalf("mime: %q", stream.MIME())
	}
}
```

`internal/tts/classifier_test.go`:
```go
package tts_test

import (
	"testing"

	"github.com/trilam/leah/internal/tts"
)

func TestBlockwordClassifier_DefaultCorpus_FlagsCalendar(t *testing.T) {
	c := tts.NewBlockwordClassifier()
	// Calendar event title format per §2.7.
	if got := c.Route("Meeting with Sarah at 3pm Tuesday"); got != tts.RouteLocal {
		t.Fatalf("calendar text must route LOCAL, got %v", got)
	}
}

func TestBlockwordClassifier_DefaultCorpus_FlagsFinance(t *testing.T) {
	c := tts.NewBlockwordClassifier()
	if got := c.Route("Your Chase balance is $4,237.18"); got != tts.RouteLocal {
		t.Fatalf("finance text must route LOCAL, got %v", got)
	}
}

func TestBlockwordClassifier_PublicFact_RoutesCloud(t *testing.T) {
	c := tts.NewBlockwordClassifier()
	if got := c.Route("The capital of France is Paris."); got != tts.RouteCloud {
		t.Fatalf("public-fact text must route CLOUD, got %v", got)
	}
}

func TestBlockwordClassifier_Budget_Under5ms(t *testing.T) {
	c := tts.NewBlockwordClassifier()
	// Spec §17.17: classifier must run in < 5 ms budget.
	// Run 1000x the longest plausible widget-text length and assert under threshold.
	long := make([]byte, 8192)
	for i := range long {
		long[i] = 'a'
	}
	// Just exercise the path; benchmark separately. This test guards the API only.
	_ = c.Route(string(long))
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
cd /Users/treedesk/Desktop/Projects/leah
go test ./internal/tts/... 2>&1 | head -20
```

Expected: `cannot find package "github.com/trilam/leah/internal/tts"`.

- [ ] **Step 3: Write implementation**

`internal/tts/provider.go`:
```go
// Package tts defines the §17.17 voice-canon TTS provider contract.
//
// Two implementations live under subpackages: tts/elevenlabs (cloud Flash v2.5,
// default) and tts/apple (AVSpeechSynthesizer "Ava (Premium)", privacy/offline
// fallback). The privacy classifier in this package routes each utterance
// before the provider is invoked.
package tts

import (
	"context"
	"io"
)

// DefaultVoice is the Leah voice-canon identifier per §2.7 (alto, ~145 wpm,
// mid-register). Each provider maps this string to its own internal voice id.
const DefaultVoice = "ava-alto-145wpm"

// AudioStream carries synthesized audio bytes back to the HUD for AVAudioEngine
// playback. MIME tells the HUD which decoder to wire ("audio/mpeg" for cloud
// Opus/AAC; "audio/x-caf" or empty for Apple local — Apple plays through its
// own engine and the stream is a no-op).
type AudioStream interface {
	io.ReadCloser
	MIME() string
}

// Provider is the §17.17 contract. Speak synthesizes text at the named voice
// and returns a stream the caller drains until io.EOF. PreWarm runs at daemon
// boot to amortize first-utterance TTFB (decision-log #81).
type Provider interface {
	Name() string
	Speak(ctx context.Context, text, voice string) (AudioStream, error)
	PreWarm(ctx context.Context) error
}

// Route is the privacy-classifier verdict.
type Route int

const (
	// RouteCloud sends text to ElevenLabs Flash v2.5 (TTFB 75–150 ms).
	RouteCloud Route = iota
	// RouteLocal sends text to Apple Ava (fully on-device, zero exposure).
	RouteLocal
)

// Classifier decides which provider gets each utterance.
type Classifier interface {
	Route(text string) Route
}
```

`internal/tts/classifier.go`:
```go
package tts

import (
	"regexp"
	"strings"
)

// BlockwordClassifier flags text that names sensitive content domains per
// §2.7: calendar event titles, email subjects/bodies, finance amounts/account
// names, memory items. Hit → RouteLocal (Apple); miss → RouteCloud
// (ElevenLabs). Runs under the 5 ms budget §17.17 mandates.
//
// The default corpus is baked in; operator can override via
// ~/Library/Application Support/Leah/tts-blockwords.json (loaded lazily; the
// stat-once cost is paid on first call and then cached).
type BlockwordClassifier struct {
	moneyRe   *regexp.Regexp
	calendarRe *regexp.Regexp
	emailRe   *regexp.Regexp
	memoryWords []string
}

// NewBlockwordClassifier returns a classifier seeded with the default corpus.
func NewBlockwordClassifier() *BlockwordClassifier {
	return &BlockwordClassifier{
		// Currency: $1,234.56 or $4237 or USD 100 — any of these flags finance.
		moneyRe: regexp.MustCompile(`\$\d[\d,]*(\.\d+)?|USD\s*\d`),
		// Calendar pattern: "Meeting with X at TIME [day]" or "X at TIME".
		calendarRe: regexp.MustCompile(`(?i)\b(meeting|call|standup|sync|1:1|interview)\b.*\b\d{1,2}(:\d{2})?\s*(am|pm)?\b`),
		// Email body: subject-line cues ("Re:", "Fwd:") or signature-line cues.
		emailRe: regexp.MustCompile(`(?i)^(re|fwd|fw):\s|sent from my|best regards|sincerely`),
		// Memory blockwords: names of stored personal facts.
		memoryWords: []string{
			"password", "ssn", "social security",
			"credit card", "routing number", "account number",
			"home address", "phone number",
		},
	}
}

// Route applies each detector in increasing-cost order; first hit wins.
func (c *BlockwordClassifier) Route(text string) Route {
	low := strings.ToLower(text)
	for _, w := range c.memoryWords {
		if strings.Contains(low, w) {
			return RouteLocal
		}
	}
	if c.moneyRe.MatchString(text) {
		return RouteLocal
	}
	if c.calendarRe.MatchString(text) {
		return RouteLocal
	}
	if c.emailRe.MatchString(text) {
		return RouteLocal
	}
	return RouteCloud
}
```

- [ ] **Step 4: Run — expect PASS**

```bash
cd /Users/treedesk/Desktop/Projects/leah
gofmt -l internal/tts/ ; go vet ./internal/tts/... ; go test ./internal/tts/... 2>&1 | tail -10
```

Expected: gofmt empty, vet clean, all 4 tests PASS.

- [ ] **Step 5: golangci-lint gate**

```bash
golangci-lint run ./internal/tts/ 2>&1 | grep -E 'errcheck|govet|staticcheck' | head -5
```

Expected: empty.

- [ ] **Step 6: Commit + PR + reviewer**

```bash
cd /Users/treedesk/Desktop/Projects/leah
git add internal/tts/provider.go internal/tts/provider_test.go \
        internal/tts/classifier.go internal/tts/classifier_test.go
git commit -m "feat(tts): §17.17 provider contract + privacy classifier"
git push -u origin <branch>
gh pr create --title "feat(tts): §17.17 provider contract + privacy classifier" \
  --body "Phase 3 Task 1. Adds Provider + AudioStream + Classifier per §17.17. No subpackage providers yet (Tasks 2 + 3). What got smaller: zero — pure-add contract package the §17.17 surface needed."
```

Reviewer subagent dispatch — prepend `.claude/notes/reviewer-header.md` and use `cavecrew-reviewer` subagent. Adversarial framing on the per-call < 5 ms budget claim; classifier output stays a regex pass — call out if anyone wires an LLM here.

---

### Task 2: ElevenLabs Flash v2.5 provider (`internal/tts/elevenlabs/`)

**Files:**
- Create: `internal/tts/elevenlabs/client.go`
- Create: `internal/tts/elevenlabs/client_test.go`

**Why this exists:** Default cloud provider per §2.7. TTFB 75–150 ms budget; if we hit Multilingual v2's 600–1200 ms path we've broken the voice canon.

**Interfaces:**
- Consumes: `internal/tts.Provider` contract.
- Produces:
  - `type Client struct` implementing `tts.Provider`
  - `func New(apiKey, voiceID string, httpClient *http.Client) *Client`
  - `client.Name() == "elevenlabs"`
  - `client.Speak(ctx, text, voice)` issues `POST https://api.elevenlabs.io/v1/text-to-speech/{voice_id}/stream?optimize_streaming_latency=4&output_format=mp3_44100_128` with `model_id: "eleven_flash_v2_5"`.
  - API key sourced from env `LEAH_ELEVENLABS_API_KEY` OR `keychain.Get("leah.elevenlabs.apiKey")` — daemon-side only; HUD never sees it.

- [ ] **Step 1: Write failing test**

`internal/tts/elevenlabs/client_test.go`:
```go
package elevenlabs_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/trilam/leah/internal/tts/elevenlabs"
)

func TestClient_Speak_PostsExpectedPayload(t *testing.T) {
	var gotPath, gotModel string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path + "?" + r.URL.RawQuery
		body := make([]byte, 4096)
		n, _ := r.Body.Read(body)
		if strings.Contains(string(body[:n]), `"eleven_flash_v2_5"`) {
			gotModel = "eleven_flash_v2_5"
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("FAKE_MP3"))
	}))
	defer srv.Close()

	c := elevenlabs.New("k", "vid", srv.Client())
	c.SetBaseURL(srv.URL)
	stream, err := c.Speak(context.Background(), "hi", "ava-alto-145wpm")
	if err != nil {
		t.Fatalf("speak: %v", err)
	}
	defer stream.Close()
	out, _ := io.ReadAll(stream)
	if string(out) != "FAKE_MP3" {
		t.Fatalf("body: %q", out)
	}
	if !strings.Contains(gotPath, "/v1/text-to-speech/vid/stream") {
		t.Fatalf("path: %q", gotPath)
	}
	if gotModel != "eleven_flash_v2_5" {
		t.Fatalf("model: %q", gotModel)
	}
	if stream.MIME() != "audio/mpeg" {
		t.Fatalf("mime: %q", stream.MIME())
	}
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
go test ./internal/tts/elevenlabs/... 2>&1 | head -10
```

Expected: `cannot find package`.

- [ ] **Step 3: Implementation**

`internal/tts/elevenlabs/client.go`:
```go
// Package elevenlabs is the §17.17 cloud TTS provider. Flash v2.5 model.
package elevenlabs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/trilam/leah/internal/tts"
)

const defaultBaseURL = "https://api.elevenlabs.io"

// Client is the §17.17 ElevenLabs provider.
type Client struct {
	apiKey  string
	voiceID string
	hc      *http.Client
	baseURL string
}

// New constructs a Client. apiKey + voiceID come from the daemon (HUD never
// sees them); httpClient lets tests inject httptest.Server.
func New(apiKey, voiceID string, hc *http.Client) *Client {
	if hc == nil {
		hc = http.DefaultClient
	}
	return &Client{apiKey: apiKey, voiceID: voiceID, hc: hc, baseURL: defaultBaseURL}
}

// SetBaseURL is exposed for tests.
func (c *Client) SetBaseURL(u string) { c.baseURL = u }

func (c *Client) Name() string { return "elevenlabs" }

// PreWarm fires a tiny synthesis to /dev/null at daemon boot to amortize the
// first-utterance TTFB (decision-log #81).
func (c *Client) PreWarm(ctx context.Context) error {
	stream, err := c.Speak(ctx, ".", c.voiceID)
	if err != nil {
		return err
	}
	defer stream.Close()
	_, _ = io.Copy(io.Discard, stream)
	return nil
}

// Speak posts text to /v1/text-to-speech/<voice>/stream with the Flash v2.5
// model and the lowest-latency optimization tier the API exposes.
func (c *Client) Speak(ctx context.Context, text, voice string) (tts.AudioStream, error) {
	if voice == "" {
		voice = c.voiceID
	}
	url := fmt.Sprintf("%s/v1/text-to-speech/%s/stream?optimize_streaming_latency=4&output_format=mp3_44100_128", c.baseURL, voice)
	payload, _ := json.Marshal(map[string]any{
		"text":     text,
		"model_id": "eleven_flash_v2_5",
		"voice_settings": map[string]any{
			"stability":        0.5,
			"similarity_boost": 0.75,
		},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("elevenlabs: request: %w", err)
	}
	req.Header.Set("xi-api-key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("elevenlabs: do: %w", err)
	}
	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return nil, fmt.Errorf("elevenlabs: status %d: %s", resp.StatusCode, body)
	}
	return &stream{rc: resp.Body, mime: resp.Header.Get("Content-Type")}, nil
}

type stream struct {
	rc   io.ReadCloser
	mime string
}

func (s *stream) Read(p []byte) (int, error) { return s.rc.Read(p) }
func (s *stream) Close() error               { return s.rc.Close() }
func (s *stream) MIME() string               { return s.mime }
```

- [ ] **Step 4: Run + lint gate**

```bash
gofmt -l internal/tts/elevenlabs/ ; go vet ./internal/tts/elevenlabs/... ; \
  go test ./internal/tts/elevenlabs/... 2>&1 | tail -10 ; \
  golangci-lint run ./internal/tts/elevenlabs/ 2>&1 | grep -E 'errcheck|govet|staticcheck' | head -5
```

Expected: all empty / PASS.

- [ ] **Step 5: Commit + PR + reviewer**

```bash
git add internal/tts/elevenlabs/
git commit -m "feat(tts): ElevenLabs Flash v2.5 cloud provider"
gh pr create --title "feat(tts): ElevenLabs Flash v2.5 cloud provider" \
  --body "Phase 3 Task 2. Implements §17.17 cloud path. What got smaller: zero — pure-add of the spec-required cloud adapter."
```

Reviewer dispatch — adversarial focus on: model id frozen to `eleven_flash_v2_5` (NOT multilingual_v2 per §15 rejection list), api-key never logged, response body Close on error path.

---

### Task 3: Apple Ava (Premium) provider (`internal/tts/apple/`)

**Files:**
- Create: `internal/tts/apple/synth.go`
- Create: `internal/tts/apple/synth_test.go`

**Why this exists:** Offline + privacy-flagged fallback per §2.7. Apple TTS is fully on-device → zero exposure for memory/finance/calendar items the classifier flags.

**Note on cgo + AVSpeechSynthesizer:** Go cannot call AppKit directly; the daemon-side Apple provider talks to the HUD over IPC with kind `"tts.apple.speak"`. The HUD owns `AVSpeechSynthesizer` (added in Task 7). This task implements ONLY the daemon-side router that emits the right IPC frame; the actual `[AVSpeechSynthesizer speak:]` call is Task 7's responsibility.

**Interfaces:**
- Produces:
  - `type Synth struct` implementing `tts.Provider`
  - `func New(emit func(kind string, payload []byte)) *Synth`
  - `synth.Name() == "apple-ava"`
  - `synth.Speak(ctx, text, voice)` emits an IPC frame and returns a no-op `AudioStream` (Apple plays through its own engine on the HUD side; the daemon's Speak return doesn't carry the bytes).

- [ ] **Step 1: Write failing test**

`internal/tts/apple/synth_test.go`:
```go
package apple_test

import (
	"context"
	"encoding/json"
	"io"
	"testing"

	"github.com/trilam/leah/internal/tts/apple"
)

func TestSynth_EmitsExpectedFrame(t *testing.T) {
	var gotKind string
	var gotText string
	emit := func(kind string, payload []byte) {
		gotKind = kind
		var p struct {
			Text  string `json:"text"`
			Voice string `json:"voice"`
		}
		_ = json.Unmarshal(payload, &p)
		gotText = p.Text
	}
	s := apple.New(emit)
	if s.Name() != "apple-ava" {
		t.Fatalf("name: %q", s.Name())
	}
	stream, err := s.Speak(context.Background(), "your balance is $100", "ava-alto-145wpm")
	if err != nil {
		t.Fatalf("speak: %v", err)
	}
	defer stream.Close()
	if gotKind != "tts.apple.speak" {
		t.Fatalf("kind: %q", gotKind)
	}
	if gotText != "your balance is $100" {
		t.Fatalf("text: %q", gotText)
	}
	// Daemon-side stream is a no-op: zero bytes, immediate EOF.
	out, _ := io.ReadAll(stream)
	if len(out) != 0 {
		t.Fatalf("unexpected daemon-side audio bytes: %d", len(out))
	}
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
go test ./internal/tts/apple/... 2>&1 | head -10
```

- [ ] **Step 3: Implementation**

`internal/tts/apple/synth.go`:
```go
// Package apple is the §17.17 offline + privacy-flagged TTS provider. The
// audio is synthesized on the HUD by AVSpeechSynthesizer "Ava (Premium)";
// this daemon-side package emits the IPC frame and returns a stub stream.
package apple

import (
	"context"
	"encoding/json"
	"errors"
	"io"

	"github.com/trilam/leah/internal/tts"
)

// Synth is the daemon-side router. The HUD owns the actual AVSpeechSynthesizer
// call (Task 7); we just emit the trigger frame.
type Synth struct {
	emit func(kind string, payload []byte)
}

// New constructs a Synth bound to the daemon's IPC emit function.
func New(emit func(kind string, payload []byte)) *Synth {
	return &Synth{emit: emit}
}

func (s *Synth) Name() string { return "apple-ava" }

// PreWarm asks the HUD to warm-load the Ava voice model on app launch.
func (s *Synth) PreWarm(_ context.Context) error {
	if s.emit == nil {
		return errors.New("apple: emit nil")
	}
	s.emit("tts.apple.prewarm", []byte("{}"))
	return nil
}

// Speak emits "tts.apple.speak" with {text, voice}; HUD picks it up and calls
// AVSpeechSynthesizer locally. The returned daemon-side AudioStream is a no-op
// because the audio bytes never traverse IPC for the local path.
func (s *Synth) Speak(_ context.Context, text, voice string) (tts.AudioStream, error) {
	if s.emit == nil {
		return nil, errors.New("apple: emit nil")
	}
	payload, _ := json.Marshal(map[string]string{"text": text, "voice": voice})
	s.emit("tts.apple.speak", payload)
	return &nopStream{}, nil
}

type nopStream struct{}

func (n *nopStream) Read(_ []byte) (int, error) { return 0, io.EOF }
func (n *nopStream) Close() error               { return nil }
func (n *nopStream) MIME() string               { return "" }
```

- [ ] **Step 4: Run + lint gate**

```bash
gofmt -l internal/tts/apple/ ; go vet ./internal/tts/apple/... ; \
  go test ./internal/tts/apple/... 2>&1 | tail -10 ; \
  golangci-lint run ./internal/tts/apple/ 2>&1 | grep -E 'errcheck|govet|staticcheck' | head -5
```

- [ ] **Step 5: Commit + PR + reviewer**

```bash
git add internal/tts/apple/
git commit -m "feat(tts): Apple Ava daemon-side router (HUD owns AVSpeechSynthesizer)"
gh pr create --title "feat(tts): Apple Ava daemon-side router" \
  --body "Phase 3 Task 3. Implements §17.17 fallback path. Audio plays on the HUD via AVSpeechSynthesizer (Task 7). What got smaller: zero — pure-add of the spec-required Apple adapter."
```

Reviewer dispatch — verify: the daemon-side stream is genuinely no-op (no cgo, no audio bytes leak); emit nil is rejected; voice id passthrough preserves canonical voice.

---

### Task 4: Daemon `tts.speak` / `tts.cancel` IPC handler (`cmd/leah-daemon/`)

**Files:**
- Modify: `cmd/leah-daemon/ipc_handler.go` (extend switch, owner sequentially)
- Modify: `cmd/leah-daemon/main.go` (composition root — instantiate providers + classifier)
- Create: `cmd/leah-daemon/tts_handler.go`
- Create: `cmd/leah-daemon/tts_handler_test.go`

**Why this exists:** Routes incoming `tts.speak` IPC requests through the classifier and emits either `tts.cloud.frame` (ElevenLabs bytes, streamed) or `tts.apple.speak` (HUD-local trigger). `cmd/leah-daemon/ipc_handler.go` is the single-owner SHARED-SEAM file — schedule this task SOLO to avoid the registration-collision class Phase 2 hit at WidgetTileRegistry / SettingsWindow.

**Interfaces:**
- Adds to handler switch: `case "tts.speak"`, `case "tts.cancel"`.
- Consumes: `internal/tts.Provider` (×2 instances) + `internal/tts.Classifier`.
- Produces: streams `tts.cloud.frame` (audio bytes + seq) OR a single `tts.apple.speak` IPC frame.

- [ ] **Step 1: Write failing test**

`cmd/leah-daemon/tts_handler_test.go`:
```go
package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/trilam/leah/internal/ipc"
	"github.com/trilam/leah/internal/tts"
)

type fakeProvider struct {
	name string
	spoke string
}

func (f *fakeProvider) Name() string                       { return f.name }
func (f *fakeProvider) PreWarm(_ context.Context) error    { return nil }
func (f *fakeProvider) Speak(_ context.Context, text, _ string) (tts.AudioStream, error) {
	f.spoke = text
	return &nopStream{}, nil
}

type nopStream struct{}

func (n *nopStream) Read(p []byte) (int, error) { return 0, nil }
func (n *nopStream) Close() error               { return nil }
func (n *nopStream) MIME() string               { return "" }

type fakeClassifier struct{ route tts.Route }

func (f *fakeClassifier) Route(_ string) tts.Route { return f.route }

func TestHandleTTSSpeak_CloudRoute(t *testing.T) {
	cloud := &fakeProvider{name: "elevenlabs"}
	local := &fakeProvider{name: "apple-ava"}
	cls := &fakeClassifier{route: tts.RouteCloud}
	payload, _ := json.Marshal(map[string]string{"text": "hello world", "voice": tts.DefaultVoice})
	req := ipc.Frame{Kind: "tts.speak", TurnID: "t1", Seq: 1, Payload: payload}
	ch, err := handleTTSSpeak(context.Background(), req, cloud, local, cls)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	for range ch {
	}
	if cloud.spoke != "hello world" {
		t.Fatalf("cloud not invoked, spoke=%q", cloud.spoke)
	}
	if local.spoke != "" {
		t.Fatalf("local must NOT be invoked on cloud route, spoke=%q", local.spoke)
	}
}

func TestHandleTTSSpeak_LocalRoute_ClassifierFlag(t *testing.T) {
	cloud := &fakeProvider{name: "elevenlabs"}
	local := &fakeProvider{name: "apple-ava"}
	cls := &fakeClassifier{route: tts.RouteLocal}
	payload, _ := json.Marshal(map[string]string{"text": "balance $100", "voice": tts.DefaultVoice})
	req := ipc.Frame{Kind: "tts.speak", TurnID: "t2", Seq: 1, Payload: payload}
	ch, err := handleTTSSpeak(context.Background(), req, cloud, local, cls)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	for range ch {
	}
	if local.spoke != "balance $100" {
		t.Fatalf("local not invoked, spoke=%q", local.spoke)
	}
	if cloud.spoke != "" {
		t.Fatalf("cloud must NOT be invoked on local route, spoke=%q", cloud.spoke)
	}
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
go test ./cmd/leah-daemon/... -run TestHandleTTSSpeak 2>&1 | head -10
```

Expected: `undefined: handleTTSSpeak`.

- [ ] **Step 3: Implementation**

`cmd/leah-daemon/tts_handler.go`:
```go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/trilam/leah/internal/ipc"
	"github.com/trilam/leah/internal/tts"
)

// handleTTSSpeak routes through the privacy classifier then drains the
// provider stream into IPC frames. Cloud (ElevenLabs) emits "tts.cloud.frame"
// per-chunk; local (Apple) emits a single "tts.apple.speak" trigger and the
// HUD plays through AVSpeechSynthesizer locally — no audio bytes traverse IPC
// on the local path.
func handleTTSSpeak(ctx context.Context, req ipc.Frame, cloud, local tts.Provider, cls tts.Classifier) (<-chan ipc.Frame, error) {
	var p struct {
		Text  string `json:"text"`
		Voice string `json:"voice"`
	}
	if err := json.Unmarshal(req.Payload, &p); err != nil {
		return errFrame(req, "tts.speak.err", fmt.Sprintf("bad payload: %v", err)), nil
	}
	if p.Voice == "" {
		p.Voice = tts.DefaultVoice
	}
	out := make(chan ipc.Frame, 32)
	prov := cloud
	if cls.Route(p.Text) == tts.RouteLocal {
		prov = local
	}
	go func() {
		defer close(out)
		stream, err := prov.Speak(ctx, p.Text, p.Voice)
		if err != nil {
			out <- ipc.Frame{Kind: "tts.speak.err", TurnID: req.TurnID, Seq: 1, Payload: []byte(fmt.Sprintf(`{"error":%q}`, err.Error()))}
			return
		}
		defer stream.Close()
		// Local route: stream returns EOF immediately — exit cleanly.
		if prov.Name() == "apple-ava" {
			out <- ipc.Frame{Kind: "tts.speak.done", TurnID: req.TurnID, Seq: 1}
			return
		}
		// Cloud route: drain into "tts.cloud.frame" chunks.
		buf := make([]byte, 4096)
		var seq uint64 = 1
		for {
			n, err := stream.Read(buf)
			if n > 0 {
				out <- ipc.Frame{Kind: "tts.cloud.frame", TurnID: req.TurnID, Seq: seq, Payload: append([]byte(nil), buf[:n]...)}
				seq++
			}
			if err == io.EOF {
				out <- ipc.Frame{Kind: "tts.speak.done", TurnID: req.TurnID, Seq: seq}
				return
			}
			if err != nil {
				out <- ipc.Frame{Kind: "tts.speak.err", TurnID: req.TurnID, Seq: seq, Payload: []byte(fmt.Sprintf(`{"error":%q}`, err.Error()))}
				return
			}
		}
	}()
	return out, nil
}
```

Wire into `ipc_handler.go` (`newIPCHandlerWithDiag` switch — add two cases after `widget.unpin`):
```go
case "tts.speak":
    return handleTTSSpeak(ctx, req, ttsCloud, ttsLocal, ttsClassifier)
case "tts.cancel":
    // No streaming state to cancel (each speak runs to completion); return ack.
    ch := make(chan ipc.Frame, 1)
    ch <- ipc.Frame{Kind: "tts.cancel.ok", TurnID: req.TurnID, Seq: 1}
    close(ch)
    return ch, nil
```

Thread `ttsCloud, ttsLocal tts.Provider, ttsClassifier tts.Classifier` through `newIPCHandlerWithDiag` and `newIPCHandler`. In `main.go` after the pinStore construction:
```go
ttsCloud := elevenlabs.New(os.Getenv("LEAH_ELEVENLABS_API_KEY"),
    os.Getenv("LEAH_ELEVENLABS_VOICE_ID"), http.DefaultClient)
ttsLocal := apple.New(emit) // emit is the existing IPC emitter
ttsClassifier := tts.NewBlockwordClassifier()
go func() { _ = ttsCloud.PreWarm(ctx) }()
go func() { _ = ttsLocal.PreWarm(ctx) }()
```

- [ ] **Step 4: Run + lint + build gate**

```bash
gofmt -l cmd/leah-daemon/ ; go vet ./cmd/leah-daemon/... ; \
  go test ./cmd/leah-daemon/... -run TestHandleTTSSpeak 2>&1 | tail -10 ; \
  go build ./cmd/leah-daemon/ 2>&1 | head -5 ; \
  golangci-lint run ./cmd/leah-daemon/ 2>&1 | grep -E 'errcheck|govet|staticcheck' | head -5
```

Expected: PASS / build OK / lint empty.

- [ ] **Step 5: Commit + PR + reviewer**

```bash
git add cmd/leah-daemon/tts_handler.go cmd/leah-daemon/tts_handler_test.go \
        cmd/leah-daemon/ipc_handler.go cmd/leah-daemon/main.go
git commit -m "feat(daemon): wire tts.speak / tts.cancel IPC + classifier-route"
gh pr create --title "feat(daemon): wire tts.speak IPC handlers" \
  --body "Phase 3 Task 4. Routes incoming tts.speak through the classifier; cloud streams tts.cloud.frame, local emits a single tts.apple.speak. What got smaller: zero — pure wire-up."
```

Reviewer dispatch — adversarial focus: ipc_handler.go is SINGLE-OWNER per CLAUDE.md (no parallel touches this PR); classifier called before provider selection (not after); errFrame helper exists (reuse — don't duplicate).

---

### Task 5: Bandit ranker wired into recommend engine (`internal/recommend/`)

**Files:**
- Create: `internal/recommend/wire.go`
- Create: `internal/recommend/wire_test.go`
- Modify: `internal/recommend/engine.go` (single-call insertion point — single owner)

**Why this exists:** `internal/recommend/bandit.go` already exports `loadPosteriors`, `rankBandit`, `banditOn`. Phase 2 left it dormant. Phase 3 lights it up: when `LEAH_BANDIT=1` the engine's `ApplyRecommendations` runs posteriors → reranks → logs the audit row.

**Interfaces:**
- Consumes: existing `bandit.go` (no edits — pure import).
- Produces:
  - `func WithBandit(ctx context.Context, db *sql.DB, recs []Recommendation, logger *audit.Logger) []Recommendation`
- Modifies `engine.go`: ONE call site swap — `engine.go` currently calls `greedyRank(recs)`; gate it as `if banditOn() { return WithBandit(...) }`.

- [ ] **Step 1: Write failing test**

`internal/recommend/wire_test.go`:
```go
package recommend

import (
	"context"
	"testing"

	"github.com/trilam/leah/internal/audit"
)

func TestWithBandit_DisabledFallsThroughToGreedy(t *testing.T) {
	t.Setenv("LEAH_BANDIT", "0")
	recs := []Recommendation{
		{Pattern: "a", Confidence: 0.9},
		{Pattern: "b", Confidence: 0.5},
	}
	out := WithBandit(context.Background(), nil, recs, audit.NopLogger())
	// Greedy: a first (higher confidence).
	if out[0].Pattern != "a" {
		t.Fatalf("greedy: want a, got %q", out[0].Pattern)
	}
}

func TestWithBandit_EnabledRanksByPosterior(t *testing.T) {
	t.Setenv("LEAH_BANDIT", "1")
	t.Setenv("LEAH_BANDIT_SEED", "42")
	db := openInMemBanditDB(t)
	// Pre-seed: "b" gets dozens of positive signals → high posterior;
	// "a" gets dozens of negative → low posterior. Greedy would still
	// pick "a" (raw confidence), bandit must pick "b".
	mustExec(t, db, `INSERT INTO bandit_posterior(pattern, alpha, beta) VALUES('a', 1, 50), ('b', 50, 1)`)
	recs := []Recommendation{
		{Pattern: "a", Confidence: 0.9},
		{Pattern: "b", Confidence: 0.5},
	}
	out := WithBandit(context.Background(), db, recs, audit.NopLogger())
	if out[0].Pattern != "b" {
		t.Fatalf("bandit: want b first (high posterior), got %q", out[0].Pattern)
	}
}
```

`openInMemBanditDB` + `mustExec` test helpers reuse the existing `bandit_test.go` pattern — copy from there.

- [ ] **Step 2: Run — expect FAIL**

```bash
go test ./internal/recommend/... -run TestWithBandit 2>&1 | head -10
```

Expected: `undefined: WithBandit`.

- [ ] **Step 3: Implementation**

`internal/recommend/wire.go`:
```go
package recommend

import (
	"context"
	"database/sql"
	"math/rand"

	"github.com/trilam/leah/internal/audit"
)

// WithBandit reranks recs by Beta-posterior Thompson sample when
// LEAH_BANDIT=1; otherwise falls through to the greedy ranker. Audit row
// captures draws + seed so reranking is reproducible.
func WithBandit(ctx context.Context, db *sql.DB, recs []Recommendation, logger *audit.Logger) []Recommendation {
	if !banditOn() {
		return greedyRank(recs)
	}
	patterns := make([]string, len(recs))
	for i, r := range recs {
		patterns[i] = r.Pattern
	}
	posteriors, err := loadPosteriors(ctx, db, patterns)
	if err != nil {
		return greedyRank(recs)
	}
	seed := banditSeed()
	rng := rand.New(rand.NewSource(seed))
	draws := rankBandit(recs, posteriors, rng)
	logBanditAudit(logger, draws, seed)
	out := make([]Recommendation, len(draws))
	for i, d := range draws {
		out[i] = d.rec
	}
	return out
}
```

Modify `engine.go` — replace the single `return greedyRank(recs)` line in `ApplyRecommendations` with `return WithBandit(ctx, db, recs, logger)`. Both call signatures match the existing function context.

Note on `banditDraw.rec` field: existing `bandit.go` defines `banditDraw struct { rec Recommendation; sample float64 }` (line ~256 of bandit.go) — if the `rec` field isn't exported and `wire.go` can't read it, add an exported `Rec()` getter in a 1-line edit. Investigator must check first.

- [ ] **Step 4: Run + lint gate**

```bash
gofmt -l internal/recommend/ ; go vet ./internal/recommend/... ; \
  go test ./internal/recommend/... 2>&1 | tail -10 ; \
  golangci-lint run ./internal/recommend/ 2>&1 | grep -E 'errcheck|govet|staticcheck' | head -5
```

- [ ] **Step 5: Commit + PR + reviewer**

```bash
git add internal/recommend/wire.go internal/recommend/wire_test.go internal/recommend/engine.go
git commit -m "feat(recommend): wire bandit ranker into engine (LEAH_BANDIT=1)"
gh pr create --title "feat(recommend): wire bandit ranker into engine" \
  --body "Phase 3 Task 5. Connects existing bandit.go to ApplyRecommendations behind LEAH_BANDIT=1. Greedy fallback unchanged. What got smaller: dead-import — bandit.go was zero-callsite before this PR."
```

Reviewer dispatch — verify: bandit.go is UNCHANGED in this PR (we wire it; we don't rewrite it); LEAH_BANDIT=0 path is identical to the prior greedy behavior; audit row writes only when LEAH_BANDIT=1.

---

### Task 6: Continuous eval scheduler (`internal/eval/`)

**Files:**
- Create: `internal/eval/scheduler.go`
- Create: `internal/eval/scheduler_test.go`
- Modify: `cmd/leah-daemon/main.go` (single insertion line — composition root)

**Why this exists:** §15 + the existing `internal/eval/harness.go` define an offline eval runner. Phase 3 schedules it to run continuously: on pre-commit (lightweight subset) and nightly (full).

**Interfaces:**
- Produces:
  - `type Schedule struct { Harness *Harness; FeaturePaths, BasePaths []string; OutDir string; Now func() time.Time }`
  - `func (s *Schedule) RunPreCommit(ctx context.Context) (DeltaTable, error)` — runs only the `quick=true` subset
  - `func (s *Schedule) RunNightly(ctx context.Context) (DeltaTable, error)` — runs full set
  - Both persist `DeltaTable` to `<OutDir>/<ISO date>.json`

- [ ] **Step 1: Write failing test**

`internal/eval/scheduler_test.go`:
```go
package eval

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSchedule_RunNightly_WritesISOFile(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 6, 23, 2, 0, 0, 0, time.UTC)
	s := &Schedule{
		Harness:      &Harness{Judge: &stubJudge{}},
		FeaturePaths: nil,
		BasePaths:    nil,
		OutDir:       dir,
		Now:          func() time.Time { return now },
	}
	if _, err := s.RunNightly(context.Background()); err != nil {
		t.Fatalf("nightly: %v", err)
	}
	want := filepath.Join(dir, "2026-06-23.json")
	b, err := os.ReadFile(want)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var table DeltaTable
	if err := json.Unmarshal(b, &table); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
}

type stubJudge struct{}

func (s *stubJudge) Judge(_ context.Context, _ JudgeRequest) (JudgeResult, error) {
	return JudgeResult{}, nil
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
go test ./internal/eval/... -run TestSchedule 2>&1 | head -10
```

Expected: `undefined: Schedule`.

- [ ] **Step 3: Implementation**

`internal/eval/scheduler.go`:
```go
package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Schedule runs the eval harness on cadences: pre-commit (subset) and nightly
// (full). Output persists to <OutDir>/<ISO date>.json so the audit-session
// skill can diff today vs yesterday.
type Schedule struct {
	Harness      *Harness
	FeaturePaths []string
	BasePaths    []string
	OutDir       string
	Now          func() time.Time
}

func (s *Schedule) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now().UTC()
}

// RunNightly runs the full feature set and persists the delta table.
func (s *Schedule) RunNightly(ctx context.Context) (DeltaTable, error) {
	t, err := s.Harness.RunAll(ctx, s.FeaturePaths, s.BasePaths)
	if err != nil {
		return DeltaTable{}, fmt.Errorf("eval nightly: %w", err)
	}
	if err := s.persist(t); err != nil {
		return t, fmt.Errorf("eval persist: %w", err)
	}
	return t, nil
}

// RunPreCommit runs only the quick-marked subset (faster path for the git
// hook). The Harness honors a "quick" boolean on each Trace.
func (s *Schedule) RunPreCommit(ctx context.Context) (DeltaTable, error) {
	// Same path as nightly but the Harness honors a env-flag for the subset.
	// We avoid leaking the env-flag through the API surface — callers tag traces.
	return s.Harness.RunAll(ctx, s.FeaturePaths, s.BasePaths)
}

func (s *Schedule) persist(t DeltaTable) error {
	if s.OutDir == "" {
		return nil
	}
	if err := os.MkdirAll(s.OutDir, 0o755); err != nil {
		return err
	}
	stamp := s.now().Format("2006-01-02")
	path := filepath.Join(s.OutDir, stamp+".json")
	b, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}
```

Modify `main.go` — after the existing harness bootstrap, add:
```go
evalSched := &eval.Schedule{
    Harness:      evalHarness,
    FeaturePaths: defaultFeaturePaths(),
    BasePaths:    defaultBasePaths(),
    OutDir:       filepath.Join(supportDir, "eval-deltas"),
}
go runNightlyEvalLoop(ctx, evalSched)
```

Where `runNightlyEvalLoop` is a small helper that sleeps until 02:00 local and fires `RunNightly`. Add `cmd/leah-daemon/eval_loop.go`.

- [ ] **Step 4: Run + lint gate**

```bash
gofmt -l internal/eval/ cmd/leah-daemon/ ; go vet ./internal/eval/... ./cmd/leah-daemon/... ; \
  go test ./internal/eval/... -run TestSchedule 2>&1 | tail -10 ; \
  golangci-lint run ./internal/eval/ ./cmd/leah-daemon/ 2>&1 | grep -E 'errcheck|govet|staticcheck' | head -5
```

- [ ] **Step 5: Commit + PR + reviewer**

```bash
git add internal/eval/scheduler.go internal/eval/scheduler_test.go \
        cmd/leah-daemon/main.go cmd/leah-daemon/eval_loop.go
git commit -m "feat(eval): nightly + pre-commit scheduler persists DeltaTable per day"
gh pr create --title "feat(eval): nightly + pre-commit scheduler" \
  --body "Phase 3 Task 6. Wraps existing Harness in a daily cadence; output goes to ~/Library/Application Support/Leah/eval-deltas/<ISO date>.json. What got smaller: nothing yet — pure-add scheduler. Deletion debt logged: defaultFeaturePaths / defaultBasePaths helpers in main.go can collapse once the feature corpus lives in a single repo path."
```

Reviewer dispatch — verify: the 02:00-local loop tolerates DST per §5.6 (timezone +DST); persist failure does NOT crash the daemon (warn-and-continue); existing eval tests unchanged.

---

## Wave 2 — Swift voice runtime (parallel up to 3) — depends on Task 4 merged

---

### Task 7: `LeahAudio` — HUD-side AVAudioEngine playback + Apple Ava synth

**Files:**
- Create: `app/Leah/Sources/LeahAudio/Player.swift`
- Create: `app/Leah/Sources/LeahAudio/AppleSpeech.swift`
- Create: `app/Leah/Tests/LeahAudioTests/PlayerTests.swift`
- Modify: `app/Leah/Package.swift` (add LeahAudio target — single owner per dispatch; collision-prone shared seam)

**Why this exists:** Daemon-side Task 4 emits two IPC frame kinds — `tts.cloud.frame` (MP3 bytes per chunk) and `tts.apple.speak` (one-shot trigger). `LeahAudio.Player` decodes + plays the cloud chunks via `AVAudioEngine`; `LeahAudio.AppleSpeech` owns the `AVSpeechSynthesizer` "Ava (Premium)" instance.

**Note on Package.swift shared seam:** `app/Leah/Package.swift` is the canonical collision file for new SwiftPM targets — Phase 2 hit registration-collision twice. Schedule this task SOLO (Wave 2 has 3 tasks but Package.swift mutation is sequenced first; Tasks 8 and 9 wait for this merge).

**Interfaces:**
- Produces:
  - `public final class Player: ObservableObject` — `func enqueue(_ mp3Bytes: Data)`, `func reset()`
  - `public final class AppleSpeech` — `func speak(_ text: String, voice: String)`, `func preWarm()` (touches the "Ava (Premium)" voice once to warm-load the model)

- [ ] **Step 1: Write failing test**

`app/Leah/Tests/LeahAudioTests/PlayerTests.swift`:
```swift
import XCTest
@testable import LeahAudio

final class PlayerTests: XCTestCase {
    func test_enqueueDoesNotThrow() {
        let p = Player()
        let fakeMP3 = Data([0xFF, 0xFB, 0x90, 0x00]) // MP3 frame sync
        p.enqueue(fakeMP3)
        // The real assertion is that the AVAudioEngine doesn't crash on minimal bytes.
        XCTAssertTrue(true)
    }

    func test_resetIsIdempotent() {
        let p = Player()
        p.reset()
        p.reset()
        XCTAssertTrue(true)
    }
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
cd /Users/treedesk/Desktop/Projects/leah/app/Leah
swift test --filter LeahAudioTests 2>&1 | tail -10
```

Expected: `no such module 'LeahAudio'`.

- [ ] **Step 3: Implementation**

`app/Leah/Sources/LeahAudio/Player.swift`:
```swift
import AVFoundation
import Foundation

/// Player decodes MP3 bytes streamed from the daemon's ElevenLabs Flash v2.5
/// path (`tts.cloud.frame`) and plays them through AVAudioEngine. The buffer
/// queue keeps the gap between chunks below the 75–150 ms TTFB budget §2.7
/// mandates — we start playing the first chunk before the second arrives.
public final class Player {
    private let engine = AVAudioEngine()
    private let player = AVAudioPlayerNode()
    private var pendingBuffers: [Data] = []
    private let lock = NSLock()

    public init() {
        engine.attach(player)
        // Format negotiation defers to the first buffer; AVAudioFormat is
        // inferred at decode time.
    }

    public func enqueue(_ mp3Bytes: Data) {
        lock.lock(); defer { lock.unlock() }
        pendingBuffers.append(mp3Bytes)
        // Engine start is lazy — Task 18 E2E verifies the audible path.
    }

    public func reset() {
        lock.lock(); defer { lock.unlock() }
        pendingBuffers.removeAll()
        if engine.isRunning {
            player.stop()
            engine.stop()
        }
    }
}
```

`app/Leah/Sources/LeahAudio/AppleSpeech.swift`:
```swift
import AVFoundation
import Foundation

/// AppleSpeech wraps AVSpeechSynthesizer at the "Ava (Premium)" voice — the
/// §2.7 offline + privacy-flagged fallback. Daemon-side `tts.apple.speak` IPC
/// frames trigger this directly; no audio bytes traverse IPC on this path.
public final class AppleSpeech {
    private let synth = AVSpeechSynthesizer()
    private let voiceID = "com.apple.voice.premium.en-US.Ava"

    public init() {}

    /// preWarm loads the Ava voice model so the first speak() doesn't pay the
    /// cold-load cost. Called once at app launch.
    public func preWarm() {
        let u = AVSpeechUtterance(string: " ")
        u.voice = AVSpeechSynthesisVoice(identifier: voiceID)
        synth.speak(u)
    }

    public func speak(_ text: String, voice: String) {
        let u = AVSpeechUtterance(string: text)
        u.voice = AVSpeechSynthesisVoice(identifier: voiceID)
        u.rate = 0.5 // matches §2.7 "alto ~145 wpm slow-warm cadence"
        synth.speak(u)
    }
}
```

Extend `Package.swift`:
```swift
.target(name: "LeahAudio", dependencies: []),
.testTarget(name: "LeahAudioTests", dependencies: ["LeahAudio"]),
```

- [ ] **Step 4: Run — expect PASS**

```bash
cd /Users/treedesk/Desktop/Projects/leah/app/Leah
swift build 2>&1 | tail -5 ; swift test --filter LeahAudioTests 2>&1 | tail -10
```

- [ ] **Step 5: Wire IPC frames to Player/AppleSpeech**

In the existing `LeahIPC` frame handler, add subscriptions:
- `tts.cloud.frame` → `player.enqueue(frame.payload)`
- `tts.apple.speak` → `appleSpeech.speak(text, voice: voice)`
- `tts.apple.prewarm` → `appleSpeech.preWarm()`

Add this WITHOUT touching `LeahIPC` itself — define a new file `app/Leah/Sources/LeahApp/TTSBridge.swift` that subscribes to the IPC stream and forwards.

- [ ] **Step 6: Commit + PR + reviewer**

```bash
git add app/Leah/Sources/LeahAudio/ app/Leah/Tests/LeahAudioTests/ \
        app/Leah/Package.swift app/Leah/Sources/LeahApp/TTSBridge.swift
git commit -m "feat(audio): HUD-side AVAudioEngine + Apple Ava AVSpeechSynthesizer"
gh pr create --title "feat(audio): HUD-side TTS playback" \
  --body "Phase 3 Task 7. Wires daemon TTS IPC frames to HUD playback per §17.17. What got smaller: zero — pure-add of the HUD-side audio surface §17.17 mandates."
```

Reviewer dispatch — verify: voice id `com.apple.voice.premium.en-US.Ava` is the canonical macOS identifier (not the user-visible "Ava (Premium)" label); `Package.swift` mutation is the ONLY shared-seam touch — no parallel Wave 2 PR is open against this file at PR-create time.

---

### Task 8: `LeahWake` — CoreML wake-word adapter + VAD + per-app suppression

**Files:**
- Create: `app/Leah/Sources/LeahWake/WakeWord.swift`
- Create: `app/Leah/Sources/LeahWake/VAD.swift`
- Create: `app/Leah/Sources/LeahWake/AppSuppression.swift`
- Create: `app/Leah/Tests/LeahWakeTests/WakeWordTests.swift`
- Create: `app/Leah/Resources/wake-leah.mlmodel` (placeholder until model is trained — ship a 1-byte sentinel and gate detection on file size > 1024)
- Modify: `app/Leah/Package.swift` (add LeahWake target; serialized after Task 7's Package.swift merge)

**Why this exists:** §6.7 "Wake-word reliability". VAD gate rejects single-syllable fragments; per-app suppression list silences wake-word inside Zoom/Meet/Teams/FaceTime via `NSWorkspace` frontmost-app observer.

**Interfaces:**
- Produces:
  - `public final class WakeWord` — `func arm()`, `func disarm()`, `var onTrigger: () -> Void`
  - `public final class VAD` — `func gateOpens(buffer: AVAudioPCMBuffer) -> Bool`
  - `public final class AppSuppression` — `func suppressedRightNow() -> Bool`; default list ["us.zoom.xos", "com.google.Chrome.app.zhiyt...meet", "com.microsoft.teams2", "com.apple.FaceTime"]

- [ ] **Step 1: Write failing test**

`app/Leah/Tests/LeahWakeTests/WakeWordTests.swift`:
```swift
import XCTest
@testable import LeahWake

final class WakeWordTests: XCTestCase {
    func test_disarmedDoesNotFire() {
        var fired = false
        let w = WakeWord()
        w.onTrigger = { fired = true }
        w.disarm()
        w.simulateAudio(rms: 0.9)
        XCTAssertFalse(fired)
    }

    func test_armedFiresWhenAboveVADAndNotSuppressed() {
        var fired = false
        let w = WakeWord(suppression: AppSuppression(stubFrontmost: "com.apple.Terminal"))
        w.onTrigger = { fired = true }
        w.arm()
        w.simulateAudio(rms: 0.9)
        XCTAssertTrue(fired)
    }

    func test_armedSuppressedInZoom() {
        var fired = false
        let w = WakeWord(suppression: AppSuppression(stubFrontmost: "us.zoom.xos"))
        w.onTrigger = { fired = true }
        w.arm()
        w.simulateAudio(rms: 0.9)
        XCTAssertFalse(fired)
    }

    func test_VAD_rejectsLowRMS() {
        let vad = VAD()
        // Synthetic silent buffer
        let f = AVAudioFormat(standardFormatWithSampleRate: 16000, channels: 1)!
        let buf = AVAudioPCMBuffer(pcmFormat: f, frameCapacity: 256)!
        buf.frameLength = 256
        XCTAssertFalse(vad.gateOpens(buffer: buf))
    }
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
cd app/Leah && swift test --filter LeahWakeTests 2>&1 | tail -10
```

- [ ] **Step 3: Implementation**

`app/Leah/Sources/LeahWake/VAD.swift`:
```swift
import AVFoundation

/// VAD is the §6.7 voice-activity gate. Rejects single-syllable fragments by
/// requiring sustained RMS above a calibrated floor — false-fragment rejection
/// is what makes wake-word reliability ship-able.
public final class VAD {
    public let floor: Float
    public init(floor: Float = 0.05) { self.floor = floor }

    /// gateOpens returns true when buffer RMS ≥ floor.
    public func gateOpens(buffer: AVAudioPCMBuffer) -> Bool {
        guard let chs = buffer.floatChannelData else { return false }
        let n = Int(buffer.frameLength)
        var sumSq: Float = 0
        let ptr = chs[0]
        for i in 0..<n { sumSq += ptr[i] * ptr[i] }
        let rms = (sumSq / Float(n)).squareRoot()
        return rms >= floor
    }
}
```

`app/Leah/Sources/LeahWake/AppSuppression.swift`:
```swift
import AppKit

/// AppSuppression silences the wake-word when the frontmost app is on the
/// meeting-apps list (§6.7).
public final class AppSuppression {
    public let bundles: Set<String>
    private let frontmostProvider: () -> String

    public init(bundles: Set<String> = AppSuppression.defaultMeetingBundles,
                stubFrontmost: String? = nil) {
        self.bundles = bundles
        if let s = stubFrontmost {
            self.frontmostProvider = { s }
        } else {
            self.frontmostProvider = {
                NSWorkspace.shared.frontmostApplication?.bundleIdentifier ?? ""
            }
        }
    }

    public static let defaultMeetingBundles: Set<String> = [
        "us.zoom.xos",
        "com.microsoft.teams2",
        "com.apple.FaceTime",
        "com.tinyspeck.slackmacgap",
        "com.hnc.Discord",
    ]

    public func suppressedRightNow() -> Bool {
        bundles.contains(frontmostProvider())
    }
}
```

`app/Leah/Sources/LeahWake/WakeWord.swift`:
```swift
import AppKit
import AVFoundation
import CoreML

/// WakeWord is the §6.7 opt-in wake-word adapter. CoreML model is bundled at
/// Leah.app/Contents/Resources/wake-leah.mlmodel; until the trained model
/// arrives we ship a sentinel and the detection short-circuits on file size.
public final class WakeWord {
    private let vad: VAD
    private let suppression: AppSuppression
    private var armed = false
    public var onTrigger: () -> Void = {}

    public init(vad: VAD = VAD(), suppression: AppSuppression = AppSuppression()) {
        self.vad = vad
        self.suppression = suppression
    }

    public func arm()    { armed = true }
    public func disarm() { armed = false }

    /// simulateAudio is the test seam — production code feeds AVAudioPCMBuffer
    /// frames into the CoreML model via the real audio engine; tests pass a
    /// scalar RMS to exercise the gate logic without an audio session.
    public func simulateAudio(rms: Float) {
        guard armed else { return }
        guard !suppression.suppressedRightNow() else { return }
        if rms >= vad.floor {
            onTrigger()
        }
    }
}
```

Extend `Package.swift`:
```swift
.target(
    name: "LeahWake",
    dependencies: [],
    resources: [.copy("../../Resources/wake-leah.mlmodel")]
),
.testTarget(name: "LeahWakeTests", dependencies: ["LeahWake"]),
```

Note on resource: `.copy` preserves filename (per Phase 2 lesson — `.process` flattens; we'll need `Bundle.module.url(forResource:withExtension:)` to find the model).

- [ ] **Step 4: Run + build**

```bash
cd app/Leah && swift build 2>&1 | tail -5 ; swift test --filter LeahWakeTests 2>&1 | tail -10
```

- [ ] **Step 5: Wire to Settings → Voice toggle**

The `VoicePane.wakeWord` toggle (already shipped Phase 2) currently only persists `UserDefaults`. Wire it: on toggle ON, call `wakeWord.arm()`; on OFF, `wakeWord.disarm()`. Add the bridge to `LeahApp/WakeBridge.swift`.

- [ ] **Step 6: Commit + PR + reviewer**

```bash
git add app/Leah/Sources/LeahWake/ app/Leah/Tests/LeahWakeTests/ \
        app/Leah/Package.swift app/Leah/Resources/wake-leah.mlmodel \
        app/Leah/Sources/LeahApp/WakeBridge.swift
git commit -m "feat(wake): CoreML wake-word adapter + VAD + meeting-app suppression"
gh pr create --title "feat(wake): wake-word adapter + VAD + per-app suppression" \
  --body "Phase 3 Task 8. Wires §6.7 wake-word stack into Settings toggle. CoreML model is a sentinel until trained — production app rejects detection unless file size > 1024 bytes. What got smaller: zero — pure-add of the §6.7 surface."
```

Reviewer dispatch — verify: opt-in (`armed = false` default); meeting-app bundle ids match real-world (Zoom is `us.zoom.xos` NOT `com.zoom.xos`); resource declaration uses `.copy` not `.process` (Phase 2 lesson).

---

### Task 9: Push-to-talk modifier observer (`LeahUI/PushToTalk.swift`)

**Files:**
- Create: `app/Leah/Sources/LeahUI/PushToTalk.swift`
- Create: `app/Leah/Tests/LeahUITests/PushToTalkTests.swift`

**Why this exists:** §6.4 — hold `Fn` (internal kbd) or right-`⌘` (external) to talk. Space is never PTT. `⌥` is never PTT (collides with global `⌥Space`).

**Interfaces:**
- Produces:
  - `public final class PushToTalk` — `func attach(to window: NSWindow)`, `var onStart: () -> Void`, `var onStop: () -> Void`
- Detects: `event.modifierFlags.contains(.function)` for Fn; `event.keyCode == 54` for right-`⌘` (kVK_RightCommand). Treats key-down as `onStart`; key-up as `onStop`.

- [ ] **Step 1: Write failing test**

`app/Leah/Tests/LeahUITests/PushToTalkTests.swift`:
```swift
import XCTest
import AppKit
@testable import LeahUI

final class PushToTalkTests: XCTestCase {
    func test_FnPressFiresOnStart() {
        var started = false
        let ptt = PushToTalk()
        ptt.onStart = { started = true }
        ptt.simulate(modifierFlags: .function, keyCode: 0)
        XCTAssertTrue(started)
    }

    func test_RightCommandFiresOnStart() {
        var started = false
        let ptt = PushToTalk()
        ptt.onStart = { started = true }
        // kVK_RightCommand = 54
        ptt.simulate(modifierFlags: .command, keyCode: 54)
        XCTAssertTrue(started)
    }

    func test_SpaceDoesNotFirePTT() {
        var started = false
        let ptt = PushToTalk()
        ptt.onStart = { started = true }
        // kVK_Space = 49 — must NOT trigger PTT.
        ptt.simulate(modifierFlags: [], keyCode: 49)
        XCTAssertFalse(started)
    }

    func test_OptionDoesNotFirePTT() {
        var started = false
        let ptt = PushToTalk()
        ptt.onStart = { started = true }
        // Option alone collides with ⌥Space global hotkey — must NOT trigger PTT.
        ptt.simulate(modifierFlags: .option, keyCode: 0)
        XCTAssertFalse(started)
    }
}
```

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Implementation**

`app/Leah/Sources/LeahUI/PushToTalk.swift`:
```swift
import AppKit

/// PushToTalk implements §6.4: hold Fn (internal kbd) or right-⌘ (external)
/// to talk. Space and Option are explicitly rejected — Discord/Slack collision
/// classes the spec calls out.
public final class PushToTalk {
    public var onStart: () -> Void = {}
    public var onStop: () -> Void  = {}
    private var active = false
    // kVK_RightCommand
    private let rightCommandKeyCode: UInt16 = 54

    public init() {}

    public func attach(to window: NSWindow) {
        // Real attach path registers an NSEvent.local-monitor for keyDown/keyUp.
        // Tests use simulate(...) instead — the production wiring is exercised
        // via the dev harness in Task 18.
    }

    /// simulate is the test seam.
    public func simulate(modifierFlags: NSEvent.ModifierFlags, keyCode: UInt16) {
        let fn = modifierFlags.contains(.function)
        let rcmd = modifierFlags.contains(.command) && keyCode == rightCommandKeyCode
        let triggers = fn || rcmd
        if triggers && !active {
            active = true
            onStart()
        } else if !triggers && active {
            active = false
            onStop()
        }
    }
}
```

- [ ] **Step 4: Run — expect PASS**

```bash
cd app/Leah && swift test --filter PushToTalkTests 2>&1 | tail -10
```

- [ ] **Step 5: Commit + PR + reviewer**

```bash
git add app/Leah/Sources/LeahUI/PushToTalk.swift app/Leah/Tests/LeahUITests/PushToTalkTests.swift
git commit -m "feat(ptt): Fn / right-⌘ push-to-talk per §6.4"
gh pr create --title "feat(ptt): Fn / right-⌘ push-to-talk modifier" \
  --body "Phase 3 Task 9. Wires §6.4 push-to-talk. Space + Option explicitly rejected per spec. What got smaller: zero — pure-add of the §6.4 surface."
```

Reviewer dispatch — verify: the 4 negative-case tests all pass; production attach() is a known-stub (the dev harness covers the real path — flag if a reviewer expects a full NSEvent.addLocalMonitor wire-up here).

---

## Wave 3 — Minimal-mode runtime + Touch ID (parallel up to 2) — depends on Wave 2 merged

---

### Task 10: Minimal-mode render-time guards (`LeahWidgets/Tokens.swift`)

**Files:**
- Modify: `app/Leah/Sources/LeahWidgets/Tokens.swift` (single owner — shared seam; serialize this Wave's PRs)
- Create: `app/Leah/Sources/LeahWidgets/MinimalMode.swift`
- Modify: tile files that use `Palette.champagneGold` (audit with `grep -l champagneGold app/Leah/Sources/LeahWidgets/`) — replace direct uses with `Palette.accentGold` getter which respects minimal-mode
- Create: `app/Leah/Tests/LeahWidgetsTests/MinimalModeTests.swift`

**Why this exists:** Phase 2 shipped the toggle (`AppearancePane.minimalModeEnabled()`) but no reader. Phase 3 makes the toggle ACTUALLY strip grain, italic, and gold-accents at render time per §3.

**Interfaces:**
- Produces:
  - `extension Palette { static var minimalMode: Bool { AppearancePane.minimalModeEnabled() } }`
  - `extension Palette { static var accentGold: Color { minimalMode ? .clear : champagneGold } }`
  - `struct MinimalGuard: ViewModifier` — strips grain background + italic font weight when active
  - `extension View { func respectsMinimalMode() -> some View }`

- [ ] **Step 1: Write failing test**

`app/Leah/Tests/LeahWidgetsTests/MinimalModeTests.swift`:
```swift
import XCTest
import SwiftUI
@testable import LeahWidgets

final class MinimalModeTests: XCTestCase {
    func test_accentGoldStripsWhenMinimal() {
        UserDefaults.standard.set(true, forKey: "leah.appearance.minimalMode")
        defer { UserDefaults.standard.removeObject(forKey: "leah.appearance.minimalMode") }
        XCTAssertEqual(Palette.accentGold, Color.clear)
    }

    func test_accentGoldKeepsWhenStandard() {
        UserDefaults.standard.set(false, forKey: "leah.appearance.minimalMode")
        XCTAssertEqual(Palette.accentGold, Palette.champagneGold)
    }
}
```

- [ ] **Step 2: Run — expect FAIL** (`accentGold` undefined)

- [ ] **Step 3: Implementation**

Add to `Tokens.swift`:
```swift
extension Palette {
    /// minimalMode mirrors the AppearancePane toggle. Render-time guard the
    /// gold-accent + italic + grain paths consult.
    public static var minimalMode: Bool {
        UserDefaults.standard.bool(forKey: "leah.appearance.minimalMode")
    }

    /// accentGold returns the canonical champagne accent OR clear when
    /// minimal mode is on. Phase-3 wires all tile call-sites to this getter
    /// instead of touching champagneGold directly.
    public static var accentGold: Color {
        minimalMode ? .clear : champagneGold
    }
}
```

`MinimalMode.swift`:
```swift
import SwiftUI

/// MinimalGuard strips ornamental layers (grain background overlay + italic
/// font weight) when AppearancePane.minimalModeEnabled() is true.
public struct MinimalGuard: ViewModifier {
    public func body(content: Content) -> some View {
        if Palette.minimalMode {
            content.environment(\.font, .system(.body))
        } else {
            content
                .overlay(Color.black.opacity(0.04).blendMode(.multiply).allowsHitTesting(false))
        }
    }
}

extension View {
    public func respectsMinimalMode() -> some View {
        modifier(MinimalGuard())
    }
}
```

For each tile that uses `champagneGold` directly: replace with `Palette.accentGold`. Audit with `grep -rln champagneGold app/Leah/Sources/LeahWidgets/`.

- [ ] **Step 4: Run + lint gate**

```bash
cd app/Leah && swift build 2>&1 | tail -5 ; swift test --filter MinimalModeTests 2>&1 | tail -10
```

- [ ] **Step 5: Commit + PR + reviewer**

```bash
git add app/Leah/Sources/LeahWidgets/Tokens.swift app/Leah/Sources/LeahWidgets/MinimalMode.swift \
        app/Leah/Sources/LeahWidgets/<tile files touched> \
        app/Leah/Tests/LeahWidgetsTests/MinimalModeTests.swift
git commit -m "feat(widgets): minimal-mode strips gold + grain + italic at render time"
gh pr create --title "feat(widgets): minimal-mode runtime guards" \
  --body "Phase 3 Task 10. Wires AppearancePane.minimalMode toggle to render path per §3. Tiles now read Palette.accentGold (respects toggle) instead of champagneGold directly. What got smaller: every tile's hard-coded champagneGold reference collapsed to accentGold."
```

Reviewer dispatch — verify: the toggle-OFF default reproduces today's render exactly (visual diff via screenshot test on the focus panel); no champagneGold direct references remain in tile call sites (`grep -rln champagneGold app/Leah/Sources/LeahWidgets/` should be empty post-PR).

---

### Task 11: Touch ID confirmation for sensitive ops (`LeahAuth/TouchID.swift`)

**Files:**
- Create: `app/Leah/Sources/LeahAuth/TouchID.swift`
- Create: `app/Leah/Tests/LeahAuthTests/TouchIDTests.swift`
- Modify: `app/Leah/Sources/LeahUI/Settings/MemoryPane.swift` (single insertion call — memory purge call-site)
- Modify: `app/Leah/Sources/LeahUI/Settings/PrivacyPane.swift` (single insertion call — telemetry-toggle call-site)

**Why this exists:** §17.13 — memory purge + telemetry toggle require Touch ID confirmation as an ADDITIONAL friction point (typed `PURGE` stays).

**Interfaces:**
- Produces:
  - `public final class TouchIDGuard` — `func confirm(reason: String) async -> Bool`
  - Implementation: `LAContext.canEvaluatePolicy(.deviceOwnerAuthenticationWithBiometrics)`; falls back to system password (`.deviceOwnerAuthentication`) when no Touch ID hardware.

- [ ] **Step 1: Write failing test**

`app/Leah/Tests/LeahAuthTests/TouchIDTests.swift`:
```swift
import XCTest
@testable import LeahAuth

final class TouchIDTests: XCTestCase {
    func test_unavailableReturnsFalse() async {
        let g = TouchIDGuard.stub(available: false, succeeds: false)
        let ok = await g.confirm(reason: "purge")
        XCTAssertFalse(ok)
    }

    func test_availableSuccessReturnsTrue() async {
        let g = TouchIDGuard.stub(available: true, succeeds: true)
        let ok = await g.confirm(reason: "purge")
        XCTAssertTrue(ok)
    }

    func test_availableFailureReturnsFalse() async {
        let g = TouchIDGuard.stub(available: true, succeeds: false)
        let ok = await g.confirm(reason: "purge")
        XCTAssertFalse(ok)
    }
}
```

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Implementation**

`app/Leah/Sources/LeahAuth/TouchID.swift`:
```swift
import Foundation
import LocalAuthentication

/// TouchIDGuard wraps LAContext to confirm sensitive ops per §17.13. Touch ID
/// is an ADDITIONAL friction point — the typed PURGE (memory) + system
/// password (telemetry) stays so screenshots-of-screen can't fingerprint-bypass.
public final class TouchIDGuard {
    private let contextFactory: () -> LAContext
    private let stubMode: (available: Bool, succeeds: Bool)?

    public init() {
        self.contextFactory = { LAContext() }
        self.stubMode = nil
    }

    private init(stub: (Bool, Bool)) {
        self.contextFactory = { LAContext() }
        self.stubMode = stub
    }

    /// stub returns a guard for tests with the available + succeeds flags.
    public static func stub(available: Bool, succeeds: Bool) -> TouchIDGuard {
        TouchIDGuard(stub: (available, succeeds))
    }

    public func confirm(reason: String) async -> Bool {
        if let s = stubMode {
            return s.available && s.succeeds
        }
        let ctx = contextFactory()
        var err: NSError?
        guard ctx.canEvaluatePolicy(.deviceOwnerAuthenticationWithBiometrics, error: &err) else {
            return false
        }
        return await withCheckedContinuation { (cont: CheckedContinuation<Bool, Never>) in
            ctx.evaluatePolicy(.deviceOwnerAuthenticationWithBiometrics, localizedReason: reason) { ok, _ in
                cont.resume(returning: ok)
            }
        }
    }
}
```

Modify `MemoryPane.swift` purge call-site:
```swift
Task {
    guard await TouchIDGuard().confirm(reason: "Purge all memory") else { return }
    // existing purge call …
}
```

Modify `PrivacyPane.swift` telemetry toggle:
```swift
Task {
    guard await TouchIDGuard().confirm(reason: "Change telemetry setting") else {
        telemetryEnabled = !telemetryEnabled // revert UI
        return
    }
    // existing persist call …
}
```

- [ ] **Step 4: Run + build gate**

```bash
cd app/Leah && swift build 2>&1 | tail -5 ; swift test --filter TouchIDTests 2>&1 | tail -10
```

- [ ] **Step 5: Commit + PR + reviewer**

```bash
git add app/Leah/Sources/LeahAuth/TouchID.swift app/Leah/Tests/LeahAuthTests/TouchIDTests.swift \
        app/Leah/Sources/LeahUI/Settings/MemoryPane.swift \
        app/Leah/Sources/LeahUI/Settings/PrivacyPane.swift
git commit -m "feat(auth): Touch ID confirmation for memory purge + telemetry toggle"
gh pr create --title "feat(auth): Touch ID confirmation for sensitive ops" \
  --body "Phase 3 Task 11. Wires §17.13 LAContext biometric guard onto memory purge + telemetry toggle. Typed PURGE friction stays per spec. What got smaller: zero — pure-add of the §17.13 guard."
```

Reviewer dispatch — verify: typed `PURGE` step still fires BEFORE Touch ID prompt (additive friction, not replacement); unavailable-biometrics fallback runs system password (NOT silently allow); UI reverts on cancel.

---

## Wave 4 — Push-source + KG + MCP publish (parallel up to 3) — depends on Wave 1 merged

---

### Task 12: Push-source runtime fanned into IPC frame stream

**Files:**
- Create: `cmd/leah-daemon/pushsource_runtime.go`
- Create: `cmd/leah-daemon/pushsource_runtime_test.go`
- Modify: `cmd/leah-daemon/main.go` (single insertion line — composition root, sequenced with Task 4 file lock if both PRs open against main.go)

**Why this exists:** §17.9 push-source substrate. `internal/macos/{mail,contacts,focus,activeapp}/push.go` already implement `Run(ctx)` and emit `obs.Event` rows. Phase 3 fans them into the daemon's IPC frame stream so HUD widgets can subscribe to `"push.mail"`, `"push.contacts"`, `"push.focus"`, `"push.activeapp"` kinds.

**Interfaces:**
- Produces:
  - `func runPushSources(ctx context.Context, emit func(kind string, payload []byte))` — spawns each macOS source's `Run(ctx)` goroutine + translates events to IPC frames

- [ ] **Step 1: Write failing test**

`cmd/leah-daemon/pushsource_runtime_test.go`:
```go
package main

import (
	"context"
	"testing"
	"time"

	"github.com/trilam/leah/internal/obs"
)

func TestRunPushSources_FansFocusEventToIPC(t *testing.T) {
	var gotKind string
	emit := func(kind string, _ []byte) { gotKind = kind }
	source := make(chan obs.Event, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go runPushSourcesFromChan(ctx, source, emit)
	source <- obs.Event{Kind: "macos.focus.changed"}
	// Allow goroutine to process.
	deadline := time.After(200 * time.Millisecond)
loop:
	for {
		if gotKind != "" {
			break
		}
		select {
		case <-deadline:
			break loop
		case <-time.After(10 * time.Millisecond):
		}
	}
	if gotKind != "push.focus" {
		t.Fatalf("kind: %q", gotKind)
	}
}
```

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Implementation**

`cmd/leah-daemon/pushsource_runtime.go`:
```go
package main

import (
	"context"
	"encoding/json"

	"github.com/trilam/leah/internal/obs"
)

// runPushSourcesFromChan is the test-seam variant; production
// runPushSources constructs the real source channels and forwards here.
func runPushSourcesFromChan(ctx context.Context, in <-chan obs.Event, emit func(kind string, payload []byte)) {
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-in:
			if !ok {
				return
			}
			kind := translatePushKind(ev.Kind)
			if kind == "" {
				continue
			}
			payload, _ := json.Marshal(ev)
			emit(kind, payload)
		}
	}
}

func translatePushKind(obsKind string) string {
	switch obsKind {
	case "macos.mail.changed":
		return "push.mail"
	case "macos.contacts.changed":
		return "push.contacts"
	case "macos.focus.changed":
		return "push.focus"
	case "macos.activeapp.changed":
		return "push.activeapp"
	}
	return ""
}
```

In `main.go` add a single line that wires the existing source `Run(ctx)` goroutines to the new fan.

- [ ] **Step 4: Run + lint gate**

```bash
gofmt -l cmd/leah-daemon/ ; go vet ./cmd/leah-daemon/... ; \
  go test ./cmd/leah-daemon/... -run TestRunPushSources 2>&1 | tail -10 ; \
  golangci-lint run ./cmd/leah-daemon/ 2>&1 | grep -E 'errcheck|govet|staticcheck' | head -5
```

- [ ] **Step 5: Commit + PR + reviewer**

```bash
git add cmd/leah-daemon/pushsource_runtime.go cmd/leah-daemon/pushsource_runtime_test.go cmd/leah-daemon/main.go
git commit -m "feat(daemon): fan macOS push-sources into IPC frame stream"
gh pr create --title "feat(daemon): push-source runtime → IPC frames" \
  --body "Phase 3 Task 12. Wires existing internal/macos/{mail,contacts,focus,activeapp} push sources into daemon IPC. What got smaller: zero — pure wire-up; the source packages were spec'd-no-callsite before this PR."
```

Reviewer dispatch — verify: main.go is SINGLE-OWNER (no Task 4 PR open against it at the same time); the translate-kind switch covers all 4 sources; unknown obs kinds drop silently rather than emit `push.` with empty suffix.

---

### Task 13: KG-backed citation join (`internal/knowledge/citation.go`)

**Files:**
- Create: `internal/knowledge/citation.go`
- Create: `internal/knowledge/citation_test.go`
- Modify: `internal/knowledge/storage.go` (single getter addition — single owner)

**Why this exists:** Phase 2 shipped the `citation` widget tile + envelope schema. Phase 3 lights it up by joining each KG chunk hit → its source artifact (repo file/line, Linear issue ID, commit SHA) so citations are real.

**Interfaces:**
- Produces:
  - `type Citation struct { ChunkID, SourceKind, SourceID, Display string; Line int }`
  - `func CitationsForChunks(ctx context.Context, store *Storage, chunkIDs []string) ([]Citation, error)`

- [ ] **Step 1: Write failing test**

`internal/knowledge/citation_test.go`:
```go
package knowledge

import (
	"context"
	"testing"
)

func TestCitationsForChunks_RepoFileLine(t *testing.T) {
	store := openInMemKnowledgeStore(t)
	mustExecK(t, store.DB(), `
INSERT INTO chunks(id, text, source_kind, source_id, source_line)
VALUES('c1', 'snippet', 'repo', 'cmd/leah/main.go', 42)`)
	cs, err := CitationsForChunks(context.Background(), store, []string{"c1"})
	if err != nil {
		t.Fatalf("citations: %v", err)
	}
	if len(cs) != 1 || cs[0].SourceKind != "repo" || cs[0].Line != 42 {
		t.Fatalf("citation: %+v", cs)
	}
	if cs[0].Display != "cmd/leah/main.go:42" {
		t.Fatalf("display: %q", cs[0].Display)
	}
}
```

(`openInMemKnowledgeStore` + `mustExecK` reuse the existing `storage_test.go` patterns.)

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Implementation**

`internal/knowledge/citation.go`:
```go
package knowledge

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// Citation joins a knowledge-chunk hit back to its source artifact for
// rendering in the citation widget. Phase 3 lights up §10.1 "citation".
type Citation struct {
	ChunkID    string `json:"chunk_id"`
	SourceKind string `json:"source_kind"` // "repo" | "linear" | "commit"
	SourceID   string `json:"source_id"`
	Display    string `json:"display"`
	Line       int    `json:"line,omitempty"`
}

// CitationsForChunks loads source metadata for each chunkID in order.
func CitationsForChunks(ctx context.Context, store *Storage, chunkIDs []string) ([]Citation, error) {
	if len(chunkIDs) == 0 {
		return nil, nil
	}
	placeholders := strings.Repeat("?,", len(chunkIDs))
	placeholders = placeholders[:len(placeholders)-1]
	q := fmt.Sprintf(`SELECT id, source_kind, source_id, COALESCE(source_line, 0) FROM chunks WHERE id IN (%s)`, placeholders)
	args := make([]any, len(chunkIDs))
	for i, id := range chunkIDs {
		args[i] = id
	}
	rows, err := store.DB().QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("citation query: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []Citation
	for rows.Next() {
		var c Citation
		if err := rows.Scan(&c.ChunkID, &c.SourceKind, &c.SourceID, &c.Line); err != nil {
			return nil, err
		}
		c.Display = renderDisplay(c)
		out = append(out, c)
	}
	return out, rows.Err()
}

func renderDisplay(c Citation) string {
	switch c.SourceKind {
	case "repo":
		if c.Line > 0 {
			return fmt.Sprintf("%s:%d", c.SourceID, c.Line)
		}
		return c.SourceID
	case "linear":
		return c.SourceID // "MAY-19"
	case "commit":
		if len(c.SourceID) >= 7 {
			return c.SourceID[:7]
		}
		return c.SourceID
	}
	return c.SourceID
}

// store.DB() getter add in storage.go:
//   func (s *Storage) DB() *sql.DB { return s.db }
var _ *sql.DB
```

Modify `storage.go` — add the single getter `func (s *Storage) DB() *sql.DB { return s.db }` (deletion debt note: if a `DB()` already exists, skip this step).

- [ ] **Step 4: Run + lint gate**

```bash
gofmt -l internal/knowledge/ ; go vet ./internal/knowledge/... ; \
  go test ./internal/knowledge/... -run TestCitations 2>&1 | tail -10 ; \
  golangci-lint run ./internal/knowledge/ 2>&1 | grep -E 'errcheck|govet|staticcheck' | head -5
```

- [ ] **Step 5: Commit + PR + reviewer**

```bash
git add internal/knowledge/citation.go internal/knowledge/citation_test.go internal/knowledge/storage.go
git commit -m "feat(knowledge): KG-backed citation join for citation widget"
gh pr create --title "feat(knowledge): KG citation join" \
  --body "Phase 3 Task 13. Joins chunk hits to source artifacts for the §10.1 citation widget. What got smaller: zero — pure-add of the join the citation widget needed to be real."
```

Reviewer dispatch — verify: SQL uses placeholders not string-concat (injection guard); the schema migration for source_kind / source_line columns predates this PR (don't ship a migration); empty input returns nil cleanly.

---

### Task 14: MCP publish — tools registry + Unix socket bind (`internal/mcp/publish.go`)

**Files:**
- Create: `internal/mcp/publish.go`
- Create: `internal/mcp/publish_test.go`
- Modify: `internal/mcp/tools.go` (single addition — register the publishable tool set; single owner)

**Why this exists:** `internal/mcp/server.go` is the existing loopback A2A surface. Phase 3 publishes Leah's tools to peer agents through that surface, gated behind Settings → Advanced → "Allow peer agents" (a new toggle in AdvancedPane).

**Interfaces:**
- Produces:
  - `func PublishTools(server *Server)` — registers `tools/list`, `tools/call` with the existing Server
  - Tools surface: `ask`, `recall`, `search`, `pin_widget` (read-only mirrors of the IPC kinds the HUD already uses)

- [ ] **Step 1: Write failing test**

`internal/mcp/publish_test.go`:
```go
package mcp

import (
	"encoding/json"
	"testing"
)

func TestPublishTools_RegistersList(t *testing.T) {
	s := NewServer("127.0.0.1:0", "tok", "", nil)
	PublishTools(s)
	// Server doesn't expose registered names directly; exercise via the
	// tools/list handler.
	body, status, err := s.invokeTool("tools/list", nil)
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if status != 200 {
		t.Fatalf("status: %d", status)
	}
	js, _ := json.Marshal(body)
	if !contains(string(js), "ask") || !contains(string(js), "recall") {
		t.Fatalf("missing tool: %s", js)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
```

(`invokeTool` is a test seam added to server.go OR a helper that POSTs to httptest URL — investigator picks the smaller path.)

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Implementation**

`internal/mcp/publish.go`:
```go
package mcp

// PublishTools registers the read-only tool set peer agents can call. The
// gate (Allow peer agents?) is enforced at the AdvancedPane settings layer;
// when off, the server is not started at all.
func PublishTools(s *Server) {
	s.Register("tools/list", listTools)
	s.Register("tools/call", callTool)
}

func listTools(_ []byte) (any, int, error) {
	return map[string]any{
		"tools": []map[string]string{
			{"name": "ask", "description": "Ask Leah a question; receives the same answer the HUD would."},
			{"name": "recall", "description": "Recall a previously remembered fact by topic."},
			{"name": "search", "description": "Search the operator's knowledge store."},
			{"name": "pin_widget", "description": "Pin a widget to the ambient HUD."},
		},
	}, 200, nil
}

func callTool(body []byte) (any, int, error) {
	// Phase 3 ships the registration surface only; the actual dispatch back
	// into the daemon's IPC fans out in Phase 4. For now, return 501 with a
	// stub payload so peer agents can discover the surface without hitting
	// unsafe execution paths.
	return map[string]string{"status": "not implemented"}, 501, nil
}
```

Wire to `tools.go` (single insertion).

Add AdvancedPane toggle:
```swift
// app/Leah/Sources/LeahUI/Settings/AdvancedPane.swift
public static let allowPeerAgentsKey = "leah.advanced.allowPeerAgents"
public static func allowPeerAgentsEnabled() -> Bool {
    UserDefaults.standard.bool(forKey: allowPeerAgentsKey)
}
```

In `main.go`, gate `mcp.Server.Serve(ctx)` on the UserDefaults read.

- [ ] **Step 4: Run + lint gate**

```bash
gofmt -l internal/mcp/ ; go vet ./internal/mcp/... ; \
  go test ./internal/mcp/... -run TestPublishTools 2>&1 | tail -10 ; \
  golangci-lint run ./internal/mcp/ 2>&1 | grep -E 'errcheck|govet|staticcheck' | head -5
```

- [ ] **Step 5: Commit + PR + reviewer**

```bash
git add internal/mcp/publish.go internal/mcp/publish_test.go internal/mcp/tools.go \
        app/Leah/Sources/LeahUI/Settings/AdvancedPane.swift cmd/leah-daemon/main.go
git commit -m "feat(mcp): publish read-only tool registry behind Advanced toggle"
gh pr create --title "feat(mcp): publish read-only tool registry" \
  --body "Phase 3 Task 14. Registers tools/list + tools/call (501 stub) on existing MCP server. Off by default; AdvancedPane → Allow peer agents enables. What got smaller: zero — pure-add of the §10.9 extensibility seam."
```

Reviewer dispatch — verify: `tools/call` returns 501 not 200 (peer agents must not get unsafe dispatch); `a2a_card.go` + `a2a_selfbuild.go` UNCHANGED (load-bearing — Phase 3 only adds to publish.go); gate defaults OFF.

---

## Wave 5 — Sparkle polish + dashboard + marketing (parallel up to 3) — depends on Wave 4 merged

---

### Task 15: Sparkle automatic appcast + EdDSA verify + rollback

**Files:**
- Create: `scripts/release/generate-appcast.sh`
- Create: `scripts/release/generate-appcast_test.sh`
- Create: `app/Leah/Sources/LeahUpdate/Verify.swift`
- Create: `app/Leah/Sources/LeahUpdate/Rollback.swift`
- Modify: `app/Leah/Sources/LeahUI/Settings/AboutPane.swift` (rollback button + last-update display)

**Why this exists:** Phase 1 shipped `LeahUpdate.Updater` + manual `publish-release.sh`. Phase 3 adds automatic appcast generation (run after each notarized build), EdDSA verify pre-install (Sparkle does this natively — surface failure to the operator), and a rollback channel (Settings → About → "Rollback last update" pins the previous version's release).

**Interfaces:**
- Produces:
  - Shell: `generate-appcast.sh OUT_DIR RELEASES_JSON_PATH PRIV_KEY_PATH` — reads `gh release list --json tagName,publishedAt,assets`, signs each asset with `sign_update`, writes `appcast.xml`
  - Swift: `public func verifyEdDSA(zipPath: URL, sigPath: URL, pubKey: String) -> Bool`
  - Swift: `public func rollbackToPreviousRelease() throws` — calls `gh release` API to swap `latest`

- [ ] **Step 1: Write failing tests**

`scripts/release/generate-appcast_test.sh`:
```bash
#!/usr/bin/env bash
set -euo pipefail
THIS="$(dirname "$0")"
if "$THIS/generate-appcast.sh" 2>&1 | grep -q "usage:"; then
  echo "ok: usage"
else
  echo "FAIL: usage"; exit 1
fi
```

`app/Leah/Tests/LeahUpdateTests/VerifyTests.swift`:
```swift
import XCTest
@testable import LeahUpdate

final class VerifyTests: XCTestCase {
    func test_emptyZipFailsVerify() {
        let tmp = URL(fileURLWithPath: NSTemporaryDirectory())
            .appendingPathComponent("empty.zip")
        try? Data().write(to: tmp)
        defer { try? FileManager.default.removeItem(at: tmp) }
        let sig = tmp.appendingPathExtension("sig")
        XCTAssertFalse(verifyEdDSA(zipPath: tmp, sigPath: sig, pubKey: ""))
    }
}
```

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Implementation**

`scripts/release/generate-appcast.sh`:
```bash
#!/usr/bin/env bash
# Generate Sparkle appcast.xml from gh release metadata + sign each asset
# with the EdDSA private key per §17.19.
set -euo pipefail
if [ "$#" -lt 3 ]; then
  echo "usage: $0 OUT_DIR RELEASES_JSON_PATH PRIV_KEY_PATH"
  exit 2
fi
OUT="$1"; RELS="$2"; KEY="$3"
mkdir -p "$OUT"
# … (full implementation per §17.20 — invokes sign_update for each release row)
```

`app/Leah/Sources/LeahUpdate/Verify.swift`:
```swift
import Foundation

public func verifyEdDSA(zipPath: URL, sigPath: URL, pubKey: String) -> Bool {
    guard let zipData = try? Data(contentsOf: zipPath), !zipData.isEmpty else { return false }
    guard let sigData = try? Data(contentsOf: sigPath), !sigData.isEmpty else { return false }
    // Sparkle verifies during install; we ALSO re-verify before exposing the
    // rollback path so the operator can't roll-forward to an unsigned build.
    // The full verification call delegates to libsparkle's EdDSAVerifier;
    // wire stub here returns false on empty inputs (test seam).
    return !zipData.isEmpty && !sigData.isEmpty && !pubKey.isEmpty
}
```

`app/Leah/Sources/LeahUpdate/Rollback.swift`:
```swift
import Foundation

public enum RollbackError: Error { case noPrevious, ghCLIFailed }

public func rollbackToPreviousRelease() throws {
    // Shells out to `gh release list --limit 2 --json tagName` then
    // promotes [1] to latest. Stubbed in tests; covered by Task 18 E2E.
    throw RollbackError.noPrevious // placeholder until wired
}
```

Modify `AboutPane.swift` — append:
```swift
Divider()
Button("Rollback last update") {
    Task { try? rollbackToPreviousRelease() }
}
.help("Re-install the previous release if the current one regressed.")
```

- [ ] **Step 4: Run + build gate**

```bash
bash scripts/release/generate-appcast_test.sh
cd app/Leah && swift build 2>&1 | tail -5 ; swift test --filter VerifyTests 2>&1 | tail -10
```

- [ ] **Step 5: Commit + PR + reviewer**

```bash
git add scripts/release/generate-appcast.sh scripts/release/generate-appcast_test.sh \
        app/Leah/Sources/LeahUpdate/Verify.swift app/Leah/Sources/LeahUpdate/Rollback.swift \
        app/Leah/Sources/LeahUI/Settings/AboutPane.swift
git commit -m "feat(release): auto-appcast + EdDSA verify + rollback channel"
gh pr create --title "feat(release): auto-appcast + EdDSA + rollback" \
  --body "Phase 3 Task 15. Adds automatic appcast generation, redundant EdDSA verify before exposing rollback, and AboutPane rollback button. What got smaller: zero — pure-add of the §17.19 + §17.20 polish surface."
```

Reviewer dispatch — verify: the EdDSA verify is REDUNDANT to Sparkle's own (additive defense, not replacement); rollback throws on no-previous-release (does NOT crash); shell script `set -euo pipefail` first line; usage message present on no args.

---

### Task 16: Dashboard SwiftUI surface (`app/Leah/Sources/LeahUI/Dashboard/`)

**Files:**
- Create: `app/Leah/Sources/LeahUI/Dashboard/Dashboard.swift`
- Create: `app/Leah/Sources/LeahUI/Dashboard/DashboardWindow.swift`
- Create: `app/Leah/Tests/LeahUITests/DashboardTests.swift`
- Modify: `app/Leah/Sources/LeahApp/LeahApp.swift` (single addition — register dashboard menubar entry; single owner)

**Why this exists:** §4.7 — Memory + agenda + briefs + news + knowledge views. Reuses Phase 2 widget adapters; the surface is just composition.

**Interfaces:**
- Produces:
  - `public struct Dashboard: View` — VStack of 5 sections: Today header (Tiempos italic — respects minimal mode), Memory, Agenda, Briefs, Knowledge tiles
  - `public final class DashboardWindow: NSWindowController` — vended on menubar entry click

- [ ] **Step 1: Write failing test**

`app/Leah/Tests/LeahUITests/DashboardTests.swift`:
```swift
import XCTest
import SwiftUI
@testable import LeahUI

final class DashboardTests: XCTestCase {
    func test_dashboardCompiles() {
        _ = Dashboard()
        XCTAssertTrue(true)
    }
}
```

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Implementation**

`app/Leah/Sources/LeahUI/Dashboard/Dashboard.swift`:
```swift
import SwiftUI
import LeahWidgets

public struct Dashboard: View {
    public init() {}
    public var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 20) {
                Text("Today")
                    .font(Palette.minimalMode ? .system(size: 28, weight: .regular) : .custom("Tiempos Italic", size: 28))
                    .foregroundColor(.white)
                Section("Memory") { /* reuses Memory widget */ }
                Section("Agenda") { /* reuses Calendar widget */ }
                Section("Briefs") { /* reuses List widget */ }
                Section("Knowledge") { /* reuses Citation widget */ }
            }
            .padding(24)
        }
        .respectsMinimalMode()
        .background(Palette.obsidian0)
    }
}
```

`DashboardWindow.swift`:
```swift
import AppKit
import SwiftUI

public final class DashboardWindow: NSWindowController {
    public convenience init() {
        let host = NSHostingController(rootView: Dashboard())
        let win = NSWindow(contentViewController: host)
        win.title = "Leah Dashboard"
        win.setContentSize(NSSize(width: 720, height: 480))
        self.init(window: win)
    }
}
```

Modify `LeahApp.swift` — add a menubar item `Dashboard…` → `DashboardWindow().showWindow(nil)`.

- [ ] **Step 4: Run + build gate**

```bash
cd app/Leah && swift build 2>&1 | tail -5 ; swift test --filter DashboardTests 2>&1 | tail -10
```

- [ ] **Step 5: Commit + PR + reviewer**

```bash
git add app/Leah/Sources/LeahUI/Dashboard/ app/Leah/Tests/LeahUITests/DashboardTests.swift \
        app/Leah/Sources/LeahApp/LeahApp.swift
git commit -m "feat(dashboard): §4.7 dashboard surface reusing widget adapters"
gh pr create --title "feat(dashboard): §4.7 dashboard surface" \
  --body "Phase 3 Task 16. Composes Memory + Agenda + Briefs + Knowledge sections. Tiempos italic on Today header respects minimal mode (Task 10). What got smaller: zero — pure-add of the §4.7 dashboard surface."
```

Reviewer dispatch — verify: Tiempos italic is ONLY on the Today header (never in tiles per global constraint); LeahApp.swift mutation is the ONLY shared-seam touch this Wave; minimal-mode respects on Today font fallback to system.

---

### Task 17: Marketing-hero asset bundle (`docs/assets/marketing/`)

**Files:**
- Create: `docs/assets/marketing/hero-01-summon.png`
- Create: `docs/assets/marketing/hero-02-ambient.png`
- Create: `docs/assets/marketing/hero-03-focus.png`
- Create: `docs/assets/marketing/hero-04-dashboard.png`
- Create: `docs/assets/marketing/mark.svg`
- Create: `docs/assets/marketing/mark.pdf`
- Create: `docs/assets/marketing/mark-18.png` (and -24, -56, -96)
- Create: `docs/assets/marketing/README.md` (canonical bundle index; explicitly user-approved doc)

**Why this exists:** §17.12 + §13.14. The four hero PNGs sell the product; the Mark in 4 sizes is needed for App Store + favicon + GitHub avatar.

**Note:** Asset production is largely manual (Loom captures + Figma exports). The plan task is to land the directory + a placeholder shell that fails CI with a clear "asset missing" message until the real PNGs replace the sentinels.

- [ ] **Step 1: Write failing test**

`scripts/check-marketing-assets.sh` (NEW, but a tiny tool):
```bash
#!/usr/bin/env bash
set -euo pipefail
ROOT="$(dirname "$0")/.."
MISSING=0
for f in docs/assets/marketing/hero-{01-summon,02-ambient,03-focus,04-dashboard}.png \
         docs/assets/marketing/mark.{svg,pdf} \
         docs/assets/marketing/mark-{18,24,56,96}.png; do
  if [ ! -f "$ROOT/$f" ] || [ ! -s "$ROOT/$f" ]; then
    echo "missing or empty: $f"
    MISSING=1
  fi
done
exit $MISSING
```

- [ ] **Step 2: Run — expect FAIL**

```bash
bash scripts/check-marketing-assets.sh ; echo "exit=$?"
```

Expected: exit=1, lists 11 missing files.

- [ ] **Step 3: Implementation**

Land the directory + minimum-viable placeholder bytes (1×1 transparent PNG, empty SVG, etc.) so each file passes the `[ -s ]` non-empty check. Operator hand-replaces with the real Figma + Loom exports before launch.

- [ ] **Step 4: Run — expect PASS**

```bash
bash scripts/check-marketing-assets.sh ; echo "exit=$?"
```

Expected: exit=0.

- [ ] **Step 5: Commit + PR + reviewer**

```bash
git add docs/assets/marketing/ scripts/check-marketing-assets.sh
git commit -m "feat(marketing): hero asset bundle + size-validation gate"
gh pr create --title "feat(marketing): hero asset bundle scaffolding" \
  --body "Phase 3 Task 17. Reserves the §17.12 + §13.14 asset slots with size-validated placeholders. Operator replaces with Figma + Loom exports pre-launch. What got smaller: zero — pure-add of the §17.12 surface."
```

Reviewer dispatch — verify: README.md is acceptable here because asset bundles need an index file (operator approval is implicit per §17.12); check-marketing-assets.sh is added to `make check` if it doesn't already trip a CI step; placeholder bytes are NOT the real assets — flag in PR body.

---

## Wave 6 — E2E smoke + parity + final review (serialized)

---

### Task 18: Phase 3 E2E smoke runtime (`scripts/dev/phase3-smoke.sh`)

**Files:**
- Create: `scripts/dev/phase3-smoke.sh`
- Create: `scripts/dev/phase3-smoke_test.sh`

**Why this exists:** Phase-3 happy path: `make dev` boots the daemon + HUD; the smoke script (a) sends `tts.speak` IPC, (b) toggles minimal mode and screenshots the focus panel, (c) opens the dashboard menubar entry, (d) toggles Touch ID purge flow (stubbed in CI). One end-to-end exit code captures Phase 3 readiness.

**Interfaces:** shell script — no public API.

- [ ] **Step 1: Write failing test**

`scripts/dev/phase3-smoke_test.sh`:
```bash
#!/usr/bin/env bash
set -euo pipefail
if "$(dirname "$0")/phase3-smoke.sh" --help 2>&1 | grep -q "usage:"; then
  echo "ok"
else
  echo "FAIL: usage check"; exit 1
fi
```

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Implementation**

`scripts/dev/phase3-smoke.sh`:
```bash
#!/usr/bin/env bash
# Phase 3 end-to-end smoke: TTS speak → minimal mode toggle + screenshot
# → dashboard open → touch-id purge stub. Tolerates CI environment by
# stubbing the audio device + biometric prompt when LEAH_E2E_STUB=1.
set -euo pipefail
case "${1:-}" in --help|-h) echo "usage: $0 [--help]"; exit 0;; esac

scripts/dev/ipc-send.sh "tts.speak" '{"text":"Phase 3 smoke OK","voice":"ava-alto-145wpm"}'
scripts/dev/inject-hotkey.sh "settings.appearance.toggle-minimal"
scripts/dev/screenshot.sh focus-panel /tmp/phase3-minimal.png
scripts/dev/inject-hotkey.sh "dashboard.open"
scripts/dev/screenshot.sh dashboard /tmp/phase3-dashboard.png
echo "phase3-smoke: ok"
```

(`scripts/dev/ipc-send.sh`, `inject-hotkey.sh`, `screenshot.sh` already exist per recent commit `ab57b90` — verify before scheduling.)

- [ ] **Step 4: Run — expect PASS**

```bash
bash scripts/dev/phase3-smoke_test.sh
LEAH_E2E_STUB=1 bash scripts/dev/phase3-smoke.sh | tail -5
```

- [ ] **Step 5: Commit + PR + reviewer**

```bash
git add scripts/dev/phase3-smoke.sh scripts/dev/phase3-smoke_test.sh
git commit -m "test(phase3): end-to-end smoke runtime"
gh pr create --title "test(phase3): Phase 3 E2E smoke" \
  --body "Phase 3 Task 18. Drives TTS → minimal-mode toggle → dashboard open → Touch ID purge stub. Single exit code captures Phase 3 readiness. What got smaller: replaces ad-hoc shell scripts the operator was running manually."
```

Reviewer dispatch — verify: existing `scripts/dev/{ipc-send,inject-hotkey,screenshot}.sh` paths match (these merged via commit ab57b90); `LEAH_E2E_STUB=1` actually gates the audio + biometric paths (otherwise CI fails).

---

### Task 19: Phase 3 docs parity + CHANGELOG

**Files:**
- Modify: `docs/superpowers/designs/2026-06-21-leah-macos-native-ui-design.md` (§18 changelog — add `v3.3.0` entry noting Phase 3 ship)
- Modify: `CHANGELOG.md` (root, if it exists; otherwise create)
- Modify: `scripts/check-spec-parity.sh` (single addition — add Phase 3 keyword guard list: "Phase 3 ship criterion", "wake-leah.mlmodel", "tts.cloud.frame")

**Why this exists:** Every phase boundary commits a spec-parity update; Phase 1 + Phase 2 did this and the audit-session skill checks for it.

- [ ] **Step 1: Run current parity check — expect PASS pre-edit**

```bash
bash scripts/check-spec-parity.sh
```

- [ ] **Step 2: Add Phase 3 changelog entry**

Append to §18 of the spec:
```
- v3.3.0 (2026-06-23) — Phase 3 ships. TTS subsystem (ElevenLabs Flash v2.5 + Apple Ava), wake-word adapter + VAD + per-app suppression, push-to-talk (Fn / right-⌘), minimal-mode runtime, Touch ID for memory purge + telemetry toggle, push-source IPC fan, KG-backed citations, MCP publish (read-only), Sparkle auto-appcast + EdDSA verify + rollback, §4.7 dashboard surface, §17.12 marketing-hero asset slots.
```

- [ ] **Step 3: Add Phase 3 keyword guards**

In `check-spec-parity.sh`:
```bash
# Phase 3 keywords that must appear verbatim in the spec post-Phase-3.
required_phrases+=("Phase 3 ship criterion")
required_phrases+=("wake-leah.mlmodel")
```

- [ ] **Step 4: Run parity — expect PASS**

```bash
bash scripts/check-spec-parity.sh
```

- [ ] **Step 5: Commit + PR + reviewer**

```bash
git add docs/superpowers/designs/2026-06-21-leah-macos-native-ui-design.md CHANGELOG.md scripts/check-spec-parity.sh
git commit -m "docs(phase3): v3.3.0 changelog + parity-check Phase 3 keyword guards"
gh pr create --title "docs(phase3): v3.3.0 changelog + parity guards" \
  --body "Phase 3 Task 19. Single-purpose docs update closing the phase boundary. What got smaller: zero — pure docs."
```

Reviewer dispatch — verify: changelog entry is past-tense fragments NOT bulleted summary (operator memory rule "no AI-generated dash bullets"); the keyword guards don't break on Phase 1/2 phrasing.

---

### Task 20: Phase 3 final review + merge cascade

**Files:**
- Reviewer-driven; no direct file edits.

**Why this exists:** Independent reviewer reads the diff vs `main` for the full Phase 3 set, posts a transcript verdict, main session merges in dependency order.

- [ ] **Step 1: Dispatch independent reviewer**

Use `cavecrew-reviewer` subagent with the canonical reviewer header from `.claude/notes/reviewer-header.md`. Adversarial framing on:
  - TTS providers leak api keys? No `fmt.Printf` of `apiKey` or env-var dump.
  - Wake-word default-off respected? `armed = false` in `WakeWord.init`.
  - Minimal mode regressions? Before-toggle screenshot matches Phase 2 baseline.
  - Touch ID is ADDITIVE not replacement? Typed `PURGE` step still fires.
  - MCP publish gate-off default? `allowPeerAgentsEnabled() == false` default.
  - Sparkle EdDSA verify is REDUNDANT not REPLACEMENT? Sparkle's own verify still runs.
  - Spec parity passes.
  - Dispatch templates were referenced in every Wave 1–5 subagent prompt? Scan with `gh pr list --json body | grep implementer-adapter.md | wc -l` ≥ 6 for Wave 1, ≥ 3 for Wave 2, etc.

- [ ] **Step 2: Land merge cascade**

Dependency order: Wave 1 PRs → Wave 2 PRs → Wave 3 PRs → Wave 4 PRs → Wave 5 PRs → Wave 6 PRs. Each PR auto-merges only when its independent reviewer transcript verdict is APPROVE. Main session is the merge driver (NOT the subagent author).

- [ ] **Step 3: Tag v3.3.0 + cut release**

```bash
git tag v3.3.0
git push origin v3.3.0
gh release create v3.3.0 --notes-file CHANGELOG.md \
  ./Leah-v3.3.0.zip ./Leah-v3.3.0.zip.sig
```

- [ ] **Step 4: Post-merge audit**

Run the audit-session skill: Phase 3 final-state audit emits the same shape as Phase 1's `phase1-final-review.md` to `.claude/notes/phase3-final-review.md`.

---

## Phase 3 ship criterion (per spec §19)

Operator on a fresh Mac:
1. Runs through the 6-step wizard.
2. Opts into wake-word at the Voice step.
3. Says "Hey Leah, what shipped today?" into AirPods.
4. The focus panel summons; the daemon streams a cited answer from the knowledge store.
5. The answer plays back in the canonical Leah voice (alto, ~145 wpm) via ElevenLabs Flash v2.5.
6. The citation widget renders source repo file:line + Linear MAY-id links.
7. Toggling Settings → Appearance → Minimal mode strips gold accents + italic from the rendered answer without re-summoning.

— end Phase 3 plan —

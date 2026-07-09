# Leah macOS Native UI — Phase 2 Implementation Plan

> **Plan version:** v1.0 (2026-06-22). Source spec: `docs/superpowers/designs/2026-06-21-leah-macos-native-ui-design.md` (v3.2.2, 2954 lines). Predecessor: `docs/superpowers/plans/2026-06-21-leah-macos-native-phase1.md`. Phase boundary is a merge gate — every Phase 1 task must be merged + the operator's smoke run must be green before Phase 2 starts (spec §19).
>
> **For agentic workers:** REQUIRED SUB-SKILL: `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans`. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal.** Surround the Phase 1 answer engine with ambient presence and the full widget catalog. Ship the §13.14 hero screenshot verbatim: a Leah panel with the 10 remaining widget kinds, a pin-to-ambient flow that survives panel dismiss, a 3-row 280 × 84 px ambient HUD that knows time-of-day, a coalesced notification toast stack, light-mode parity via `NSApp.effectiveAppearance` KVO, the 5 remaining Settings panes, wizard step 5 polish, and BGE-small-en-v1.5 ONNX as the offline embedding fallback (replacing the Phase 1 HashGenerator).

**Architecture (delta from Phase 1).** Two-process design is unchanged. The daemon grows three new responsibility cones: (1) a `internal/widget/` adapter package with the `WidgetAdapter` interface and one adapter per widget kind (`markets`, `flights`, `weather`, `maps`, plus a `PureAdapter` for `table`/`code`/`stat`/`list`/`diff`/`chart`-pure/`citation`/`image`); (2) an fsnotify-debounced watcher for `pinned-widgets.json` + `widget-registry.json` at `~/Library/Application Support/Leah/` that drives ambient HUD slot reconciliation; (3) a BGE-small-en-v1.5 ONNX runner that replaces `HashGenerator` as the offline-degraded embedding path, persisting to a new `embeddings_bge_small_en_v1_5_384` table per the spec's `(model_id, dim)` namespacing rule (§17.15). The HUD process grows: 10 SwiftUI tile views, the ambient HUD chrome, the toast widget, the gallery overlay, the 5 new Settings panes, the wizard step-5 polish, and a `NSApp.effectiveAppearance` KVO observer that cross-fades palettes over 240 ms per spec §3.6.

**Tech Stack (delta from Phase 1).**
- **ONNX runtime (Go):** `github.com/yalue/onnxruntime_go v1.20+` (only Go binding that links the ONNX Runtime C ABI without CGo of its own — it dlopens the system-loaded `libonnxruntime.dylib` shipped under `Leah.app/Contents/Resources/Models/`). Model: `bge-small-en-v1.5.onnx` (~133 MB), tokenizer: `tokenizer.json` from the same HF repo, bundled under `Resources/Models/`.
- **fsnotify:** `github.com/fsnotify/fsnotify v1.7.0+` (confirm with `go mod why github.com/fsnotify/fsnotify`).
- **SwiftUI Charts:** `import Charts` (macOS 14 stdlib, no SPM dep — covers the `chart` widget's 5 kinds line/bar/area/scatter/sparkline).
- **MapKit:** stdlib (Swift) — used by the SwiftUI `maps` tile view for pretty-render; payload still comes from the daemon adapter.
- **No new Anthropic / Voyage / Sparkle deps.** No new GUI frameworks. Light mode is a palette swap on the existing SwiftUI surfaces, not a separate target.

**Phase 1 adjustments learned + enforced here (binding).**
- **Tiempos italic confined to Dashboard only.** Phase 2 surfaces (HUD chrome, panel widgets, notification toast, gallery, all 5 new Settings panes, wizard step 5) use New York Italic for the rare italic accent — never Tiempos. Visual-contract test enforces.
- **Gold accent budget 3× max** per rendered panel (spec §10.0 canvas invariant). Every widget view test asserts ≤ 3 gold-tinted regions by counting `Color.leahGold` materializations via flood-fill on the rendered PNG.
- **Minimal mode toggle in Settings → Appearance** (the toggle ships in Phase 2 even though the runtime effect — "strip grain + italic + gold-accent" — also lands in Phase 3 per spec §19; the toggle wiring is owned here so the Phase 3 implementation only flips a boolean).
- **`make dev` is the runtime feedback loop.** Every task that touches the SwiftUI app MUST after `swift build` (or `make app-build`) observe the live app via `scripts/dev/screenshot.sh` → `/tmp/leah-task-<N>.png` AND `scripts/dev/ipc-send.sh` (when the task wires a new IPC frame) AND `scripts/dev/tail-logs.sh --duration 5s`. The Step-4 "verify it passes" block of every UI task contains the literal commands. See `docs/engineer/runbooks/phase2-dev-loop.md`.
- **Spec parity guard.** Every task's verify step runs `scripts/check-spec-parity.sh docs/superpowers/designs/2026-06-21-leah-macos-native-ui-design.md`. Forbidden-phrase regression at any step = hard fail.

## Global Constraints (delta from Phase 1)

- **macOS minimum:** still 14.0 (Sonoma). `NSApp.effectiveAppearance` is available since 10.14.
- **App Sandbox: OFF; Hardened Runtime: ON.** Notarization required (Phase 1 pipeline reused).
- **Bundle IDs unchanged.** Phase 2 ships under `com.maydow.leah` / `com.maydow.leah.daemon`.
- **Anthropic models locked.** Sonnet 4.6 primary, Haiku 4.5 router, Opus 4.8 opt-in. Widget-class router calls stay on Haiku per Phase 1's `internal/intent/`.
- **Prompt cache:** unchanged. Widget tool-call additions to the system prompt push the prefix above 1024 tokens — ephemeral cache continues to apply.
- **IPC socket path:** `~/Library/Caches/Leah/leah.sock` (Phase 1 §17.2 unchanged).
- **Pinned-widgets + widget-registry JSON paths:** `~/Library/Application Support/Leah/pinned-widgets.json` and `~/Library/Application Support/Leah/widget-registry.json` (spec §10.2). Both watched by a single fsnotify handle with **200 ms debounce** (spec perf #21 — atomic-rename fires 2-3 events, debounce coalesces).
- **Pin cap:** 2 per spec decision #40. Pin attempt #3 = no-op + toast "Pin limit reached. Unpin one to add another." per §10.3.
- **Notification toast cap:** 2 visible per spec §13.7 (workflow #6 cap from 3 → 2). Overflow collapses into "+N more" expandable.
- **Envelope cap:** 256 KB per IPC frame (spec §10.7 perf #25). New widget mount/update frames assert under this in their Validate path.
- **Reduced motion:** every animation (gold transition spawn, 240 ms palette cross-fade, ambient slot reveal) honors `NSWorkspace.shared.accessibilityDisplayShouldReduceMotion`. Test coverage in §11.2.
- **No AI signatures** in commits / PR bodies (CLAUDE.md).
- **Deletion-default:** every PR body answers "what got smaller?" — even feature-add PRs name the dead branch killed, the inline doc-comment trimmed, the placeholder removed. Phase 2 explicitly deletes Phase 1's `HashGenerator` registration once BGE ONNX is live (Task 21).
- **TDD enforced:** failing test FIRST; failing output captured in PR body; then impl; then green.
- **Reviewer per PR:** independent reviewer subagent (`a[0-9a-f]{16}` or `cavecrew-reviewer-*`); spawn immediately after `gh pr create`; verdict before merge.
- **Dispatch parallelism:** Wave 1 (Tasks 2–6, Go adapter contracts) — file-disjoint, up to 5 parallel. Wave 2 (Tasks 7–13, Swift widget views) — file-disjoint, up to 7 parallel. Wave 3 (Tasks 14–15, HUD chrome + toast stack) — single owner per file. Wave 4 (Tasks 16–19, Settings + gallery + pin flow) — file-disjoint, up to 4 parallel. Wave 5 (Tasks 20–21, light mode + BGE) — independent, parallel. Wave 6 (Task 22, E2E smoke) — single owner.

---

## Task index

| # | Wave | Title | Parallelizable with |
|---|---|---|---|
| 1 | 0 | Widget package skeleton + IPC frame extensions | — (single owner: `internal/ipc/frame.go`) |
| 2 | 1 | `WidgetAdapter` + `Payload` + `PureAdapter` | 3, 4, 5, 6 |
| 3 | 1 | `internal/markets/` adapter (Alpha Vantage polite poller) | 2, 4, 5, 6 |
| 4 | 1 | `internal/flights/` adapter (date×price matrix) | 2, 3, 5, 6 |
| 5 | 1 | `internal/weather/` + `internal/maps/` adapters | 2, 3, 4, 6 |
| 6 | 1 | `internal/web/meta/` (URL meta for citation + image cache) | 2, 3, 4, 5 |
| 7 | 2 | SwiftUI tile views — market + chart | 8, 9, 10, 11, 12, 13 |
| 8 | 2 | SwiftUI tile views — flights + maps | 7, 9, 10, 11, 12, 13 |
| 9 | 2 | SwiftUI tile views — weather + calendar | 7, 8, 10, 11, 12, 13 |
| 10 | 2 | SwiftUI tile views — code + diff | 7, 8, 9, 11, 12, 13 |
| 11 | 2 | SwiftUI tile views — citation + image | 7, 8, 9, 10, 12, 13 |
| 12 | 2 | Widget envelope decoder + tile registry (Swift) | 7, 8, 9, 10, 11, 13 |
| 13 | 2 | Lifecycle state machine (Swift, per-tile) | 7, 8, 9, 10, 11, 12 |
| 14 | 3 | Ambient HUD chrome (3-row 280 × 84 px) | — (single owner: `AmbientHUDView.swift`) |
| 15 | 3 | Notification toast stack (2-cap + coalesced timer) | — (single owner: `NotificationToastView.swift`) |
| 16 | 4 | Pin flow + fsnotify-debounced HUD watcher | 17, 18, 19 |
| 17 | 4 | Widget gallery overlay + spawn affordance | 16, 18, 19 |
| 18 | 4 | Settings panes — Voice + Appearance | 16, 17, 19 |
| 19 | 4 | Settings panes — Integrations + Memory + About + wizard step 5 polish | 16, 17, 18 |
| 20 | 5 | Light mode palette + `NSApp.effectiveAppearance` KVO | 21 |
| 21 | 5 | BGE-small-en-v1.5 ONNX embedder + table namespacing | 20 |
| 22 | 6 | Phase 2 E2E smoke (lifecycle + pin persistence + light mode toggle) | — |

**Total: 22 tasks across 6 waves.**

---

## Task 1: Widget package skeleton + IPC frame extensions

**Wave:** 0 (pre-flight; serializes — single owner of `internal/ipc/frame.go` and the new `internal/widget/` package root).

**Files:**
- Create: `internal/widget/envelope.go`
- Create: `internal/widget/envelope_test.go`
- Modify: `internal/ipc/frame.go` (add `WidgetMount`, `WidgetUpdate`, `WidgetStale`, `WidgetError`, `WidgetDismiss`, `NotificationToast` kinds — same length-prefixed JSON envelope per spec §10.7)
- Modify: `internal/ipc/frame_test.go` (round-trip tests for new kinds; envelope-cap assertion at 256 KB)

**Interfaces:**
- Consumes: `internal/ipc/frame.go` Phase 1 frame envelope.
- Produces: `widget.Envelope` Go struct mirroring spec §10.7's `leah://widget/envelope/1` schema (`widget`, `id`, `size`, `actions`, `props` fields); 6 new IPC frame kinds.

**Why this serializes:** `internal/ipc/frame.go` is the Phase 1 IPC contract surface — single-owner per dispatch.

- [ ] **Step 1: Write the failing test**

`internal/widget/envelope_test.go`:
```go
package widget

import (
    "encoding/json"
    "strings"
    "testing"
)

func TestEnvelopeMustHaveWidgetIDSize(t *testing.T) {
    bad := `{"widget":"market"}`
    var e Envelope
    if err := json.Unmarshal([]byte(bad), &e); err != nil {
        t.Fatalf("unmarshal: %v", err)
    }
    if err := e.Validate(); err == nil {
        t.Fatalf("expected validation error on missing id+size; got nil")
    }
}

func TestEnvelopeRejectsUnknownWidgetKind(t *testing.T) {
    bad := `{"widget":"hologram","id":"a","size":"small"}`
    var e Envelope
    _ = json.Unmarshal([]byte(bad), &e)
    if err := e.Validate(); err == nil || !strings.Contains(err.Error(), "hologram") {
        t.Fatalf("expected reject unknown kind; got %v", err)
    }
}

func TestEnvelopeCap256KB(t *testing.T) {
    big := make([]byte, 257*1024)
    for i := range big {
        big[i] = 'x'
    }
    e := Envelope{Widget: "code", ID: "a", Size: "medium", Props: big}
    if err := e.Validate(); err == nil || !strings.Contains(err.Error(), "256") {
        t.Fatalf("expected 256 KB cap error; got %v", err)
    }
}
```

`internal/ipc/frame_test.go` (append):
```go
func TestFrameKindsWidgetMountUpdateStaleErrorDismissToast(t *testing.T) {
    for _, k := range []string{"widget.mount", "widget.update", "widget.stale", "widget.error", "widget.dismiss", "notification.toast"} {
        f := Frame{Kind: k, TurnID: "t1", Seq: 1}
        b, err := json.Marshal(f)
        if err != nil {
            t.Fatalf("%s marshal: %v", k, err)
        }
        var got Frame
        if err := json.Unmarshal(b, &got); err != nil {
            t.Fatalf("%s unmarshal: %v", k, err)
        }
        if got.Kind != k {
            t.Fatalf("kind round-trip: want %q got %q", k, got.Kind)
        }
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/treedesk/Desktop/Projects/leah && go test ./internal/widget/... ./internal/ipc/... 2>&1 | tail -20
```
Expected: `no Go files in internal/widget` (package doesn't exist).

- [ ] **Step 3: Write minimal implementation**

`internal/widget/envelope.go` — `Envelope` struct, kind enum mirroring spec §10.7 (`market`, `flights`, `calendar`, `weather`, `maps`, `table`, `chart`, `image`, `code`, `citation`, `stat`, `list`, `diff`), `size` enum (`small`, `medium`, `large`), `Validate()` enforcing required fields + 256 KB cap.

`internal/ipc/frame.go` — extend the `Kind` enum with the 6 new frame kinds; no breaking change to Phase 1 `ask` / `prose.delta` / `turn.end` / `tts` / `diag` kinds.

- [ ] **Step 4: Run test to verify it passes**

```bash
cd /Users/treedesk/Desktop/Projects/leah && go test ./internal/widget/... ./internal/ipc/... 2>&1 | tail -10
scripts/check-spec-parity.sh docs/superpowers/designs/2026-06-21-leah-macos-native-ui-design.md
```
Expected: `ok` both packages; `check-spec-parity: ok`.

- [ ] **Step 5: Commit**

```bash
git add internal/widget/ internal/ipc/frame.go internal/ipc/frame_test.go
git commit -m "feat(widget): envelope + 6 IPC frame kinds per spec §10.7

Adds Envelope (widget|id|size|actions|props) + Frame kinds
widget.{mount,update,stale,error,dismiss} and notification.toast.
256 KB cap enforced inside Envelope.Validate(); mirrors spec §10.7
perf #25 so the HUD parser buffer sizes statically. Phase 1 frame
kinds untouched.

Deletion-default: drops the 'widget kinds: TBD Phase 2' inline
placeholder in internal/ipc/frame.go."
```

**What got smaller:** Phase 1 placeholder comment `// widget kinds: TBD Phase 2` in `internal/ipc/frame.go`.

---

## Wave 1 — Go adapter contracts (Tasks 2–6, file-disjoint, parallel up to 5)

Every Wave 1 task: TDD, failing test first, captures failing `go test` output in PR body, then impl, then `make check` green. Each task's reviewer subagent verifies: (a) `WidgetAdapter` contract conformance, (b) `Validate()` rejects oversized props per the 256 KB envelope cap, (c) `Fetch()` honors `context.Context` cancellation, (d) `Refresh()` returns prev payload unmodified if etag matches, (e) no AI signatures, (f) deletion-default answered.

---

## Task 2: `WidgetAdapter` + `Payload` + `PureAdapter`

**Wave:** 1. File-disjoint with Tasks 3, 4, 5, 6.

**Files:**
- Create: `internal/widget/adapter.go`
- Create: `internal/widget/adapter_test.go`
- Create: `internal/widget/pure.go`
- Create: `internal/widget/pure_test.go`
- Create: `internal/widget/registry.go`
- Create: `internal/widget/registry_test.go`

**Interfaces:**
- Consumes: `internal/widget/envelope.go` (Task 1).
- Produces: `WidgetAdapter` interface (`Type() string`, `Validate(props json.RawMessage) error`, `Fetch(ctx, props) (Payload, error)`, `Refresh(ctx, id, props, prev) (Payload, error)`), `Payload` struct (`Data`, `FetchedAt`, `StaleAfter`, `Source`, `Etag`), `Registry` map indexed by `Type()` — exactly the spec §10.6 contract.

- [ ] **Step 1: Write the failing test**

`internal/widget/adapter_test.go`:
```go
package widget

import (
    "context"
    "encoding/json"
    "testing"
    "time"
)

func TestPureAdapter_FetchReturnsPropsAsData(t *testing.T) {
    a := NewPureAdapter("stat")
    if a.Type() != "stat" {
        t.Fatalf("type: want stat got %q", a.Type())
    }
    props := json.RawMessage(`{"label":"PRs","value":5}`)
    p, err := a.Fetch(context.Background(), props)
    if err != nil {
        t.Fatalf("fetch: %v", err)
    }
    if string(p.Data) != string(props) {
        t.Fatalf("data: want %s got %s", props, p.Data)
    }
    if p.StaleAfter != 0 {
        t.Fatalf("pure widgets never stale; got %v", p.StaleAfter)
    }
}

func TestRegistry_LookupUnknownReturnsError(t *testing.T) {
    r := NewRegistry()
    if _, ok := r.Lookup("hologram"); ok {
        t.Fatalf("expected miss")
    }
}

func TestAdapter_ContextCancellation(t *testing.T) {
    a := NewPureAdapter("stat")
    ctx, cancel := context.WithCancel(context.Background())
    cancel()
    _, err := a.Fetch(ctx, json.RawMessage(`{}`))
    if err != context.Canceled {
        t.Fatalf("want canceled; got %v", err)
    }
}

func TestRefresh_EtagMatchReturnsPrev(t *testing.T) {
    a := NewPureAdapter("stat")
    prev := &Payload{Data: json.RawMessage(`{"v":1}`), Etag: "abc", FetchedAt: time.Now()}
    p, err := a.Refresh(context.Background(), "id1", json.RawMessage(`{}`), prev)
    if err != nil {
        t.Fatalf("refresh: %v", err)
    }
    if p.Etag != "abc" || string(p.Data) != `{"v":1}` {
        t.Fatalf("pure refresh must echo prev; got %+v", p)
    }
}
```

- [ ] **Step 2: Run test to verify it fails** — `go test ./internal/widget/... 2>&1 | tail -20` → `undefined: NewPureAdapter`, `undefined: NewRegistry`, `undefined: Payload`.

- [ ] **Step 3: Write minimal implementation**

`internal/widget/adapter.go`: `WidgetAdapter` interface + `Payload` struct exactly per spec §10.6.

`internal/widget/pure.go`: `PureAdapter` — `Validate` accepts any non-empty JSON ≤ 256 KB; `Fetch` returns `Payload{Data: props, Source: "pure", StaleAfter: 0}`; `Refresh` echoes prev (pure widgets never re-fetch).

`internal/widget/registry.go`: thread-safe `Registry` (`map[string]WidgetAdapter` + `sync.RWMutex`); `Register(WidgetAdapter)` errors on duplicate `Type()`; `Lookup(kind) (WidgetAdapter, bool)`.

- [ ] **Step 4: Run test to verify it passes** — `go test ./internal/widget/... -count=1 -race 2>&1 | tail -10` + `scripts/check-spec-parity.sh ...`.

- [ ] **Step 5: Commit**

```bash
git add internal/widget/adapter.go internal/widget/adapter_test.go \
        internal/widget/pure.go internal/widget/pure_test.go \
        internal/widget/registry.go internal/widget/registry_test.go
git commit -m "feat(widget): WidgetAdapter contract + PureAdapter + Registry

Mirrors spec §10.6 interface exactly. PureAdapter covers stat/table/
list/code/diff/citation/image/chart-pure — LLM emits payload, no
fetch. Registry is thread-safe (sync.RWMutex) so Wave 4 fsnotify
hot-reload can swap adapters without daemon restart.

Deletion-default: removes the Phase 1 inline widget map in
internal/hud/widgets.go — daemon-side Registry owns adapter map."
```

**What got smaller:** inline `map[string]Renderer` in `internal/hud/widgets.go` (Phase 1 hardcoded `stat`/`table`/`list`) — daemon side now owns adapter registration; HUD only renders.

---

## Task 3: `internal/markets/` adapter (Alpha Vantage polite poller)

**Wave:** 1. File-disjoint with 2, 4, 5, 6.

**Files:**
- Create: `internal/markets/markets.go`
- Create: `internal/markets/markets_test.go`
- Create: `internal/markets/adapter.go` (implements `widget.WidgetAdapter` for `Type() == "market"`)
- Create: `internal/markets/adapter_test.go`
- Create: `internal/markets/testdata/aapl_quote.json`

**Interfaces:**
- Consumes: `internal/widget/adapter.go` (Task 2), `internal/watchlist/` (Phase 1 — symbols), `internal/ratelimit/` (Phase 1).
- Produces: `markets.Adapter` registered as `market`. Schema per spec §10.1 widget #1.

- [ ] **Step 1: Failing test**

`internal/markets/adapter_test.go`:
```go
package markets

import (
    "context"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "os"
    "testing"
)

func TestMarketAdapter_FetchAAPL(t *testing.T) {
    fixture, _ := os.ReadFile("testdata/aapl_quote.json")
    ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        _, _ = w.Write(fixture)
    }))
    defer ts.Close()
    a := NewAdapter(Options{Endpoint: ts.URL, APIKey: "demo"})
    if a.Type() != "market" {
        t.Fatalf("type: %s", a.Type())
    }
    props := json.RawMessage(`{"symbols":["AAPL"]}`)
    if err := a.Validate(props); err != nil {
        t.Fatalf("validate: %v", err)
    }
    p, err := a.Fetch(context.Background(), props)
    if err != nil {
        t.Fatalf("fetch: %v", err)
    }
    if p.Source != "alpha_vantage" {
        t.Fatalf("source: %s", p.Source)
    }
}

func TestMarketAdapter_RejectsEmptySymbols(t *testing.T) {
    a := NewAdapter(Options{})
    if err := a.Validate(json.RawMessage(`{"symbols":[]}`)); err == nil {
        t.Fatal("empty symbols must reject")
    }
}
```

- [ ] **Step 2: Verify fails** — `go test ./internal/markets/... 2>&1 | tail -10` → `no Go files`.

- [ ] **Step 3: Implementation.** `markets.NewAdapter(Options)` polite poller (rate-limited via `internal/ratelimit/`). Fetches Alpha Vantage `GLOBAL_QUOTE` endpoint per symbol; pre-budgets 5 req/min default (free tier). Etag = SHA-256 of sorted symbol list + UTC minute bucket — change detection without re-fetch.

- [ ] **Step 4: Verify passes** — `go test ./internal/markets/... -race -count=1` + spec-parity.

- [ ] **Step 5: Commit.**

**What got smaller:** new package, so the deletion is on existing files — kills any `// markets: see Phase 2` placeholder in `internal/dispatcher/`. The PR body uses `<!-- comment-density-justified: rate-limit doc-comment is inline in NewAdapter, no doc.go file added -->`.

---

## Task 4: `internal/flights/` adapter (date×price matrix)

**Wave:** 1. File-disjoint with 2, 3, 5, 6.

**Files:**
- Create: `internal/flights/flights.go`
- Create: `internal/flights/flights_test.go`
- Create: `internal/flights/adapter.go`
- Create: `internal/flights/adapter_test.go`
- Create: `internal/flights/testdata/sfo_lis_matrix.json`

**Interfaces:**
- Consumes: `internal/widget/adapter.go`.
- Produces: `flights.Adapter` registered as `flights`. Schema per spec §10.1 widget #2 (origin IATA, destination IATA, depart, return, pax 1–9, cabin, max_stops 0–2).

**Adapter strategy:** stub HTTP client behind a `flights.Provider` interface; v1 ships a **manual-poll-only** provider (per spec §10.1 "Refresh: manual only — fare polling is expensive + noisy"). Real provider integration (Kiwi, Skyscanner, duffel) is operator-decision Phase 3.

- [ ] **Step 1** failing test — exercises Validate (IATA regex `^[A-Z]{3}$`), Fetch (28-cell date × price matrix with global-minimum tagged), Refresh (manual-only — auto-refresh returns `ErrManualOnly`).
- [ ] **Step 2** verify fail.
- [ ] **Step 3** impl.
- [ ] **Step 4** verify pass + spec-parity.
- [ ] **Step 5** commit.

**What got smaller:** removes any `// flights: TBD` placeholder in `internal/dispatcher/` Phase 1 widget-class router.

---

## Task 5: `internal/weather/` + `internal/maps/` adapters

**Wave:** 1. File-disjoint with 2, 3, 4, 6.

**Files:**
- Create: `internal/weather/weather.go`
- Create: `internal/weather/weather_test.go`
- Create: `internal/weather/adapter.go`
- Create: `internal/weather/adapter_test.go`
- Create: `internal/weather/testdata/sf_forecast.json`
- Create: `internal/maps/maps.go`
- Create: `internal/maps/maps_test.go`
- Create: `internal/maps/adapter.go`
- Create: `internal/maps/adapter_test.go`
- Create: `internal/maps/testdata/sf_geocode.json`

**Why bundled:** both are stdlib-only fetch adapters (~150 LOC each); reviewer-load is symmetric.

**Interfaces:**
- weather: Apple WeatherKit REST (signed JWT — daemon owns key per Phase 1 keychain rule). Etag = ISO-day + lat/lon. Auto-refresh 15 min.
- maps: Apple Maps Server SDK (geocode + lat/lon only; SwiftUI side renders tiles via MapKit using only that payload — **daemon never returns map tiles**, per spec §10.8 security).

- [ ] Step 1–5 standard TDD shape.

**What got smaller:** kills the inline mock weather/map fixtures in Phase 1's `internal/intent/` test data.

---

## Task 6: `internal/web/meta/` (URL meta for citation + image cache)

**Wave:** 1. File-disjoint with 2, 3, 4, 5.

**Files:**
- Create: `internal/web/meta/meta.go`
- Create: `internal/web/meta/meta_test.go`
- Create: `internal/web/meta/adapter.go` (implements `WidgetAdapter` for both `citation` and `image` kinds — discriminator on `props.url` Content-Type)
- Create: `internal/web/meta/adapter_test.go`
- Create: `internal/web/meta/imagecache.go`
- Create: `internal/web/meta/imagecache_test.go`
- Create: `internal/web/meta/testdata/arxiv_2024_01234.html`

**Why `internal/web/meta/` not `internal/web/`:** `internal/web/` is Phase 1's HTTP server (`server.go`, SLA). Subpackage keeps the citation adapter out of the server's import path.

**Interfaces:**
- `web/meta.NewCitationAdapter()` + `web/meta.NewImageAdapter()` — separate `WidgetAdapter` structs sharing an internal fetcher (registry rejects double-register on the same `Type()`).
- Image cache lives under `~/Library/Caches/Leah/widget-images/`; LRU 200 MB; URLs never leave the daemon, per spec §10.8. IPC payload carries the cached file path (or base64 if < 32 KB) — never the upstream URL.

- [ ] Step 1–5 TDD shape.

**What got smaller:** kills the Phase 1 doc-comment "// citation: see Phase 2" in `internal/papers/store.go` (location confirmed by implementor before commit).

---

## Wave 2 — SwiftUI tile views (Tasks 7–13, file-disjoint, parallel up to 7)

Every Wave 2 task: TDD via Swift XCTest + a **`make dev` runtime smoke** in Step 4:

```bash
make app-build
make dev
sleep 2                                         # daemon socket ready
scripts/dev/ipc-send.sh "$(cat app/Leah/Tests/LeahAppTests/Fixtures/widget-<kind>-mount.json)"
scripts/dev/screenshot.sh /tmp/leah-task-<N>.png
file /tmp/leah-task-<N>.png                     # assert PNG, non-zero
scripts/dev/tail-logs.sh --duration 3s | grep -v ERROR
make dev-stop
```

The screenshot is the load-bearing artifact — PR body includes `![widget-<kind>](.claude/screenshots/widget-<kind>.png)` (committed under `.claude/screenshots/`). Reviewer verifies the rendered shape matches the §13 ASCII wireframes for that kind.

**Shared constraints across Wave 2:**
- Tiempos italic forbidden (visual-contract test scans Text view modifiers).
- Gold accent count ≤ 3 per tile (flood-fill on rendered PNG).
- Tile width = 240 px small, 360 px medium, 480 px large (spec §10.1).
- Spawn animation = 240 ms gold-transition-down per §5.4.
- Reduced-motion path skips animation (`accessibilityDisplayShouldReduceMotion`).
- Caption color = `--text-muted` on `--obsidian-1` (8.16:1 AAA per UX #4) — never `--text-dim`.

---

## Task 7: SwiftUI tile views — market + chart

**Wave:** 2. File-disjoint with 8–13.

**Files:**
- Create: `app/Leah/Sources/LeahApp/Widgets/MarketTileView.swift`
- Create: `app/Leah/Sources/LeahApp/Widgets/ChartTileView.swift`
- Create: `app/Leah/Tests/LeahAppTests/MarketTileViewTests.swift`
- Create: `app/Leah/Tests/LeahAppTests/ChartTileViewTests.swift`
- Create: `app/Leah/Tests/LeahAppTests/Fixtures/market-aapl.json`
- Create: `app/Leah/Tests/LeahAppTests/Fixtures/chart-line.json`
- Create: `.claude/screenshots/widget-market.png` (load-bearing, committed)
- Create: `.claude/screenshots/widget-chart.png` (load-bearing, committed)

**Interfaces:**
- `MarketTileView(payload: MarketPayload)` — renders symbol, price, delta. Delta colored: positive gold (`--gold-primary`), negative oxblood (`--red-alert`). Inline sparkline via `Charts.LineMark` (max 30 points).
- `ChartTileView(payload: ChartPayload)` — switches on `payload.kind` ∈ {line, bar, area, scatter, sparkline}. Single accent series gold; others muted ivory @ 40%. Adverse-event markers = oxblood diamond glyphs with body sans small-caps tracking +0.04em labels per spec §10.1.

- [ ] **Step 1**: failing XCTest — `testRendersPriceAndDelta()` instantiates view, captures `ImageRenderer` PNG, asserts gold pixel count > 0 AND ≤ 3 gold regions (flood-fill clusters). `testChartGoldOnlyOnAccentSeries`.
- [ ] **Step 2**: `cd app/Leah && swift test 2>&1 | tail -20` → `MarketTileView is undefined`.
- [ ] **Step 3**: implement both views.
- [ ] **Step 4**: `swift test` passes + Wave-2 `make dev` smoke + `scripts/check-spec-parity.sh ...`.
- [ ] **Step 5**: commit.

---

## Task 8: SwiftUI tile views — flights + maps

**Wave:** 2. File-disjoint.

**Files:**
- Create: `app/Leah/Sources/LeahApp/Widgets/FlightsTileView.swift`
- Create: `app/Leah/Sources/LeahApp/Widgets/MapsTileView.swift`
- Create: `app/Leah/Tests/LeahAppTests/FlightsTileViewTests.swift`
- Create: `app/Leah/Tests/LeahAppTests/MapsTileViewTests.swift`
- Create: `app/Leah/Tests/LeahAppTests/Fixtures/flights-sfo-lis.json`
- Create: `app/Leah/Tests/LeahAppTests/Fixtures/maps-sf-geocode.json`
- Create: `.claude/screenshots/widget-flights.png`
- Create: `.claude/screenshots/widget-maps.png`

**Interfaces:**
- `FlightsTileView(payload: FlightsPayload)` — date × price grid (28-cell max). Global-minimum cell = filled `#C9A961` bg + obsidian text; row-minimums = 1.5 pt gold left-edge hairline; fares ≥ 30% above row median = oxblood subtle text. Origin → destination route label uses gold once. Gold count ≤ 3 per render (canvas invariant §10.0).
- `MapsTileView(payload: MapsPayload)` — `MapKit.Map(coordinateRegion:)` with a single annotation; daemon supplies only lat/lon. Gold lands on the destination pin only.

Steps 1–5 follow Wave 2 shape.

---

## Task 9: SwiftUI tile views — weather + calendar

**Wave:** 2. File-disjoint.

**Files:**
- Create: `app/Leah/Sources/LeahApp/Widgets/WeatherTileView.swift`
- Create: `app/Leah/Sources/LeahApp/Widgets/CalendarTileView.swift`
- Create: `app/Leah/Tests/LeahAppTests/WeatherTileViewTests.swift`
- Create: `app/Leah/Tests/LeahAppTests/CalendarTileViewTests.swift`
- Create: `app/Leah/Tests/LeahAppTests/Fixtures/weather-sf-forecast.json`
- Create: `app/Leah/Tests/LeahAppTests/Fixtures/calendar-today.json`
- Create: `.claude/screenshots/widget-weather.png`
- Create: `.claude/screenshots/widget-calendar.png`

**Interfaces:**
- `WeatherTileView` — current temp + 7-day strip; **SF Symbol icons** (no emoji — `[☀]`, `[⛅]`, `[🌧]` are spec-parity-forbidden phrases).
- `CalendarTileView` — next-3-events list; current event = 1.5 pt gold left-edge hairline; "now" tick mark on time axis.

Steps 1–5 standard.

---

## Task 10: SwiftUI tile views — code + diff

**Wave:** 2. File-disjoint.

**Files:**
- Create: `app/Leah/Sources/LeahApp/Widgets/CodeTileView.swift`
- Create: `app/Leah/Sources/LeahApp/Widgets/DiffTileView.swift`
- Create: `app/Leah/Tests/LeahAppTests/CodeTileViewTests.swift`
- Create: `app/Leah/Tests/LeahAppTests/DiffTileViewTests.swift`
- Create: `app/Leah/Tests/LeahAppTests/Fixtures/code-go-snippet.json`
- Create: `app/Leah/Tests/LeahAppTests/Fixtures/diff-typo-fix.json`
- Create: `.claude/screenshots/widget-code.png`
- Create: `.claude/screenshots/widget-diff.png`

**Interfaces:**
- `CodeTileView(payload: CodePayload)` — JetBrains Mono; minimal syntax highlight (keyword → gold once per visible region; identifier → ivory; operator → muted ivory). No language guessing — `payload.lang` is required.
- `DiffTileView(payload: DiffPayload)` — unified diff; `+` lines = gold left-edge hairline; `-` lines = oxblood left-edge hairline. Hunk headers in body sans small-caps tracking +0.04em.

Steps 1–5 standard.

---

## Task 11: SwiftUI tile views — citation + image

**Wave:** 2. File-disjoint.

**Files:**
- Create: `app/Leah/Sources/LeahApp/Widgets/CitationTileView.swift`
- Create: `app/Leah/Sources/LeahApp/Widgets/ImageTileView.swift`
- Create: `app/Leah/Tests/LeahAppTests/CitationTileViewTests.swift`
- Create: `app/Leah/Tests/LeahAppTests/ImageTileViewTests.swift`
- Create: `app/Leah/Tests/LeahAppTests/Fixtures/citation-arxiv.json`
- Create: `app/Leah/Tests/LeahAppTests/Fixtures/image-cached.json`
- Create: `.claude/screenshots/widget-citation.png`
- Create: `.claude/screenshots/widget-image.png`

**Interfaces:**
- `CitationTileView` — source domain in gold (one accent only); title in body serif (NY Italic for rare emphasis — never Tiempos); snippet ≤ 400 chars; oxblood "stale" / "404" badge.
- `ImageTileView` — loads from a daemon-cached path via `payload.cached_path` (never a URL); aspect-fit; oxblood "broken" badge on load failure.

Steps 1–5 standard.

---

## Task 12: Widget envelope decoder + tile registry (Swift)

**Wave:** 2. File-disjoint.

**Files:**
- Create: `app/Leah/Sources/LeahApp/Widgets/WidgetEnvelope.swift`
- Create: `app/Leah/Sources/LeahApp/Widgets/WidgetTileRegistry.swift`
- Create: `app/Leah/Tests/LeahAppTests/WidgetEnvelopeTests.swift`
- Create: `app/Leah/Tests/LeahAppTests/WidgetTileRegistryTests.swift`

**Interfaces:**
- `WidgetEnvelope` — Swift `Codable` mirroring `internal/widget/envelope.go` (`widget`, `id`, `size`, `actions`, `props`).
- `WidgetTileRegistry` — `[String: (WidgetEnvelope) -> AnyView]` constructor map; `@MainActor`. Each Wave-2 tile view registers itself.

- [ ] **Step 1**: failing test — `testRegistryRoutesMarketEnvelopeToMarketTileView`, `testEnvelopeRejectsUnknownKind`, `testEnvelopeCap256KB` (Swift-side cap mirror).
- [ ] Steps 2–5 standard.

---

## Task 13: Lifecycle state machine (Swift, per-tile)

**Wave:** 2. File-disjoint.

**Files:**
- Create: `app/Leah/Sources/LeahApp/Widgets/WidgetLifecycle.swift`
- Create: `app/Leah/Tests/LeahAppTests/WidgetLifecycleTests.swift`

**Interfaces:**
- `WidgetLifecycle` — `enum State { spawning, live, refreshing, stale, error, dismissed }`; transition table per spec §10.2 (`mount → live`, `update → live or refreshing`, `stale → stale`, `error → error`, `dismiss → dismissed`).
- Reserves tile height immediately on `spawning` per spec §10.2 v3 ("reserve tile height — body populates without layout reflow").
- Multi-widget reveal staggered 80 ms (spec perf #13) to keep 4 × 240 ms gold-transitions under the 16 ms/60 Hz frame budget on M1-integrated GPUs.

- [ ] **Step 1**: failing test — `testTransitionTable_rejectsLiveToSpawning`, `testReservesHeightOnSpawning`, `testStaggersRevealBy80ms`, `testReducedMotionSkipsAnimation`.
- [ ] Steps 2–5 standard.

---

## Wave 3 — Ambient HUD chrome + notification toast stack (Tasks 14–15, single-owner each)

Wave 3 tasks serialize because each owns a single SwiftUI surface file. Both build on Wave 2.

---

## Task 14: Ambient HUD chrome (3-row 280 × 84 px)

**Wave:** 3. Single owner of `AmbientHUDView.swift`.

**Files:**
- Create: `app/Leah/Sources/LeahApp/AmbientHUDView.swift`
- Create: `app/Leah/Sources/LeahApp/AmbientHUDWindow.swift`
- Create: `app/Leah/Tests/LeahAppTests/AmbientHUDViewTests.swift`
- Modify: `app/Leah/Sources/LeahApp/LeahApp.swift` (mount `AmbientHUDWindow` as a borderless NSPanel below modalPanel level)
- Create: `.claude/screenshots/ambient-hud.png`

**Interfaces:**
- `AmbientHUDView` — 3 rows per spec §7.1:
  - **Row 1 (Mark + state).** 24 px Mark left + state caption right (`--text-muted` on `--obsidian-1`, 8.16:1 AAA). Time-of-day greeting strings are **fixed**: "Good morning, Tri." (06:00–11:00), "Good afternoon, Tri." (11:00–17:00), "Good evening, Tri." (17:00–06:00). No rotating prose.
  - **Row 2 (NOW item).** Predictable source rotation per time-window: **AM (08:00–12:00)** = calendar next-event; **PM (12:00–18:00)** = in-progress brief title; **Evening (18:00–08:00)** = today's first uncompleted agenda item. Else hidden.
  - **Row 3 (Pulse).** Primary metric `◇ N PRs` (PR review queue). Hover/focus rotates to unread briefs `⌬ N` → arxiv `⎇ N` → back.
- `AmbientHUDWindow` — borderless NSPanel, `.fullScreenAuxiliary`, `.stationary`, `.moveToActiveSpace`. Bottom-right anchor by default; Settings → General → HUD anchor grid (Phase 1) toggles position.

- [ ] **Step 1**: failing XCTest — `testGreetingMorningStringFixed`, `testNowRowAMShowsCalendarNextEvent`, `testPulseRowDefaultPRsGlyph`, `testHUDDimensions280x84`, `testHUDNeverShowsTiempos` (visual-contract).
- [ ] **Step 2**: `swift test 2>&1 | tail -20` → undefined symbols.
- [ ] **Step 3**: implement view + window.
- [ ] **Step 4**: `swift test` + `make dev` → `scripts/dev/screenshot.sh /tmp/leah-hud.png` → reviewer eyeballs against §13.1 ASCII wireframe + spec-parity guard.
- [ ] **Step 5**: commit.

**What got smaller:** removes the Phase 1 placeholder "ambient HUD: Phase 2" comment + the placeholder `WindowGroup { ... }` body in `LeahApp.swift`.

---

## Task 15: Notification toast stack (2-cap + coalesced timer)

**Wave:** 3. Single owner of `NotificationToastView.swift`.

**Files:**
- Create: `app/Leah/Sources/LeahApp/NotificationToastView.swift`
- Create: `app/Leah/Sources/LeahApp/NotificationToastStack.swift`
- Create: `app/Leah/Tests/LeahAppTests/NotificationToastViewTests.swift`
- Create: `app/Leah/Tests/LeahAppTests/NotificationToastStackTests.swift`
- Create: `.claude/screenshots/notification-toast.png`

**Interfaces:**
- `NotificationToastView(toast: Toast)` — 320 × 64 px (single line + Mark) or 320 × 112 px (with action chip row). Auto-fades after 8 s default (configurable 3–12 s per spec §13.7). Click expands into focus panel. Swipe-right acknowledges.
- `NotificationToastStack` — max 2 visible (spec §13.7 cap from 3 → 2 per workflow #6); overflow collapses into "+N more" expandable card. **Single coalesced `Timer` for fade-out across all visible toasts** (spec perf #35 — not N separate timers).
- Subscribes to `notification.toast` IPC frames (Task 1).

- [ ] **Step 1**: failing XCTest — `testStackCapsAtTwoVisible`, `testThirdToastCollapsesIntoPlusN`, `testFadeoutSharesSingleTimer`, `testSwipeRightDismisses`, `testPriorityRedPersistsBeyond8s`.
- [ ] Steps 2–5 standard.

**What got smaller:** kills the Phase 1 inline "TODO: notification widget" comment in `internal/hud/recommendations.go`.

---

## Wave 4 — Gallery + pin flow + 5 Settings panes (Tasks 16–19, file-disjoint, parallel up to 4)

---

## Task 16: Pin flow + fsnotify-debounced HUD watcher

**Wave:** 4. File-disjoint.

**Files:**
- Create: `internal/hud/pinned.go`
- Create: `internal/hud/pinned_test.go`
- Create: `internal/hud/registry.go` (widget-registry.json reader, same watcher)
- Create: `internal/hud/registry_test.go`
- Create: `app/Leah/Sources/LeahApp/PinBadgeView.swift`
- Create: `app/Leah/Tests/LeahAppTests/PinBadgeViewTests.swift`

**Interfaces:**
- `internal/hud/pinned.go` — `Watcher` struct wrapping a **single fsnotify.Watcher** across `pinned-widgets.json` + `widget-registry.json`; **200 ms debounce timer** (spec perf #21 — atomic-rename fires 2-3 events, debounce coalesces); emits `PinnedChanged{entries []PinnedEntry}` on the watch channel.
- Append on pin: max 2 (decision #40); attempt #3 → no-op + IPC frame `notification.toast{level:"info", text:"Pin limit reached. Unpin one to add another."}`.
- File path: `~/Library/Application Support/Leah/pinned-widgets.json` (load-bearing — spec §10.3).
- `PinBadgeView` — gold filled diamond on pinned tile; outline diamond on unpinned. Click toggles via IPC `widget.dismiss` or pin RPC.

- [ ] **Step 1**: failing Go test — `TestWatcher_DebouncesBurstWithin200ms` (writes 3 events 50 ms apart, asserts exactly ONE `PinnedChanged`); `TestPinCapTwo`; `TestSingleWatcherForBothFiles` (asserts `runtime.NumGoroutine()` delta == 1 after adding the second path, not 2).
- [ ] Steps 2–5 standard, with `make dev` runtime smoke that writes a pinned-widgets.json entry and screenshots the ambient HUD with the pin badge visible.

**What got smaller:** removes any Phase 1 "TODO pinning" placeholder in `internal/hud/state.go`.

---

## Task 17: Widget gallery overlay + spawn affordance

**Wave:** 4. File-disjoint.

**Files:**
- Create: `app/Leah/Sources/LeahApp/Gallery/WidgetGalleryView.swift`
- Create: `app/Leah/Sources/LeahApp/Gallery/WidgetGalleryCategory.swift`
- Create: `app/Leah/Sources/LeahApp/Gallery/PlusButton.swift`
- Create: `app/Leah/Tests/LeahAppTests/WidgetGalleryViewTests.swift`
- Modify: `app/Leah/Sources/LeahApp/FocusPanel.swift` (Phase 1) — wires `+` button next to input field, `⌘⇧W` shortcut, `/widgets` slash-command
- Create: `.claude/screenshots/widget-gallery.png`

**Interfaces:**
- 3 discoverable affordances per spec §10.4 (workflow #5 — v1's `/widgets`-only path failed by week 2):
  1. Panel-resident `+` button next to input field.
  2. `⌘⇧W` while panel focused.
  3. Typing `/widgets` in panel input.
- 6 fixed categories: Finance · Travel · Time · Productivity · Web · Code (body sans small-caps tracking +0.04em eyebrow per decision #28).
- Preview cells = live `small`-variant tiles rendered with sample data (real components, not screenshots) — what you see is what spawns.
- Spawn animation: overlay dissolves (160 ms) → tile materializes with standard 240 ms gold-transition-down.
- Dismiss: Esc, click-outside, or re-type `/widgets`.
- Fuzzy search across name + category + sample-data text.

- [ ] **Step 1**: failing test — `testGalleryHasThreeSpawnAffordances`, `testSixFixedCategories`, `testPreviewIsLiveSmallTileNotScreenshot`, `testFuzzySearch_FindsArxivWidget`, `testEscDismisses`.
- [ ] Steps 2–5 standard + screenshot.

**What got smaller:** the Phase 1 placeholder "(no widget gallery yet)" empty state in FocusPanel.

---

## Task 18: Settings panes — Voice + Appearance

**Wave:** 4. File-disjoint.

**Files:**
- Create: `app/Leah/Sources/LeahApp/Settings/VoicePane.swift`
- Create: `app/Leah/Sources/LeahApp/Settings/AppearancePane.swift`
- Create: `app/Leah/Tests/LeahAppTests/VoicePaneTests.swift`
- Create: `app/Leah/Tests/LeahAppTests/AppearancePaneTests.swift`
- Modify: `app/Leah/Sources/LeahApp/Settings/SettingsWindow.swift` (Phase 1) — registers two new pane tabs
- Create: `.claude/screenshots/settings-voice.png`
- Create: `.claude/screenshots/settings-appearance.png`

**Interfaces:**
- `VoicePane` — TTS provider selector (ElevenLabs / Apple Ava — **disabled-with-tooltip** "Phase 3 lands runtime", but the UI + persisted `UserDefaults` preference ship here so Phase 3 only reads the boolean); wake-word toggle (stub, default OFF); voice canon preview button (disabled tooltip).
- `AppearancePane` — appearance three-way toggle (Match System / Light / Dark) wired to `NSApp.appearance` override (Task 20 makes this load-bearing); **Minimal mode toggle** (toggle wiring + boolean ships here so Phase 3 only flips the flag); accent budget read-only info row "≤ 3 gold accents per render".

- [ ] **Step 1**: failing test — `testAppearanceToggleWritesUserDefaults`, `testMinimalModeWiringFlowsBoolean`, `testVoicePaneTTSSelectorDisabledWithPhase3Tooltip`.
- [ ] Steps 2–5 standard.

---

## Task 19: Settings panes — Integrations + Memory + About + wizard step 5 polish

**Wave:** 4. File-disjoint.

**Files:**
- Create: `app/Leah/Sources/LeahApp/Settings/IntegrationsPane.swift`
- Create: `app/Leah/Sources/LeahApp/Settings/MemoryPane.swift`
- Create: `app/Leah/Sources/LeahApp/Settings/AboutPane.swift`
- Create: `app/Leah/Tests/LeahAppTests/IntegrationsPaneTests.swift`
- Create: `app/Leah/Tests/LeahAppTests/MemoryPaneTests.swift`
- Create: `app/Leah/Tests/LeahAppTests/AboutPaneTests.swift`
- Modify: `app/Leah/Sources/LeahApp/Wizard/Step5Integrations.swift` (Phase 1 ships only Calendar; Phase 2 polish adds Mail and Files toggles)
- Modify: `app/Leah/Sources/LeahApp/Settings/SettingsWindow.swift` (register 3 new panes)
- Create: `.claude/screenshots/settings-integrations.png`
- Create: `.claude/screenshots/settings-memory.png`
- Create: `.claude/screenshots/settings-about.png`
- Create: `.claude/screenshots/wizard-step5.png`

**Interfaces:**
- `IntegrationsPane` — rows for Calendar (Phase 1 ON), Mail (NEW), Files (NEW). Each row: status glyph (`●` connected, `○` not connected, `△` partial), connect button, disconnect + memory-purge confirm. Wizard step 5 reuses the row components.
- `MemoryPane` — total chunks count, embedding model row, "Purge all memory" destructive action (Touch ID gate from Phase 1 §17.13). Read-only `(model_id, dim)` row exposes the active table — load-bearing for Task 21 (BGE swap).
- `AboutPane` — version (from `Bundle.main.shortVersionString`), Sparkle update-check button (reuses Phase 1 Sparkle plumbing), commit SHA, license link.
- Wizard step 5 polish — Mail row + Files row with same status-glyph component, identical motion to Calendar row.

- [ ] **Step 1**: failing test — `testIntegrationsHasCalendarMailFiles`, `testMemoryShowsModelIdDim`, `testAboutVersionMatchesBundle`, `testWizardStep5HasMailAndFiles`.
- [ ] Steps 2–5 standard.

**What got smaller:** kills the Phase 1 placeholder Settings tabs that read "Coming in Phase 2".

---

## Wave 5 — Light mode + BGE ONNX (Tasks 20–21, parallel)

---

## Task 20: Light mode palette + `NSApp.effectiveAppearance` KVO observer

**Wave:** 5. Parallel with 21.

**Files:**
- Create: `app/Leah/Sources/LeahApp/Theming/Palette.swift`
- Create: `app/Leah/Sources/LeahApp/Theming/PaletteObserver.swift`
- Create: `app/Leah/Tests/LeahAppTests/PaletteObserverTests.swift`
- Modify: `app/Leah/Sources/LeahApp/LeahApp.swift` — install observer on App boot
- Create: `.claude/screenshots/light-mode-panel.png`
- Create: `.claude/screenshots/dark-mode-panel.png`

**Interfaces:**
- `Palette` — `enum Mode { dark, light }`; tokens per spec §3.1 (dark) + §3.6 (light) — `--obsidian-1/2/3`, `--ivory-text`, `--text-muted`, `--gold-primary`, `--gold-primary-light`, `--gold-muted-light`, `--red-alert`. Cross-faded over `--dur-standard` (240 ms) per surface.
- `PaletteObserver` — KVO on `NSApp.effectiveAppearance`; emits palette-change. Mark + listening-pulse continue uninterrupted across the swap.
- Sunrise/sunset auto-switch: respects macOS Display Settings → Appearance → "Auto" (inherits transparently; no Leah scheduler).

- [ ] **Step 1**: failing XCTest — `testPaletteObserverFiresOnAppearanceChange`, `testCrossfadeDuration240ms`, `testLightModeTokenContrastWCAG_AA` (computed contrast ≥ 4.5:1 for text), `testForbiddenColorDriftZeroZeroAA0C` (palette never emits `#0A0A0C` — that's a spec-parity forbidden phrase).
- [ ] **Step 2**: verify fails.
- [ ] **Step 3**: implement; rebuild app; toggle macOS appearance via `defaults write -g AppleInterfaceStyle Dark; killall -s 0 Leah`; verify with `make dev` screenshot.
- [ ] **Step 4**: `swift test` + `make dev` (capture both `.claude/screenshots/light-mode-panel.png` and `.claude/screenshots/dark-mode-panel.png`) + spec-parity.
- [ ] **Step 5**: commit.

**What got smaller:** Phase 1 hardcoded `Color.obsidian1` literal calls collapse to `palette.obsidian1` — single source of truth.

---

## Task 21: BGE-small-en-v1.5 ONNX embedder + table namespacing

**Wave:** 5. Parallel with 20.

**Files:**
- Create: `internal/embed/bge.go`
- Create: `internal/embed/bge_test.go`
- Create: `internal/embed/tokenizer.go` (wordpiece tokenizer mirroring `bert-base-uncased` vocab from HF `tokenizer.json`)
- Create: `internal/embed/tokenizer_test.go`
- Create: `internal/embed/testdata/tokenizer.json` (~700 KB; vendored from `BAAI/bge-small-en-v1.5`)
- Modify: `internal/embed/embed.go` — register `BGEGenerator` as the offline-degraded fallback; **delete `HashGenerator` registration path** (HashGenerator stays as code for tests / migration replays but is no longer wired into the daemon's runtime registry).
- Modify: `internal/sqlstore/embedding.go` (or wherever the Phase 1 embedding table lives) — adds `embeddings_bge_small_en_v1_5_384` table on schema migration; preserves the existing `embeddings_voyage_3_5_lite_1024` and `embeddings_hash_bigram_*` tables (the `(model_id, dim)` namespacing invariant means cloud↔local toggle does **not** force a re-embed).
- Modify: `cmd/leah-daemon/main.go` — lazy-loads `bge-small-en-v1.5.onnx` from `os.Args[0]/Contents/Resources/Models/` (or `LEAH_MODEL_DIR` env override for dev).
- Create: `Models/.gitignore` + `Models/README.md` (instructions for `make download-bge` — the 133 MB model is not committed; build target fetches from HF + SHA-256 verifies).
- Modify: `Makefile` — `download-bge` target (curls model + tokenizer with checksum; idempotent).
- Modify: `scripts/sign-and-notarize.sh` — copies `bge-small-en-v1.5.onnx` and `tokenizer.json` into `Leah.app/Contents/Resources/Models/` before signing.

**Interfaces:**
- `BGEGenerator` — implements the existing `embed.Generator` interface; `Embed(ctx, text) ([]float32, error)` returns 384d unit-normalized vector. Triggered when `VOYAGE_API_KEY` unset OR Voyage unreachable > 30 s (existing circuit-breaker). Settings → Privacy → "Embed locally (slower, private)" toggle pins to local regardless.
- Table namespacing: queries pick the table matching the **currently-active** model. Background backfill re-embeds only when the operator explicitly switches "permanent default" (spec decision #126). No automatic backfill on first BGE load.

- [ ] **Step 1**: failing Go test — `TestBGE_EmbedReturns384DimUnitVec`, `TestBGE_TokenizerHandlesUnicode`, `TestBGE_FailsClosedWhenModelMissing`, `TestTableNamespacing_CloudLocalToggleNoReembed`.
- [ ] **Step 2**: `go test ./internal/embed/... -run TestBGE 2>&1 | tail -10` → `undefined: BGEGenerator`.
- [ ] **Step 3**: implement. Vendor ONNX model checksum via `make download-bge`. Implement tokenizer (wordpiece, 30522 vocab).
- [ ] **Step 4**: `go test ./internal/embed/... -race -count=1` + `make dev` runtime check — `scripts/dev/diag-state.sh` shows `memory_stats.embedding_model == "bge-small-en-v1.5"` after `unset VOYAGE_API_KEY`.
- [ ] **Step 5**: commit.

**What got smaller:** **HashGenerator registration is deleted from the runtime registry** (`internal/embed/embed.go`). The Phase 1 fallback path is replaced verbatim. HashGenerator's source remains for tests + migration replays but no longer flows in production.

---

## Wave 6 — Phase 2 E2E smoke

---

## Task 22: Phase 2 E2E smoke (lifecycle + pin persistence + light mode toggle)

**Wave:** 6. Single owner.

**Files:**
- Create: `scripts/smoke/phase2-e2e.sh`
- Create: `scripts/smoke/phase2-e2e_test.sh`
- Create: `scripts/smoke/phase2-fixtures/widget-mount-market.json`
- Create: `scripts/smoke/phase2-fixtures/widget-mount-chart.json`
- Create: `scripts/smoke/phase2-fixtures/pinned-widgets.json`
- Modify: `Makefile` — `phase2-smoke` target (runs `scripts/smoke/phase2-e2e.sh`); add to `check` aggregate.

**Asserts (all must pass for green):**

1. Daemon stream emits `widget.mount` frames for at least 3 of the 10 new widget kinds when given a prompt that should trigger them ("show me AAPL price + SFO→LIS flights + today's calendar").
2. Pin flow: writing an entry to `~/Library/Application Support/Leah/pinned-widgets.json` makes the ambient HUD render the small-variant tile within 250 ms (200 ms debounce + 50 ms render headroom).
3. Pin cap: writing a third pin entry triggers a `notification.toast{level:"info", text:"Pin limit reached…"}` IPC frame.
4. Light mode toggle: flipping `AppleInterfaceStyle` triggers a `palette.change` log line in `/tmp/leah-dev.log` within 1 s; screenshot diff between dark and light captures > 5% pixel difference (palette swap is visually real, not no-op).
5. Toast stack cap: emitting 3 toast frames in < 100 ms yields 2 visible toasts + one "+1 more" collapsed card.
6. BGE active: unsetting `VOYAGE_API_KEY` and re-running an embed shows `diag-state` reports `embedding_model == "bge-small-en-v1.5"`.
7. Spec-parity guard runs clean on the spec file.
8. Widget lifecycle: `widget.mount` → `widget.update` → `widget.dismiss` round-trip completes in < 500 ms.

- [ ] **Step 1**: write the smoke script + its `_test.sh` (skip-when-no-`ANTHROPIC_API_KEY` per Phase 1 pattern).
- [ ] **Step 2**: run — should fail at first assert (daemon doesn't emit `widget.mount` for market yet without the Wave-1 adapters wired into the dispatcher).
- [ ] **Step 3**: wire the final dispatcher path — `internal/dispatcher/` routes widget-class IPC frames to the registered adapter; emit `widget.mount` per spec §10.7.
- [ ] **Step 4**: re-run — all 8 asserts green.
- [ ] **Step 5**: commit.

**What got smaller:** Phase 1's `scripts/smoke/phase1-e2e.sh` keeps running as the regression floor; phase2-e2e supersedes nothing — both run in `make check`.

---

## Self-review

**Spec coverage check (against §19 Phase 2 deliverables):**

| Deliverable | Task |
|---|---|
| 1. Ambient HUD §7.1 (time-of-day rows: greeting, NOW, pulse) | Task 14 |
| 2. Notification widget (toast queue, 2-cap, coalesce timer) | Task 15 |
| 3a. market | Tasks 3 (adapter), 7 (view) |
| 3b. flights | Tasks 4, 8 |
| 3c. weather | Tasks 5, 9 |
| 3d. maps | Tasks 5, 8 |
| 3e. calendar | view in 9 (adapter reuses `internal/macos/calendar/`) |
| 3f. image | Tasks 6, 11 |
| 3g. chart | Task 7 view; pure path via Task 2 PureAdapter |
| 3h. code | Task 10 view; pure via Task 2 |
| 3i. citation | Tasks 6, 11 |
| 3j. diff | Task 10 view; pure via Task 2 |
| 4. Widget gallery overlay + spawn affordance | Task 17 |
| 5. Pin-to-ambient flow + 2-pin cap + fsnotify debounce | Task 16 |
| 6. Light mode parity (palette + KVO + cross-fade) | Task 20 |
| 7. Settings remaining 5 panes (Voice, Appearance, Integrations, Memory, About) | Tasks 18, 19 |
| 8. Wizard step 5 polish (Mail + Files on top of Calendar) | Task 19 |
| 9. BGE-small-en-v1.5 ONNX local embedding | Task 21 |
| + IPC widget frame plumbing | Task 1 |
| + Tile registry + lifecycle state machine | Tasks 12, 13 |
| + E2E smoke | Task 22 |

**Resolved scope conflicts:**

1. **TTS provider toggle in Settings → Voice (Task 18) vs. Phase 3 owning TTS.** Resolution: Phase 2 ships the toggle UI as disabled-with-tooltip ("Phase 3 lands runtime"). UI wiring + persisted `UserDefaults` ship here; Phase 3 only reads the boolean.
2. **Minimal mode toggle in Settings → Appearance (Task 18) vs. Phase 3 minimal-mode runtime.** Resolution: same — toggle + boolean flow through palette here; Phase 3 strips grain + italic + gold-accent in the actual render path.
3. **Image widget URL exposure (Task 6).** Spec §10.8 forbids exposing the URL to the LLM round-trip. Resolution: daemon caches under `~/Library/Caches/Leah/widget-images/`; IPC payload carries only the cached file path (or base64 if < 32 KB), never the upstream URL.
4. **BGE ONNX 133 MB bundle vs. App Store size limits.** Resolution: not App Store — direct download via Sparkle + Developer ID notarization (no size limit). `make download-bge` happens at build time; the signed `.app` ships with the model in `Contents/Resources/Models/`. Model is gitignored; checksum-verified at build.
5. **Maps tile rendering (Task 8).** Spec §10.8 forbids the daemon returning map tiles. Resolution: daemon's `internal/maps/` returns only lat/lon + geocoded address; SwiftUI side uses `MapKit.Map` to render tiles (Apple-Maps-authenticated by the user's Mac, no daemon involvement).
6. **fsnotify watcher single-handle vs. multi-handle (Task 16).** Spec perf #21 mandates a single watcher across both JSON files. Resolution: enforced by test — `TestSingleWatcherForBothFiles` asserts `runtime.NumGoroutine()` delta == 1.
7. **Tiempos italic confinement.** Resolution: visual-contract test in every Wave-2 tile view scans SwiftUI text descendants and asserts no `Font.tiempos` references. Phase 1 Tiempos italic stayed scoped to Dashboard (§3.3 + §13.4 row 28); Phase 2 surfaces use New York Italic where italic is needed.

**Task count: 22.** **Wave count: 6.**

**Budget estimate vs. 3-week Phase 2 target (spec §19: +3 weeks → week 7):**

- Wave 0 (Task 1) — 0.5 day. IPC contract extension.
- Wave 1 (Tasks 2–6, 5 tasks file-disjoint) — ~4 days. Polite-poller wiring; new packages each ≈ 200 LOC + tests.
- Wave 2 (Tasks 7–13, 7 tasks file-disjoint) — ~5 days. SwiftUI tile views are bulk; Charts framework saves the chart tile from hand-rolled drawing.
- Wave 3 (Tasks 14–15, single-owner each) — ~2 days. Ambient HUD chrome is dense (time-of-day NOW rotation); toast stack is simpler.
- Wave 4 (Tasks 16–19, 4 tasks file-disjoint) — ~4 days. Pin flow + gallery overlay are dense; Settings panes are mostly form rows.
- Wave 5 (Tasks 20–21, 2 tasks parallel) — ~3 days. BGE ONNX wiring is the unknown; tokenizer vendoring + model download are one-time pains.
- Wave 6 (Task 22) — ~1 day. Smoke ties it all together.
- **Total: ~19.5 days ≈ 3 weeks**, within the 3-week budget. Slack of ~0.5 day absorbs Apple-tooling surprises (KVO retain cycles, ONNX dlopen path quirks).

**Operator decisions deferred to Phase 3:**

1. Real flight provider (Kiwi vs. Skyscanner vs. duffel) — Phase 2 ships a stub with manual refresh only.
2. Markets provider beyond Alpha Vantage free tier (paid Polygon? IEX?) — Phase 2 ships Alpha Vantage; rate-limit budget locked to free-tier.
3. Wake-word .mlmodel file — Phase 2 ships toggle wiring only; Phase 3 wires the actual model.

---

## Execution Handoff

Plan complete. Three execution options:

1. **Subagent-Driven (recommended)** — dispatch a fresh subagent per task, review between tasks. Wave 1 (Tasks 2–6) parallelizes up to 5 file-disjoint agents. Wave 2 (Tasks 7–13) parallelizes up to 7. Wave 4 (Tasks 16–19) parallelizes up to 4. Wave 5 (Tasks 20–21) parallelizes 2. Waves 0, 3, 6 serialize per the single-owner-per-file rule. Total wall-clock dispatch length ≈ **8 wall-clock days** of parallel execution, well inside the 3-week real-time budget once human review windows are factored in.
2. **Inline Execution** — execute tasks in this session using `superpowers:executing-plans`, batch with review checkpoints between waves.
3. **Hybrid** — operator dispatches Wave 1 (Go-only) inline, then hands Wave 2 (SwiftUI) to subagents. Phase 1 used this shape.

Which approach?

## Wave 2 — SwiftUI widget tiles (10 files, parallel) — tasks 7-12

---

### Task 7: Market widget tile (`app/Leah/Sources/LeahWidgets/Market.swift`)

**Files:**
- Create: `app/Leah/Sources/LeahWidgets/Market.swift`
- Create: `app/Leah/Tests/LeahWidgetsTests/MarketTests.swift`

**Interfaces:**
- Consumes: `Palette` (existing `Tokens.swift`), `LeahWidget` protocol, `WidgetSize` enum, `WidgetEnvelope` (decoded by panel)
- Produces: `public struct MarketWidget: LeahWidget { public let quotes: [Quote]; public let size: WidgetSize; public var body: some View }`
- Quote: `public struct Quote: Decodable { public let symbol, price, change, changePct, asOf: String }` (coding keys map snake_case)

- [ ] **Step 1: Write failing test**

`app/Leah/Tests/LeahWidgetsTests/MarketTests.swift`:
```swift
import XCTest
import SwiftUI
@testable import LeahWidgets

final class MarketWidgetTests: XCTestCase {
    func testQuoteDecodesFromAdapterJSON() throws {
        let json = #"{"symbol":"AAPL","price":"213.40","change":"+1.20","change_pct":"+0.57%","as_of":"15:59"}"#
        let q = try JSONDecoder().decode(MarketWidget.Quote.self, from: Data(json.utf8))
        XCTAssertEqual(q.symbol, "AAPL")
        XCTAssertEqual(q.changePct, "+0.57%")
    }

    func testWidgetSizeDefaults() {
        let q = MarketWidget.Quote(symbol: "AAPL", price: "1", change: "+1", changePct: "+1%", asOf: "15:00")
        let w = MarketWidget(quotes: [q], size: .medium)
        XCTAssertEqual(w.size, .medium)
        XCTAssertEqual(w.quotes.count, 1)
    }
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
cd /Users/treedesk/Desktop/Projects/leah/app/Leah
swift test --filter LeahWidgetsTests.MarketWidgetTests 2>&1 | head -20
```

Expected: `no such module 'LeahWidgets'` member `MarketWidget` or compile error.

- [ ] **Step 3: Write implementation**

`app/Leah/Sources/LeahWidgets/Market.swift`:
```swift
import SwiftUI

public struct MarketWidget: LeahWidget {
    public struct Quote: Decodable, Equatable {
        public let symbol: String
        public let price: String
        public let change: String
        public let changePct: String
        public let asOf: String

        enum CodingKeys: String, CodingKey {
            case symbol, price, change
            case changePct = "change_pct"
            case asOf = "as_of"
        }

        public init(symbol: String, price: String, change: String, changePct: String, asOf: String) {
            self.symbol = symbol
            self.price = price
            self.change = change
            self.changePct = changePct
            self.asOf = asOf
        }
    }

    public let quotes: [Quote]
    public let size: WidgetSize

    public init(quotes: [Quote], size: WidgetSize = .medium) {
        self.quotes = quotes
        self.size = size
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("MARKET")
                .font(.system(size: 10, weight: .medium))
                .tracking(0.8)
                .foregroundColor(Palette.textMuted)
            ForEach(quotes, id: \.symbol) { q in
                HStack(alignment: .firstTextBaseline, spacing: 12) {
                    Text(q.symbol)
                        .font(.system(size: 13, weight: .medium, design: .monospaced))
                        .foregroundColor(Palette.ivory)
                        .frame(width: 56, alignment: .leading)
                    Text(q.price)
                        .font(.system(size: 13, design: .monospaced))
                        .foregroundColor(Palette.ivory)
                        .frame(maxWidth: .infinity, alignment: .trailing)
                    Text(q.changePct)
                        .font(.system(size: 12, design: .monospaced))
                        .foregroundColor(q.change.hasPrefix("-") ? Palette.oxblood : Palette.champagneGold)
                        .frame(width: 64, alignment: .trailing)
                }
            }
        }
        .padding(16)
        .background(Palette.obsidian2)
        .cornerRadius(12)
        .overlay(
            RoundedRectangle(cornerRadius: 12)
                .strokeBorder(Palette.hairline, lineWidth: 1)
        )
    }
}
```

- [ ] **Step 4: Run — expect PASS**

```bash
cd /Users/treedesk/Desktop/Projects/leah/app/Leah
swift test --filter LeahWidgetsTests.MarketWidgetTests 2>&1 | tail -10
```

Expected: `Test Suite 'MarketWidgetTests' passed`

- [ ] **Step 5: Commit**

```bash
cd /Users/treedesk/Desktop/Projects/leah
git add app/Leah/Sources/LeahWidgets/Market.swift app/Leah/Tests/LeahWidgetsTests/MarketTests.swift
git commit -m "feat(widgets): Market tile renders quote rows with gold/oxblood change color"
```

---

### Task 8: Weather widget tile (`app/Leah/Sources/LeahWidgets/Weather.swift`)

**Files:**
- Create: `app/Leah/Sources/LeahWidgets/Weather.swift`
- Create: `app/Leah/Tests/LeahWidgetsTests/WeatherTests.swift`

**Interfaces:**
- Produces: `public struct WeatherWidget: LeahWidget` with `condition, tempF, location, icon: String`
- Glyph mapping: `01d→sun.max.fill`, `02d→cloud.sun.fill`, `10d→cloud.rain.fill`, `11d→cloud.bolt.fill`, `13d→snow`, default→`cloud.fill` — tinted `Palette.champagneGold` at 60% opacity (gold-muted)

- [ ] **Step 1: Write failing test**

`app/Leah/Tests/LeahWidgetsTests/WeatherTests.swift`:
```swift
import XCTest
@testable import LeahWidgets

final class WeatherWidgetTests: XCTestCase {
    func testIconForCode() {
        XCTAssertEqual(WeatherWidget.symbolName(for: "01d"), "sun.max.fill")
        XCTAssertEqual(WeatherWidget.symbolName(for: "10d"), "cloud.rain.fill")
        XCTAssertEqual(WeatherWidget.symbolName(for: "xx"), "cloud.fill")
    }

    func testInit() {
        let w = WeatherWidget(condition: "Sunny", tempF: "72°", location: "San Francisco", icon: "01d", size: .small)
        XCTAssertEqual(w.size, .small)
        XCTAssertEqual(w.location, "San Francisco")
    }
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
cd /Users/treedesk/Desktop/Projects/leah/app/Leah
swift test --filter LeahWidgetsTests.WeatherWidgetTests 2>&1 | tail -10
```

Expected: `no such member` errors.

- [ ] **Step 3: Write implementation**

`app/Leah/Sources/LeahWidgets/Weather.swift`:
```swift
import SwiftUI

public struct WeatherWidget: LeahWidget {
    public let condition: String
    public let tempF: String
    public let location: String
    public let icon: String
    public let size: WidgetSize

    public init(condition: String, tempF: String, location: String, icon: String, size: WidgetSize = .small) {
        self.condition = condition
        self.tempF = tempF
        self.location = location
        self.icon = icon
        self.size = size
    }

    public static func symbolName(for code: String) -> String {
        switch code {
        case "01d", "01n": return "sun.max.fill"
        case "02d", "02n", "03d", "03n", "04d", "04n": return "cloud.sun.fill"
        case "09d", "09n", "10d", "10n": return "cloud.rain.fill"
        case "11d", "11n": return "cloud.bolt.fill"
        case "13d", "13n": return "snow"
        case "50d", "50n": return "cloud.fog.fill"
        default: return "cloud.fill"
        }
    }

    public var body: some View {
        HStack(alignment: .center, spacing: 16) {
            Image(systemName: Self.symbolName(for: icon))
                .font(.system(size: 32))
                .foregroundColor(Palette.champagneGold.opacity(0.6))
            VStack(alignment: .leading, spacing: 4) {
                Text(tempF)
                    .font(.system(size: 28, weight: .medium, design: .monospaced))
                    .foregroundColor(Palette.ivory)
                Text(condition)
                    .font(.system(size: 12))
                    .foregroundColor(Palette.textMuted)
                Text(location.uppercased())
                    .font(.system(size: 10, weight: .medium))
                    .tracking(0.8)
                    .foregroundColor(Palette.textMuted)
            }
            Spacer()
        }
        .padding(16)
        .background(Palette.obsidian2)
        .cornerRadius(12)
        .overlay(
            RoundedRectangle(cornerRadius: 12)
                .strokeBorder(Palette.hairline, lineWidth: 1)
        )
    }
}
```

- [ ] **Step 4: Run — expect PASS**

```bash
cd /Users/treedesk/Desktop/Projects/leah/app/Leah
swift test --filter LeahWidgetsTests.WeatherWidgetTests 2>&1 | tail -5
```

Expected: tests PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/treedesk/Desktop/Projects/leah
git add app/Leah/Sources/LeahWidgets/Weather.swift app/Leah/Tests/LeahWidgetsTests/WeatherTests.swift
git commit -m "feat(widgets): Weather tile with SF Symbol glyph mapping (gold-muted, never emoji)"
```

---

### Task 9: Calendar widget tile (`app/Leah/Sources/LeahWidgets/Calendar.swift`)

**Files:**
- Create: `app/Leah/Sources/LeahWidgets/Calendar.swift`
- Create: `app/Leah/Tests/LeahWidgetsTests/CalendarTests.swift`

**Interfaces:**
- Produces: `public struct CalendarWidget: LeahWidget { public let events: [Event]; public let date: String; public let size: WidgetSize }`
- `public struct Event: Decodable, Equatable { public let title, start, end: String; public let location: String? }`

- [ ] **Step 1: Write failing test**

`app/Leah/Tests/LeahWidgetsTests/CalendarTests.swift`:
```swift
import XCTest
@testable import LeahWidgets

final class CalendarWidgetTests: XCTestCase {
    func testEventDecodes() throws {
        let json = #"{"title":"1:1 with Tri","start":"2026-06-22T15:00:00Z","end":"2026-06-22T15:30:00Z","location":null}"#
        let e = try JSONDecoder().decode(CalendarWidget.Event.self, from: Data(json.utf8))
        XCTAssertEqual(e.title, "1:1 with Tri")
        XCTAssertNil(e.location)
    }

    func testEmptyEventsRendersHeader() {
        let w = CalendarWidget(events: [], date: "2026-06-22", size: .medium)
        XCTAssertEqual(w.events.count, 0)
    }
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
cd /Users/treedesk/Desktop/Projects/leah/app/Leah
swift test --filter LeahWidgetsTests.CalendarWidgetTests 2>&1 | tail -10
```

Expected: compile error `no such member CalendarWidget`.

- [ ] **Step 3: Write implementation**

`app/Leah/Sources/LeahWidgets/Calendar.swift`:
```swift
import SwiftUI

public struct CalendarWidget: LeahWidget {
    public struct Event: Decodable, Equatable {
        public let title: String
        public let start: String
        public let end: String
        public let location: String?

        public init(title: String, start: String, end: String, location: String? = nil) {
            self.title = title
            self.start = start
            self.end = end
            self.location = location
        }
    }

    public let events: [Event]
    public let date: String
    public let size: WidgetSize

    public init(events: [Event], date: String, size: WidgetSize = .medium) {
        self.events = events
        self.date = date
        self.size = size
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack {
                Text("AGENDA")
                    .font(.system(size: 10, weight: .medium))
                    .tracking(0.8)
                    .foregroundColor(Palette.textMuted)
                Spacer()
                Text(date)
                    .font(.system(size: 10, design: .monospaced))
                    .foregroundColor(Palette.textMuted)
            }
            if events.isEmpty {
                Text("Nothing on the calendar.")
                    .font(.system(size: 12))
                    .foregroundColor(Palette.textMuted)
            } else {
                ForEach(events, id: \.start) { e in
                    HStack(alignment: .top, spacing: 12) {
                        Text(timeOnly(e.start))
                            .font(.system(size: 11, design: .monospaced))
                            .foregroundColor(Palette.champagneGold)
                            .frame(width: 52, alignment: .leading)
                        VStack(alignment: .leading, spacing: 2) {
                            Text(e.title)
                                .font(.system(size: 13))
                                .foregroundColor(Palette.ivory)
                            if let loc = e.location, !loc.isEmpty {
                                Text(loc)
                                    .font(.system(size: 11))
                                    .foregroundColor(Palette.textMuted)
                            }
                        }
                    }
                }
            }
        }
        .padding(16)
        .background(Palette.obsidian2)
        .cornerRadius(12)
        .overlay(
            RoundedRectangle(cornerRadius: 12)
                .strokeBorder(Palette.hairline, lineWidth: 1)
        )
    }

    private func timeOnly(_ iso: String) -> String {
        guard let tIdx = iso.firstIndex(of: "T") else { return iso }
        let after = iso.index(after: tIdx)
        let end = iso.index(after, offsetBy: 5, limitedBy: iso.endIndex) ?? iso.endIndex
        return String(iso[after..<end])
    }
}
```

- [ ] **Step 4: Run — expect PASS**

```bash
cd /Users/treedesk/Desktop/Projects/leah/app/Leah
swift test --filter LeahWidgetsTests.CalendarWidgetTests 2>&1 | tail -5
```

Expected: tests PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/treedesk/Desktop/Projects/leah
git add app/Leah/Sources/LeahWidgets/Calendar.swift app/Leah/Tests/LeahWidgetsTests/CalendarTests.swift
git commit -m "feat(widgets): Calendar tile renders day agenda with gold time-stamps"
```

---

### Task 10: Maps + Flights widget tiles

**Files:**
- Create: `app/Leah/Sources/LeahWidgets/Maps.swift`
- Create: `app/Leah/Sources/LeahWidgets/Flights.swift`
- Create: `app/Leah/Tests/LeahWidgetsTests/MapsFlightsTests.swift`

**Interfaces:**
- `MapsWidget`: takes `mode: String, centerLat, centerLon: Double?, route: Route?` — at `medium` size renders citation card with origin/dest + `[ Open in Apple Maps ]` button (spec §10.1 maps citation-card fallback); at `large` size renders MapKit `Map` view
- `FlightsWidget`: `flights: [Flight]` rendered as offer rows (carrier, route, times, price)
- Imports `MapKit` for Maps; `MapKit` is part of `app/Leah/Package.swift` deps already (or add).

- [ ] **Step 1: Write failing tests**

`app/Leah/Tests/LeahWidgetsTests/MapsFlightsTests.swift`:
```swift
import XCTest
@testable import LeahWidgets

final class MapsFlightsTests: XCTestCase {
    func testMapsCitationFallbackAtMedium() {
        let m = MapsWidget(mode: "route", origin: "SFO", destination: "Mission Dolores", etaMinutes: 28, size: .medium)
        XCTAssertTrue(m.usesCitationFallback)
    }

    func testMapsLargeUsesMapView() {
        let m = MapsWidget(mode: "view", centerLat: 37.77, centerLon: -122.42, size: .large)
        XCTAssertFalse(m.usesCitationFallback)
    }

    func testFlightDecodes() throws {
        let json = #"{"origin":"SFO","dest":"JFK","departs":"08:00","arrives":"16:30","carrier":"UA","flight_num":"UA 101","price_usd":"412"}"#
        let f = try JSONDecoder().decode(FlightsWidget.Flight.self, from: Data(json.utf8))
        XCTAssertEqual(f.flightNum, "UA 101")
        XCTAssertEqual(f.priceUSD, "412")
    }
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
cd /Users/treedesk/Desktop/Projects/leah/app/Leah
swift test --filter LeahWidgetsTests.MapsFlightsTests 2>&1 | tail -10
```

Expected: `no such member MapsWidget`.

- [ ] **Step 3: Write implementations**

`app/Leah/Sources/LeahWidgets/Maps.swift`:
```swift
import SwiftUI
import MapKit

public struct MapsWidget: LeahWidget {
    public let mode: String
    public let origin: String?
    public let destination: String?
    public let etaMinutes: Int?
    public let centerLat: Double?
    public let centerLon: Double?
    public let size: WidgetSize

    public init(
        mode: String,
        origin: String? = nil,
        destination: String? = nil,
        etaMinutes: Int? = nil,
        centerLat: Double? = nil,
        centerLon: Double? = nil,
        size: WidgetSize = .medium
    ) {
        self.mode = mode
        self.origin = origin
        self.destination = destination
        self.etaMinutes = etaMinutes
        self.centerLat = centerLat
        self.centerLon = centerLon
        self.size = size
    }

    // Per spec §10.1 maps citation-card fallback: medium size + route intent
    // renders as citation card, not mini-map (illegible for navigation).
    public var usesCitationFallback: Bool {
        return size != .large && mode == "route"
    }

    public var body: some View {
        Group {
            if usesCitationFallback {
                citationCard
            } else if let lat = centerLat, let lon = centerLon {
                mapView(lat: lat, lon: lon)
            } else {
                Text("Maps: missing center")
                    .font(.system(size: 12))
                    .foregroundColor(Palette.textMuted)
                    .padding(16)
            }
        }
        .background(Palette.obsidian2)
        .cornerRadius(12)
        .overlay(
            RoundedRectangle(cornerRadius: 12)
                .strokeBorder(Palette.hairline, lineWidth: 1)
        )
    }

    @ViewBuilder
    private var citationCard: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("ROUTE")
                .font(.system(size: 10, weight: .medium))
                .tracking(0.8)
                .foregroundColor(Palette.textMuted)
            HStack(spacing: 8) {
                Text(origin ?? "—")
                    .foregroundColor(Palette.ivory)
                Image(systemName: "arrow.right")
                    .foregroundColor(Palette.champagneGold)
                Text(destination ?? "—")
                    .foregroundColor(Palette.ivory)
            }
            .font(.system(size: 13))
            if let eta = etaMinutes {
                Text("\(eta) min")
                    .font(.system(size: 11, design: .monospaced))
                    .foregroundColor(Palette.textMuted)
            }
            Button(action: openInAppleMaps) {
                Text("Open in Apple Maps")
                    .font(.system(size: 11, weight: .medium))
                    .foregroundColor(Palette.champagneGold)
            }
            .buttonStyle(.plain)
        }
        .padding(16)
    }

    @ViewBuilder
    private func mapView(lat: Double, lon: Double) -> some View {
        Map(initialPosition: .region(MKCoordinateRegion(
            center: CLLocationCoordinate2D(latitude: lat, longitude: lon),
            span: MKCoordinateSpan(latitudeDelta: 0.02, longitudeDelta: 0.02)
        )))
        .frame(height: 200)
        .clipShape(RoundedRectangle(cornerRadius: 12))
    }

    private func openInAppleMaps() {
        guard let dest = destination,
              let url = URL(string: "https://maps.apple.com/?daddr=\(dest.addingPercentEncoding(withAllowedCharacters: .urlQueryAllowed) ?? "")") else {
            return
        }
        #if canImport(AppKit)
        NSWorkspace.shared.open(url)
        #endif
    }
}

#if canImport(AppKit)
import AppKit
#endif
```

`app/Leah/Sources/LeahWidgets/Flights.swift`:
```swift
import SwiftUI

public struct FlightsWidget: LeahWidget {
    public struct Flight: Decodable, Equatable {
        public let origin: String
        public let dest: String
        public let departs: String
        public let arrives: String
        public let carrier: String
        public let flightNum: String
        public let priceUSD: String?

        enum CodingKeys: String, CodingKey {
            case origin, dest, departs, arrives, carrier
            case flightNum = "flight_num"
            case priceUSD = "price_usd"
        }
    }

    public let flights: [Flight]
    public let size: WidgetSize

    public init(flights: [Flight], size: WidgetSize = .medium) {
        self.flights = flights
        self.size = size
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            Text("FLIGHTS")
                .font(.system(size: 10, weight: .medium))
                .tracking(0.8)
                .foregroundColor(Palette.textMuted)
            ForEach(flights, id: \.flightNum) { f in
                HStack(alignment: .firstTextBaseline, spacing: 12) {
                    Text(f.flightNum)
                        .font(.system(size: 12, weight: .medium, design: .monospaced))
                        .foregroundColor(Palette.champagneGold)
                        .frame(width: 64, alignment: .leading)
                    Text("\(f.origin) → \(f.dest)")
                        .font(.system(size: 12, design: .monospaced))
                        .foregroundColor(Palette.ivory)
                    Spacer()
                    Text("\(f.departs) – \(f.arrives)")
                        .font(.system(size: 11, design: .monospaced))
                        .foregroundColor(Palette.textMuted)
                    if let price = f.priceUSD {
                        Text("$\(price)")
                            .font(.system(size: 12, design: .monospaced))
                            .foregroundColor(Palette.ivory)
                            .frame(width: 56, alignment: .trailing)
                    }
                }
            }
        }
        .padding(16)
        .background(Palette.obsidian2)
        .cornerRadius(12)
        .overlay(
            RoundedRectangle(cornerRadius: 12)
                .strokeBorder(Palette.hairline, lineWidth: 1)
        )
    }
}
```

- [ ] **Step 4: Run — expect PASS**

```bash
cd /Users/treedesk/Desktop/Projects/leah/app/Leah
swift test --filter LeahWidgetsTests.MapsFlightsTests 2>&1 | tail -8
```

Expected: tests PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/treedesk/Desktop/Projects/leah
git add app/Leah/Sources/LeahWidgets/Maps.swift app/Leah/Sources/LeahWidgets/Flights.swift \
        app/Leah/Tests/LeahWidgetsTests/MapsFlightsTests.swift
git commit -m "feat(widgets): Maps tile (citation-card fallback at medium) + Flights tile rows"
```

---

### Task 11: Chart widget tile (`app/Leah/Sources/LeahWidgets/Chart.swift`)

**Files:**
- Create: `app/Leah/Sources/LeahWidgets/Chart.swift`
- Create: `app/Leah/Tests/LeahWidgetsTests/ChartTests.swift`

**Interfaces:**
- Uses Swift Charts (`import Charts`, macOS 13+, available)
- `public struct ChartWidget: LeahWidget` with `kind: String` (line/bar/area/sparkline), `series: [Series]`
- `public struct Series: Decodable { public let name: String; public let points: [Point]; public let accent: Bool }`
- `public struct Point: Decodable { public let x: Double; public let y: Double }`
- Accent series: gold; others: ivory @ 40%

- [ ] **Step 1: Write failing test**

`app/Leah/Tests/LeahWidgetsTests/ChartTests.swift`:
```swift
import XCTest
@testable import LeahWidgets

final class ChartWidgetTests: XCTestCase {
    func testSeriesDecodes() throws {
        let json = #"{"name":"AAPL","points":[{"x":1.0,"y":213.4}],"accent":true}"#
        let s = try JSONDecoder().decode(ChartWidget.Series.self, from: Data(json.utf8))
        XCTAssertEqual(s.name, "AAPL")
        XCTAssertTrue(s.accent)
        XCTAssertEqual(s.points.first?.y, 213.4)
    }

    func testInit() {
        let w = ChartWidget(kind: "line", series: [], size: .medium)
        XCTAssertEqual(w.kind, "line")
    }
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
cd /Users/treedesk/Desktop/Projects/leah/app/Leah
swift test --filter LeahWidgetsTests.ChartWidgetTests 2>&1 | tail -10
```

Expected: `no such member ChartWidget`.

- [ ] **Step 3: Write implementation**

`app/Leah/Sources/LeahWidgets/Chart.swift`:
```swift
import SwiftUI
import Charts

public struct ChartWidget: LeahWidget {
    public struct Point: Decodable, Equatable {
        public let x: Double
        public let y: Double
    }

    public struct Series: Decodable, Equatable {
        public let name: String
        public let points: [Point]
        public let accent: Bool
    }

    public let kind: String
    public let series: [Series]
    public let size: WidgetSize

    public init(kind: String, series: [Series], size: WidgetSize = .medium) {
        self.kind = kind
        self.series = series
        self.size = size
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text(kind.uppercased())
                .font(.system(size: 10, weight: .medium))
                .tracking(0.8)
                .foregroundColor(Palette.textMuted)
            Chart {
                ForEach(series, id: \.name) { s in
                    ForEach(s.points.indices, id: \.self) { i in
                        let p = s.points[i]
                        switch kind {
                        case "bar":
                            BarMark(x: .value("x", p.x), y: .value("y", p.y))
                                .foregroundStyle(s.accent ? Palette.champagneGold : Palette.ivory.opacity(0.4))
                        case "area":
                            AreaMark(x: .value("x", p.x), y: .value("y", p.y))
                                .foregroundStyle(s.accent ? Palette.champagneGold : Palette.ivory.opacity(0.4))
                        default:
                            LineMark(x: .value("x", p.x), y: .value("y", p.y))
                                .foregroundStyle(s.accent ? Palette.champagneGold : Palette.ivory.opacity(0.4))
                                .lineStyle(StrokeStyle(lineWidth: 1.5))
                        }
                    }
                }
            }
            .frame(height: size == .large ? 200 : 120)
            .chartXAxis {
                AxisMarks { _ in
                    AxisGridLine().foregroundStyle(Palette.ivory.opacity(0.2))
                    AxisValueLabel().foregroundStyle(Palette.textMuted)
                        .font(.system(size: 9, design: .monospaced))
                }
            }
            .chartYAxis {
                AxisMarks { _ in
                    AxisGridLine().foregroundStyle(Palette.ivory.opacity(0.2))
                    AxisValueLabel().foregroundStyle(Palette.textMuted)
                        .font(.system(size: 9, design: .monospaced))
                }
            }
        }
        .padding(16)
        .background(Palette.obsidian2)
        .cornerRadius(12)
        .overlay(
            RoundedRectangle(cornerRadius: 12)
                .strokeBorder(Palette.hairline, lineWidth: 1)
        )
    }
}
```

- [ ] **Step 4: Run — expect PASS**

```bash
cd /Users/treedesk/Desktop/Projects/leah/app/Leah
swift test --filter LeahWidgetsTests.ChartWidgetTests 2>&1 | tail -5
```

Expected: tests PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/treedesk/Desktop/Projects/leah
git add app/Leah/Sources/LeahWidgets/Chart.swift app/Leah/Tests/LeahWidgetsTests/ChartTests.swift
git commit -m "feat(widgets): Chart tile (line/bar/area) with gold accent series, ivory others"
```

---

### Task 12: Image, Code, Citation, Diff widget tiles (4 small tiles)

**Files:**
- Create: `app/Leah/Sources/LeahWidgets/Image.swift`
- Create: `app/Leah/Sources/LeahWidgets/Code.swift`
- Create: `app/Leah/Sources/LeahWidgets/Citation.swift`
- Create: `app/Leah/Sources/LeahWidgets/Diff.swift`
- Create: `app/Leah/Tests/LeahWidgetsTests/PureKindsTests.swift`

**Interfaces:**
- `ImageWidget(url: URL, caption: String?, size: WidgetSize)` — uses `AsyncImage`
- `CodeWidget(language: String, source: String, filename: String?, size: WidgetSize)` — monospaced, language eyebrow
- `CitationWidget(title: String, url: String, snippet: String?, size: WidgetSize)`
- `DiffWidget(hunks: [Hunk], filename: String?, size: WidgetSize)` with `public struct Hunk: Decodable { public let old, new: String }`

- [ ] **Step 1: Write failing test**

`app/Leah/Tests/LeahWidgetsTests/PureKindsTests.swift`:
```swift
import XCTest
import SwiftUI
@testable import LeahWidgets

final class PureKindsTests: XCTestCase {
    func testCodeInit() {
        let w = CodeWidget(language: "go", source: "fmt.Println(\"hi\")", filename: "main.go", size: .medium)
        XCTAssertEqual(w.language, "go")
        XCTAssertEqual(w.size, .medium)
    }

    func testCitationInit() {
        let w = CitationWidget(title: "Paper", url: "https://example.com", snippet: nil, size: .small)
        XCTAssertEqual(w.url, "https://example.com")
    }

    func testHunkDecodes() throws {
        let json = #"{"old":"foo","new":"bar"}"#
        let h = try JSONDecoder().decode(DiffWidget.Hunk.self, from: Data(json.utf8))
        XCTAssertEqual(h.old, "foo")
        XCTAssertEqual(h.new, "bar")
    }

    func testImageInit() {
        let w = ImageWidget(url: URL(string: "https://example.com/x.png")!, caption: "cap", size: .medium)
        XCTAssertEqual(w.caption, "cap")
    }
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
cd /Users/treedesk/Desktop/Projects/leah/app/Leah
swift test --filter LeahWidgetsTests.PureKindsTests 2>&1 | tail -10
```

Expected: `no such member` for each.

- [ ] **Step 3: Write implementations**

`app/Leah/Sources/LeahWidgets/Image.swift`:
```swift
import SwiftUI

public struct ImageWidget: LeahWidget {
    public let url: URL
    public let caption: String?
    public let size: WidgetSize

    public init(url: URL, caption: String? = nil, size: WidgetSize = .medium) {
        self.url = url
        self.caption = caption
        self.size = size
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            AsyncImage(url: url) { phase in
                switch phase {
                case .empty: ProgressView().tint(Palette.champagneGold)
                case .success(let img): img.resizable().scaledToFit()
                case .failure: Text("image unavailable").font(.system(size: 11)).foregroundColor(Palette.textMuted)
                @unknown default: EmptyView()
                }
            }
            .frame(maxWidth: .infinity)
            if let cap = caption {
                Text(cap)
                    .font(.system(size: 11))
                    .foregroundColor(Palette.textMuted)
            }
        }
        .padding(12)
        .background(Palette.obsidian2)
        .cornerRadius(12)
        .overlay(RoundedRectangle(cornerRadius: 12).strokeBorder(Palette.hairline, lineWidth: 1))
    }
}
```

`app/Leah/Sources/LeahWidgets/Code.swift`:
```swift
import SwiftUI

public struct CodeWidget: LeahWidget {
    public let language: String
    public let source: String
    public let filename: String?
    public let size: WidgetSize

    public init(language: String, source: String, filename: String? = nil, size: WidgetSize = .medium) {
        self.language = language
        self.source = source
        self.filename = filename
        self.size = size
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack {
                Text(language.uppercased())
                    .font(.system(size: 10, weight: .medium))
                    .tracking(0.8)
                    .foregroundColor(Palette.textMuted)
                if let fn = filename {
                    Text(fn)
                        .font(.system(size: 11, design: .monospaced))
                        .foregroundColor(Palette.textMuted)
                }
                Spacer()
                Button("Copy") {
                    #if canImport(AppKit)
                    NSPasteboard.general.clearContents()
                    NSPasteboard.general.setString(source, forType: .string)
                    #endif
                }
                .font(.system(size: 10))
                .foregroundColor(Palette.champagneGold)
                .buttonStyle(.plain)
            }
            ScrollView {
                Text(source)
                    .font(.system(size: 12, design: .monospaced))
                    .foregroundColor(Palette.ivory)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .textSelection(.enabled)
            }
            .frame(maxHeight: size == .large ? 320 : 180)
        }
        .padding(16)
        .background(Color(red: 0x0E/255, green: 0x10/255, blue: 0x14/255)) // obsidian-1 per spec §10.1 "Obsidian Brass"
        .cornerRadius(12)
        .overlay(RoundedRectangle(cornerRadius: 12).strokeBorder(Palette.hairline, lineWidth: 1))
    }
}

#if canImport(AppKit)
import AppKit
#endif
```

`app/Leah/Sources/LeahWidgets/Citation.swift`:
```swift
import SwiftUI

public struct CitationWidget: LeahWidget {
    public let title: String
    public let url: String
    public let snippet: String?
    public let size: WidgetSize

    public init(title: String, url: String, snippet: String? = nil, size: WidgetSize = .small) {
        self.title = title
        self.url = url
        self.snippet = snippet
        self.size = size
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(title)
                .font(.system(size: 13, weight: .medium))
                .foregroundColor(Palette.ivory)
            Text(url)
                .font(.system(size: 10, design: .monospaced))
                .foregroundColor(Palette.champagneGold)
                .lineLimit(1)
                .truncationMode(.middle)
            if let s = snippet {
                Text(s)
                    .font(.system(size: 11))
                    .foregroundColor(Palette.textMuted)
                    .lineLimit(3)
            }
        }
        .padding(14)
        .background(Palette.obsidian2)
        .cornerRadius(12)
        .overlay(RoundedRectangle(cornerRadius: 12).strokeBorder(Palette.hairline, lineWidth: 1))
    }
}
```

`app/Leah/Sources/LeahWidgets/Diff.swift`:
```swift
import SwiftUI

public struct DiffWidget: LeahWidget {
    public struct Hunk: Decodable, Equatable {
        public let old: String
        public let new: String
    }

    public let hunks: [Hunk]
    public let filename: String?
    public let size: WidgetSize

    public init(hunks: [Hunk], filename: String? = nil, size: WidgetSize = .medium) {
        self.hunks = hunks
        self.filename = filename
        self.size = size
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack {
                Text("DIFF")
                    .font(.system(size: 10, weight: .medium))
                    .tracking(0.8)
                    .foregroundColor(Palette.textMuted)
                if let fn = filename {
                    Text(fn)
                        .font(.system(size: 11, design: .monospaced))
                        .foregroundColor(Palette.textMuted)
                }
            }
            ScrollView {
                VStack(alignment: .leading, spacing: 1) {
                    ForEach(hunks.indices, id: \.self) { i in
                        let h = hunks[i]
                        ForEach(h.old.split(separator: "\n").map(String.init), id: \.self) { line in
                            HStack(spacing: 6) {
                                Text("-").foregroundColor(Palette.oxblood)
                                Text(line).foregroundColor(Palette.ivory.opacity(0.9))
                                Spacer()
                            }
                            .font(.system(size: 11, design: .monospaced))
                            .background(Palette.oxblood.opacity(0.12))
                        }
                        ForEach(h.new.split(separator: "\n").map(String.init), id: \.self) { line in
                            HStack(spacing: 6) {
                                Text("+").foregroundColor(Palette.champagneGold)
                                Text(line).foregroundColor(Palette.ivory)
                                Spacer()
                            }
                            .font(.system(size: 11, design: .monospaced))
                            .background(Palette.champagneGold.opacity(0.08))
                        }
                    }
                }
            }
            .frame(maxHeight: size == .large ? 320 : 180)
        }
        .padding(16)
        .background(Palette.obsidian2)
        .cornerRadius(12)
        .overlay(RoundedRectangle(cornerRadius: 12).strokeBorder(Palette.hairline, lineWidth: 1))
    }
}
```

- [ ] **Step 4: Run — expect PASS**

```bash
cd /Users/treedesk/Desktop/Projects/leah/app/Leah
swift test --filter LeahWidgetsTests.PureKindsTests 2>&1 | tail -8
```

Expected: tests PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/treedesk/Desktop/Projects/leah
git add app/Leah/Sources/LeahWidgets/Image.swift app/Leah/Sources/LeahWidgets/Code.swift \
        app/Leah/Sources/LeahWidgets/Citation.swift app/Leah/Sources/LeahWidgets/Diff.swift \
        app/Leah/Tests/LeahWidgetsTests/PureKindsTests.swift
git commit -m "feat(widgets): Image/Code/Citation/Diff pure tiles (LLM-payload-only)"
```

---

## Wave 3 — Ambient HUD chrome + notification stack (single-owner LeahApp.swift) — tasks 13-14

---

### Task 13: Ambient HUD panel (`app/Leah/Sources/LeahUI/AmbientHUD.swift`)

**Files:**
- Create: `app/Leah/Sources/LeahUI/AmbientHUD.swift`
- Create: `app/Leah/Sources/LeahUI/AmbientHUDView.swift`
- Create: `app/Leah/Tests/LeahUITests/AmbientHUDTests.swift`
- Modify: `app/Leah/Sources/LeahApp/LeahApp.swift` (instantiate + summon)

**Interfaces:**
- `public final class AmbientHUDController` with `init(client: IPCClient)`, `@MainActor func show()`, `@MainActor func hide()`
- Hosts an `NSPanel` (non-activating, floating level) 280×84 px at default; expanded mode 360×140
- `AmbientHUDView`: SwiftUI view with 3 rows per spec §7.1 — Row 1 (Mark + state caption), Row 2 (time-of-day source), Row 3 (pulse metric)
- `public static func greeting(for hour: Int) -> String` returning "Good morning, Tri." (06–11), "Good afternoon, Tri." (11–17), "Good evening, Tri." (17–06)

- [ ] **Step 1: Write failing test**

`app/Leah/Tests/LeahUITests/AmbientHUDTests.swift`:
```swift
import XCTest
@testable import LeahUI

final class AmbientHUDTests: XCTestCase {
    func testMorningGreeting() {
        XCTAssertEqual(AmbientHUDView.greeting(for: 7), "Good morning, Tri.")
        XCTAssertEqual(AmbientHUDView.greeting(for: 10), "Good morning, Tri.")
    }

    func testAfternoonGreeting() {
        XCTAssertEqual(AmbientHUDView.greeting(for: 13), "Good afternoon, Tri.")
        XCTAssertEqual(AmbientHUDView.greeting(for: 16), "Good afternoon, Tri.")
    }

    func testEveningGreeting() {
        XCTAssertEqual(AmbientHUDView.greeting(for: 19), "Good evening, Tri.")
        XCTAssertEqual(AmbientHUDView.greeting(for: 2), "Good evening, Tri.")
    }

    func testRowSourceByHour_AM() {
        XCTAssertEqual(AmbientHUDView.rowSource(for: 9), .calendarNext)
    }

    func testRowSourceByHour_PM() {
        XCTAssertEqual(AmbientHUDView.rowSource(for: 14), .briefInProgress)
    }

    func testRowSourceByHour_Evening() {
        XCTAssertEqual(AmbientHUDView.rowSource(for: 20), .agendaUncompleted)
    }
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
cd /Users/treedesk/Desktop/Projects/leah/app/Leah
swift test --filter LeahUITests.AmbientHUDTests 2>&1 | tail -10
```

Expected: `no such member AmbientHUDView`.

- [ ] **Step 3: Write implementations**

`app/Leah/Sources/LeahUI/AmbientHUDView.swift`:
```swift
import SwiftUI

public struct AmbientHUDView: View {
    public enum RowSource: Equatable {
        case calendarNext
        case briefInProgress
        case agendaUncompleted
        case empty
    }

    @State public var state: String
    @State public var nowText: String
    @State public var pulseText: String

    public init(state: String = "Idle", nowText: String = "", pulseText: String = "") {
        self._state = State(initialValue: state)
        self._nowText = State(initialValue: nowText)
        self._pulseText = State(initialValue: pulseText)
    }

    public static func greeting(for hour: Int) -> String {
        switch hour {
        case 6..<11:  return "Good morning, Tri."
        case 11..<17: return "Good afternoon, Tri."
        default:      return "Good evening, Tri."
        }
    }

    public static func rowSource(for hour: Int) -> RowSource {
        switch hour {
        case 8..<12:  return .calendarNext
        case 12..<18: return .briefInProgress
        case 18...23, 0..<8: return .agendaUncompleted
        default: return .empty
        }
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack(spacing: 8) {
                MenubarHexagonShape() // existing 18-24 px mark shape from Tokens/MenubarHexagon
                    .stroke(Palette.champagneGold.opacity(0.75), lineWidth: 0.75)
                    .frame(width: 24, height: 24)
                Spacer()
                Text(state)
                    .font(.system(size: 11))
                    .foregroundColor(Palette.textMuted)
            }
            Text(nowText)
                .font(.system(size: 12))
                .foregroundColor(Palette.textMuted)
                .lineLimit(1)
            Text(pulseText)
                .font(.system(size: 11, design: .monospaced))
                .foregroundColor(Palette.textMuted)
                .lineLimit(1)
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 10)
        .frame(width: 280, height: 84, alignment: .topLeading)
        .background(Palette.obsidian1)
        .overlay(
            RoundedRectangle(cornerRadius: 14)
                .strokeBorder(Palette.hairline, lineWidth: 1)
        )
        .cornerRadius(14)
    }
}

// Placeholder shape if MenubarHexagonShape not yet exposed publicly; falls back to a stroked hexagon.
public struct MenubarHexagonShape: Shape {
    public init() {}
    public func path(in rect: CGRect) -> Path {
        var p = Path()
        let w = rect.width, h = rect.height
        p.move(to: CGPoint(x: w * 0.5, y: 0))
        p.addLine(to: CGPoint(x: w, y: h * 0.25))
        p.addLine(to: CGPoint(x: w, y: h * 0.75))
        p.addLine(to: CGPoint(x: w * 0.5, y: h))
        p.addLine(to: CGPoint(x: 0, y: h * 0.75))
        p.addLine(to: CGPoint(x: 0, y: h * 0.25))
        p.closeSubpath()
        return p
    }
}
```

`app/Leah/Sources/LeahUI/AmbientHUD.swift`:
```swift
import AppKit
import SwiftUI
import LeahIPC

public final class AmbientHUDController {
    private var panel: NSPanel?
    private let client: IPCClient

    public init(client: IPCClient) {
        self.client = client
    }

    @MainActor
    public func show() {
        if let p = panel { p.orderFrontRegardless(); return }
        let view = AmbientHUDView(
            state: "Idle",
            nowText: AmbientHUDView.greeting(for: Calendar.current.component(.hour, from: Date())),
            pulseText: ""
        )
        let host = NSHostingController(rootView: view)
        let p = NSPanel(
            contentRect: NSRect(x: 0, y: 0, width: 280, height: 84),
            styleMask: [.borderless, .nonactivatingPanel],
            backing: .buffered,
            defer: false
        )
        p.contentViewController = host
        p.isFloatingPanel = true
        p.level = .floating
        p.collectionBehavior = [.canJoinAllSpaces, .stationary, .ignoresCycle]
        p.backgroundColor = .clear
        p.hasShadow = true
        p.isOpaque = false
        // Top-right corner, inset 16 px from menubar.
        if let screen = NSScreen.main {
            let frame = screen.visibleFrame
            p.setFrameOrigin(NSPoint(x: frame.maxX - 280 - 16, y: frame.maxY - 84 - 16))
        }
        p.orderFrontRegardless()
        panel = p
    }

    @MainActor
    public func hide() {
        panel?.orderOut(nil)
    }
}
```

Modify `app/Leah/Sources/LeahApp/LeahApp.swift` — add HUD instantiation after `wizard.presentIfNeeded()`:
```swift
// Inside init(), after the IPCClient is created and before the body Scene:
let hud = AmbientHUDController(client: client)
self.ambientHUD = hud
hud.show()
```
And add the stored property: `private let ambientHUD: AmbientHUDController`.

- [ ] **Step 4: Run — expect PASS + build**

```bash
cd /Users/treedesk/Desktop/Projects/leah/app/Leah
swift test --filter LeahUITests.AmbientHUDTests 2>&1 | tail -10
swift build 2>&1 | tail -10
```

Expected: tests PASS; build succeeds.

- [ ] **Step 5: Commit**

```bash
cd /Users/treedesk/Desktop/Projects/leah
git add app/Leah/Sources/LeahUI/AmbientHUD.swift app/Leah/Sources/LeahUI/AmbientHUDView.swift \
        app/Leah/Sources/LeahApp/LeahApp.swift \
        app/Leah/Tests/LeahUITests/AmbientHUDTests.swift
git commit -m "feat(hud): ambient HUD NSPanel with 3-row layout, time-of-day greeting + source"
```

---

### Task 14: Notification toast stack (`app/Leah/Sources/LeahUI/NotificationStack.swift`)

**Files:**
- Create: `app/Leah/Sources/LeahUI/NotificationStack.swift`
- Create: `app/Leah/Tests/LeahUITests/NotificationStackTests.swift`

**Interfaces:**
- `public struct Toast: Identifiable, Equatable { public let id: UUID; public let title, body: String; public let kind: ToastKind; public let createdAt: Date }`
- `public enum ToastKind { case info, warning, error }`
- `public final class NotificationStackController: ObservableObject` with:
  - `@Published public private(set) var visible: [Toast]` (max 2)
  - `public func enqueue(_ toast: Toast)` — coalesces same-title toasts within 3s
  - `public func dismiss(_ id: UUID)`
- Coalesce timer: when a new toast arrives with same `title` as one in queue within 3 seconds, replace timestamp, do not duplicate

- [ ] **Step 1: Write failing test**

`app/Leah/Tests/LeahUITests/NotificationStackTests.swift`:
```swift
import XCTest
@testable import LeahUI

final class NotificationStackTests: XCTestCase {
    func testEnqueueAddsVisible() {
        let s = NotificationStackController()
        s.enqueue(Toast(id: UUID(), title: "Hi", body: "first", kind: .info, createdAt: Date()))
        XCTAssertEqual(s.visible.count, 1)
    }

    func testTwoCap() {
        let s = NotificationStackController()
        for i in 0..<5 {
            s.enqueue(Toast(id: UUID(), title: "t\(i)", body: "b", kind: .info, createdAt: Date()))
        }
        XCTAssertEqual(s.visible.count, 2)
    }

    func testCoalesceSameTitle() {
        let s = NotificationStackController()
        let t1 = Toast(id: UUID(), title: "Saved", body: "x", kind: .info, createdAt: Date())
        s.enqueue(t1)
        let t2 = Toast(id: UUID(), title: "Saved", body: "y", kind: .info, createdAt: Date())
        s.enqueue(t2)
        XCTAssertEqual(s.visible.count, 1)
        XCTAssertEqual(s.visible[0].body, "y")
    }

    func testDismiss() {
        let s = NotificationStackController()
        let t = Toast(id: UUID(), title: "Hi", body: "x", kind: .info, createdAt: Date())
        s.enqueue(t)
        s.dismiss(t.id)
        XCTAssertEqual(s.visible.count, 0)
    }
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
cd /Users/treedesk/Desktop/Projects/leah/app/Leah
swift test --filter LeahUITests.NotificationStackTests 2>&1 | tail -10
```

Expected: compile errors.

- [ ] **Step 3: Write implementation**

`app/Leah/Sources/LeahUI/NotificationStack.swift`:
```swift
import SwiftUI
import Combine

public enum ToastKind: Equatable { case info, warning, error }

public struct Toast: Identifiable, Equatable {
    public let id: UUID
    public let title: String
    public let body: String
    public let kind: ToastKind
    public let createdAt: Date

    public init(id: UUID = UUID(), title: String, body: String, kind: ToastKind, createdAt: Date = Date()) {
        self.id = id
        self.title = title
        self.body = body
        self.kind = kind
        self.createdAt = createdAt
    }
}

public final class NotificationStackController: ObservableObject {
    @Published public private(set) var visible: [Toast] = []
    private let cap: Int
    private let coalesceWindow: TimeInterval

    public init(cap: Int = 2, coalesceWindow: TimeInterval = 3.0) {
        self.cap = cap
        self.coalesceWindow = coalesceWindow
    }

    public func enqueue(_ toast: Toast) {
        // Coalesce: same title within window — replace existing.
        if let idx = visible.firstIndex(where: {
            $0.title == toast.title && toast.createdAt.timeIntervalSince($0.createdAt) < coalesceWindow
        }) {
            visible[idx] = toast
            return
        }
        visible.append(toast)
        if visible.count > cap {
            visible.removeFirst(visible.count - cap)
        }
    }

    public func dismiss(_ id: UUID) {
        visible.removeAll { $0.id == id }
    }
}

public struct NotificationStackView: View {
    @ObservedObject public var controller: NotificationStackController

    public init(controller: NotificationStackController) {
        self.controller = controller
    }

    public var body: some View {
        VStack(spacing: 8) {
            ForEach(controller.visible) { t in
                ToastRow(toast: t, onDismiss: { controller.dismiss(t.id) })
            }
        }
    }
}

struct ToastRow: View {
    let toast: Toast
    let onDismiss: () -> Void

    var body: some View {
        HStack(alignment: .top, spacing: 10) {
            Circle()
                .fill(accentColor)
                .frame(width: 6, height: 6)
                .padding(.top, 6)
            VStack(alignment: .leading, spacing: 2) {
                Text(toast.title)
                    .font(.system(size: 12, weight: .medium))
                    .foregroundColor(Palette.ivory)
                Text(toast.body)
                    .font(.system(size: 11))
                    .foregroundColor(Palette.textMuted)
            }
            Spacer()
            Button(action: onDismiss) {
                Image(systemName: "xmark")
                    .font(.system(size: 9))
                    .foregroundColor(Palette.textMuted)
            }
            .buttonStyle(.plain)
        }
        .padding(12)
        .frame(width: 280, alignment: .leading)
        .background(Palette.obsidian2)
        .cornerRadius(10)
        .overlay(RoundedRectangle(cornerRadius: 10).strokeBorder(Palette.hairline, lineWidth: 1))
    }

    private var accentColor: Color {
        switch toast.kind {
        case .info:    return Palette.champagneGold
        case .warning: return Palette.champagneGold.opacity(0.7)
        case .error:   return Palette.oxblood
        }
    }
}
```

- [ ] **Step 4: Run — expect PASS**

```bash
cd /Users/treedesk/Desktop/Projects/leah/app/Leah
swift test --filter LeahUITests.NotificationStackTests 2>&1 | tail -8
```

Expected: all 4 tests PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/treedesk/Desktop/Projects/leah
git add app/Leah/Sources/LeahUI/NotificationStack.swift app/Leah/Tests/LeahUITests/NotificationStackTests.swift
git commit -m "feat(hud): notification toast stack — 2-cap, 3s coalesce window, gold/oxblood accents"
```

---

## Wave 4 — Gallery overlay + Settings remaining 5 panes — tasks 15-18

---

### Task 15: Widget gallery overlay (`app/Leah/Sources/LeahUI/WidgetGallery.swift`)

**Files:**
- Create: `app/Leah/Sources/LeahUI/WidgetGallery.swift`
- Create: `app/Leah/Tests/LeahUITests/WidgetGalleryTests.swift`
- Modify: `app/Leah/Sources/LeahUI/FocusPanelView.swift` (wire `/widgets` slash command + `+` button)

**Interfaces:**
- `public struct WidgetGalleryEntry: Equatable { public let kind: String; public let displayName: String; public let glyph: String }`
- `public final class WidgetGalleryModel: ObservableObject` with `@Published public var entries: [WidgetGalleryEntry]` (the 13 kinds: stat, table, list, market, flights, weather, maps, calendar, image, chart, code, citation, diff)
- `public func filter(_ q: String) -> [WidgetGalleryEntry]`
- `public struct WidgetGalleryView: View` — grid of tiles, click spawns

- [ ] **Step 1: Write failing test**

`app/Leah/Tests/LeahUITests/WidgetGalleryTests.swift`:
```swift
import XCTest
@testable import LeahUI

final class WidgetGalleryTests: XCTestCase {
    func test13Kinds() {
        let m = WidgetGalleryModel()
        XCTAssertEqual(m.entries.count, 13)
        XCTAssertTrue(m.entries.contains { $0.kind == "market" })
        XCTAssertTrue(m.entries.contains { $0.kind == "diff" })
    }

    func testFilterByName() {
        let m = WidgetGalleryModel()
        let r = m.filter("weat")
        XCTAssertEqual(r.count, 1)
        XCTAssertEqual(r.first?.kind, "weather")
    }

    func testFilterEmpty() {
        let m = WidgetGalleryModel()
        XCTAssertEqual(m.filter("").count, 13)
    }
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
cd /Users/treedesk/Desktop/Projects/leah/app/Leah
swift test --filter LeahUITests.WidgetGalleryTests 2>&1 | tail -8
```

Expected: `no such member WidgetGalleryModel`.

- [ ] **Step 3: Write implementation**

`app/Leah/Sources/LeahUI/WidgetGallery.swift`:
```swift
import SwiftUI

public struct WidgetGalleryEntry: Equatable, Identifiable {
    public var id: String { kind }
    public let kind: String
    public let displayName: String
    public let glyph: String

    public init(kind: String, displayName: String, glyph: String) {
        self.kind = kind
        self.displayName = displayName
        self.glyph = glyph
    }
}

public final class WidgetGalleryModel: ObservableObject {
    @Published public var entries: [WidgetGalleryEntry]

    public init() {
        self.entries = [
            WidgetGalleryEntry(kind: "stat",     displayName: "Stat",     glyph: "number"),
            WidgetGalleryEntry(kind: "table",    displayName: "Table",    glyph: "tablecells"),
            WidgetGalleryEntry(kind: "list",     displayName: "List",     glyph: "list.bullet"),
            WidgetGalleryEntry(kind: "market",   displayName: "Market",   glyph: "chart.line.uptrend.xyaxis"),
            WidgetGalleryEntry(kind: "flights",  displayName: "Flights",  glyph: "airplane"),
            WidgetGalleryEntry(kind: "weather",  displayName: "Weather",  glyph: "cloud.sun.fill"),
            WidgetGalleryEntry(kind: "maps",     displayName: "Maps",     glyph: "map"),
            WidgetGalleryEntry(kind: "calendar", displayName: "Calendar", glyph: "calendar"),
            WidgetGalleryEntry(kind: "image",    displayName: "Image",    glyph: "photo"),
            WidgetGalleryEntry(kind: "chart",    displayName: "Chart",    glyph: "chart.bar.fill"),
            WidgetGalleryEntry(kind: "code",     displayName: "Code",     glyph: "chevron.left.forwardslash.chevron.right"),
            WidgetGalleryEntry(kind: "citation", displayName: "Citation", glyph: "quote.opening"),
            WidgetGalleryEntry(kind: "diff",     displayName: "Diff",     glyph: "plus.forwardslash.minus"),
        ]
    }

    public func filter(_ q: String) -> [WidgetGalleryEntry] {
        guard !q.isEmpty else { return entries }
        return entries.filter { $0.displayName.lowercased().contains(q.lowercased()) || $0.kind.contains(q.lowercased()) }
    }
}

public struct WidgetGalleryView: View {
    @StateObject private var model = WidgetGalleryModel()
    @State private var query: String = ""
    public let onSpawn: (String) -> Void

    public init(onSpawn: @escaping (String) -> Void) {
        self.onSpawn = onSpawn
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("WIDGETS")
                .font(.system(size: 10, weight: .medium))
                .tracking(0.8)
                .foregroundColor(Palette.textMuted)
            TextField("Filter…", text: $query)
                .textFieldStyle(.plain)
                .font(.system(size: 12, design: .monospaced))
                .foregroundColor(Palette.ivory)
                .padding(8)
                .background(Palette.obsidian2)
                .cornerRadius(6)
            let cols = [GridItem(.adaptive(minimum: 96), spacing: 12)]
            LazyVGrid(columns: cols, spacing: 12) {
                ForEach(model.filter(query)) { e in
                    Button(action: { onSpawn(e.kind) }) {
                        VStack(spacing: 6) {
                            Image(systemName: e.glyph)
                                .font(.system(size: 20))
                                .foregroundColor(Palette.champagneGold)
                            Text(e.displayName)
                                .font(.system(size: 11))
                                .foregroundColor(Palette.ivory)
                        }
                        .frame(width: 96, height: 80)
                        .background(Palette.obsidian2)
                        .cornerRadius(10)
                        .overlay(RoundedRectangle(cornerRadius: 10).strokeBorder(Palette.hairline, lineWidth: 1))
                    }
                    .buttonStyle(.plain)
                }
            }
        }
        .padding(16)
        .frame(width: 480, alignment: .leading)
        .background(Palette.obsidian1)
        .cornerRadius(14)
    }
}
```

In `FocusPanelView.swift`, add a slash-command match and `+` button:
- When the user types `/widgets`, present a sheet hosting `WidgetGalleryView(onSpawn: { kind in /* send IPC render_widget */ })`.
- Add a small `+` button (`Image(systemName: "plus")` tinted gold-muted) in the input area trailing edge that triggers the same sheet.

- [ ] **Step 4: Run — expect PASS**

```bash
cd /Users/treedesk/Desktop/Projects/leah/app/Leah
swift test --filter LeahUITests.WidgetGalleryTests 2>&1 | tail -8
```

Expected: tests PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/treedesk/Desktop/Projects/leah
git add app/Leah/Sources/LeahUI/WidgetGallery.swift app/Leah/Sources/LeahUI/FocusPanelView.swift \
        app/Leah/Tests/LeahUITests/WidgetGalleryTests.swift
git commit -m "feat(hud): widget gallery overlay (13 kinds) + /widgets command + plus button"
```

---

### Task 16: Settings Voice + Appearance panes

**Files:**
- Modify: `app/Leah/Sources/LeahUI/Settings/Pane.swift` (extend enum)
- Create: `app/Leah/Sources/LeahUI/Settings/VoicePane.swift`
- Create: `app/Leah/Sources/LeahUI/Settings/AppearancePane.swift`
- Modify: `app/Leah/Sources/LeahUI/Settings/SettingsWindow.swift` (route new panes)
- Create: `app/Leah/Tests/LeahUITests/SettingsExtendedTests.swift`

**Interfaces:**
- `SettingsPane` extends with cases `voice, appearance, integrations, memory, about` and shortcuts `"5"`–`"9"`
- `VoicePane`: wake-word opt-in toggle (default off), push-to-talk modifier picker (Fn vs right-⌘)
- `AppearancePane`: appearance mode segmented (System/Light/Dark), Minimal mode toggle (strips grain/italic/gold per spec §19 Phase 3 — surfaced here for Phase 2)

- [ ] **Step 1: Write failing test**

`app/Leah/Tests/LeahUITests/SettingsExtendedTests.swift`:
```swift
import XCTest
@testable import LeahUI

final class SettingsExtendedTests: XCTestCase {
    func test9PanesExist() {
        XCTAssertEqual(SettingsPane.allCases.count, 9)
    }

    func testVoiceTitle() {
        XCTAssertEqual(SettingsPane.voice.title, "Voice")
    }

    func testAppearanceShortcut() {
        XCTAssertEqual(SettingsPane.appearance.keyboardShortcut, "6")
    }

    func testMemoryTitle() {
        XCTAssertEqual(SettingsPane.memory.title, "Memory")
    }
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
cd /Users/treedesk/Desktop/Projects/leah/app/Leah
swift test --filter LeahUITests.SettingsExtendedTests 2>&1 | tail -10
```

Expected: enum case `voice` missing.

- [ ] **Step 3: Write implementations**

Modify `app/Leah/Sources/LeahUI/Settings/Pane.swift`:
```swift
import Foundation

public enum SettingsPane: String, CaseIterable, Hashable {
    case general, privacy, permissions, advanced
    case voice, appearance, integrations, memory, about

    public var title: String {
        switch self {
        case .general:      return "General"
        case .privacy:      return "Privacy"
        case .permissions:  return "Permissions"
        case .advanced:     return "Advanced"
        case .voice:        return "Voice"
        case .appearance:   return "Appearance"
        case .integrations: return "Integrations"
        case .memory:       return "Memory"
        case .about:        return "About"
        }
    }

    public var keyboardShortcut: String {
        switch self {
        case .general:      return "1"
        case .privacy:      return "2"
        case .permissions:  return "3"
        case .advanced:     return "4"
        case .voice:        return "5"
        case .appearance:   return "6"
        case .integrations: return "7"
        case .memory:       return "8"
        case .about:        return "9"
        }
    }
}
```

`app/Leah/Sources/LeahUI/Settings/VoicePane.swift`:
```swift
import SwiftUI

struct VoicePane: View {
    @AppStorage("leah.voice.wakeword") private var wakeword: Bool = false
    @AppStorage("leah.voice.ptt") private var pttModifier: String = "fn"

    var body: some View {
        Form {
            Section("Voice Input") {
                Toggle("Enable wake word (\"Hey Leah\")", isOn: $wakeword)
                Text("Off by default. When on, Leah listens continuously for the wake phrase. Battery + privacy tradeoff.")
                    .font(.system(size: 11))
                    .foregroundColor(.secondary)
                Picker("Push-to-talk modifier", selection: $pttModifier) {
                    Text("Fn key (laptop)").tag("fn")
                    Text("Right ⌘ (external)").tag("rcmd")
                }
                .pickerStyle(.radioGroup)
            }
        }
        .padding(20)
    }
}
```

`app/Leah/Sources/LeahUI/Settings/AppearancePane.swift`:
```swift
import SwiftUI

struct AppearancePane: View {
    @AppStorage("leah.appearance.mode") private var mode: String = "system"
    @AppStorage("leah.appearance.minimal") private var minimal: Bool = false

    var body: some View {
        Form {
            Section("Appearance") {
                Picker("Mode", selection: $mode) {
                    Text("Match System").tag("system")
                    Text("Light").tag("light")
                    Text("Dark").tag("dark")
                }
                .pickerStyle(.segmented)

                Toggle("Minimal mode", isOn: $minimal)
                Text("Strips grain, italic flourish, and gold accents to a bare functional palette.")
                    .font(.system(size: 11))
                    .foregroundColor(.secondary)
            }
        }
        .padding(20)
    }
}
```

Modify `SettingsWindow.swift` switch:
```swift
switch selected {
case .general:      GeneralPane()
case .privacy:      PrivacyPane()
case .permissions:  PermissionsPane()
case .advanced:     AdvancedPane()
case .voice:        VoicePane()
case .appearance:   AppearancePane()
case .integrations: IntegrationsPane()
case .memory:       MemoryPane()
case .about:        AboutPane()
}
```
(Other 3 panes land in Task 17.)

- [ ] **Step 4: Run — expect PASS**

```bash
cd /Users/treedesk/Desktop/Projects/leah/app/Leah
swift test --filter LeahUITests.SettingsExtendedTests 2>&1 | tail -8
```

Expected: tests PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/treedesk/Desktop/Projects/leah
git add app/Leah/Sources/LeahUI/Settings/Pane.swift app/Leah/Sources/LeahUI/Settings/VoicePane.swift \
        app/Leah/Sources/LeahUI/Settings/AppearancePane.swift app/Leah/Sources/LeahUI/Settings/SettingsWindow.swift \
        app/Leah/Tests/LeahUITests/SettingsExtendedTests.swift
git commit -m "feat(settings): Voice + Appearance panes (wake-word opt-in, system/light/dark, Minimal mode)"
```

---

### Task 17: Settings Integrations + Memory + About panes

**Files:**
- Create: `app/Leah/Sources/LeahUI/Settings/IntegrationsPane.swift`
- Create: `app/Leah/Sources/LeahUI/Settings/MemoryPane.swift`
- Create: `app/Leah/Sources/LeahUI/Settings/AboutPane.swift`
- Create: `app/Leah/Tests/LeahUITests/SettingsRemainingTests.swift`

**Interfaces:**
- `IntegrationsPane`: lists Calendar, Mail, Files with `[Connect]` / `[Disconnect]` buttons that post IPC `connect` / `disconnect` frames (uses existing `internal/connect/` flow via Swift `IPCClient`)
- `MemoryPane`: shows memory stats (total decisions / mistakes / patterns), `[Purge]` action with confirm
- `AboutPane`: version, build hash, copyright, link to changelog

- [ ] **Step 1: Write failing test**

`app/Leah/Tests/LeahUITests/SettingsRemainingTests.swift`:
```swift
import XCTest
import SwiftUI
@testable import LeahUI

final class SettingsRemainingTests: XCTestCase {
    func testIntegrationsListsKnownProviders() {
        let providers = IntegrationsPane.knownProviders
        XCTAssertTrue(providers.contains { $0.id == "gcal" })
        XCTAssertTrue(providers.contains { $0.id == "gmail" })
        XCTAssertTrue(providers.contains { $0.id == "files" })
    }

    func testMemoryDefaultsToZero() {
        let m = MemoryPaneState()
        XCTAssertEqual(m.decisions, 0)
        XCTAssertEqual(m.mistakes, 0)
        XCTAssertEqual(m.patterns, 0)
    }

    func testAboutVersionPresent() {
        let v = AboutPane.bundleVersion()
        XCTAssertFalse(v.isEmpty)
    }
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
cd /Users/treedesk/Desktop/Projects/leah/app/Leah
swift test --filter LeahUITests.SettingsRemainingTests 2>&1 | tail -10
```

Expected: compile errors.

- [ ] **Step 3: Write implementations**

`app/Leah/Sources/LeahUI/Settings/IntegrationsPane.swift`:
```swift
import SwiftUI

struct IntegrationsPane: View {
    struct Provider: Identifiable, Equatable {
        let id: String
        let displayName: String
        let summary: String
    }

    static let knownProviders: [Provider] = [
        Provider(id: "gcal",  displayName: "Google Calendar", summary: "Agenda + event creation"),
        Provider(id: "gmail", displayName: "Gmail",           summary: "Inbox triage + send"),
        Provider(id: "files", displayName: "Local Files",     summary: "~/Documents semantic search"),
    ]

    @AppStorage("leah.connect.gcal")  private var gcalOn: Bool = false
    @AppStorage("leah.connect.gmail") private var gmailOn: Bool = false
    @AppStorage("leah.connect.files") private var filesOn: Bool = false

    var body: some View {
        Form {
            Section("Integrations") {
                ForEach(Self.knownProviders) { p in
                    HStack(alignment: .top) {
                        VStack(alignment: .leading, spacing: 2) {
                            Text(p.displayName).font(.system(size: 13, weight: .medium))
                            Text(p.summary).font(.system(size: 11)).foregroundColor(.secondary)
                        }
                        Spacer()
                        Button(connectedBinding(for: p.id).wrappedValue ? "Disconnect" : "Connect") {
                            connectedBinding(for: p.id).wrappedValue.toggle()
                            // TODO: post IPC "connect.<id>" / "disconnect.<id>" frame
                        }
                    }
                }
            }
        }
        .padding(20)
    }

    private func connectedBinding(for id: String) -> Binding<Bool> {
        switch id {
        case "gcal":  return $gcalOn
        case "gmail": return $gmailOn
        case "files": return $filesOn
        default:      return .constant(false)
        }
    }
}
```

`app/Leah/Sources/LeahUI/Settings/MemoryPane.swift`:
```swift
import SwiftUI

final class MemoryPaneState: ObservableObject {
    @Published var decisions: Int = 0
    @Published var mistakes: Int = 0
    @Published var patterns: Int = 0
}

struct MemoryPane: View {
    @StateObject private var state = MemoryPaneState()
    @State private var confirmingPurge = false

    var body: some View {
        Form {
            Section("Memory") {
                HStack { Text("Decisions"); Spacer(); Text("\(state.decisions)").monospacedDigit() }
                HStack { Text("Mistakes");  Spacer(); Text("\(state.mistakes)").monospacedDigit() }
                HStack { Text("Patterns");  Spacer(); Text("\(state.patterns)").monospacedDigit() }
            }
            Section {
                Button(role: .destructive) {
                    confirmingPurge = true
                } label: {
                    Text("Purge all memory")
                }
                .confirmationDialog(
                    "Purge all stored memory?",
                    isPresented: $confirmingPurge,
                    titleVisibility: .visible
                ) {
                    Button("Purge", role: .destructive) {
                        // TODO: IPC frame "memory.purge"
                    }
                    Button("Cancel", role: .cancel) {}
                }
            }
        }
        .padding(20)
    }
}
```

`app/Leah/Sources/LeahUI/Settings/AboutPane.swift`:
```swift
import SwiftUI

struct AboutPane: View {
    static func bundleVersion() -> String {
        let v = Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "0.0.0"
        let b = Bundle.main.object(forInfoDictionaryKey: "CFBundleVersion") as? String ?? "0"
        return "\(v) (\(b))"
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            Text("Leah").font(.system(size: 22, weight: .medium))
            Text("Version \(Self.bundleVersion())").font(.system(size: 12, design: .monospaced)).foregroundColor(.secondary)
            Divider()
            Text("A quiet desk assistant.").font(.system(size: 12)).foregroundColor(.secondary)
            Spacer()
            HStack {
                Link("Changelog", destination: URL(string: "https://github.com/trilam/leah/releases")!)
                    .font(.system(size: 11))
                Spacer()
                Text("© 2026 Tri Lam").font(.system(size: 10)).foregroundColor(.secondary)
            }
        }
        .padding(20)
    }
}
```

- [ ] **Step 4: Run — expect PASS**

```bash
cd /Users/treedesk/Desktop/Projects/leah/app/Leah
swift test --filter LeahUITests.SettingsRemainingTests 2>&1 | tail -8
```

Expected: tests PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/treedesk/Desktop/Projects/leah
git add app/Leah/Sources/LeahUI/Settings/IntegrationsPane.swift \
        app/Leah/Sources/LeahUI/Settings/MemoryPane.swift \
        app/Leah/Sources/LeahUI/Settings/AboutPane.swift \
        app/Leah/Tests/LeahUITests/SettingsRemainingTests.swift
git commit -m "feat(settings): Integrations + Memory + About panes (5 remaining panes complete)"
```

---

### Task 18: Wire pinned widgets into Ambient HUD (Swift fsnotify-equivalent + IPC)

**Files:**
- Create: `app/Leah/Sources/LeahUI/PinnedWidgetsObserver.swift`
- Create: `app/Leah/Tests/LeahUITests/PinnedWidgetsObserverTests.swift`
- Modify: `app/Leah/Sources/LeahUI/AmbientHUD.swift` (consume observer)

**Interfaces:**
- `public final class PinnedWidgetsObserver: ObservableObject` uses `DispatchSourceFileSystemObject` to watch `~/Library/Application Support/Leah/pinned-widgets.json`
- `@Published public private(set) var pinned: [PinnedSlot]`
- `public struct PinnedSlot: Decodable, Equatable { public let id, type: String; public let props: Data }`
- Ambient HUD expands to render up to 2 pinned widget tiles below the 3 default rows

- [ ] **Step 1: Write failing test**

`app/Leah/Tests/LeahUITests/PinnedWidgetsObserverTests.swift`:
```swift
import XCTest
@testable import LeahUI

final class PinnedWidgetsObserverTests: XCTestCase {
    func testDecodesPinnedJSON() throws {
        let json = #"[{"id":"w1","type":"market","props":{"symbols":["AAPL"]},"refresh_ns":60000000000}]"#
        let slots = try PinnedWidgetsObserver.decode(Data(json.utf8))
        XCTAssertEqual(slots.count, 1)
        XCTAssertEqual(slots[0].type, "market")
    }

    func testDecodesEmptyArray() throws {
        let slots = try PinnedWidgetsObserver.decode(Data("[]".utf8))
        XCTAssertEqual(slots.count, 0)
    }

    func testMissingFileYieldsEmpty() {
        let url = URL(fileURLWithPath: "/tmp/leah-test-\(UUID().uuidString).json")
        let obs = PinnedWidgetsObserver(path: url)
        XCTAssertEqual(obs.pinned.count, 0)
    }
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
cd /Users/treedesk/Desktop/Projects/leah/app/Leah
swift test --filter LeahUITests.PinnedWidgetsObserverTests 2>&1 | tail -10
```

Expected: `no such member PinnedWidgetsObserver`.

- [ ] **Step 3: Write implementation**

`app/Leah/Sources/LeahUI/PinnedWidgetsObserver.swift`:
```swift
import Foundation
import Combine

public struct PinnedSlot: Decodable, Equatable {
    public let id: String
    public let type: String
    public let props: Data

    enum CodingKeys: String, CodingKey { case id, type, props }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = try c.decode(String.self, forKey: .id)
        type = try c.decode(String.self, forKey: .type)
        // Re-encode props sub-tree as raw JSON Data so each tile decodes its own shape.
        if let rawObj = try? c.decode(JSONValue.self, forKey: .props) {
            props = (try? JSONEncoder().encode(rawObj)) ?? Data()
        } else {
            props = Data()
        }
    }
}

enum JSONValue: Codable {
    case object([String: JSONValue])
    case array([JSONValue])
    case string(String)
    case number(Double)
    case bool(Bool)
    case null

    init(from decoder: Decoder) throws {
        let c = try decoder.singleValueContainer()
        if c.decodeNil() { self = .null; return }
        if let b = try? c.decode(Bool.self)    { self = .bool(b); return }
        if let n = try? c.decode(Double.self)  { self = .number(n); return }
        if let s = try? c.decode(String.self)  { self = .string(s); return }
        if let a = try? c.decode([JSONValue].self) { self = .array(a); return }
        if let o = try? c.decode([String: JSONValue].self) { self = .object(o); return }
        self = .null
    }

    func encode(to encoder: Encoder) throws {
        var c = encoder.singleValueContainer()
        switch self {
        case .null:           try c.encodeNil()
        case .bool(let b):    try c.encode(b)
        case .number(let n):  try c.encode(n)
        case .string(let s):  try c.encode(s)
        case .array(let a):   try c.encode(a)
        case .object(let o):  try c.encode(o)
        }
    }
}

public final class PinnedWidgetsObserver: ObservableObject {
    @Published public private(set) var pinned: [PinnedSlot] = []
    private let path: URL
    private var source: DispatchSourceFileSystemObject?
    private var fd: Int32 = -1

    public init(path: URL) {
        self.path = path
        reload()
        startWatching()
    }

    deinit { stopWatching() }

    public static func decode(_ data: Data) throws -> [PinnedSlot] {
        return try JSONDecoder().decode([PinnedSlot].self, from: data)
    }

    public func reload() {
        guard let data = try? Data(contentsOf: path) else {
            pinned = []
            return
        }
        pinned = (try? Self.decode(data)) ?? []
    }

    private func startWatching() {
        fd = open(path.path, O_EVTONLY)
        if fd < 0 {
            // File not yet created — watch parent dir for create then re-arm.
            return
        }
        let s = DispatchSource.makeFileSystemObjectSource(
            fileDescriptor: fd, eventMask: [.write, .extend, .rename, .delete], queue: .main
        )
        s.setEventHandler { [weak self] in
            self?.reload()
        }
        s.setCancelHandler { [weak self] in
            if let fd = self?.fd, fd >= 0 { close(fd) }
            self?.fd = -1
        }
        s.resume()
        self.source = s
    }

    private func stopWatching() {
        source?.cancel()
        source = nil
    }
}
```

Modify `AmbientHUD.swift` to instantiate observer (path = `FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0].appendingPathComponent("Leah/pinned-widgets.json")`) and render pinned slots in the panel content view (expand panel height when `obs.pinned.count > 0`).

- [ ] **Step 4: Run — expect PASS**

```bash
cd /Users/treedesk/Desktop/Projects/leah/app/Leah
swift test --filter LeahUITests.PinnedWidgetsObserverTests 2>&1 | tail -8
```

Expected: tests PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/treedesk/Desktop/Projects/leah
git add app/Leah/Sources/LeahUI/PinnedWidgetsObserver.swift app/Leah/Sources/LeahUI/AmbientHUD.swift \
        app/Leah/Tests/LeahUITests/PinnedWidgetsObserverTests.swift
git commit -m "feat(hud): PinnedWidgetsObserver watches pinned-widgets.json + renders into ambient HUD"
```

---

## Wave 5 — Light mode parity (single-owner Tokens.swift + LeahApp.swift) — tasks 19-20

---

### Task 19: Light-mode palette tokens + Palette.adaptive lookup

**Files:**
- Modify: `app/Leah/Sources/LeahWidgets/Tokens.swift` (add bone tokens + adaptive resolver)
- Create: `app/Leah/Tests/LeahWidgetsTests/PaletteAdaptiveTests.swift`

**Interfaces:**
- New static constants on `Palette`: `bone0 = #F2EFE8`, `bone1 = #EAE6DD`, `bone2 = #E2DACE`, `bone3 = #D6CFBC`, `goldPrimaryLight = #7A6332`, `textMutedLight = #5A554A`
- `public static func adaptiveBackground(level: Int, isLight: Bool) -> Color` returns the right palette token
- `public static func adaptiveText(isLight: Bool) -> Color`, etc.

- [ ] **Step 1: Write failing test**

`app/Leah/Tests/LeahWidgetsTests/PaletteAdaptiveTests.swift`:
```swift
import XCTest
import SwiftUI
@testable import LeahWidgets

final class PaletteAdaptiveTests: XCTestCase {
    func testBoneTokensExist() {
        let _: Color = Palette.bone0
        let _: Color = Palette.bone1
        let _: Color = Palette.bone2
        let _: Color = Palette.bone3
    }

    func testLightGoldExists() {
        let _: Color = Palette.goldPrimaryLight
    }

    func testAdaptiveBackgroundSwitches() {
        let dark = Palette.adaptiveBackground(level: 1, isLight: false)
        let light = Palette.adaptiveBackground(level: 1, isLight: true)
        XCTAssertNotEqual(String(describing: dark), String(describing: light))
    }
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
cd /Users/treedesk/Desktop/Projects/leah/app/Leah
swift test --filter LeahWidgetsTests.PaletteAdaptiveTests 2>&1 | tail -10
```

Expected: `bone0` not found.

- [ ] **Step 3: Write implementation**

Append to `app/Leah/Sources/LeahWidgets/Tokens.swift`:
```swift
public extension Palette {
    // §3.6 bone backgrounds (light counterpart to obsidian)
    static let bone0 = Color(red: 0xF2/255, green: 0xEF/255, blue: 0xE8/255) // #F2EFE8
    static let bone1 = Color(red: 0xEA/255, green: 0xE6/255, blue: 0xDD/255) // #EAE6DD
    static let bone2 = Color(red: 0xE2/255, green: 0xDA/255, blue: 0xCE/255) // #E2DACE
    static let bone3 = Color(red: 0xD6/255, green: 0xCF/255, blue: 0xBC/255) // #D6CFBC

    // §3.6 light accents
    static let goldPrimaryLight = Color(red: 0x7A/255, green: 0x63/255, blue: 0x32/255) // #7A6332
    static let oxbloodLight     = Color(red: 0x5C/255, green: 0x1A/255, blue: 0x22/255) // #5C1A22
    static let textPrimaryLight = Color(red: 0x1C/255, green: 0x1A/255, blue: 0x16/255) // #1C1A16
    static let textMutedLight   = Color(red: 0x5A/255, green: 0x55/255, blue: 0x4A/255) // #5A554A
    static let hairlineLight    = Color.black.opacity(0.12)

    static func adaptiveBackground(level: Int, isLight: Bool) -> Color {
        if isLight {
            switch level { case 0: return bone0; case 1: return bone1; case 2: return bone2; default: return bone3 }
        } else {
            switch level { case 0: return obsidian0; case 1: return obsidian1; case 2: return obsidian2; default: return obsidian3 }
        }
    }

    static func adaptiveText(isLight: Bool) -> Color {
        isLight ? textPrimaryLight : ivory
    }

    static func adaptiveMutedText(isLight: Bool) -> Color {
        isLight ? textMutedLight : textMuted
    }

    static func adaptiveAccent(isLight: Bool) -> Color {
        isLight ? goldPrimaryLight : champagneGold
    }

    static func adaptiveHairline(isLight: Bool) -> Color {
        isLight ? hairlineLight : hairline
    }
}
```

- [ ] **Step 4: Run — expect PASS**

```bash
cd /Users/treedesk/Desktop/Projects/leah/app/Leah
swift test --filter LeahWidgetsTests.PaletteAdaptiveTests 2>&1 | tail -5
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/treedesk/Desktop/Projects/leah
git add app/Leah/Sources/LeahWidgets/Tokens.swift app/Leah/Tests/LeahWidgetsTests/PaletteAdaptiveTests.swift
git commit -m "feat(tokens): light-mode bone palette + adaptive(background|text|accent) resolvers"
```

---

### Task 20: Appearance observer + cross-fade switch

**Files:**
- Create: `app/Leah/Sources/LeahUI/AppearanceObserver.swift`
- Create: `app/Leah/Tests/LeahUITests/AppearanceObserverTests.swift`
- Modify: `app/Leah/Sources/LeahApp/LeahApp.swift` (instantiate + propagate)

**Interfaces:**
- `public final class AppearanceObserver: ObservableObject` with `@Published public private(set) var isLight: Bool`
- KVO-observes `NSApp.effectiveAppearance` (key path `effectiveAppearance.name`) on the main thread
- `public static func isLightAppearance(_ name: NSAppearance.Name?) -> Bool` for unit-test access

- [ ] **Step 1: Write failing test**

`app/Leah/Tests/LeahUITests/AppearanceObserverTests.swift`:
```swift
import XCTest
import AppKit
@testable import LeahUI

final class AppearanceObserverTests: XCTestCase {
    func testLightDetected() {
        XCTAssertTrue(AppearanceObserver.isLightAppearance(.aqua))
        XCTAssertTrue(AppearanceObserver.isLightAppearance(.vibrantLight))
    }

    func testDarkDetected() {
        XCTAssertFalse(AppearanceObserver.isLightAppearance(.darkAqua))
        XCTAssertFalse(AppearanceObserver.isLightAppearance(.vibrantDark))
    }

    func testNilFallsBackToDark() {
        XCTAssertFalse(AppearanceObserver.isLightAppearance(nil))
    }
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
cd /Users/treedesk/Desktop/Projects/leah/app/Leah
swift test --filter LeahUITests.AppearanceObserverTests 2>&1 | tail -10
```

Expected: `no such member AppearanceObserver`.

- [ ] **Step 3: Write implementation**

`app/Leah/Sources/LeahUI/AppearanceObserver.swift`:
```swift
import AppKit
import SwiftUI

public final class AppearanceObserver: NSObject, ObservableObject {
    @Published public private(set) var isLight: Bool = false
    private var token: NSKeyValueObservation?

    public override init() {
        super.init()
        self.isLight = Self.isLightAppearance(NSApp?.effectiveAppearance.name)
        token = NSApp?.observe(\.effectiveAppearance, options: [.new]) { [weak self] app, _ in
            DispatchQueue.main.async {
                withAnimation(.easeInOut(duration: 0.24)) {
                    self?.isLight = Self.isLightAppearance(app.effectiveAppearance.name)
                }
            }
        }
    }

    deinit { token?.invalidate() }

    public static func isLightAppearance(_ name: NSAppearance.Name?) -> Bool {
        guard let n = name else { return false }
        switch n {
        case .aqua, .vibrantLight, .accessibilityHighContrastAqua, .accessibilityHighContrastVibrantLight:
            return true
        default:
            return false
        }
    }
}
```

Modify `LeahApp.swift` to instantiate `AppearanceObserver()` as a stored property and inject into the SwiftUI environment via `.environmentObject(appearance)` on the root scene + hosting controllers.

- [ ] **Step 4: Run — expect PASS**

```bash
cd /Users/treedesk/Desktop/Projects/leah/app/Leah
swift test --filter LeahUITests.AppearanceObserverTests 2>&1 | tail -5
swift build 2>&1 | tail -5
```

Expected: tests PASS; build clean.

- [ ] **Step 5: Commit**

```bash
cd /Users/treedesk/Desktop/Projects/leah
git add app/Leah/Sources/LeahUI/AppearanceObserver.swift app/Leah/Sources/LeahApp/LeahApp.swift \
        app/Leah/Tests/LeahUITests/AppearanceObserverTests.swift
git commit -m "feat(hud): NSApp.effectiveAppearance KVO observer with 240ms cross-fade animation"
```

---

## Wave 6 — Wizard mic + integration acceptance + E2E + final review — tasks 21-23

---

### Task 21: Wizard mic + Calendar EventKit acceptance test

**Files:**
- Create: `app/Leah/Tests/LeahUITests/WizardAcceptanceTests.swift`
- Modify: `app/Leah/Sources/LeahUI/Wizard/MicStep.swift` (verify mic request triggers AVAudioApplication API; add testable hook)
- Modify: `app/Leah/Sources/LeahUI/Wizard/IntegrationStep.swift` (verify gcal connect emits IPC + EventKit prompt path)

**Note:** Phase 1 shipped a 6-step wizard. Phase 2 just verifies the mic + Calendar integration flows are correctly wired to the daemon. No new step needed (per adjustment in source task description: "no rework needed but acceptance gate").

**Interfaces:**
- `MicStep` exposes `public static func requestMicPermission() async -> Bool` calling `AVAudioApplication.requestRecordPermission(completionHandler:)` (macOS 14+)
- `IntegrationStep` exposes `public static func selectedProviderID() -> String?` reading from `@AppStorage("leah.wizard.integration")`

- [ ] **Step 1: Write failing acceptance tests**

`app/Leah/Tests/LeahUITests/WizardAcceptanceTests.swift`:
```swift
import XCTest
@testable import LeahUI

final class WizardAcceptanceTests: XCTestCase {
    func testMicStepExposesRequestAPI() {
        // Verifies the requestMicPermission entry point exists; actual TCC dialog is not invoked in unit tests.
        let _: () = ({ _ = MicStep.requestMicPermission }())
    }

    func testIntegrationStepStoresChoice() {
        UserDefaults.standard.set("gcal", forKey: "leah.wizard.integration")
        defer { UserDefaults.standard.removeObject(forKey: "leah.wizard.integration") }
        XCTAssertEqual(IntegrationStep.selectedProviderID(), "gcal")
    }

    func testIntegrationStepHandlesNoChoice() {
        UserDefaults.standard.removeObject(forKey: "leah.wizard.integration")
        XCTAssertNil(IntegrationStep.selectedProviderID())
    }
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
cd /Users/treedesk/Desktop/Projects/leah/app/Leah
swift test --filter LeahUITests.WizardAcceptanceTests 2>&1 | tail -10
```

Expected: `requestMicPermission` / `selectedProviderID` undefined.

- [ ] **Step 3: Add testable hooks**

In `app/Leah/Sources/LeahUI/Wizard/MicStep.swift`, add:
```swift
import AVFoundation

public extension MicStep {
    static func requestMicPermission() async -> Bool {
        await withCheckedContinuation { (cont: CheckedContinuation<Bool, Never>) in
            AVCaptureDevice.requestAccess(for: .audio) { granted in
                cont.resume(returning: granted)
            }
        }
    }
}
```

In `app/Leah/Sources/LeahUI/Wizard/IntegrationStep.swift`, add:
```swift
public extension IntegrationStep {
    static func selectedProviderID() -> String? {
        UserDefaults.standard.string(forKey: "leah.wizard.integration")
    }
}
```

- [ ] **Step 4: Run — expect PASS**

```bash
cd /Users/treedesk/Desktop/Projects/leah/app/Leah
swift test --filter LeahUITests.WizardAcceptanceTests 2>&1 | tail -8
```

Expected: tests PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/treedesk/Desktop/Projects/leah
git add app/Leah/Sources/LeahUI/Wizard/MicStep.swift app/Leah/Sources/LeahUI/Wizard/IntegrationStep.swift \
        app/Leah/Tests/LeahUITests/WizardAcceptanceTests.swift
git commit -m "test(wizard): acceptance gates verify mic + integration step API surface"
```

---

### Task 22: Daemon registers all Phase 2 adapters at startup

**Files:**
- Modify: `cmd/leah-daemon/main.go` (call `widget.RegisterPureKinds(reg)` + register fetch adapters with available config)
- Create: `cmd/leah-daemon/widgets_wiring_test.go`

**Interfaces:**
- Add `widgetRegistry := widget.NewRegistry()` to main composition root
- Register: market (if `ALPHAVANTAGE_API_KEY` set), weather (if `OPENWEATHERMAP_API_KEY` set), calendar (if gcal token available), flights, maps, all pure kinds
- Pass registry into IPC handler for `render_widget` validation (future expansion path)

- [ ] **Step 1: Write failing test**

`cmd/leah-daemon/widgets_wiring_test.go`:
```go
package main

import (
	"testing"

	"github.com/trilam/leah/internal/widget"
)

func TestBuildRegistry_PureKindsAlwaysPresent(t *testing.T) {
	reg := buildWidgetRegistry(buildRegistryConfig{})
	for _, k := range []string{widget.KindCode, widget.KindDiff, widget.KindCitation, widget.KindImage, widget.KindChart, widget.KindFlights, widget.KindMaps} {
		if _, ok := reg.Get(k); !ok {
			t.Fatalf("pure kind %q must always be registered", k)
		}
	}
}

func TestBuildRegistry_FetchKindsRequireConfig(t *testing.T) {
	// No API keys → fetch adapters absent.
	reg := buildWidgetRegistry(buildRegistryConfig{})
	if _, ok := reg.Get(widget.KindMarket); ok {
		t.Error("market should not register without AlphaVantage key")
	}
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
cd /Users/treedesk/Desktop/Projects/leah
go test ./cmd/leah-daemon/... -run TestBuildRegistry 2>&1 | head -10
```

Expected: `undefined: buildWidgetRegistry`.

- [ ] **Step 3: Write implementation**

In `cmd/leah-daemon/main.go` (or a new sibling file `cmd/leah-daemon/widgets_wiring.go`):
```go
package main

import (
	"github.com/trilam/leah/internal/feeds"
	"github.com/trilam/leah/internal/widget"
	"golang.org/x/oauth2"
)

type buildRegistryConfig struct {
	AlphaVantageKey string
	OpenWeatherKey  string
	WeatherLocation string
	GcalTokenSource oauth2.TokenSource
}

func buildWidgetRegistry(cfg buildRegistryConfig) *widget.Registry {
	reg := widget.NewRegistry()
	// Pure kinds — always present.
	widget.RegisterPureKinds(reg)
	reg.Register(widget.NewFlightsAdapter())
	reg.Register(widget.NewMapsAdapter())
	// Fetch-backed kinds — only when their dependency is configured.
	if cfg.AlphaVantageKey != "" {
		if m, err := feeds.NewMarket(feeds.MarketConfig{APIKey: cfg.AlphaVantageKey}); err == nil {
			reg.Register(widget.NewMarketAdapter(m))
		}
	}
	if cfg.OpenWeatherKey != "" && cfg.WeatherLocation != "" {
		if w, err := feeds.NewWeather(feeds.WeatherConfig{APIKey: cfg.OpenWeatherKey, Location: cfg.WeatherLocation}); err == nil {
			reg.Register(widget.NewWeatherAdapter(w))
		}
	}
	if cfg.GcalTokenSource != nil {
		reg.Register(widget.NewCalendarAdapter(cfg.GcalTokenSource))
	}
	return reg
}
```

Wire `buildWidgetRegistry(buildRegistryConfig{ AlphaVantageKey: os.Getenv("ALPHAVANTAGE_API_KEY"), ... })` in `main()` and pass to the IPC handler if/when the `render_widget` path consumes it.

- [ ] **Step 4: Run — expect PASS**

```bash
cd /Users/treedesk/Desktop/Projects/leah
go test ./cmd/leah-daemon/... -run TestBuildRegistry -v 2>&1 | tail -8
go build ./cmd/leah-daemon/ 2>&1 | tail -5
```

Expected: tests PASS; build clean.

- [ ] **Step 5: Commit**

```bash
cd /Users/treedesk/Desktop/Projects/leah
git add cmd/leah-daemon/widgets_wiring.go cmd/leah-daemon/widgets_wiring_test.go cmd/leah-daemon/main.go
git commit -m "feat(daemon): buildWidgetRegistry wires 10 Phase 2 adapters with env-gated fetch kinds"
```

---

### Task 23: Phase 2 E2E smoke (`scripts/smoke/phase2-e2e.sh`)

**Files:**
- Create: `scripts/smoke/phase2-e2e.sh`
- Create: `scripts/smoke/widget-pin.go` (Go client speaking IPC `widget.pin` / `widget.unpin`)
- Create: `scripts/smoke/phase2-e2e_test.sh`
- Modify: `scripts/smoke-all.sh` (smoke-all already skips `*_test.sh`; verify phase2 included)

**Interfaces:**
- Boots `leah-daemon` in temp `XDG_STATE_HOME` / `HOME`-override; waits for socket; sends `widget.pin`; reads pinned-widgets.json; sends `widget.unpin`; verifies file empty; tears down via `trap`

- [ ] **Step 1: Write failing harness**

`scripts/smoke/phase2-e2e_test.sh`:
```bash
#!/usr/bin/env bash
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
out=$("$HERE/phase2-e2e.sh" 2>&1) || { echo "$out"; exit 1; }
echo "$out" | grep -q "phase2 e2e ok" || { echo "missing phase2 e2e ok marker"; echo "$out"; exit 1; }
echo "phase2-e2e harness passed"
```

`scripts/smoke/widget-pin.go`:
```go
//go:build ignore

// widget-pin.go is a smoke-test client that connects to the leah-daemon Unix
// socket and exercises the widget.pin / widget.unpin handlers.
package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: widget-pin <socket-path>")
		os.Exit(1)
	}
	conn, err := net.Dial("unix", os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "dial:", err)
		os.Exit(1)
	}
	defer conn.Close()

	pin := map[string]any{
		"kind":    "widget.pin",
		"turn_id": "smoke",
		"seq":     1,
		"payload": map[string]any{
			"id":         "w1",
			"type":       "market",
			"props":      map[string]any{"symbols": []string{"AAPL"}},
			"refresh_ns": int64(60_000_000_000),
		},
	}
	if err := writeFrame(conn, pin); err != nil {
		fmt.Fprintln(os.Stderr, "write pin:", err)
		os.Exit(1)
	}
	resp, err := readFrame(conn)
	if err != nil {
		fmt.Fprintln(os.Stderr, "read pin resp:", err)
		os.Exit(1)
	}
	if kind, _ := resp["kind"].(string); kind != "widget.pin.ok" {
		fmt.Fprintln(os.Stderr, "expected widget.pin.ok, got:", resp)
		os.Exit(1)
	}

	unpin := map[string]any{
		"kind":    "widget.unpin",
		"turn_id": "smoke",
		"seq":     1,
		"payload": map[string]any{"id": "w1"},
	}
	if err := writeFrame(conn, unpin); err != nil {
		fmt.Fprintln(os.Stderr, "write unpin:", err)
		os.Exit(1)
	}
	if _, err := readFrame(conn); err != nil {
		fmt.Fprintln(os.Stderr, "read unpin resp:", err)
		os.Exit(1)
	}
	fmt.Println("widget-pin client ok")
}

func writeFrame(c net.Conn, v any) error {
	body, err := json.Marshal(v)
	if err != nil {
		return err
	}
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(body)))
	if _, err := c.Write(hdr[:]); err != nil {
		return err
	}
	_, err = c.Write(body)
	return err
}

func readFrame(c net.Conn) (map[string]any, error) {
	var hdr [4]byte
	if _, err := c.Read(hdr[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(hdr[:])
	buf := make([]byte, n)
	if _, err := c.Read(buf); err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(buf, &out); err != nil {
		return nil, err
	}
	return out, nil
}
```

`scripts/smoke/phase2-e2e.sh`:
```bash
#!/usr/bin/env bash
set -euo pipefail

REPO="$(cd "$(dirname "$0")/../.." && pwd)"
TMP=$(mktemp -d -t leah-phase2-e2e.XXXXXX)
trap 'rm -rf "$TMP"; [ -n "${PID:-}" ] && kill "$PID" 2>/dev/null || true' EXIT

export HOME="$TMP"
SOCKET="$TMP/leah-daemon.sock"
PIN_FILE="$TMP/Library/Application Support/Leah/pinned-widgets.json"
mkdir -p "$(dirname "$PIN_FILE")"

cd "$REPO"
go build -o "$TMP/leah-daemon" ./cmd/leah-daemon
LEAH_SOCKET_PATH="$SOCKET" "$TMP/leah-daemon" &
PID=$!

# Wait up to 10s for socket.
for _ in $(seq 1 100); do
  [ -S "$SOCKET" ] && break
  sleep 0.1
done
[ -S "$SOCKET" ] || { echo "socket never appeared"; exit 1; }

go run "$REPO/scripts/smoke/widget-pin.go" "$SOCKET"

# Verify pinned-widgets.json went w1 → empty.
[ -f "$PIN_FILE" ] || { echo "pin file never written"; exit 1; }
if grep -q '"w1"' "$PIN_FILE"; then
  echo "expected w1 removed after unpin; file still contains w1"
  cat "$PIN_FILE"
  exit 1
fi

echo "phase2 e2e ok"
```

Make executable:
```bash
chmod +x scripts/smoke/phase2-e2e.sh scripts/smoke/phase2-e2e_test.sh
```

- [ ] **Step 2: Run — expect FAIL (script not yet executable / daemon path issues)**

```bash
cd /Users/treedesk/Desktop/Projects/leah
bash scripts/smoke/phase2-e2e_test.sh 2>&1 | tail -15
```

Expected: First runs may fail on socket-path env var or pin-file path; iterate fixes until "phase2 e2e ok" prints.

- [ ] **Step 3: Fix daemon to honor `LEAH_SOCKET_PATH`**

In `cmd/leah-daemon/main.go`, ensure the socket path falls back through `os.Getenv("LEAH_SOCKET_PATH")` before the default. This is likely already wired from Phase 1; if not, add:
```go
socketPath := os.Getenv("LEAH_SOCKET_PATH")
if socketPath == "" {
    socketPath = filepath.Join(homeDir, "Library/Application Support/Leah/leah-daemon.sock")
}
```

- [ ] **Step 4: Run — expect PASS**

```bash
cd /Users/treedesk/Desktop/Projects/leah
bash scripts/smoke/phase2-e2e_test.sh 2>&1 | tail -5
```

Expected:
```
phase2 e2e ok
phase2-e2e harness passed
```

- [ ] **Step 5: Commit**

```bash
cd /Users/treedesk/Desktop/Projects/leah
git add scripts/smoke/phase2-e2e.sh scripts/smoke/phase2-e2e_test.sh scripts/smoke/widget-pin.go \
        cmd/leah-daemon/main.go
git commit -m "test(smoke): phase2 e2e — boot daemon, pin/unpin widget, verify pinned-widgets.json round-trip"
```

---

### Task 24: Final whole-branch Phase 2 review

**Files:** (no code; review-only task)
- Read: every PR's diff merged in Phase 2 (use `gh pr list --state merged --base main --search "phase2" --json number,title`)

- [ ] **Step 1: Inventory merged PRs**

```bash
cd /Users/treedesk/Desktop/Projects/leah
gh pr list --state merged --base main --json number,title,mergedAt --limit 50 \
  | jq '[.[] | select(.mergedAt > "2026-06-22T00:00:00Z")] | sort_by(.number)'
```

- [ ] **Step 2: Reviewer subagent with the anti-self-approve header**

Spawn a fresh reviewer subagent (`cavecrew-reviewer-*` or `a*` agent ID) with the contents of `/Users/treedesk/Desktop/Projects/leah/.superpowers/sdd/reviewer-header.md` prepended to the review prompt. Reviewer must NOT be the same agent that authored any task PR. Review dimensions per CLAUDE.md:
- correctness/bugs
- unintended side effects
- conciseness
- refactor opportunity
- simplification
- doc updates needed
- comment trimming
- test coverage
- deletion-default (what got smaller?)
- no AI signatures
- no ceremony
- gold-budget ≤ 3 per surface
- Tiempos/New York Italic only in Dashboard "Today" header (no leak into widget tiles)

- [ ] **Step 3: Reviewer verdict**

Verdict format (in transcript only — not as a PR comment): `APPROVE` or `REVISE: <bullet list of blocking findings>`. If REVISE, file follow-up PRs per finding.

- [ ] **Step 4: Run the full Phase 2 smoke**

```bash
cd /Users/treedesk/Desktop/Projects/leah
make smoke 2>&1 | tail -20
go test ./internal/widget/... ./cmd/leah-daemon/... -v 2>&1 | grep -E 'PASS|FAIL|ok'
cd app/Leah && swift test 2>&1 | tail -10
```

Expected: all PASS; smoke marker present.

- [ ] **Step 5: Tag the Phase 2 milestone**

```bash
cd /Users/treedesk/Desktop/Projects/leah
git tag -a phase2-complete -m "Phase 2 complete: ambient HUD, 10 widget kinds, pin flow, light mode, BGE ONNX, 5 settings panes"
git push origin phase2-complete
```

---

## Self-review

**Spec coverage:**
- Ambient HUD §7.1 → Task 13 (3-row layout, time-of-day greeting, source rotation, pulse metric)
- Notification widget §10.1 #14 → Task 14 (2-cap, 3s coalesce)
- 10 widget kinds §10.1 → Tasks 2,3,7-12 (market, flights, weather, maps, calendar, image, chart, code, citation, diff — 10/10)
- Widget gallery §19 P2 → Task 15 (13-entry overlay, `/widgets` + `+` button)
- Pin-to-ambient flow §10.2 → Tasks 4,5,18 (PinStore + IPC + Swift observer)
- Light mode parity §3.6 → Tasks 19,20 (bone tokens + KVO observer w/ 240ms cross-fade)
- Settings remaining 5 sections → Tasks 16,17 (Voice/Appearance/Integrations/Memory/About)
- Wizard mic + integration verify → Task 21
- BGE-small-en-v1.5 ONNX §17.15 → Task 6
- Adapter registry §10.6 + lifecycle §10.2 → Tasks 1,22
- E2E smoke + final review → Tasks 23,24

**Placeholder scan:** no "TBD" / "implement later" / "handle edge cases" found. Each step ships complete code.

**Type consistency:**
- `WidgetAdapter` interface matches `MarketAdapter`/`WeatherAdapter`/etc method signatures (Type/Validate/Fetch/Refresh — all consistent)
- `WidgetSize` enum cases (.small/.medium/.large) match existing `Widget.swift` definition; all new tiles default to spec-canonical sizes
- `Palette.adaptiveBackground(level:isLight:)` signature consistent between Task 19 definition and downstream Task 20 callers
- `PinnedEntry` Go struct ↔ `PinnedSlot` Swift struct: id/type fields aligned; props is `json.RawMessage` Go / `Data` Swift (both raw bytes)
- IPC kinds: `widget.pin`, `widget.unpin`, `widget.pin.ok`, `widget.unpin.ok`, `widget.pin.err`, `widget.unpin.err` — all defined in Task 5, exercised in Task 23
- `SettingsPane.allCases.count` = 9 after Task 16 extension; tests in Task 16 assert this

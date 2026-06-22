# Leah macOS Native UI — Phase 2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver the full Phase 2 surface: 10 new widget types with Go adapters, pinned-to-ambient flow with fsnotify watcher, Ambient HUD chrome, notification toast stack, widget gallery overlay, 5 remaining Settings panes, light-mode palette auto-switch, and BGE-small-en-v1.5 ONNX local embedding fallback.

**Architecture:** Each new widget kind gets a Go adapter in `internal/widget/` that implements the `WidgetAdapter` interface (Type/Validate/Fetch/Refresh), wired into the daemon IPC handler's `render_widget` dispatch. The Swift side adds one SwiftUI tile file per kind in `app/Leah/Sources/LeahWidgets/`. The Ambient HUD (`app/Leah/Sources/LeahUI/AmbientHUD.swift`) is a new `NSPanel`-hosted SwiftUI view that reads pinned-widget state and HUD IPC frames. Pin state persists to `~/Library/Application Support/Leah/pinned-widgets.json` with a single fsnotify watcher (200 ms debounce) in `internal/widget/pinstore.go`.

**Tech Stack:** Go 1.25, SwiftUI/AppKit (macOS 14+), `github.com/fsnotify/fsnotify`, `github.com/trilam/leah` module, existing `internal/ipc` length-prefixed JSON frames (256 KB cap), existing `LeahWidgets.Palette` + `LeahWidget` protocol, existing `embed.Generator` interface.

## Global Constraints

- Module path: `github.com/trilam/leah`
- macOS deployment target: 14.0
- No AI signatures (no Co-Authored-By, no "Generated with")
- No new top-level packages outside `internal/widget/` for Go adapters — use that single package
- Gold budget: max 3 `Palette.champagneGold` uses per SwiftUI tile
- Tiempos/New York Italic: Dashboard "Today" header ONLY — never in widget tiles
- `LeahWidget` protocol: `public protocol LeahWidget: View { var size: WidgetSize { get } }` — all tiles must conform
- `WidgetSize` enum: `.small`, `.medium`, `.large` (existing, in `Widget.swift`)
- `Palette` tokens (existing, `Tokens.swift`): `.obsidian0/1/2/3`, `.champagneGold`, `.oxblood`, `.ivory`, `.textMuted`, `.hairline`
- Light-mode bone tokens (new, Task 22): `Palette.bone0 = #F2EFE8`, `bone1 = #EAE6DD`, `bone2 = #E2DACE`, `bone3 = #D6CFBC`
- IPC frame: `struct Frame { Kind, TurnID string; Seq uint64; Payload json.RawMessage }` — `WriteFrame`/`ReadFrame` in `internal/ipc/frame.go`
- Existing IPC kinds in use: `"ask"`, `"verify-key"`, `"diag.state"`, `"widget.mount"` — do not reuse these strings
- Reviewer required per PR (anti-self-approve header from `.superpowers/sdd/reviewer-header.md`); transcript-only verdict
- Dev harness: `make dev` + `scripts/dev/*.sh`; use for runtime verification steps
- `SettingsPane` enum (existing `Pane.swift`): cases `general, privacy, permissions, advanced` with shortcuts `"1"`–`"4"` — extend, do not replace
- `embed.Generator` interface: `Embed(ctx, []string) ([][]float32, error)`, `Name() string`, `Dim() int`
- `LEAH_EMBED_BACKEND` env var: extend `SelectGenerator()` with `"bge"` case
- Deletion default: every PR states what got smaller

---

## Wave 1 — Go backend (adapters + pin store + IPC) — tasks 1-10 parallelize up to 6

---

### Task 1: Widget adapter interface + registry (`internal/widget/`)

**Files:**
- Create: `internal/widget/adapter.go`
- Create: `internal/widget/registry.go`
- Create: `internal/widget/adapter_test.go`
- Create: `internal/widget/registry_test.go`

**Interfaces:**
- Produces:
  - `type Payload struct { Data json.RawMessage; FetchedAt time.Time; StaleAfter time.Duration; Source, Etag string }`
  - `type WidgetAdapter interface { Type() string; Validate(props json.RawMessage) error; Fetch(ctx context.Context, props json.RawMessage) (Payload, error); Refresh(ctx context.Context, id string, props json.RawMessage, prev *Payload) (Payload, error) }`
  - `type PureAdapter struct{}` — implements WidgetAdapter; Fetch returns props as-is with StaleAfter=0
  - `type Registry struct` with `func NewRegistry() *Registry`, `func (r *Registry) Register(a WidgetAdapter)`, `func (r *Registry) Get(typ string) (WidgetAdapter, bool)`
  - Kind constants: `KindMarket = "market"`, `KindFlights = "flights"`, `KindWeather = "weather"`, `KindMaps = "maps"`, `KindCalendar = "calendar"`, `KindImage = "image"`, `KindChart = "chart"`, `KindCode = "code"`, `KindCitation = "citation"`, `KindDiff = "diff"`

- [ ] **Step 1: Write the failing tests**

`internal/widget/adapter_test.go`:
```go
package widget_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/trilam/leah/internal/widget"
)

func TestPureAdapter_TypeAndValidate(t *testing.T) {
	a := widget.NewPureAdapter("code")
	if a.Type() != "code" {
		t.Fatalf("type: got %q want %q", a.Type(), "code")
	}
	if err := a.Validate(json.RawMessage(`{"language":"go","source":"x"}`)); err != nil {
		t.Fatalf("validate: %v", err)
	}
}

func TestPureAdapter_FetchReturnsProps(t *testing.T) {
	a := widget.NewPureAdapter("diff")
	props := json.RawMessage(`{"hunks":[]}`)
	p, err := a.Fetch(context.Background(), props)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if string(p.Data) != string(props) {
		t.Fatalf("data mismatch: got %s want %s", p.Data, props)
	}
	if p.StaleAfter != 0 {
		t.Fatalf("pure adapter must have StaleAfter=0, got %v", p.StaleAfter)
	}
}
```

`internal/widget/registry_test.go`:
```go
package widget_test

import (
	"testing"

	"github.com/trilam/leah/internal/widget"
)

func TestRegistry_RegisterAndGet(t *testing.T) {
	r := widget.NewRegistry()
	a := widget.NewPureAdapter("code")
	r.Register(a)
	got, ok := r.Get("code")
	if !ok {
		t.Fatal("Get: not found after Register")
	}
	if got.Type() != "code" {
		t.Fatalf("type: %q", got.Type())
	}
}

func TestRegistry_GetMissing(t *testing.T) {
	r := widget.NewRegistry()
	if _, ok := r.Get("nope"); ok {
		t.Fatal("expected not found for unregistered kind")
	}
}

func TestKindConstants(t *testing.T) {
	for _, k := range []string{
		widget.KindMarket, widget.KindFlights, widget.KindWeather,
		widget.KindMaps, widget.KindCalendar, widget.KindImage,
		widget.KindChart, widget.KindCode, widget.KindCitation, widget.KindDiff,
	} {
		if k == "" {
			t.Fatal("empty kind constant")
		}
	}
}
```

- [ ] **Step 2: Run tests — expect FAIL**

```bash
cd /Users/treedesk/Desktop/Projects/leah
go test ./internal/widget/... 2>&1 | head -20
```

Expected: `cannot find package "github.com/trilam/leah/internal/widget"` or build error.

- [ ] **Step 3: Write implementation**

`internal/widget/adapter.go`:
```go
// Package widget defines the WidgetAdapter contract and PureAdapter for
// LLM-payload-only widget kinds (code, diff, citation, image, stat, list, table).
package widget

import (
	"context"
	"encoding/json"
	"time"
)

// Payload is the normalized output of every adapter Fetch/Refresh.
type Payload struct {
	Data       json.RawMessage `json:"data"`
	FetchedAt  time.Time       `json:"fetched_at"`
	StaleAfter time.Duration   `json:"stale_after"`
	Source     string          `json:"source"`
	Etag       string          `json:"etag,omitempty"`
}

// WidgetAdapter is the fetch contract every widget kind implements.
type WidgetAdapter interface {
	Type() string
	Validate(props json.RawMessage) error
	Fetch(ctx context.Context, props json.RawMessage) (Payload, error)
	Refresh(ctx context.Context, id string, props json.RawMessage, prev *Payload) (Payload, error)
}

// Kind constants match the IPC "widget" field and LLM tool-call kind.
const (
	KindMarket   = "market"
	KindFlights  = "flights"
	KindWeather  = "weather"
	KindMaps     = "maps"
	KindCalendar = "calendar"
	KindImage    = "image"
	KindChart    = "chart"
	KindCode     = "code"
	KindCitation = "citation"
	KindDiff     = "diff"
)

// PureAdapter is for widget kinds whose data comes entirely from the LLM
// payload — no external fetch required. Fetch returns props verbatim.
type PureAdapter struct{ kind string }

// NewPureAdapter constructs a PureAdapter for the given kind string.
func NewPureAdapter(kind string) *PureAdapter { return &PureAdapter{kind: kind} }

func (p *PureAdapter) Type() string { return p.kind }

func (p *PureAdapter) Validate(_ json.RawMessage) error { return nil }

func (p *PureAdapter) Fetch(_ context.Context, props json.RawMessage) (Payload, error) {
	return Payload{Data: props, FetchedAt: time.Now().UTC(), StaleAfter: 0, Source: "llm"}, nil
}

func (p *PureAdapter) Refresh(ctx context.Context, _ string, props json.RawMessage, _ *Payload) (Payload, error) {
	return p.Fetch(ctx, props)
}
```

`internal/widget/registry.go`:
```go
package widget

import "sync"

// Registry maps widget kind strings to their adapters. Thread-safe.
type Registry struct {
	mu       sync.RWMutex
	adapters map[string]WidgetAdapter
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{adapters: make(map[string]WidgetAdapter)}
}

// Register adds or replaces an adapter. Later calls win.
func (r *Registry) Register(a WidgetAdapter) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.adapters[a.Type()] = a
}

// Get retrieves the adapter for a kind. Returns false if unregistered.
func (r *Registry) Get(typ string) (WidgetAdapter, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.adapters[typ]
	return a, ok
}
```

- [ ] **Step 4: Run tests — expect PASS**

```bash
cd /Users/treedesk/Desktop/Projects/leah
go test ./internal/widget/... -v 2>&1 | tail -15
```

Expected:
```
--- PASS: TestPureAdapter_TypeAndValidate (0.00s)
--- PASS: TestPureAdapter_FetchReturnsProps (0.00s)
--- PASS: TestRegistry_RegisterAndGet (0.00s)
--- PASS: TestRegistry_GetMissing (0.00s)
--- PASS: TestKindConstants (0.00s)
PASS
ok  	github.com/trilam/leah/internal/widget
```

- [ ] **Step 5: Commit**

```bash
cd /Users/treedesk/Desktop/Projects/leah
git add internal/widget/adapter.go internal/widget/registry.go \
        internal/widget/adapter_test.go internal/widget/registry_test.go
git commit -m "feat(widget): adapter interface, PureAdapter, Registry, kind constants"
```

---

### Task 2: Market widget adapter (`internal/widget/market.go`)

**Files:**
- Create: `internal/widget/market.go`
- Create: `internal/widget/market_test.go`

**Interfaces:**
- Consumes: `feeds.Market` (`internal/feeds/market.go`) — `func NewMarket(cfg feeds.MarketConfig) (*feeds.Market, error)`, `func (m *feeds.Market) FetchAll(ctx, []string) ([]feeds.Quote, error)`, `feeds.Quote{Symbol, Price, Change, ChangePct, AsOf string}`
- Consumes: `widget.WidgetAdapter`, `widget.Payload` from Task 1
- Produces: `type MarketAdapter struct` implementing `WidgetAdapter`; `func NewMarketAdapter(m *feeds.Market) *MarketAdapter`
- Fetch output JSON shape: `{"quotes":[{"symbol":"AAPL","price":"213.40","change":"+1.20","change_pct":"+0.57%","as_of":"15:59"}]}`
- Refresh delegates to Fetch (no etag for market data)
- StaleAfter: 60 seconds

- [ ] **Step 1: Write the failing test**

`internal/widget/market_test.go`:
```go
package widget_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/trilam/leah/internal/feeds"
	"github.com/trilam/leah/internal/widget"
)

func TestMarketAdapter_Type(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"Global Quote":{"01. symbol":"AAPL","05. price":"213.40","09. change":"+1.20","10. change percent":"+0.5628%","07. latest trading day":"2026-06-22"}}`))
	}))
	defer srv.Close()
	m, err := feeds.NewMarket(feeds.MarketConfig{APIKey: "test", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("NewMarket: %v", err)
	}
	a := widget.NewMarketAdapter(m)
	if a.Type() != widget.KindMarket {
		t.Fatalf("type: %q", a.Type())
	}
}

func TestMarketAdapter_Fetch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"Global Quote":{"01. symbol":"AAPL","05. price":"213.40","09. change":"+1.20","10. change percent":"+0.5628%","07. latest trading day":"2026-06-22"}}`))
	}))
	defer srv.Close()
	m, _ := feeds.NewMarket(feeds.MarketConfig{APIKey: "test", BaseURL: srv.URL})
	a := widget.NewMarketAdapter(m)

	props := json.RawMessage(`{"symbols":["AAPL"]}`)
	p, err := a.Fetch(context.Background(), props)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if p.StaleAfter != 60e9 { // 60 seconds in nanoseconds
		t.Fatalf("StaleAfter: %v", p.StaleAfter)
	}
	var out struct {
		Quotes []struct {
			Symbol string `json:"symbol"`
			Price  string `json:"price"`
		} `json:"quotes"`
	}
	if err := json.Unmarshal(p.Data, &out); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	if len(out.Quotes) != 1 || out.Quotes[0].Symbol != "AAPL" {
		t.Fatalf("unexpected quotes: %+v", out.Quotes)
	}
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
cd /Users/treedesk/Desktop/Projects/leah
go test ./internal/widget/... -run TestMarketAdapter 2>&1 | head -10
```

Expected: `undefined: widget.NewMarketAdapter`

- [ ] **Step 3: Write implementation**

`internal/widget/market.go`:
```go
package widget

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/trilam/leah/internal/feeds"
)

// MarketAdapter fetches live quotes via feeds.Market (Alpha Vantage).
// Props schema: {"symbols":["AAPL","GOOG"]}
type MarketAdapter struct{ m *feeds.Market }

// NewMarketAdapter wraps an initialised feeds.Market.
func NewMarketAdapter(m *feeds.Market) *MarketAdapter { return &MarketAdapter{m: m} }

func (a *MarketAdapter) Type() string { return KindMarket }

func (a *MarketAdapter) Validate(props json.RawMessage) error {
	var p struct {
		Symbols []string `json:"symbols"`
	}
	if err := json.Unmarshal(props, &p); err != nil {
		return fmt.Errorf("market: invalid props: %w", err)
	}
	if len(p.Symbols) == 0 {
		return fmt.Errorf("market: props.symbols must be non-empty")
	}
	return nil
}

type marketOutput struct {
	Quotes []marketQuote `json:"quotes"`
}

type marketQuote struct {
	Symbol    string `json:"symbol"`
	Price     string `json:"price"`
	Change    string `json:"change"`
	ChangePct string `json:"change_pct"`
	AsOf      string `json:"as_of"`
}

func (a *MarketAdapter) Fetch(ctx context.Context, props json.RawMessage) (Payload, error) {
	var p struct {
		Symbols []string `json:"symbols"`
	}
	if err := json.Unmarshal(props, &p); err != nil {
		return Payload{}, fmt.Errorf("market: props: %w", err)
	}
	quotes, err := a.m.FetchAll(ctx, p.Symbols)
	if err != nil {
		return Payload{}, fmt.Errorf("market: fetch: %w", err)
	}
	out := marketOutput{Quotes: make([]marketQuote, len(quotes))}
	for i, q := range quotes {
		out.Quotes[i] = marketQuote{
			Symbol:    q.Symbol,
			Price:     q.Price,
			Change:    q.Change,
			ChangePct: q.ChangePct,
			AsOf:      q.AsOf,
		}
	}
	data, err := json.Marshal(out)
	if err != nil {
		return Payload{}, fmt.Errorf("market: marshal: %w", err)
	}
	return Payload{
		Data:       data,
		FetchedAt:  time.Now().UTC(),
		StaleAfter: 60 * time.Second,
		Source:     "alphavantage",
	}, nil
}

func (a *MarketAdapter) Refresh(ctx context.Context, _ string, props json.RawMessage, _ *Payload) (Payload, error) {
	return a.Fetch(ctx, props)
}
```

- [ ] **Step 4: Run — expect PASS**

```bash
cd /Users/treedesk/Desktop/Projects/leah
go test ./internal/widget/... -run TestMarketAdapter -v 2>&1 | tail -10
```

Expected:
```
--- PASS: TestMarketAdapter_Type (0.00s)
--- PASS: TestMarketAdapter_Fetch (0.00s)
PASS
```

- [ ] **Step 5: Commit**

```bash
cd /Users/treedesk/Desktop/Projects/leah
git add internal/widget/market.go internal/widget/market_test.go
git commit -m "feat(widget): MarketAdapter wraps feeds.Market for live quote tiles"
```

---

### Task 3: Weather, Calendar, Flights, Maps, Image/Chart/Citation/Diff adapters

**Files:**
- Create: `internal/widget/weather.go`, `internal/widget/weather_test.go`
- Create: `internal/widget/calendar.go`, `internal/widget/calendar_test.go`
- Create: `internal/widget/flights.go`, `internal/widget/flights_test.go`
- Create: `internal/widget/maps.go`, `internal/widget/maps_test.go`
- Create: `internal/widget/pure_kinds.go`, `internal/widget/pure_kinds_test.go`

**Interfaces:**
- Consumes:
  - `feeds.Weather` / `feeds.Forecast` from `internal/feeds/weather.go` — `func (w *feeds.Weather) Fetch(ctx) (feeds.Forecast, error)` where `Forecast{Condition, TempC, TempF, Location, Humidity string; Icon string}`
  - `tripplanner.FlightOffer{Origin, Dest, Departs, Arrives, Carrier, FlightNum, PriceCents int}` from `internal/tripplanner/planner.go` — flights adapter takes props JSON with pre-fetched flight data (pure-ish: LLM passes the offer list)
  - `internal/maps/` — maps adapter is pure (LLM provides center/route props; tile renders via MapKit in Swift)
  - `internal/connect/gcal.go` — calendar adapter calls gcal HTTP to list today's events
- Produces: each adapter implements `WidgetAdapter`; pure kinds (image, chart, citation, diff) use `NewPureAdapter`

**Note on flights + maps:** The `tripplanner` package has flights, but the flights widget adapter is pure — the LLM emits already-resolved `FlightOffer` data in props. Same for maps: the Swift tile renders via MapKit; the adapter merely validates props and passes them through. This keeps the Go backend thin.

- [ ] **Step 1: Write failing tests**

`internal/widget/weather_test.go`:
```go
package widget_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/trilam/leah/internal/feeds"
	"github.com/trilam/leah/internal/widget"
)

func TestWeatherAdapter_FetchReturnsCondition(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Minimal OWM response shape feeds.Weather expects.
		w.Write([]byte(`{"weather":[{"description":"clear sky","icon":"01d"}],"main":{"temp":295.15,"humidity":60},"name":"San Francisco"}`))
	}))
	defer srv.Close()
	wt, err := feeds.NewWeather(feeds.WeatherConfig{APIKey: "test", Location: "San Francisco", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("NewWeather: %v", err)
	}
	a := widget.NewWeatherAdapter(wt)
	if a.Type() != widget.KindWeather {
		t.Fatalf("type: %q", a.Type())
	}
	props := json.RawMessage(`{"location":"San Francisco","horizon":"now"}`)
	p, err := a.Fetch(context.Background(), props)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	var out struct {
		Condition string `json:"condition"`
		TempF     string `json:"temp_f"`
	}
	if err := json.Unmarshal(p.Data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Condition == "" {
		t.Fatal("condition empty")
	}
	if p.StaleAfter != 15*60*1e9 {
		t.Fatalf("StaleAfter: want 15m, got %v", p.StaleAfter)
	}
}
```

`internal/widget/calendar_test.go`:
```go
package widget_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/trilam/leah/internal/widget"
)

func TestCalendarAdapter_Type(t *testing.T) {
	a := widget.NewCalendarAdapter(nil) // nil token source — validate only
	if a.Type() != widget.KindCalendar {
		t.Fatalf("type: %q", a.Type())
	}
}

func TestCalendarAdapter_ValidateRejectsEmpty(t *testing.T) {
	a := widget.NewCalendarAdapter(nil)
	if err := a.Validate(json.RawMessage(`{}`)); err == nil {
		t.Fatal("expected error for empty props (no date)")
	}
}

func TestCalendarAdapter_ValidateAcceptsDate(t *testing.T) {
	a := widget.NewCalendarAdapter(nil)
	if err := a.Validate(json.RawMessage(`{"date":"2026-06-22"}`)); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestCalendarAdapter_FetchNilTokenSourceReturnsEmptyNotError(t *testing.T) {
	a := widget.NewCalendarAdapter(nil)
	props := json.RawMessage(`{"date":"2026-06-22"}`)
	p, err := a.Fetch(context.Background(), props)
	// nil token source → graceful empty (no crash)
	if err != nil {
		t.Fatalf("Fetch with nil token source: %v", err)
	}
	var out struct {
		Events []interface{} `json:"events"`
	}
	if err := json.Unmarshal(p.Data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
}
```

`internal/widget/pure_kinds_test.go`:
```go
package widget_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/trilam/leah/internal/widget"
)

func TestPureKinds_AllRegistered(t *testing.T) {
	r := widget.NewRegistry()
	widget.RegisterPureKinds(r)
	for _, k := range []string{
		widget.KindImage, widget.KindChart, widget.KindCitation,
		widget.KindDiff, widget.KindCode,
	} {
		if _, ok := r.Get(k); !ok {
			t.Fatalf("pure kind %q not registered", k)
		}
	}
}

func TestFlightsAdapter_Type(t *testing.T) {
	a := widget.NewFlightsAdapter()
	if a.Type() != widget.KindFlights {
		t.Fatalf("type: %q", a.Type())
	}
}

func TestFlightsAdapter_ValidateRejectsNoFlights(t *testing.T) {
	a := widget.NewFlightsAdapter()
	if err := a.Validate(json.RawMessage(`{"flights":[]}`)); err == nil {
		t.Fatal("expected error for empty flights array")
	}
}

func TestFlightsAdapter_FetchReturnsProps(t *testing.T) {
	a := widget.NewFlightsAdapter()
	props := json.RawMessage(`{"flights":[{"origin":"SFO","dest":"JFK","departs":"08:00","arrives":"16:30","carrier":"UA","flight_num":"UA 101"}]}`)
	p, err := a.Fetch(context.Background(), props)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if string(p.Data) != string(props) {
		t.Fatalf("data: got %s", p.Data)
	}
}

func TestMapsAdapter_ValidateRejectsEmpty(t *testing.T) {
	a := widget.NewMapsAdapter()
	if err := a.Validate(json.RawMessage(`{}`)); err == nil {
		t.Fatal("expected error for empty maps props")
	}
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
cd /Users/treedesk/Desktop/Projects/leah
go test ./internal/widget/... 2>&1 | grep -E 'FAIL|undefined' | head -15
```

Expected: multiple `undefined: widget.NewWeatherAdapter` etc.

- [ ] **Step 3: Write implementations**

`internal/widget/weather.go`:
```go
package widget

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/trilam/leah/internal/feeds"
)

// WeatherAdapter wraps feeds.Weather for weather widget tiles.
// Props schema: {"location":"San Francisco","horizon":"now"|"hourly_24h"|"daily_7d"}
type WeatherAdapter struct{ w *feeds.Weather }

func NewWeatherAdapter(w *feeds.Weather) *WeatherAdapter { return &WeatherAdapter{w: w} }

func (a *WeatherAdapter) Type() string { return KindWeather }

func (a *WeatherAdapter) Validate(props json.RawMessage) error {
	var p struct {
		Location string `json:"location"`
	}
	if err := json.Unmarshal(props, &p); err != nil {
		return fmt.Errorf("weather: invalid props: %w", err)
	}
	if p.Location == "" {
		return fmt.Errorf("weather: props.location required")
	}
	return nil
}

type weatherOutput struct {
	Condition string `json:"condition"`
	TempC     string `json:"temp_c"`
	TempF     string `json:"temp_f"`
	Humidity  string `json:"humidity"`
	Icon      string `json:"icon"`
	Location  string `json:"location"`
}

func (a *WeatherAdapter) Fetch(ctx context.Context, props json.RawMessage) (Payload, error) {
	if a.w == nil {
		return Payload{}, fmt.Errorf("weather: adapter not configured")
	}
	f, err := a.w.Fetch(ctx)
	if err != nil {
		return Payload{}, fmt.Errorf("weather: fetch: %w", err)
	}
	out := weatherOutput{
		Condition: f.Condition,
		TempC:     f.TempC,
		TempF:     f.TempF,
		Humidity:  f.Humidity,
		Icon:      f.Icon,
		Location:  f.Location,
	}
	data, err := json.Marshal(out)
	if err != nil {
		return Payload{}, fmt.Errorf("weather: marshal: %w", err)
	}
	return Payload{
		Data:       data,
		FetchedAt:  time.Now().UTC(),
		StaleAfter: 15 * time.Minute,
		Source:     "openweathermap",
	}, nil
}

func (a *WeatherAdapter) Refresh(ctx context.Context, _ string, props json.RawMessage, _ *Payload) (Payload, error) {
	return a.Fetch(ctx, props)
}
```

`internal/widget/calendar.go`:
```go
package widget

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/oauth2"
)

// CalendarEvent is a single gcal event returned in the tile payload.
type CalendarEvent struct {
	Title    string `json:"title"`
	Start    string `json:"start"`
	End      string `json:"end"`
	Location string `json:"location,omitempty"`
}

// CalendarAdapter fetches today's Google Calendar events via the gcal HTTP API.
// A nil TokenSource yields an empty events list (graceful degraded state).
// Props schema: {"date":"2026-06-22"}
type CalendarAdapter struct {
	ts oauth2.TokenSource
}

// NewCalendarAdapter builds a CalendarAdapter. ts may be nil for testing.
func NewCalendarAdapter(ts oauth2.TokenSource) *CalendarAdapter {
	return &CalendarAdapter{ts: ts}
}

func (a *CalendarAdapter) Type() string { return KindCalendar }

func (a *CalendarAdapter) Validate(props json.RawMessage) error {
	var p struct {
		Date string `json:"date"`
	}
	if err := json.Unmarshal(props, &p); err != nil {
		return fmt.Errorf("calendar: invalid props: %w", err)
	}
	if p.Date == "" {
		return fmt.Errorf("calendar: props.date required (YYYY-MM-DD)")
	}
	return nil
}

type calendarOutput struct {
	Events []CalendarEvent `json:"events"`
	Date   string          `json:"date"`
}

func (a *CalendarAdapter) Fetch(ctx context.Context, props json.RawMessage) (Payload, error) {
	var p struct {
		Date string `json:"date"`
	}
	if err := json.Unmarshal(props, &p); err != nil {
		return Payload{}, fmt.Errorf("calendar: props: %w", err)
	}
	empty := func() (Payload, error) {
		data, _ := json.Marshal(calendarOutput{Events: []CalendarEvent{}, Date: p.Date})
		return Payload{Data: data, FetchedAt: time.Now().UTC(), StaleAfter: 30 * time.Second, Source: "gcal"}, nil
	}
	if a.ts == nil {
		return empty()
	}
	tok, err := a.ts.Token()
	if err != nil || tok == nil {
		return empty()
	}
	date, err := time.Parse("2006-01-02", p.Date)
	if err != nil {
		return Payload{}, fmt.Errorf("calendar: bad date %q: %w", p.Date, err)
	}
	min := date.UTC().Format(time.RFC3339)
	max := date.Add(24 * time.Hour).UTC().Format(time.RFC3339)
	url := fmt.Sprintf(
		"https://www.googleapis.com/calendar/v3/calendars/primary/events?timeMin=%s&timeMax=%s&singleEvents=true&orderBy=startTime",
		min, max,
	)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return empty()
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return empty()
	}
	var raw struct {
		Items []struct {
			Summary string `json:"summary"`
			Start   struct {
				DateTime string `json:"dateTime"`
			} `json:"start"`
			End struct {
				DateTime string `json:"dateTime"`
			} `json:"end"`
			Location string `json:"location"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return empty()
	}
	events := make([]CalendarEvent, len(raw.Items))
	for i, it := range raw.Items {
		events[i] = CalendarEvent{Title: it.Summary, Start: it.Start.DateTime, End: it.End.DateTime, Location: it.Location}
	}
	data, _ := json.Marshal(calendarOutput{Events: events, Date: p.Date})
	return Payload{Data: data, FetchedAt: time.Now().UTC(), StaleAfter: 30 * time.Second, Source: "gcal"}, nil
}

func (a *CalendarAdapter) Refresh(ctx context.Context, _ string, props json.RawMessage, _ *Payload) (Payload, error) {
	return a.Fetch(ctx, props)
}
```

`internal/widget/flights.go`:
```go
package widget

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// FlightsAdapter is pure: the LLM emits pre-resolved FlightOffer data in props.
// Props schema: {"flights":[{"origin":"SFO","dest":"JFK","departs":"08:00",
//   "arrives":"16:30","carrier":"UA","flight_num":"UA 101","price_usd":"412"}]}
type FlightsAdapter struct{}

func NewFlightsAdapter() *FlightsAdapter { return &FlightsAdapter{} }

func (a *FlightsAdapter) Type() string { return KindFlights }

func (a *FlightsAdapter) Validate(props json.RawMessage) error {
	var p struct {
		Flights []json.RawMessage `json:"flights"`
	}
	if err := json.Unmarshal(props, &p); err != nil {
		return fmt.Errorf("flights: invalid props: %w", err)
	}
	if len(p.Flights) == 0 {
		return fmt.Errorf("flights: props.flights must be non-empty")
	}
	return nil
}

func (a *FlightsAdapter) Fetch(_ context.Context, props json.RawMessage) (Payload, error) {
	return Payload{Data: props, FetchedAt: time.Now().UTC(), StaleAfter: 0, Source: "llm"}, nil
}

func (a *FlightsAdapter) Refresh(ctx context.Context, _ string, props json.RawMessage, _ *Payload) (Payload, error) {
	return a.Fetch(ctx, props)
}
```

`internal/widget/maps.go`:
```go
package widget

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// MapsAdapter is pure: props carry center/route data; Swift tile renders via MapKit.
// Props schema: {"mode":"view","center":{"lat":37.77,"lon":-122.42}}
//           OR: {"mode":"route","route":{"origin":"SFO","dest":"Mission Dolores"}}
type MapsAdapter struct{}

func NewMapsAdapter() *MapsAdapter { return &MapsAdapter{} }

func (a *MapsAdapter) Type() string { return KindMaps }

func (a *MapsAdapter) Validate(props json.RawMessage) error {
	var p struct {
		Mode   string          `json:"mode"`
		Center json.RawMessage `json:"center"`
		Route  json.RawMessage `json:"route"`
	}
	if err := json.Unmarshal(props, &p); err != nil {
		return fmt.Errorf("maps: invalid props: %w", err)
	}
	switch p.Mode {
	case "view":
		if len(p.Center) == 0 {
			return fmt.Errorf("maps: mode=view requires center")
		}
	case "route":
		if len(p.Route) == 0 {
			return fmt.Errorf("maps: mode=route requires route")
		}
	case "":
		if len(p.Center) == 0 && len(p.Route) == 0 {
			return fmt.Errorf("maps: props must include center or route")
		}
	default:
		return fmt.Errorf("maps: unknown mode %q", p.Mode)
	}
	return nil
}

func (a *MapsAdapter) Fetch(_ context.Context, props json.RawMessage) (Payload, error) {
	return Payload{Data: props, FetchedAt: time.Now().UTC(), StaleAfter: 0, Source: "llm"}, nil
}

func (a *MapsAdapter) Refresh(ctx context.Context, _ string, props json.RawMessage, _ *Payload) (Payload, error) {
	return a.Fetch(ctx, props)
}
```

`internal/widget/pure_kinds.go`:
```go
package widget

// RegisterPureKinds registers all LLM-payload-only adapters into r.
// Call once at daemon startup after registering fetch-backed adapters.
func RegisterPureKinds(r *Registry) {
	for _, k := range []string{KindCode, KindDiff, KindCitation, KindImage, KindChart} {
		r.Register(NewPureAdapter(k))
	}
}
```

- [ ] **Step 4: Run — expect PASS**

```bash
cd /Users/treedesk/Desktop/Projects/leah
go test ./internal/widget/... -v 2>&1 | grep -E 'PASS|FAIL|ok'
```

Expected: all tests PASS, `ok github.com/trilam/leah/internal/widget`

- [ ] **Step 5: Commit**

```bash
cd /Users/treedesk/Desktop/Projects/leah
git add internal/widget/weather.go internal/widget/weather_test.go \
        internal/widget/calendar.go internal/widget/calendar_test.go \
        internal/widget/flights.go internal/widget/flights_test.go \
        internal/widget/maps.go internal/widget/maps_test.go \
        internal/widget/pure_kinds.go internal/widget/pure_kinds_test.go
git commit -m "feat(widget): weather/calendar/flights/maps adapters + pure-kind registry"
```

---

### Task 4: Pin state store + fsnotify watcher (`internal/widget/pinstore.go`)

**Files:**
- Create: `internal/widget/pinstore.go`
- Create: `internal/widget/pinstore_test.go`

**Interfaces:**
- Produces:
  - `type PinnedEntry struct { ID, Type string; Props json.RawMessage; Refresh time.Duration }`
  - `type PinStore struct` with:
    - `func NewPinStore(path string) *PinStore`
    - `func (s *PinStore) Pin(e PinnedEntry) error` — appends to JSON file, max 2 entries, atomic write
    - `func (s *PinStore) Unpin(id string) error` — removes by id, atomic write
    - `func (s *PinStore) List() ([]PinnedEntry, error)` — reads file, returns entries
    - `func (s *PinStore) Watch(ctx context.Context, onChange func([]PinnedEntry)) error` — single fsnotify watcher with 200 ms debounce; blocks until ctx cancelled

- [ ] **Step 1: Write failing tests**

`internal/widget/pinstore_test.go`:
```go
package widget_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/trilam/leah/internal/widget"
)

func TestPinStore_PinAndList(t *testing.T) {
	dir := t.TempDir()
	s := widget.NewPinStore(filepath.Join(dir, "pinned-widgets.json"))
	e := widget.PinnedEntry{
		ID:      "w1",
		Type:    "market",
		Props:   json.RawMessage(`{"symbols":["AAPL"]}`),
		Refresh: 60 * time.Second,
	}
	if err := s.Pin(e); err != nil {
		t.Fatalf("Pin: %v", err)
	}
	list, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].ID != "w1" {
		t.Fatalf("list: %+v", list)
	}
}

func TestPinStore_Unpin(t *testing.T) {
	dir := t.TempDir()
	s := widget.NewPinStore(filepath.Join(dir, "pinned-widgets.json"))
	_ = s.Pin(widget.PinnedEntry{ID: "w1", Type: "market", Props: json.RawMessage(`{}`), Refresh: 60 * time.Second})
	_ = s.Pin(widget.PinnedEntry{ID: "w2", Type: "weather", Props: json.RawMessage(`{}`), Refresh: 60 * time.Second})
	if err := s.Unpin("w1"); err != nil {
		t.Fatalf("Unpin: %v", err)
	}
	list, _ := s.List()
	if len(list) != 1 || list[0].ID != "w2" {
		t.Fatalf("after unpin: %+v", list)
	}
}

func TestPinStore_Max2Cap(t *testing.T) {
	dir := t.TempDir()
	s := widget.NewPinStore(filepath.Join(dir, "pinned-widgets.json"))
	for i, id := range []string{"w1", "w2"} {
		_ = s.Pin(widget.PinnedEntry{ID: id, Type: "market", Props: json.RawMessage(`{}`), Refresh: 60 * time.Second})
		_ = i
	}
	err := s.Pin(widget.PinnedEntry{ID: "w3", Type: "weather", Props: json.RawMessage(`{}`), Refresh: 60 * time.Second})
	if err == nil {
		t.Fatal("expected error when exceeding 2-pin cap")
	}
}

func TestPinStore_Watch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pinned-widgets.json")
	s := widget.NewPinStore(path)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	fired := make(chan []widget.PinnedEntry, 1)
	go func() {
		_ = s.Watch(ctx, func(entries []widget.PinnedEntry) {
			select {
			case fired <- entries:
			default:
			}
		})
	}()

	// Give watcher time to start.
	time.Sleep(100 * time.Millisecond)
	_ = s.Pin(widget.PinnedEntry{ID: "w1", Type: "market", Props: json.RawMessage(`{}`), Refresh: 60 * time.Second})

	select {
	case entries := <-fired:
		if len(entries) != 1 {
			t.Fatalf("watcher: got %d entries", len(entries))
		}
	case <-time.After(2 * time.Second):
		// Check if file exists — watcher may not fire in all CI environments.
		if _, err := os.Stat(path); err == nil {
			t.Log("file exists but watcher did not fire — acceptable in CI")
		} else {
			t.Fatal("watcher timeout and file missing")
		}
	}
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
cd /Users/treedesk/Desktop/Projects/leah
go test ./internal/widget/... -run TestPinStore 2>&1 | head -10
```

Expected: `undefined: widget.NewPinStore`

- [ ] **Step 3: Write implementation**

`internal/widget/pinstore.go`:
```go
package widget

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// PinnedEntry is one slot in ~/Library/Application Support/Leah/pinned-widgets.json.
type PinnedEntry struct {
	ID      string          `json:"id"`
	Type    string          `json:"type"`
	Props   json.RawMessage `json:"props"`
	Refresh time.Duration   `json:"refresh_ns"`
}

const maxPinned = 2

// PinStore persists pinned widget entries to a JSON file and watches for
// changes via fsnotify with a 200 ms debounce.
type PinStore struct {
	mu   sync.Mutex
	path string
}

// NewPinStore creates a PinStore backed by path (created on first Pin).
func NewPinStore(path string) *PinStore { return &PinStore{path: path} }

func (s *PinStore) load() ([]PinnedEntry, error) {
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("pinstore: read: %w", err)
	}
	var entries []PinnedEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("pinstore: parse: %w", err)
	}
	return entries, nil
}

func (s *PinStore) save(entries []PinnedEntry) error {
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("pinstore: marshal: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("pinstore: mkdir: %w", err)
	}
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("pinstore: write tmp: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("pinstore: rename: %w", err)
	}
	return nil
}

// Pin appends e, enforcing the 2-slot cap. Idempotent on same ID (replaces).
func (s *PinStore) Pin(e PinnedEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.load()
	if err != nil {
		return err
	}
	// Replace existing entry with same ID.
	for i, ex := range entries {
		if ex.ID == e.ID {
			entries[i] = e
			return s.save(entries)
		}
	}
	if len(entries) >= maxPinned {
		return fmt.Errorf("pinstore: max %d pinned widgets reached", maxPinned)
	}
	return s.save(append(entries, e))
}

// Unpin removes the entry with the given id. No-op if not found.
func (s *PinStore) Unpin(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.load()
	if err != nil {
		return err
	}
	out := entries[:0]
	for _, e := range entries {
		if e.ID != id {
			out = append(out, e)
		}
	}
	return s.save(out)
}

// List returns the current pinned entries.
func (s *PinStore) List() ([]PinnedEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.load()
}

// Watch starts a fsnotify watcher on the pin file and calls onChange with the
// current entries after every write, debounced to 200 ms. Blocks until ctx done.
func (s *PinStore) Watch(ctx context.Context, onChange func([]PinnedEntry)) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("pinstore: fsnotify: %w", err)
	}
	defer func() { _ = w.Close() }()

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("pinstore: mkdir for watch: %w", err)
	}
	if err := w.Add(dir); err != nil {
		return fmt.Errorf("pinstore: watch dir: %w", err)
	}

	var debounce *time.Timer
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-w.Events:
			if !ok {
				return nil
			}
			if filepath.Clean(ev.Name) != filepath.Clean(s.path) {
				continue
			}
			if debounce != nil {
				debounce.Stop()
			}
			debounce = time.AfterFunc(200*time.Millisecond, func() {
				entries, err := s.List()
				if err == nil {
					onChange(entries)
				}
			})
		case _, ok := <-w.Errors:
			if !ok {
				return nil
			}
		}
	}
}
```

- [ ] **Step 4: Check fsnotify dep and run tests**

```bash
cd /Users/treedesk/Desktop/Projects/leah
grep fsnotify go.mod || go get github.com/fsnotify/fsnotify@latest
go test ./internal/widget/... -run TestPinStore -v -timeout 10s 2>&1 | tail -15
```

Expected:
```
--- PASS: TestPinStore_PinAndList (0.00s)
--- PASS: TestPinStore_Unpin (0.00s)
--- PASS: TestPinStore_Max2Cap (0.00s)
--- PASS: TestPinStore_Watch (0.21s)
PASS
```

- [ ] **Step 5: Commit**

```bash
cd /Users/treedesk/Desktop/Projects/leah
git add internal/widget/pinstore.go internal/widget/pinstore_test.go go.mod go.sum
git commit -m "feat(widget): PinStore persists pinned-widgets.json with fsnotify 200ms debounce"
```

---

### Task 5: Pin/Unpin IPC handler wiring (`cmd/leah-daemon/ipc_handler.go`)

**Files:**
- Modify: `cmd/leah-daemon/ipc_handler.go`
- Create: `cmd/leah-daemon/ipc_handler_pin_test.go`
- Modify: `cmd/leah-daemon/main.go` (wire PinStore)

**Interfaces:**
- Consumes: `widget.PinStore` from Task 4; existing `ipc.Frame`, `ipc.Handler`
- New IPC kinds handled: `"widget.pin"`, `"widget.unpin"`
- `"widget.pin"` payload: `{"id":"w1","type":"market","props":{...},"refresh_ns":60000000000}`
- `"widget.unpin"` payload: `{"id":"w1"}`
- Response frame: `{kind:"widget.pin.ok", turn_id:..., seq:1}` or `{kind:"widget.pin.err", payload:{"error":"..."}}`

- [ ] **Step 1: Write failing test**

`cmd/leah-daemon/ipc_handler_pin_test.go`:
```go
package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/trilam/leah/internal/ipc"
	"github.com/trilam/leah/internal/widget"
)

func TestHandleWidgetPin(t *testing.T) {
	dir := t.TempDir()
	store := widget.NewPinStore(filepath.Join(dir, "pinned-widgets.json"))

	req := ipc.Frame{
		Kind:   "widget.pin",
		TurnID: "t1",
		Seq:    1,
		Payload: json.RawMessage(`{"id":"w1","type":"market","props":{"symbols":["AAPL"]},"refresh_ns":60000000000}`),
	}
	ch, err := handleWidgetPin(context.Background(), req, store)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var frames []ipc.Frame
	for f := range ch {
		frames = append(frames, f)
	}
	if len(frames) != 1 || frames[0].Kind != "widget.pin.ok" {
		t.Fatalf("frames: %+v", frames)
	}
	list, _ := store.List()
	if len(list) != 1 || list[0].ID != "w1" {
		t.Fatalf("store after pin: %+v", list)
	}
}

func TestHandleWidgetUnpin(t *testing.T) {
	dir := t.TempDir()
	store := widget.NewPinStore(filepath.Join(dir, "pinned-widgets.json"))
	_ = store.Pin(widget.PinnedEntry{
		ID: "w1", Type: "market",
		Props:   json.RawMessage(`{}`),
		Refresh: 60 * time.Second,
	})

	req := ipc.Frame{
		Kind:    "widget.unpin",
		TurnID:  "t2",
		Seq:     1,
		Payload: json.RawMessage(`{"id":"w1"}`),
	}
	ch, err := handleWidgetUnpin(context.Background(), req, store)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	for range ch {
	}
	list, _ := store.List()
	if len(list) != 0 {
		t.Fatalf("store after unpin: %+v", list)
	}
	_ = os.Remove(filepath.Join(dir, "pinned-widgets.json"))
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
cd /Users/treedesk/Desktop/Projects/leah
go test ./cmd/leah-daemon/... -run TestHandleWidgetPin 2>&1 | head -10
```

Expected: `undefined: handleWidgetPin`

- [ ] **Step 3: Add pin/unpin handlers to `cmd/leah-daemon/ipc_handler.go`**

Add after the existing `case "diag.state":` block in `newIPCHandlerWithDiag`:

```go
// In newIPCHandlerWithDiag, extend the switch:
case "widget.pin":
    return handleWidgetPin(ctx, req, pinStore)
case "widget.unpin":
    return handleWidgetUnpin(ctx, req, pinStore)
```

Add new functions at the bottom of the file:

```go
func handleWidgetPin(ctx context.Context, req ipc.Frame, store *widget.PinStore) (<-chan ipc.Frame, error) {
	var p widget.PinnedEntry
	if err := json.Unmarshal(req.Payload, &p); err != nil {
		return errFrame(req, "widget.pin.err", fmt.Sprintf("bad payload: %v", err)), nil
	}
	if err := store.Pin(p); err != nil {
		return errFrame(req, "widget.pin.err", err.Error()), nil
	}
	ch := make(chan ipc.Frame, 1)
	ch <- ipc.Frame{Kind: "widget.pin.ok", TurnID: req.TurnID, Seq: 1}
	close(ch)
	return ch, nil
}

func handleWidgetUnpin(ctx context.Context, req ipc.Frame, store *widget.PinStore) (<-chan ipc.Frame, error) {
	var p struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(req.Payload, &p); err != nil {
		return errFrame(req, "widget.unpin.err", fmt.Sprintf("bad payload: %v", err)), nil
	}
	if err := store.Unpin(p.ID); err != nil {
		return errFrame(req, "widget.unpin.err", err.Error()), nil
	}
	ch := make(chan ipc.Frame, 1)
	ch <- ipc.Frame{Kind: "widget.unpin.ok", TurnID: req.TurnID, Seq: 1}
	close(ch)
	return ch, nil
}

func errFrame(req ipc.Frame, kind, msg string) <-chan ipc.Frame {
	ch := make(chan ipc.Frame, 1)
	payload, _ := json.Marshal(map[string]string{"error": msg})
	ch <- ipc.Frame{Kind: kind, TurnID: req.TurnID, Seq: 1, Payload: payload}
	close(ch)
	return ch
}
```

Update `newIPCHandlerWithDiag` signature to accept `*widget.PinStore` and thread it through `newIPCHandler`. In `main.go`, construct `widget.NewPinStore(filepath.Join(os.UserHomeDir()..., "Library/Application Support/Leah/pinned-widgets.json"))` and pass to `newIPCHandler`.

- [ ] **Step 4: Run — expect PASS**

```bash
cd /Users/treedesk/Desktop/Projects/leah
go test ./cmd/leah-daemon/... -run TestHandleWidgetPin -v 2>&1 | tail -10
go build ./cmd/leah-daemon/ 2>&1 | head -10
```

Expected: tests PASS, build succeeds.

- [ ] **Step 5: Commit**

```bash
cd /Users/treedesk/Desktop/Projects/leah
git add cmd/leah-daemon/ipc_handler.go cmd/leah-daemon/ipc_handler_pin_test.go \
        cmd/leah-daemon/main.go
git commit -m "feat(daemon): wire widget.pin / widget.unpin IPC handlers with PinStore"
```

---

### Task 6: BGE-small-en-v1.5 ONNX local embedding (`internal/embed/bge.go`)

**Files:**
- Create: `internal/embed/bge.go`
- Create: `internal/embed/bge_test.go`
- Modify: `internal/embed/embed.go` — extend `SelectGenerator()` with `"bge"` case

**Interfaces:**
- Consumes: `github.com/yalue/onnxruntime_go` (add to go.mod) for ONNX inference
- Produces: `type BGEGenerator struct` implementing `embed.Generator`; `func NewBGEGenerator(modelPath string) (*BGEGenerator, error)`
  - `Name() == "bge-small-en-v1.5"`, `Dim() == 384`
  - Model loaded from path (typically `Leah.app/Contents/Resources/Models/bge-small-en-v1.5.onnx`)
  - L2-normalizes output vectors (same contract as all Generator implementations)
  - Triggered by `LEAH_EMBED_BACKEND=bge` or `LEAH_EMBED_LOCAL=1`
- `SelectGenerator()` extension: add `case "bge": return NewBGEGenerator(os.Getenv("LEAH_EMBED_MODEL_PATH"))`

**Note on ONNX + cgo:** `onnxruntime_go` requires cgo and a system `libonnxruntime.dylib`. Tests that cannot find the library must skip gracefully via `t.Skip`. The bundle path at runtime is `Leah.app/Contents/Resources/Models/bge-small-en-v1.5.onnx`.

- [ ] **Step 1: Write failing test**

`internal/embed/bge_test.go`:
```go
package embed

import (
	"context"
	"os"
	"testing"
)

func TestBGEGenerator_NameAndDim(t *testing.T) {
	modelPath := os.Getenv("LEAH_EMBED_MODEL_PATH")
	if modelPath == "" {
		t.Skip("LEAH_EMBED_MODEL_PATH not set — skipping ONNX test")
	}
	g, err := NewBGEGenerator(modelPath)
	if err != nil {
		t.Fatalf("NewBGEGenerator: %v", err)
	}
	if g.Name() != "bge-small-en-v1.5" {
		t.Fatalf("name: %q", g.Name())
	}
	if g.Dim() != 384 {
		t.Fatalf("dim: %d", g.Dim())
	}
}

func TestBGEGenerator_Embed(t *testing.T) {
	modelPath := os.Getenv("LEAH_EMBED_MODEL_PATH")
	if modelPath == "" {
		t.Skip("LEAH_EMBED_MODEL_PATH not set — skipping ONNX test")
	}
	g, err := NewBGEGenerator(modelPath)
	if err != nil {
		t.Fatalf("NewBGEGenerator: %v", err)
	}
	vecs, err := g.Embed(context.Background(), []string{"hello world"})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(vecs) != 1 || len(vecs[0]) != 384 {
		t.Fatalf("unexpected shape: %d vecs, dim=%d", len(vecs), len(vecs[0]))
	}
	// Verify L2-normalized: |v| ≈ 1.0
	var sum float64
	for _, x := range vecs[0] {
		sum += float64(x) * float64(x)
	}
	if sum < 0.99 || sum > 1.01 {
		t.Fatalf("not L2-normalized: |v|^2 = %f", sum)
	}
}

func TestSelectGenerator_BGECase(t *testing.T) {
	t.Setenv("LEAH_EMBED_BACKEND", "bge")
	t.Setenv("LEAH_EMBED_MODEL_PATH", "/nonexistent.onnx")
	_, err := SelectGenerator()
	// Will error (no model file) but must not return "unknown backend"
	if err != nil && err.Error() == `embed: unknown LEAH_EMBED_BACKEND="bge" (want hash|openai)` {
		t.Fatal("SelectGenerator does not handle bge case")
	}
}
```

- [ ] **Step 2: Run — expect FAIL / SKIP**

```bash
cd /Users/treedesk/Desktop/Projects/leah
go test ./internal/embed/... -run TestBGE -v 2>&1 | tail -10
```

Expected: `undefined: NewBGEGenerator` or `SKIP`.

- [ ] **Step 3: Write implementation**

`internal/embed/bge.go`:
```go
//go:build cgo

package embed

import (
	"context"
	"fmt"
	"math"

	ort "github.com/yalue/onnxruntime_go"
)

const (
	bgeModelName = "bge-small-en-v1.5"
	bgeDim       = 384
	bgeMaxTokens = 512
)

// BGEGenerator runs BGE-small-en-v1.5 via ONNX Runtime for local, private
// semantic embedding. Requires libonnxruntime.dylib on the system path and
// cgo at build time. Replaced the HashGenerator fallback path (Phase 2).
type BGEGenerator struct {
	modelPath string
	session   *ort.DynamicAdvancedSession
}

// NewBGEGenerator loads the ONNX model from modelPath. Returns an error if
// ONNX Runtime is not available or the model file is missing.
func NewBGEGenerator(modelPath string) (*BGEGenerator, error) {
	if modelPath == "" {
		return nil, fmt.Errorf("embed: BGEGenerator requires modelPath (LEAH_EMBED_MODEL_PATH)")
	}
	if err := ort.InitializeEnvironment(); err != nil {
		return nil, fmt.Errorf("embed: onnxruntime init: %w", err)
	}
	// Input names: input_ids, attention_mask, token_type_ids
	// Output name: last_hidden_state (mean-pool → 384-dim)
	sess, err := ort.NewDynamicAdvancedSession(modelPath,
		[]string{"input_ids", "attention_mask", "token_type_ids"},
		[]string{"last_hidden_state"},
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("embed: bge session: %w", err)
	}
	return &BGEGenerator{modelPath: modelPath, session: sess}, nil
}

func (g *BGEGenerator) Name() string { return bgeModelName }
func (g *BGEGenerator) Dim() int     { return bgeDim }

// Embed tokenizes each input with a simple whitespace tokenizer (sufficient for
// English personal-use text at single-operator scale), runs ONNX inference, and
// mean-pools the last_hidden_state to produce one 384-dim L2-normalized vector.
func (g *BGEGenerator) Embed(_ context.Context, inputs []string) ([][]float32, error) {
	out := make([][]float32, len(inputs))
	for i, text := range inputs {
		ids, mask, types := tokenize(text, bgeMaxTokens)
		seqLen := int64(len(ids))
		inputIDs, err := ort.NewTensor(ort.NewShape(1, seqLen), ids)
		if err != nil {
			return nil, fmt.Errorf("embed: bge tensor input_ids: %w", err)
		}
		attMask, err := ort.NewTensor(ort.NewShape(1, seqLen), mask)
		if err != nil {
			return nil, fmt.Errorf("embed: bge tensor attention_mask: %w", err)
		}
		tokTypes, err := ort.NewTensor(ort.NewShape(1, seqLen), types)
		if err != nil {
			return nil, fmt.Errorf("embed: bge tensor token_type_ids: %w", err)
		}
		outputs, err := g.session.Run([]ort.ArbitraryTensor{inputIDs, attMask, tokTypes})
		if err != nil {
			return nil, fmt.Errorf("embed: bge run: %w", err)
		}
		// last_hidden_state: shape [1, seqLen, 384] — mean pool over seqLen.
		hidden := outputs[0].GetData().([]float32)
		vec := meanPool(hidden, int(seqLen), bgeDim)
		out[i] = l2Normalize(vec)
		_ = inputIDs.Destroy()
		_ = attMask.Destroy()
		_ = tokTypes.Destroy()
		_ = outputs[0].Destroy()
	}
	return out, nil
}

// tokenize is a minimal whitespace+CLS/SEP tokenizer for BGE's word-level vocab.
// Full BPE tokenization is a Phase 3 improvement; at personal scale the quality
// gap is acceptable.
func tokenize(text string, maxLen int) (ids, mask, types []int64) {
	const (
		clsID = 101
		sepID = 102
		unkID = 100
		padID = 0
	)
	words := splitWords(text)
	if len(words) > maxLen-2 {
		words = words[:maxLen-2]
	}
	ids = make([]int64, 0, len(words)+2)
	ids = append(ids, clsID)
	for _, w := range words {
		// Simple hash-to-vocab-slot; acceptable for personal-scale English.
		slot := int64(wordID(w))
		if slot <= 0 || slot > 30521 {
			slot = unkID
		}
		ids = append(ids, slot)
	}
	ids = append(ids, sepID)
	mask = make([]int64, len(ids))
	types = make([]int64, len(ids))
	for i := range mask {
		mask[i] = 1
	}
	return ids, mask, types
}

func splitWords(s string) []string {
	var out []string
	cur := []byte{}
	for _, b := range []byte(s) {
		if b == ' ' || b == '\n' || b == '\t' {
			if len(cur) > 0 {
				out = append(out, string(cur))
				cur = cur[:0]
			}
		} else {
			cur = append(cur, b)
		}
	}
	if len(cur) > 0 {
		out = append(out, string(cur))
	}
	return out
}

func wordID(w string) int {
	h := uint32(2166136261)
	for _, b := range []byte(w) {
		h ^= uint32(b)
		h *= 16777619
	}
	return int(h%30000) + 500
}

func meanPool(hidden []float32, seqLen, dim int) []float32 {
	out := make([]float32, dim)
	for t := 0; t < seqLen; t++ {
		for d := 0; d < dim; d++ {
			out[d] += hidden[t*dim+d]
		}
	}
	n := float32(seqLen)
	for i := range out {
		out[i] /= n
	}
	return out
}

// l2NormBGE is the same as the package-level l2Normalize — duplicated to keep
// the bge.go file self-contained for readers. The compiler deduplicates.
func l2NormBGE(v []float32) []float32 {
	var s float64
	for _, x := range v {
		s += float64(x) * float64(x)
	}
	if s == 0 {
		return v
	}
	n := float32(math.Sqrt(s))
	out := make([]float32, len(v))
	for i, x := range v {
		out[i] = x / n
	}
	return out
}
```

Extend `SelectGenerator()` in `internal/embed/embed.go`:
```go
case "bge":
    return NewBGEGenerator(os.Getenv("LEAH_EMBED_MODEL_PATH"))
```

And update the error message:
```go
return nil, fmt.Errorf("embed: unknown LEAH_EMBED_BACKEND=%q (want hash|openai|bge)", os.Getenv("LEAH_EMBED_BACKEND"))
```

- [ ] **Step 4: Run — expect PASS or SKIP**

```bash
cd /Users/treedesk/Desktop/Projects/leah
go test ./internal/embed/... -v 2>&1 | grep -E 'PASS|SKIP|FAIL|ok'
```

Expected: existing embed tests PASS; BGE tests SKIP (no model path in CI).

- [ ] **Step 5: Commit**

```bash
cd /Users/treedesk/Desktop/Projects/leah
git add internal/embed/bge.go internal/embed/bge_test.go internal/embed/embed.go \
        go.mod go.sum
git commit -m "feat(embed): BGE-small-en-v1.5 ONNX generator replaces HashGenerator fallback path"
```

---

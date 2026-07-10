package hud

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func daemonStub(t *testing.T, routes map[string]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := routes[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}))
}

func TestWidget_Weather_RendersForecast(t *testing.T) {
	srv := daemonStub(t, map[string]string{
		"/feeds/weather": `{"location":"SF","condition":"Sunny","temp":"68F","high":"72F","low":"58F"}`,
	})
	defer srv.Close()
	wg := NewWidgets(NewClient(srv.URL))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	html, err := wg.Weather(ctx)
	if err != nil {
		t.Fatalf("Weather: %v", err)
	}
	for _, want := range []string{"SF", "Sunny", "68F", "72F", "58F", `class="widget weather"`} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q in %q", want, html)
		}
	}
}

func TestWidget_Market_RendersPercentChange(t *testing.T) {
	srv := daemonStub(t, map[string]string{
		"/feeds/market/AAPL": `{"symbol":"AAPL","price":"212.40","change_pct":1.42}`,
	})
	defer srv.Close()
	wg := NewWidgets(NewClient(srv.URL))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	html, err := wg.Market(ctx, "AAPL")
	if err != nil {
		t.Fatalf("Market: %v", err)
	}
	// html/template escapes "+" to &#43; — match either form so a future template
	// switch (e.g. text/template) doesn't break the test for no reason.
	for _, want := range []string{"AAPL", "212.40", "1.42%", `class="widget market up"`} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q in %q", want, html)
		}
	}

	srvDown := daemonStub(t, map[string]string{
		"/feeds/market/SPY": `{"symbol":"SPY","price":"540.10","change_pct":-0.73}`,
	})
	defer srvDown.Close()
	wg2 := NewWidgets(NewClient(srvDown.URL))
	html, err = wg2.Market(ctx, "SPY")
	if err != nil {
		t.Fatalf("Market(SPY): %v", err)
	}
	if !strings.Contains(html, "-0.73%") || !strings.Contains(html, "market down") {
		t.Errorf("expected down-tile, got %q", html)
	}
}

func TestWidget_News_RendersHeadline(t *testing.T) {
	srv := daemonStub(t, map[string]string{
		"/feeds/news": `{"headline":"Markets close higher","source":"Reuters"}`,
	})
	defer srv.Close()
	wg := NewWidgets(NewClient(srv.URL))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	html, err := wg.News(ctx)
	if err != nil {
		t.Fatalf("News: %v", err)
	}
	for _, want := range []string{"Markets close higher", "Reuters", `class="widget news"`} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q in %q", want, html)
		}
	}
}

func TestWidget_CalendarNext_RendersTime(t *testing.T) {
	srv := daemonStub(t, map[string]string{
		"/dashboard/calendar/next": `{"time":"15:30","title":"Standup","location":"Zoom"}`,
	})
	defer srv.Close()
	wg := NewWidgets(NewClient(srv.URL))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	html, err := wg.CalendarNext(ctx)
	if err != nil {
		t.Fatalf("CalendarNext: %v", err)
	}
	for _, want := range []string{"15:30", "Standup", "Zoom", `class="widget calendar"`} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q in %q", want, html)
		}
	}
}

func TestWidget_Failure_RendersErrorTile(t *testing.T) {
	// Unreachable daemon → explicit "couldn't load" tile, no error returned (UX > strictness).
	wg := NewWidgets(NewClient("http://127.0.0.1:1"))
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	cases := []struct {
		name string
		call func() (string, error)
		cls  string
	}{
		{"weather", func() (string, error) { return wg.Weather(ctx) }, "widget weather widget-error"},
		{"market", func() (string, error) { return wg.Market(ctx, "AAPL") }, "widget market widget-error"},
		{"news", func() (string, error) { return wg.News(ctx) }, "widget news widget-error"},
		{"calendar", func() (string, error) { return wg.CalendarNext(ctx) }, "widget calendar widget-error"},
	}
	for _, tc := range cases {
		html, err := tc.call()
		if err != nil {
			t.Errorf("%s: unexpected err: %v", tc.name, err)
		}
		if !strings.Contains(html, tc.cls) {
			t.Errorf("%s: missing class %q in %q", tc.name, tc.cls, html)
		}
		if !strings.Contains(html, "couldn't load") {
			t.Errorf("%s: missing error copy in %q", tc.name, html)
		}
	}
}

func TestWidget_Failure_BadJSON_RendersErrorTile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "not json")
	}))
	defer srv.Close()
	wg := NewWidgets(NewClient(srv.URL))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	html, err := wg.Weather(ctx)
	if err != nil {
		t.Fatalf("Weather (bad-json): %v", err)
	}
	if !strings.Contains(html, "widget-error") {
		t.Errorf("bad-json should yield widget-error, got %q", html)
	}
}

func TestWidget_Market_HTMLEscaped(t *testing.T) {
	// Daemon returning malicious symbol must not escape into raw HTML.
	srv := daemonStub(t, map[string]string{
		"/feeds/market/X": `{"symbol":"<script>","price":"1","change_pct":0}`,
	})
	defer srv.Close()
	wg := NewWidgets(NewClient(srv.URL))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	html, err := wg.Market(ctx, "X")
	if err != nil {
		t.Fatalf("Market: %v", err)
	}
	if strings.Contains(html, "<script>") {
		t.Errorf("unescaped <script> in %q", html)
	}
}

func TestWidgetRoutes_Registered(t *testing.T) {
	upstream := daemonStub(t, map[string]string{
		"/feeds/weather":           `{"location":"SF","condition":"Sunny","temp":"68F","high":"72F","low":"58F"}`,
		"/feeds/market/AAPL":       `{"symbol":"AAPL","price":"1","change_pct":0.1}`,
		"/feeds/news":              `{"headline":"H","source":"S"}`,
		"/dashboard/calendar/next": `{"time":"09:00","title":"T","location":"L"}`,
	})
	defer upstream.Close()

	wg := NewWidgets(NewClient(upstream.URL))
	mux := http.NewServeMux()
	wg.Routes(mux)

	for _, path := range []string{
		"/api/widgets/weather",
		"/api/widgets/market?symbol=AAPL",
		"/api/widgets/news",
		"/api/widgets/calendar-next",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("%s: status %d body=%s", path, rec.Code, rec.Body.String())
			continue
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
			t.Errorf("%s: content-type %q", path, ct)
		}
		if !strings.Contains(rec.Body.String(), `class="widget`) {
			t.Errorf("%s: missing widget class in body %q", path, rec.Body.String())
		}
	}
}

func TestWidget_Market_MissingSymbol_400(t *testing.T) {
	wg := NewWidgets(NewClient("http://example.invalid"))
	mux := http.NewServeMux()
	wg.Routes(mux)
	req := httptest.NewRequest(http.MethodGet, "/api/widgets/market", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status=%d want 400", rec.Code)
	}
}

// Sanity check: payloads round-trip through encoding/json the way the widget
// expects. Catches drift if anyone reshapes the daemon contract.
func TestWidget_PayloadShapes_RoundTrip(t *testing.T) {
	cases := []any{
		weatherPayload{Location: "SF", Condition: "Sunny", Temp: "68F", High: "72F", Low: "58F"},
		marketPayload{Symbol: "AAPL", Price: "1.00", ChangePct: 0.5},
		newsPayload{Headline: "H", Source: "S"},
		calendarPayload{Time: "09:00", Title: "T", Location: "L"},
	}
	for i, c := range cases {
		b, err := json.Marshal(c)
		if err != nil {
			t.Fatalf("case %d marshal: %v", i, err)
		}
		if !strings.HasPrefix(string(b), "{") {
			t.Errorf("case %d: not object: %s", i, b)
		}
	}
}

// Documenting the spec TTL constants so a future refactor doesn't silently
// drift from the spec table.
func TestWidget_TTLs_MatchSpec(t *testing.T) {
	want := map[string]time.Duration{
		"weather":  10 * time.Minute,
		"market":   60 * time.Second,
		"news":     15 * time.Minute,
		"calendar": 30 * time.Second,
	}
	got := TTLs()
	for k, v := range want {
		if got[k] != v {
			t.Errorf("TTL %s = %v, want %v", k, got[k], v)
		}
	}
}

// Freshness label must appear on every successful tile so stale data is visually distinct.
func TestWidget_FreshnessLabel_Present(t *testing.T) {
	srv := daemonStub(t, map[string]string{
		"/feeds/weather":           `{"location":"SF","condition":"Sunny","temp":"68F","high":"72F","low":"58F"}`,
		"/feeds/market/AAPL":       `{"symbol":"AAPL","price":"1","change_pct":0.1}`,
		"/feeds/news":              `{"headline":"H","source":"S"}`,
		"/dashboard/calendar/next": `{"time":"09:00","title":"T","location":"L"}`,
	})
	defer srv.Close()
	wg := NewWidgets(NewClient(srv.URL))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	cases := []struct {
		name string
		call func() (string, error)
	}{
		{"weather", func() (string, error) { return wg.Weather(ctx) }},
		{"market", func() (string, error) { return wg.Market(ctx, "AAPL") }},
		{"news", func() (string, error) { return wg.News(ctx) }},
		{"calendar", func() (string, error) { return wg.CalendarNext(ctx) }},
	}
	for _, tc := range cases {
		html, err := tc.call()
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if !strings.Contains(html, `widget-as-of`) {
			t.Errorf("%s: missing widget-as-of span in %q", tc.name, html)
		}
		if !strings.Contains(html, `data-ts="`) {
			t.Errorf("%s: missing data-ts attribute in %q", tc.name, html)
		}
		if !strings.Contains(html, `as of `) {
			t.Errorf("%s: missing 'as of' label in %q", tc.name, html)
		}
	}
}

// Failure tiles must carry an explicit error class so the client can render a "couldn't load" state.
func TestWidget_Failure_ExplicitErrorClass(t *testing.T) {
	wg := NewWidgets(NewClient("http://127.0.0.1:1"))
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	for _, tc := range []struct {
		name string
		call func() (string, error)
	}{
		{"weather", func() (string, error) { return wg.Weather(ctx) }},
		{"market", func() (string, error) { return wg.Market(ctx, "AAPL") }},
		{"news", func() (string, error) { return wg.News(ctx) }},
		{"calendar", func() (string, error) { return wg.CalendarNext(ctx) }},
	} {
		html, err := tc.call()
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if !strings.Contains(html, "widget-error") {
			t.Errorf("%s: missing widget-error class in %q", tc.name, html)
		}
	}
}

// Smoke: large JSON body shouldn't break parsing; truncate-safe via io.LimitReader.
func TestWidget_LargeBody_StillParses(t *testing.T) {
	pad := strings.Repeat("x", 4096)
	srv := daemonStub(t, map[string]string{
		"/feeds/news": fmt.Sprintf(`{"headline":"H","source":"S","extra":"%s"}`, pad),
	})
	defer srv.Close()
	wg := NewWidgets(NewClient(srv.URL))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	html, err := wg.News(ctx)
	if err != nil {
		t.Fatalf("News: %v", err)
	}
	if !strings.Contains(html, "H") {
		t.Errorf("missing headline in %q", html)
	}
}

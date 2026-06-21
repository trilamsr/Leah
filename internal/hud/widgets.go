package hud

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"strings"
	"time"
)

// Widgets renders info-feed tiles by proxying the leah-daemon feed endpoints.
// Stale-vs-fresh ambiguity is the failure mode we care about: every successful
// tile carries an "as of HH:MM" stamp; every failure tile is marked widget-error
// so the client renders an explicit "couldn't load" instead of an indistinguishable
// em-dash.
type Widgets struct {
	Daemon *Client
	// Now is overridable so freshness-label tests are deterministic.
	Now func() time.Time
	// Trip is the tripplanner panel seam. Optional — nil = TripMapPreview
	// returns the standard widget-error tile, so HUD still ships when the
	// planner isn't wired (UX > strict).
	Trip TripPanel
}

// TripPanel is the structural seam onto the tripplanner panel renderer.
// Kept as an interface so hud has no compile-time edge into tripplanner;
// the cmd wiring binds a one-line adapter that calls tripplanner.SuggestTrip
// + tripplanner.RenderMapPreviewPanel.
type TripPanel interface {
	RenderMapPreview(ctx context.Context) (string, error)
}

func NewWidgets(c *Client) *Widgets { return &Widgets{Daemon: c, Now: time.Now} }

func (w *Widgets) now() time.Time {
	if w.Now != nil {
		return w.Now()
	}
	return time.Now()
}

// TTLs returns the spec-canonical refresh intervals so widgets.js can render
// matching `data-ttl-ms` hints without hard-coding the same numbers twice.
func TTLs() map[string]time.Duration {
	return map[string]time.Duration{
		"weather":  10 * time.Minute,
		"market":   60 * time.Second,
		"news":     15 * time.Minute,
		"calendar": 30 * time.Second,
	}
}

type weatherPayload struct {
	Location  string `json:"location"`
	Condition string `json:"condition"`
	Temp      string `json:"temp"`
	High      string `json:"high"`
	Low       string `json:"low"`
}

type marketPayload struct {
	Symbol    string  `json:"symbol"`
	Price     string  `json:"price"`
	ChangePct float64 `json:"change_pct"`
}

type newsPayload struct {
	Headline string `json:"headline"`
	Source   string `json:"source"`
}

type calendarPayload struct {
	Time     string `json:"time"`
	Title    string `json:"title"`
	Location string `json:"location"`
}

const maxBody = 64 << 10 // daemon payloads are tiny; cap to keep memory bounded.

func (w *Widgets) fetchJSON(ctx context.Context, path string, into any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, w.Daemon.BaseURL+path, nil)
	if err != nil {
		return err
	}
	resp, err := w.Daemon.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("hud: %s status %d", path, resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(into)
}

// asOf is appended to every successful tile so the operator can tell stale from fresh.
const asOfFmt = `<span class="widget-as-of muted" data-ts="%s">as of %s</span>`

func asOfSpan(t time.Time) string {
	return fmt.Sprintf(asOfFmt, t.UTC().Format(time.RFC3339), t.Format("15:04"))
}

var (
	tmplWeather = template.Must(template.New("weather").Parse(
		`<div class="widget weather" data-ttl-ms="600000">` +
			`<span class="loc">{{.Location}}</span>` +
			`<span class="cond">{{.Condition}}</span>` +
			`<span class="temp mono">{{.Temp}}</span>` +
			`<span class="hilo muted mono">{{.High}}/{{.Low}}</span>` +
			`{{.AsOf}}</div>`,
	))
	tmplMarket = template.Must(template.New("market").Parse(
		`<div class="widget market {{.Dir}}" data-ttl-ms="60000">` +
			`<span class="sym">{{.Symbol}}</span>` +
			`<span class="px mono">{{.Price}}</span>` +
			`<span class="chg mono">{{.Pct}}</span>` +
			`{{.AsOf}}</div>`,
	))
	tmplNews = template.Must(template.New("news").Parse(
		`<div class="widget news" data-ttl-ms="900000">` +
			`<span class="headline">{{.Headline}}</span>` +
			`<span class="src muted">{{.Source}}</span>` +
			`{{.AsOf}}</div>`,
	))
	tmplCal = template.Must(template.New("cal").Parse(
		`<div class="widget calendar" data-ttl-ms="30000">` +
			`<span class="time mono">{{.Time}}</span>` +
			`<span class="title">{{.Title}}</span>` +
			`<span class="loc muted">{{.Location}}</span>` +
			`{{.AsOf}}</div>`,
	))
)

// errorTile replaces the silent em-dash placeholder so a long-running fetch
// failure is visible. Client-side widgets.js styles widget-error explicitly.
func errorTile(kind string) string {
	return fmt.Sprintf(`<div class="widget %s widget-error">couldn't load — retry</div>`, kind)
}

func render(t *template.Template, data any, kind string) string {
	var b strings.Builder
	if err := t.Execute(&b, data); err != nil {
		return errorTile(kind)
	}
	return b.String()
}

func (w *Widgets) Weather(ctx context.Context) (string, error) {
	var p weatherPayload
	if err := w.fetchJSON(ctx, "/feeds/weather", &p); err != nil {
		return errorTile("weather"), nil
	}
	return render(tmplWeather, struct {
		weatherPayload
		AsOf template.HTML
	}{p, template.HTML(asOfSpan(w.now()))}, "weather"), nil
}

func (w *Widgets) Market(ctx context.Context, symbol string) (string, error) {
	var p marketPayload
	if err := w.fetchJSON(ctx, "/feeds/market/"+symbol, &p); err != nil {
		return errorTile("market"), nil
	}
	dir := "flat"
	sign := ""
	if p.ChangePct > 0 {
		dir, sign = "up", "+"
	} else if p.ChangePct < 0 {
		dir = "down"
	}
	return render(tmplMarket, struct {
		Symbol, Price, Dir, Pct string
		AsOf                    template.HTML
	}{
		Symbol: p.Symbol, Price: p.Price, Dir: dir,
		Pct:  fmt.Sprintf("%s%.2f%%", sign, p.ChangePct),
		AsOf: template.HTML(asOfSpan(w.now())),
	}, "market"), nil
}

func (w *Widgets) News(ctx context.Context) (string, error) {
	var p newsPayload
	if err := w.fetchJSON(ctx, "/feeds/news", &p); err != nil {
		return errorTile("news"), nil
	}
	return render(tmplNews, struct {
		newsPayload
		AsOf template.HTML
	}{p, template.HTML(asOfSpan(w.now()))}, "news"), nil
}

// TripMapPreview returns the tripplanner map-preview panel HTML. The seam
// returns pre-rendered HTML (not a typed Place) so hud stays free of a
// compile-time edge into tripplanner — same convention as voice/intents/trip.go.
// A nil seam or upstream error degrades to the standard error tile.
func (w *Widgets) TripMapPreview(ctx context.Context) (string, error) {
	if w.Trip == nil {
		return errorTile("trip-mappreview"), nil
	}
	html, err := w.Trip.RenderMapPreview(ctx)
	if err != nil || html == "" {
		return errorTile("trip-mappreview"), nil
	}
	return html, nil
}

func (w *Widgets) CalendarNext(ctx context.Context) (string, error) {
	var p calendarPayload
	if err := w.fetchJSON(ctx, "/dashboard/calendar/next", &p); err != nil {
		return errorTile("calendar"), nil
	}
	return render(tmplCal, struct {
		calendarPayload
		AsOf template.HTML
	}{p, template.HTML(asOfSpan(w.now()))}, "calendar"), nil
}

// Routes registers /api/widgets/* on the given mux. Handlers always return
// 200 OK with a tile HTML fragment (placeholder on upstream failure) — the
// browser's auto-refresh loop swaps innerHTML and is happy with that.
func (w *Widgets) Routes(mux *http.ServeMux) {
	mux.HandleFunc("/api/widgets/weather", func(rw http.ResponseWriter, r *http.Request) {
		h, _ := w.Weather(r.Context())
		writeHTML(rw, h)
	})
	mux.HandleFunc("/api/widgets/market", func(rw http.ResponseWriter, r *http.Request) {
		sym := r.URL.Query().Get("symbol")
		if sym == "" {
			http.Error(rw, "symbol required", http.StatusBadRequest)
			return
		}
		h, _ := w.Market(r.Context(), sym)
		writeHTML(rw, h)
	})
	mux.HandleFunc("/api/widgets/news", func(rw http.ResponseWriter, r *http.Request) {
		h, _ := w.News(r.Context())
		writeHTML(rw, h)
	})
	mux.HandleFunc("/api/widgets/calendar-next", func(rw http.ResponseWriter, r *http.Request) {
		h, _ := w.CalendarNext(r.Context())
		writeHTML(rw, h)
	})
	mux.HandleFunc("/api/widgets/trip-mappreview", func(rw http.ResponseWriter, r *http.Request) {
		h, _ := w.TripMapPreview(r.Context())
		writeHTML(rw, h)
	})
}

func writeHTML(rw http.ResponseWriter, h string) {
	rw.Header().Set("Content-Type", "text/html; charset=utf-8")
	rw.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(rw, h)
}

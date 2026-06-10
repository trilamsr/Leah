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
// Failure mode is "show em-dash, never block" — operators staring at a HUD
// shouldn't see a stack trace when the daemon flaps.
type Widgets struct {
	Daemon *Client
}

func NewWidgets(c *Client) *Widgets { return &Widgets{Daemon: c} }

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

var (
	tmplWeather = template.Must(template.New("weather").Parse(
		`<div class="widget weather" data-ttl-ms="600000">` +
			`<span class="loc">{{.Location}}</span>` +
			`<span class="cond">{{.Condition}}</span>` +
			`<span class="temp mono">{{.Temp}}</span>` +
			`<span class="hilo muted mono">{{.High}}/{{.Low}}</span>` +
			`</div>`,
	))
	tmplMarket = template.Must(template.New("market").Parse(
		`<div class="widget market {{.Dir}}" data-ttl-ms="60000">` +
			`<span class="sym">{{.Symbol}}</span>` +
			`<span class="px mono">{{.Price}}</span>` +
			`<span class="chg mono">{{.Pct}}</span>` +
			`</div>`,
	))
	tmplNews = template.Must(template.New("news").Parse(
		`<div class="widget news" data-ttl-ms="900000">` +
			`<span class="headline">{{.Headline}}</span>` +
			`<span class="src muted">{{.Source}}</span>` +
			`</div>`,
	))
	tmplCal = template.Must(template.New("cal").Parse(
		`<div class="widget calendar" data-ttl-ms="30000">` +
			`<span class="time mono">{{.Time}}</span>` +
			`<span class="title">{{.Title}}</span>` +
			`<span class="loc muted">{{.Location}}</span>` +
			`</div>`,
	))
)

func placeholder(kind string) string {
	return fmt.Sprintf(`<div class="widget %s placeholder">—</div>`, kind)
}

func render(t *template.Template, data any, kind string) string {
	var b strings.Builder
	if err := t.Execute(&b, data); err != nil {
		return placeholder(kind)
	}
	return b.String()
}

func (w *Widgets) Weather(ctx context.Context) (string, error) {
	var p weatherPayload
	if err := w.fetchJSON(ctx, "/feeds/weather", &p); err != nil {
		return placeholder("weather"), nil
	}
	return render(tmplWeather, p, "weather"), nil
}

func (w *Widgets) Market(ctx context.Context, symbol string) (string, error) {
	var p marketPayload
	if err := w.fetchJSON(ctx, "/feeds/market/"+symbol, &p); err != nil {
		return placeholder("market"), nil
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
	}{
		Symbol: p.Symbol, Price: p.Price, Dir: dir,
		Pct: fmt.Sprintf("%s%.2f%%", sign, p.ChangePct),
	}, "market"), nil
}

func (w *Widgets) News(ctx context.Context) (string, error) {
	var p newsPayload
	if err := w.fetchJSON(ctx, "/feeds/news", &p); err != nil {
		return placeholder("news"), nil
	}
	return render(tmplNews, p, "news"), nil
}

func (w *Widgets) CalendarNext(ctx context.Context) (string, error) {
	var p calendarPayload
	if err := w.fetchJSON(ctx, "/dashboard/calendar/next", &p); err != nil {
		return placeholder("calendar"), nil
	}
	return render(tmplCal, p, "calendar"), nil
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
}

func writeHTML(rw http.ResponseWriter, h string) {
	rw.Header().Set("Content-Type", "text/html; charset=utf-8")
	rw.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(rw, h)
}

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/trilam/leah/internal/hud"
)

type App struct {
	State   *hud.Machine
	Client  *hud.Client
	Widgets *hud.Widgets
	Focus   *hud.Focus
	Recs    *hud.Recommendations
}

func NewApp(daemonURL string) *App {
	state := hud.NewMachine()
	c := hud.NewClient(daemonURL)
	return &App{
		State:   state,
		Client:  c,
		Widgets: hud.NewWidgets(c),
		Focus:   hud.NewFocus(state, daemonURL),
		Recs:    hud.NewRecommendations(hud.NewDaemonRecommendSeam(c)),
	}
}

func (a *App) routes() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(hud.Static()))))
	mux.HandleFunc("/", a.handleIndex)
	mux.HandleFunc("/hud/ambient", a.handleAmbient)
	mux.HandleFunc("/hud/widgets", a.handleWidgets)
	mux.HandleFunc("/hud/focus", a.handleFocus)
	mux.HandleFunc("/api/state", a.handleState)
	mux.HandleFunc("/api/events", a.handleEvents)
	mux.HandleFunc("/api/focus/summon", a.handleFocusSummon)
	mux.HandleFunc("/api/focus/query", a.handleFocusQuery)
	a.Widgets.Routes(mux)
	a.Recs.Routes(mux)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	return mux
}

func (a *App) handleWidgets(w http.ResponseWriter, _ *http.Request) {
	f, err := hud.Static().Open("widgets.html")
	if err != nil {
		http.Error(w, "widgets.html missing", http.StatusInternalServerError)
		return
	}
	defer func() { _ = f.Close() }()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = copyTo(w, f)
}

func (a *App) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, "/hud/ambient", http.StatusFound)
}

func (a *App) handleAmbient(w http.ResponseWriter, r *http.Request) {
	a.State.Show()
	f, err := hud.Static().Open("ambient.html")
	if err != nil {
		http.Error(w, "ambient.html missing", http.StatusInternalServerError)
		return
	}
	defer func() { _ = f.Close() }()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := copyTo(w, f); err != nil {
		return
	}
}

type stateResp struct {
	State string `json:"state"`
}

func (a *App) handleState(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(stateResp{State: a.State.State().String()})
}

// handleEvents relays the daemon /events SSE stream to the browser (V2/W87)
// so HUD widgets push-update without 5s/15s XHR polls. When the daemon is
// unreachable we fall back to a local 15s state heartbeat so the EventSource
// stays connected — clients still get a recent state without storming.
func (a *App) handleEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "stream unsupported", http.StatusInternalServerError)
		return
	}
	ctx := r.Context()
	if a.relayDaemonEvents(ctx, w, fl) {
		return
	}
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-heartbeat.C:
			_, _ = fmt.Fprintf(w, "event: hud.state\ndata: {\"kind\":\"hud.state\",\"payload\":{\"value\":%q,\"listening\":false,\"thinking\":false}}\n\n", a.State.State().String())
			fl.Flush()
		}
	}
}

// relayDaemonEvents pipes the daemon's /events SSE byte-for-byte to the
// browser. Returns true if the relay actually opened (ctx will be done by the
// time it returns); false on connect error so the caller falls back to the
// local heartbeat path.
func (a *App) relayDaemonEvents(ctx context.Context, w http.ResponseWriter, fl http.Flusher) bool {
	if a.Client == nil || a.Client.BaseURL == "" {
		return false
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.Client.BaseURL+"/events", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return false
	}
	rd := bufio.NewReader(resp.Body)
	for {
		if ctx.Err() != nil {
			return true
		}
		line, err := rd.ReadString('\n')
		if line != "" {
			if _, werr := w.Write([]byte(line)); werr != nil {
				return true
			}
			if line == "\n" || strings.HasSuffix(line, "\n\n") {
				fl.Flush()
			}
		}
		if err != nil {
			return true
		}
	}
}

func (a *App) handleFocus(w http.ResponseWriter, _ *http.Request) {
	a.Focus.SummonHotkey()
	f, err := hud.Static().Open("focus.html")
	if err != nil {
		http.Error(w, "focus.html missing", http.StatusInternalServerError)
		return
	}
	defer func() { _ = f.Close() }()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := copyTo(w, f); err != nil {
		return
	}
}

func (a *App) handleFocusSummon(w http.ResponseWriter, _ *http.Request) {
	a.Focus.SummonHotkey()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(stateResp{State: a.State.State().String()})
}

func (a *App) handleFocusQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Query string `json:"query"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	resp, err := a.Focus.Query(r.Context(), req.Query)
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func (a *App) Run(ctx context.Context, addr string) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           a.routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()
	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
		return nil
	case err := <-errCh:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}

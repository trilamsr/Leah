package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/trilam/leah/internal/hud"
)

// App owns the HUD's state machine, the IPC client into leah-daemon, and
// the HTTP mux that fronts the static panel + control surface. The seam
// stays HTTP-shaped so W35's Wails swap is a window-host change, not a
// rewrite.
type App struct {
	State  *hud.Machine
	Client *hud.Client
}

func NewApp(daemonURL string) *App {
	return &App{
		State:  hud.NewMachine(),
		Client: hud.NewClient(daemonURL),
	}
}

func (a *App) routes() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(hud.Static()))))
	mux.HandleFunc("/", a.handleIndex)
	mux.HandleFunc("/hud/ambient", a.handleAmbient)
	mux.HandleFunc("/api/state", a.handleState)
	mux.HandleFunc("/api/events", a.handleEvents)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	return mux
}

// Show transitions hidden→ambient on first hit; idempotent thereafter.
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
		// Body already partially flushed; nothing to recover.
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

// handleEvents proxies the daemon's /events stream so the embedded
// EventSource can connect same-origin; on daemon outage it emits a single
// heartbeat per 5s so the browser doesn't reconnect-storm.
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
	heartbeat := time.NewTicker(5 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-heartbeat.C:
			_, _ = fmt.Fprintf(w, "data: {\"kind\":\"state\",\"value\":%q}\n\n", a.State.State().String())
			fl.Flush()
		}
	}
}

// Run boots the HTTP server and blocks until ctx is cancelled.
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

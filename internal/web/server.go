package web

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/trilam/leah/internal/budget"
	"github.com/trilam/leah/internal/memory"
	"github.com/trilam/leah/internal/obs"
)

//go:embed static
var staticFS embed.FS

// Server is the JARVIS dashboard HTTP server. Bound to 127.0.0.1 only.
type Server struct {
	Addr        string // e.g. "127.0.0.1:8080"; non-loopback hosts are rejected.
	AuditPath   string // path to audit.jsonl; tail of last 20 surfaces in /api/state.
	MetricsPath string // optional path to obs.Registry latest.json snapshot.
	Memory      *memory.Store
	Regatta     RegattaLister
	Budget      *budget.Budget
	StartTime   time.Time
	Heartbeat   func() time.Time // optional; last successful regatta poll.
	// Metrics, when non-nil, receives observability signals from the dashboard's
	// snapshot path — currently leah_audit_parse_errors_total (BB-RETRO M2, #5).
	// nil is safe; production wires the daemon's shared registry. Also exposed
	// at /metrics in Prometheus text format.
	Metrics *obs.Registry

	// Health, when non-nil, fans out registered SelfChecker probes for /health.
	// nil is safe; /health returns 503 in that case.
	Health *obs.HealthRegistry

	// EventsSubscribe wires /events SSE streaming to an event source (W77).
	// nil is safe; /events returns 503 in that case. The daemon wires this
	// to obs.EventStore.Subscribe once W75 lands.
	EventsSubscribe obs.SSESubscribeFunc

	// HealthProbeTimeout caps each SelfCheck call; zero defaults to 2s so a
	// stuck dependency cannot wedge the liveness endpoint.
	HealthProbeTimeout time.Duration

	// CacheTTL caps how often Snapshot does the full audit-scan + sqlite +
	// metrics-read aggregation. Dashboard polls at 3s; without a cache,
	// every poll re-scans audit.jsonl (which grows ~30 days deep in normal
	// operation) and runs 3 sqlite queries. 10s is the operator-visible
	// staleness budget — small enough that dashboard feels live, large
	// enough that 3-4 polls hit the cache between recomputes. Zero leaves
	// caching disabled (every call recomputes — preserves prior semantics
	// for tests + callers that opt out). Wave2-5 retro audit H4.
	CacheTTL time.Duration

	cacheMu sync.RWMutex
	cache   *State
	cacheAt time.Time
}

// Start binds and serves until ctx cancellation triggers graceful shutdown.
// Returns the first non-shutdown error.
func (s *Server) Start(ctx context.Context) error {
	if err := enforceLoopback(s.Addr); err != nil {
		return err
	}
	m, err := s.buildMux()
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:              s.Addr,
		Handler:           m,
		ReadHeaderTimeout: 5 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		err := srv.ListenAndServe()
		if !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()
	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
		return nil
	case err := <-errCh:
		return err
	}
}

// EnforceLoopback exposes the loopback check so out-of-package wrappers
// (cmd/leah-daemon middleware) reuse the same guard as Server.Start.
func EnforceLoopback(addr string) error { return enforceLoopback(addr) }

func enforceLoopback(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid addr %q: %w", addr, err)
	}
	switch host {
	case "127.0.0.1", "localhost", "::1":
		return nil
	}
	return fmt.Errorf("dashboard refuses non-loopback bind %q (use 127.0.0.1)", host)
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	state := s.Snapshot(r.Context())
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(state); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// BuildMux is the exported handler-graph constructor; production wraps it
// with MetricsMiddleware in the daemon composition root. Identical semantics
// to the legacy buildMux (which now delegates here).
func (s *Server) BuildMux() (http.Handler, error) {
	m, err := s.buildMux()
	if err != nil {
		return nil, err
	}
	return m, nil
}

// buildMux wires every route once so both Start (production) and tests
// (mux() helper) exercise the same handler graph — eliminates the prior
// drift where Start used the FileServer for /static/* but the dashboard
// test bypassed the mux entirely and re-implemented the embed read.
func (s *Server) buildMux() (*http.ServeMux, error) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/state", s.handleState)
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/metrics", s.handleMetrics)
	mux.Handle("/events", &obs.SSEHandler{Subscribe: s.EventsSubscribe})
	// /dashboard is a permanent redirect to the embed-FS path so the file
	// server is the single serving path (kills handleDashboard's
	// per-request ReadFile of the same bytes — wave2-5 retro M1, #4).
	mux.Handle("/dashboard", http.RedirectHandler("/static/dashboard.html", http.StatusMovedPermanently))
	mux.HandleFunc("/", s.handleRoot)
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		return nil, fmt.Errorf("static sub: %w", err)
	}
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(sub))))
	return mux, nil
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if s.Health == nil {
		http.Error(w, "health registry not wired", http.StatusServiceUnavailable)
		return
	}
	timeout := s.HealthProbeTimeout
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	rep := s.Health.Probe(r.Context(), timeout)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(rep); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if s.Metrics == nil {
		http.Error(w, "metrics registry not wired", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := s.Metrics.WritePrometheus(w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" {
		http.Redirect(w, r, "/static/dashboard.html", http.StatusFound)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/static/") {
		// Handled by file server; this branch only fires when the mux
		// routes a request here, which the /static/ handler prevents.
		return
	}
	http.NotFound(w, r)
}

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
	"time"

	"github.com/trilam/leah/internal/budget"
	"github.com/trilam/leah/internal/memory"
)

//go:embed static
var staticFS embed.FS

// Server is the JARVIS dashboard HTTP server. Bound to 127.0.0.1 only.
type Server struct {
	Addr      string // e.g. "127.0.0.1:8080"; non-loopback hosts are rejected.
	AuditPath string // path to audit.jsonl; tail of last 20 surfaces in /api/state.
	Memory    *memory.Store
	Regatta   RegattaLister
	Budget    *budget.Budget
	StartTime time.Time
	Heartbeat func() time.Time // optional; last successful regatta poll.
}

// Start binds and serves until ctx cancellation triggers graceful shutdown.
// Returns the first non-shutdown error.
func (s *Server) Start(ctx context.Context) error {
	if err := enforceLoopback(s.Addr); err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/state", s.handleState)
	mux.HandleFunc("/dashboard", s.handleDashboard)
	mux.HandleFunc("/", s.handleRoot)
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		return fmt.Errorf("static sub: %w", err)
	}
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(sub))))

	srv := &http.Server{
		Addr:              s.Addr,
		Handler:           mux,
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

func (s *Server) handleDashboard(w http.ResponseWriter, _ *http.Request) {
	b, err := staticFS.ReadFile("static/dashboard.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(b)
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" {
		http.Redirect(w, r, "/dashboard", http.StatusFound)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/static/") {
		// Handled by file server.
		return
	}
	http.NotFound(w, r)
}

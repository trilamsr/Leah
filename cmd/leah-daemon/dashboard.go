package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/trilam/leah/internal/adapters/discord"
	"github.com/trilam/leah/internal/budget"
	"github.com/trilam/leah/internal/daemonloop"
	"github.com/trilam/leah/internal/memory"
	"github.com/trilam/leah/internal/obs"
	"github.com/trilam/leah/internal/regattaclient"
	"github.com/trilam/leah/internal/web"
)

// startDashboard boots the JARVIS dashboard HTTP server in a goroutine and
// returns a closer. Caller owns the memory store lifecycle (also fed into
// the memory SelfChecker registered at daemon boot). addr is the listen
// address; auditPath + snapPath feed /api/state. 10s cache TTL absorbs the
// dashboard's 3s poll cadence (H4 audit fix).
func startDashboard(ctx context.Context, addr, sd, auditPath, snapPath string, rc *regattaclient.Client, loop *daemonloop.Loop, registry *telemetry.Registry, health *telemetry.HealthRegistry, store *memory.Store, discordAdpt *discord.Adapter) (func(), error) {
	_ = sd
	srv := &web.Server{
		Addr:        addr,
		AuditPath:   auditPath,
		MetricsPath: snapPath,
		Memory:      store,
		Regatta:     rc,
		Budget:      budget.New(),
		StartTime:   time.Now(),
		Heartbeat:   loop.LastTick,
		CacheTTL:    10 * time.Second,
		Metrics:     registry,
		Health:      health,
		SpamStats:   spamStatsFor(discordAdpt),
	}
	go func() {
		if err := startWithMiddleware(ctx, srv, registry); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "leah-daemon: dashboard: %v\n", err)
		}
	}()
	_, _ = fmt.Fprintf(os.Stdout, "leah-daemon: dashboard at http://%s/dashboard\n", addr)
	return func() {}, nil
}

// startWithMiddleware wraps the dashboard mux with the metrics middleware so
// /metrics records leah_web_requests_total per request. Server.Start owns
// the listener; this helper reuses BuildMux + Server.StartHandler when
// available, otherwise falls back to Start (no middleware).
func startWithMiddleware(ctx context.Context, srv *web.Server, registry *telemetry.Registry) error {
	if err := web.EnforceLoopback(srv.Addr); err != nil {
		return err
	}
	mux, err := srv.BuildMux()
	if err != nil {
		return err
	}
	handler := web.MetricsMiddleware(registry, mux)
	httpSrv := &http.Server{
		Addr:              srv.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() { errCh <- httpSrv.ListenAndServe() }()
	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shutCtx)
		return nil
	case err := <-errCh:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}

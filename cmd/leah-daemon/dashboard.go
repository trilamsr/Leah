package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/trilam/leah/internal/budget"
	"github.com/trilam/leah/internal/daemonloop"
	"github.com/trilam/leah/internal/memory"
	"github.com/trilam/leah/internal/obs"
	"github.com/trilam/leah/internal/regattaclient"
	"github.com/trilam/leah/internal/web"
)

// startDashboard boots the JARVIS dashboard HTTP server in a goroutine and
// returns a closer that releases the underlying memory store. addr is the
// listen address; auditPath + snapPath feed /api/state. The 10s cache TTL
// absorbs the dashboard's 3s poll cadence so /api/state re-scans
// audit.jsonl + sqlite at most ~once every 10s (H4 audit fix).
func startDashboard(ctx context.Context, addr, sd, auditPath, snapPath string, rc *regattaclient.Client, loop *daemonloop.Loop, registry *obs.Registry, health *obs.HealthRegistry) (func(), error) {
	store, err := memory.NewStore(filepath.Join(sd, "memory.db"))
	if err != nil {
		return nil, fmt.Errorf("memory store: %w", err)
	}
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
	}
	go func() {
		if err := srv.Start(ctx); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "leah-daemon: dashboard: %v\n", err)
		}
	}()
	_, _ = fmt.Fprintf(os.Stdout, "leah-daemon: dashboard at http://%s/dashboard\n", addr)
	return func() { _ = store.Close() }, nil
}

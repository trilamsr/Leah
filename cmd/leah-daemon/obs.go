package main

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/trilam/leah/internal/obs"
)

// startMetricsSnapshotter spawns the 60s registry-snapshot goroutine that
// writes to <sd>/metrics/latest.json. SafeGo recovers panics into the
// panic dir + counter, never kills the daemon.
func startMetricsSnapshotter(ctx context.Context, lg *slog.Logger, registry *telemetry.Registry, sd string) string {
	snapPath := filepath.Join(sd, "metrics", "latest.json")
	_ = os.MkdirAll(filepath.Dir(snapPath), 0o700)
	telemetry.SafeGo(lg, registry, "metrics-snapshotter", func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := registry.Snapshot(snapPath); err != nil {
					lg.Error("metrics snapshot failed", "err", err)
				}
			}
		}
	})
	return snapPath
}

// startLogRetention caps <sd>/logs growth: one sweep on start, then every 6h.
func startLogRetention(ctx context.Context, lg *slog.Logger, registry *telemetry.Registry, sd string) {
	logsDir := filepath.Join(sd, "logs")
	telemetry.SafeGo(lg, registry, "log-retention", func() {
		ticker := time.NewTicker(6 * time.Hour)
		defer ticker.Stop()
		sweep := func() {
			if err := telemetry.PruneLogs(logsDir, time.Now(), telemetry.RetentionDays()); err != nil {
				lg.Error("log retention sweep failed", "err", err)
			}
		}
		sweep()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				sweep()
			}
		}
	})
}

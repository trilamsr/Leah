package obs

import (
	"context"
	"sync"
	"time"
)

type HealthStatus string

const (
	HealthOK        HealthStatus = "ok"
	HealthDegraded  HealthStatus = "degraded"
	HealthUnhealthy HealthStatus = "unhealthy"
)

// SelfChecker is the per-package liveness contract: nil = ok, error = degraded.
type SelfChecker interface {
	SelfCheck(ctx context.Context) error
}

type HealthRegistry struct {
	mu      sync.RWMutex
	checks  map[string]SelfChecker
	started time.Time
}

func NewHealthRegistry() *HealthRegistry {
	return &HealthRegistry{checks: map[string]SelfChecker{}, started: time.Now()}
}

// Register replaces any prior checker for name.
func (r *HealthRegistry) Register(name string, c SelfChecker) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.checks[name] = c
}

type HealthReport struct {
	Status        HealthStatus      `json:"status"`
	UptimeSeconds float64           `json:"uptime_seconds"`
	PackageHealth map[string]string `json:"package_health"`
}

// Probe fans out every checker concurrently under perCheckTimeout; aggregate
// status escalates to unhealthy at >=3 failures so a single bad upstream
// doesn't trip the daemon's liveness signal.
func (r *HealthRegistry) Probe(ctx context.Context, perCheckTimeout time.Duration) HealthReport {
	r.mu.RLock()
	names := make([]string, 0, len(r.checks))
	checks := make([]SelfChecker, 0, len(r.checks))
	for n, c := range r.checks {
		names = append(names, n)
		checks = append(checks, c)
	}
	started := r.started
	r.mu.RUnlock()

	pkg := make(map[string]string, len(checks))
	degraded := 0
	var wg sync.WaitGroup
	var mu sync.Mutex
	for i := range checks {
		wg.Add(1)
		go func(name string, c SelfChecker) {
			defer wg.Done()
			cctx, cancel := context.WithTimeout(ctx, perCheckTimeout)
			defer cancel()
			err := c.SelfCheck(cctx)
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				pkg[name] = string(HealthOK)
				return
			}
			pkg[name] = "degraded: " + err.Error()
			degraded++
		}(names[i], checks[i])
	}
	wg.Wait()

	status := HealthOK
	switch {
	case degraded >= 3:
		status = HealthUnhealthy
	case degraded > 0:
		status = HealthDegraded
	}
	return HealthReport{
		Status:        status,
		UptimeSeconds: time.Since(started).Seconds(),
		PackageHealth: pkg,
	}
}

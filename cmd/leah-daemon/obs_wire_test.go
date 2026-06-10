package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/budget"
	"github.com/trilam/leah/internal/daemonloop"
	"github.com/trilam/leah/internal/memory"
	"github.com/trilam/leah/internal/obs"
	"github.com/trilam/leah/internal/web"
)

// bootObsServer wires the same surface as cmd/leah-daemon/main.go and returns
// the live /metrics + /health surface. Production wires (counters, SelfCheckers)
// must come from instrumentation.go in each package — empty body or empty
// package_health signals the producer side is unwired.
func bootObsServer(t *testing.T) (*httptest.Server, *obs.Registry) {
	t.Helper()
	sd := t.TempDir()
	auditPath := filepath.Join(sd, "audit.jsonl")
	a := &audit.Logger{Path: auditPath}
	store, err := memory.NewStore(filepath.Join(sd, "memory.db"))
	if err != nil {
		t.Fatalf("memory store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	registry := obs.NewRegistry()
	health := obs.NewHealthRegistry()

	loop := daemonloop.New(stubRegatta{}, nil, nil, a, nopWriter{}, 30*time.Second)

	wireInstrumentation(registry, health, a, store, loop)

	mux, err := buildMuxForTest(&web.Server{
		AuditPath: auditPath,
		Memory:    store,
		Budget:    budget.New(),
		StartTime: time.Now(),
		Metrics:   registry,
		Health:    health,
	})
	if err != nil {
		t.Fatalf("mux: %v", err)
	}
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts, registry
}

// TestDaemon_Metrics_NonEmpty: /metrics body must contain every core counter
// once production code feeds the registry.
func TestDaemon_Metrics_NonEmpty(t *testing.T) {
	ts, registry := bootObsServer(t)

	driveProducersForTest(t, registry)

	body := getBody(t, ts.URL+"/metrics")
	wants := []string{
		"leah_audit_append_total",
		"leah_daemonloop_tick_total",
		"leah_dispatcher_ship_total",
		"leah_memory_queries_total",
		"leah_voice_speak_total",
		"leah_attestation_attempts_total",
	}
	for _, w := range wants {
		if !strings.Contains(body, w) {
			t.Errorf("missing metric %q in /metrics body:\n%s", w, body)
		}
	}
}

// TestDaemon_Health_PackagesPopulated: ≥5 SelfCheckers registered.
func TestDaemon_Health_PackagesPopulated(t *testing.T) {
	ts, _ := bootObsServer(t)

	body := getBody(t, ts.URL+"/health")
	var rep obs.HealthReport
	if err := json.Unmarshal([]byte(body), &rep); err != nil {
		t.Fatalf("decode: %v; body=%s", err, body)
	}
	if got := len(rep.PackageHealth); got < 5 {
		t.Errorf("package_health entries: got %d want >= 5; body=%s", got, body)
	}
}

func getBody(t *testing.T, url string) string {
	t.Helper()
	resp, err := http.Get(url) //nolint:noctx // test helper
	if err != nil {
		t.Fatalf("get %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("%s status: %d", url, resp.StatusCode)
	}
	buf := make([]byte, 256*1024)
	n, _ := resp.Body.Read(buf)
	return string(buf[:n])
}

type nopWriter struct{}

func (nopWriter) Write(p []byte) (int, error) { return len(p), nil }

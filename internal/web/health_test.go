package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/trilam/leah/internal/obs"
)

// TestHTTP_HealthEndpoint_ReturnsJSON: /health returns the registry probe as JSON.
func TestHTTP_HealthEndpoint_ReturnsJSON(t *testing.T) {
	s := newTestServer(t)
	s.Health = telemetry.NewHealthRegistry()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	s.mux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("content-type: got %q, want application/json", ct)
	}
	var rep telemetry.HealthReport
	if err := json.Unmarshal(rec.Body.Bytes(), &rep); err != nil {
		t.Fatalf("decode: %v; body=%s", err, rec.Body.String())
	}
	if rep.Status != telemetry.HealthOK {
		t.Errorf("status: got %q, want ok", rep.Status)
	}
}

// TestHTTP_MetricsEndpoint_ReturnsPromFormat: /metrics returns Prometheus text.
func TestHTTP_MetricsEndpoint_ReturnsPromFormat(t *testing.T) {
	s := newTestServer(t)
	s.Metrics = telemetry.NewRegistry()
	s.Metrics.Counter("leah_test_total").Add(map[string]string{"k": "v"}, 4)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	s.mux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("content-type: got %q, want text/plain prefix", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "# TYPE leah_test_total counter") {
		t.Errorf("missing TYPE: %s", body)
	}
	if !strings.Contains(body, `leah_test_total{k="v"} 4`) {
		t.Errorf("missing series: %s", body)
	}
}

// TestHTTP_HealthEndpoint_NoRegistry returns 503 when not wired.
func TestHTTP_HealthEndpoint_NoRegistry(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	s.mux().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status: got %d, want 503", rec.Code)
	}
}

// TestHTTP_MetricsEndpoint_NoRegistry returns 503 when not wired.
func TestHTTP_MetricsEndpoint_NoRegistry(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	s.mux().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status: got %d, want 503", rec.Code)
	}
}

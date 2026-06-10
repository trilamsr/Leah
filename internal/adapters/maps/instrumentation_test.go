package maps

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/trilam/leah/internal/obs"
)

func TestRegisterMetrics_AddsToRegistry(t *testing.T) {
	r := obs.NewRegistry()
	RegisterMetrics(r)
	got := snapshotKeys(t, r)
	for _, want := range []string{
		"leah_connect_api_call_total",
		"leah_connect_exchange_total",
		"leah_connect_refresh_total",
		"leah_connect_token_age_seconds",
		"leah_connect_api_latency_seconds",
	} {
		if !containsPrefix(got, want) {
			t.Fatalf("series %q missing from snapshot keys %v", want, got)
		}
	}
}

func TestObserve_IncrementsCounter(t *testing.T) {
	r := obs.NewRegistry()
	Observe(r, "ListUnread", "ok", 0.1)
	keys := snapshotKeys(t, r)
	want := "leah_connect_api_call_total|endpoint=ListUnread,provider=" + provider
	if !containsExact(keys, want) {
		t.Fatalf("expected counter key %q after Observe; got %v", want, keys)
	}
}

func snapshotKeys(t *testing.T, r *obs.Registry) []string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "m.json")
	if err := r.Snapshot(path); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var out struct {
		Counters   map[string]int64       `json:"counters"`
		Gauges     map[string]float64     `json:"gauges"`
		Histograms map[string]interface{} `json:"histograms"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	var keys []string
	for k := range out.Counters {
		keys = append(keys, k)
	}
	for k := range out.Gauges {
		keys = append(keys, k)
	}
	for k := range out.Histograms {
		keys = append(keys, k)
	}
	return keys
}

func containsPrefix(keys []string, prefix string) bool {
	for _, k := range keys {
		if k == prefix || strings.HasPrefix(k, prefix+"|") {
			return true
		}
	}
	return false
}

func containsExact(keys []string, want string) bool {
	for _, k := range keys {
		if k == want {
			return true
		}
	}
	return false
}

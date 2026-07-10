package connectadapter

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/trilam/leah/internal/platform/telemetry"
	"github.com/trilam/leah/internal/platform/telemetry/obstest"
)

func readJSON(path string) (map[string]any, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// TestRegister_AllSeriesPresent asserts every spec §4.7 series surfaces.
func TestRegister_AllSeriesPresent(t *testing.T) {
	r := telemetry.NewRegistry()
	For("slack", r).Register()
	keys := obstest.SnapshotKeys(t, r)
	for _, want := range []string{
		"leah_connect_api_call_total",
		"leah_connect_exchange_total",
		"leah_connect_refresh_total",
		"leah_connect_token_age_seconds",
		"leah_connect_api_latency_seconds",
	} {
		if !obstest.ContainsPrefix(keys, want) {
			t.Fatalf("series %q missing from %v", want, keys)
		}
	}
}

// TestObserveAPI_DoesNotBumpExchange guards against a double-bump regression:
// ObserveAPI must NOT touch exchange_total.
func TestObserveAPI_DoesNotBumpExchange(t *testing.T) {
	r := telemetry.NewRegistry()
	For("slack", r).ObserveAPI("ListUnread", 0.1)
	keys := obstest.SnapshotKeys(t, r)
	for _, k := range keys {
		if k == "leah_connect_exchange_total|outcome=ok,provider=slack" ||
			k == "leah_connect_exchange_total|provider=slack,outcome=ok" {
			t.Fatalf("ObserveAPI illegally bumped exchange_total: %q", k)
		}
	}
	wantAPI := "leah_connect_api_call_total|endpoint=ListUnread,provider=slack"
	if !obstest.ContainsExact(keys, wantAPI) {
		t.Fatalf("expected %q in %v", wantAPI, keys)
	}
}

// TestObserveExchange_OnlyBumpsExchange asserts the converse split.
func TestObserveExchange_OnlyBumpsExchange(t *testing.T) {
	r := telemetry.NewRegistry()
	For("slack", r).ObserveExchange("ok")
	keys := obstest.SnapshotKeys(t, r)
	for _, k := range keys {
		if k == "leah_connect_api_call_total|endpoint=ok,provider=slack" {
			t.Fatalf("ObserveExchange illegally bumped api_call_total: %q", k)
		}
	}
	want := "leah_connect_exchange_total|outcome=ok,provider=slack"
	if !obstest.ContainsExact(keys, want) {
		t.Fatalf("expected %q in %v", want, keys)
	}
}

// TestRegister_HistogramHasNoSample asserts Register cold-seeds the
// histogram series with count=0/sum=0 — Observe(0) would skew p50.
func TestRegister_HistogramHasNoSample(t *testing.T) {
	r := telemetry.NewRegistry()
	For("slack", r).Register()
	// One real observation; if Register leaked a 0-sample, count would be 2.
	r.Histogram("leah_connect_api_latency_seconds", latencyBuckets).
		Observe(map[string]string{"provider": "slack"}, 0.5)

	path := t.TempDir() + "/m.json"
	if err := r.Snapshot(path); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	raw, err := readJSON(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	hist := raw["histograms"].(map[string]any)
	s := hist["leah_connect_api_latency_seconds|provider=slack"].(map[string]any)
	if got := s["count"].(float64); got != 1 {
		t.Fatalf("expected count=1 after Register+Observe; got %v — Register leaked a sample", got)
	}
	if got := s["sum"].(float64); got != 0.5 {
		t.Fatalf("expected sum=0.5; got %v", got)
	}
}

// Package obstest dedupes the snapshot-key helpers that 19 instrumentation
// tests would otherwise carry verbatim. Test-only — keep impl tiny.
package obstest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/trilam/leah/internal/obs"
)

// SnapshotKeys writes r to a tempfile and returns the union of counter,
// gauge, and histogram keys it contains.
func SnapshotKeys(t *testing.T, r *telemetry.Registry) []string {
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
	keys := make([]string, 0, len(out.Counters)+len(out.Gauges)+len(out.Histograms))
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

// ContainsPrefix reports whether any key equals prefix or starts with
// "prefix|" — matches a metric with any label set.
func ContainsPrefix(keys []string, prefix string) bool {
	for _, k := range keys {
		if k == prefix || strings.HasPrefix(k, prefix+"|") {
			return true
		}
	}
	return false
}

// ContainsExact reports whether keys contains want verbatim.
func ContainsExact(keys []string, want string) bool {
	for _, k := range keys {
		if k == want {
			return true
		}
	}
	return false
}

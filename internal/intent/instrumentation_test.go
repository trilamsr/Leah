package intent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/trilam/leah/internal/obs"
)

// TestClassifyTimedRecordsHistogram fails until ClassifyTimed exists and emits
// the leah_intent_classify_seconds series with a kind label.
func TestClassifyTimedRecordsHistogram(t *testing.T) {
	reg := obs.NewRegistry()
	got := ClassifyTimed(reg, "ship the fix for bug 1086")
	if got != KindShip {
		t.Fatalf("ClassifyTimed verb = %v, want KindShip", got)
	}
	dir := t.TempDir()
	snap := filepath.Join(dir, "metrics.json")
	if err := reg.Snapshot(snap); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	raw, err := os.ReadFile(snap)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	if !strings.Contains(string(raw), "leah_intent_classify_seconds") {
		t.Fatalf("snapshot missing leah_intent_classify_seconds: %s", raw)
	}
	if !strings.Contains(string(raw), "kind=ship") {
		t.Fatalf("snapshot missing kind=ship label: %s", raw)
	}
}

// TestClassifyTimedNilRegistryIsNoop fails if the helper panics or rejects nil.
func TestClassifyTimedNilRegistryIsNoop(t *testing.T) {
	if got := ClassifyTimed(nil, "status"); got != KindStatus {
		t.Fatalf("ClassifyTimed(nil) = %v, want KindStatus", got)
	}
}

// TestClassifyTimedBucketsSubFiftyMs fails when buckets don't span the ≤50ms
// target — A3 demands a histogram whose top sub-+Inf bound is ≤0.05s so the
// p95 SLO is enforceable from /metrics alone.
func TestClassifyTimedBucketsSubFiftyMs(t *testing.T) {
	reg := obs.NewRegistry()
	_ = ClassifyTimed(reg, "review pr 1112")
	dir := t.TempDir()
	snap := filepath.Join(dir, "metrics.json")
	if err := reg.Snapshot(snap); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	raw, err := os.ReadFile(snap)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	var doc struct {
		Histograms map[string]struct {
			Buckets map[string]int64 `json:"buckets"`
		} `json:"histograms"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	var hit string
	for k := range doc.Histograms {
		if strings.HasPrefix(k, "leah_intent_classify_seconds") {
			hit = k
			break
		}
	}
	if hit == "" {
		t.Fatalf("no leah_intent_classify_seconds series: %s", raw)
	}
	for _, b := range []string{"0.0001", "0.001", "0.01", "0.05"} {
		if _, ok := doc.Histograms[hit].Buckets[b]; !ok {
			t.Fatalf("missing sub-50ms bucket %s: %v", b, doc.Histograms[hit].Buckets)
		}
	}
}

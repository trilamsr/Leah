package intent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/trilam/leah/internal/obs"
)

// TestClassifyTimedRecordsHistogram covers every Kind so a label-routing bug
// in one verb can't hide behind another's coverage, and asserts count==1 so a
// silent double-Observe regresses the suite.
func TestClassifyTimedRecordsHistogram(t *testing.T) {
	cases := []struct {
		in   string
		want Kind
	}{
		{"ship the fix for bug 1086", KindShip},
		{"review pr 1112", KindReview},
		{"status", KindStatus},
		{"what does regatta currently support", KindAsk},
	}
	for _, c := range cases {
		reg := obs.NewRegistry()
		got := ClassifyTimed(reg, c.in)
		if got != c.want {
			t.Fatalf("ClassifyTimed(%q) = %v, want %v", c.in, got, c.want)
		}
		s := histSeries(t, reg, c.want.String())
		if s.Count != 1 {
			t.Fatalf("kind=%s count = %d, want 1 (double-Observe?)", c.want, s.Count)
		}
		if s.Sum < 0 {
			t.Fatalf("kind=%s sum = %v, want ≥0", c.want, s.Sum)
		}
	}
}

// TestClassifyTimedNilRegistryIsNoop fails if the helper panics or rejects nil.
func TestClassifyTimedNilRegistryIsNoop(t *testing.T) {
	if got := ClassifyTimed(nil, "status"); got != KindStatus {
		t.Fatalf("ClassifyTimed(nil) = %v, want KindStatus", got)
	}
}

// TestClassifyTimedBucketsSubFiftyMs fails when buckets don't span the ≤50ms
// SLO — the histogram's bound list must encode the SLO so /metrics alone proves
// it.
func TestClassifyTimedBucketsSubFiftyMs(t *testing.T) {
	reg := obs.NewRegistry()
	_ = ClassifyTimed(reg, "review pr 1112")
	s := histSeries(t, reg, "review")
	for _, b := range []string{"0.0001", "0.001", "0.01", "0.05"} {
		if _, ok := s.Buckets[b]; !ok {
			t.Fatalf("missing sub-50ms bucket %s: %v", b, s.Buckets)
		}
	}
}

// TestRegisterMetricsSeedsAllKinds guards the kindMax-bounded loop — if a new
// verb lands but RegisterMetrics drifts, cold-seed coverage gaps surface here.
func TestRegisterMetricsSeedsAllKinds(t *testing.T) {
	reg := obs.NewRegistry()
	RegisterMetrics(reg)
	for k := KindAsk; k < kindMax; k++ {
		s := histSeries(t, reg, k.String())
		if s.Count != 0 {
			t.Fatalf("kind=%s seed leaked sample: count=%d", k, s.Count)
		}
	}
}

type seriesView struct {
	Count   int64            `json:"count"`
	Sum     float64          `json:"sum"`
	Buckets map[string]int64 `json:"buckets"`
}

func histSeries(t *testing.T, reg *obs.Registry, wantKind string) seriesView {
	t.Helper()
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
		Histograms map[string]seriesView `json:"histograms"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	want := "leah_intent_classify_seconds|kind=" + wantKind
	s, ok := doc.Histograms[want]
	if !ok {
		t.Fatalf("series %q missing; got %v", want, keys(doc.Histograms))
	}
	return s
}

func keys(m map[string]seriesView) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

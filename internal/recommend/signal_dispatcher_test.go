package recommend

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/trilam/leah/internal/obs"
	"github.com/trilam/leah/internal/testutil"
)

// staticMatcher emits one Recommendation when sig.Kind matches kind.
type staticMatcher struct {
	kind string
	rec  Recommendation
}

func (m *staticMatcher) Match(_ context.Context, sig Signal) ([]Recommendation, error) {
	if sig.Kind != m.kind {
		return nil, nil
	}
	return []Recommendation{m.rec}, nil
}

func TestSignalEngine_OnSignal_ProposesRecommendations(t *testing.T) {
	logger, _ := newAuditLogger(t)
	eng := NewMemoryEngine(logger)
	eng.RegisterMatcher(&staticMatcher{
		kind: "app_focus",
		rec:  Recommendation{ID: "r-1", Pattern: "focus.slack", Tier: TierConfirm, Source: "patterns", Confidence: 0.7},
	})

	recs, err := eng.OnSignal(context.Background(), Signal{Kind: "app_focus", At: time.Unix(1700000000, 0).UTC()})
	if err != nil {
		t.Fatalf("OnSignal: %v", err)
	}
	if len(recs) != 1 || recs[0].ID != "r-1" {
		t.Fatalf("want one rec r-1, got %+v", recs)
	}
	pending, _ := eng.Propose(context.Background())
	if len(pending) != 1 {
		t.Fatalf("rec not seeded, pending=%d", len(pending))
	}
}

func TestSignalEngine_OnSignal_DebouncesPerPattern_30s(t *testing.T) {
	logger, _ := newAuditLogger(t)
	eng := NewMemoryEngine(logger)
	eng.RegisterMatcher(&staticMatcher{
		kind: "app_focus",
		rec:  Recommendation{ID: "r-1", Pattern: "focus.slack", Tier: TierConfirm, Source: "patterns", Confidence: 0.7},
	})
	t0 := time.Unix(1700000000, 0).UTC()
	eng.now = func() time.Time { return t0 }

	if _, err := eng.OnSignal(context.Background(), Signal{Kind: "app_focus", At: t0}); err != nil {
		t.Fatalf("first OnSignal: %v", err)
	}
	eng.now = func() time.Time { return t0.Add(29 * time.Second) }
	recs, err := eng.OnSignal(context.Background(), Signal{Kind: "app_focus", At: t0.Add(29 * time.Second)})
	if err != nil {
		t.Fatalf("second OnSignal: %v", err)
	}
	if len(recs) != 0 {
		t.Fatalf("debounce broken: 29s repeat returned %d recs", len(recs))
	}
	eng.now = func() time.Time { return t0.Add(31 * time.Second) }
	recs, err = eng.OnSignal(context.Background(), Signal{Kind: "app_focus", At: t0.Add(31 * time.Second)})
	if err != nil {
		t.Fatalf("third OnSignal: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("post-debounce: want 1 rec, got %d", len(recs))
	}
}

func TestSignalEngine_NoMatchingPattern_ReturnsEmpty(t *testing.T) {
	logger, _ := newAuditLogger(t)
	eng := NewMemoryEngine(logger)
	eng.RegisterMatcher(&staticMatcher{kind: "calendar_imminent"})

	recs, err := eng.OnSignal(context.Background(), Signal{Kind: "app_focus", At: time.Unix(1700000000, 0).UTC()})
	if err != nil {
		t.Fatalf("OnSignal: %v", err)
	}
	if len(recs) != 0 {
		t.Fatalf("non-matching signal: want empty, got %d", len(recs))
	}
}

// captureEngine records OnSignal calls without producing recs. Embeds
// *MemoryEngine (pointer) so the test struct never copies sync.Mutex.
type captureEngine struct {
	*MemoryEngine
	mu   sync.Mutex
	seen []Signal
}

func (c *captureEngine) OnSignal(ctx context.Context, sig Signal) ([]Recommendation, error) {
	c.mu.Lock()
	c.seen = append(c.seen, sig)
	c.mu.Unlock()
	return nil, nil
}

func TestSignalDispatcher_TranslatesObsEventToSignal(t *testing.T) {
	logger, _ := newAuditLogger(t)
	cap := &captureEngine{MemoryEngine: NewMemoryEngine(logger)}
	b := obs.NewBroadcaster()

	d := NewSignalDispatcher(cap, b, func() string { return "voice" })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := d.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer d.Stop()

	b.Emit(obs.Event{Kind: "voice.speak", TS: time.Unix(1700000000, 0).UTC(), Detail: "wake"})

	testutil.Eventually(t, time.Second, 5*time.Millisecond, func() bool {
		cap.mu.Lock()
		defer cap.mu.Unlock()
		return len(cap.seen) >= 1
	})
	cap.mu.Lock()
	defer cap.mu.Unlock()
	if len(cap.seen) != 1 {
		t.Fatalf("want 1 translated signal, got %d", len(cap.seen))
	}
	got := cap.seen[0]
	if got.Kind != "voice.speak" || got.Context != "voice" || got.Detail != "wake" {
		t.Fatalf("translation mismatch: %+v", got)
	}
}

// TestSignalDispatcher_DefaultKinds_MatchActualEmitters asserts every
// DefaultSignalKinds entry exists in obs.KnownEventKinds. Without this gate
// the dispatcher subscribes to phantom kinds — silent zero-match filter.
func TestSignalDispatcher_DefaultKinds_MatchActualEmitters(t *testing.T) {
	known := make(map[string]struct{}, len(obs.KnownEventKinds))
	for _, k := range obs.KnownEventKinds {
		known[k] = struct{}{}
	}
	for _, k := range DefaultSignalKinds {
		if _, ok := known[k]; !ok {
			t.Errorf("DefaultSignalKinds entry %q not in obs.KnownEventKinds (phantom kind = silent filter)", k)
		}
	}
}

// TestSignalDispatcher_DefaultKinds_NoFeedbackLoop guards against the engine's
// own recommendation.propose emit being routed back into OnSignal.
func TestSignalDispatcher_DefaultKinds_NoFeedbackLoop(t *testing.T) {
	for _, k := range DefaultSignalKinds {
		if k == "recommendation.propose" {
			t.Fatalf("DefaultSignalKinds must not include %q (feedback loop)", k)
		}
	}
}

func TestSignalDispatcher_WithKinds_RejectsFeedbackLoopKind(t *testing.T) {
	logger, _ := newAuditLogger(t)
	eng := NewMemoryEngine(logger)
	b := obs.NewBroadcaster()
	d := NewSignalDispatcher(eng, b, nil)
	if _, err := d.WithKinds([]string{"recommendation.propose"}); !errors.Is(err, ErrFeedbackLoopKind) {
		t.Fatalf("want ErrFeedbackLoopKind, got %v", err)
	}
}

func TestSignalDispatcher_DropMonitor_EmitsCounterDelta(t *testing.T) {
	logger, _ := newAuditLogger(t)
	cap := &captureEngine{MemoryEngine: NewMemoryEngine(logger)}
	b := obs.NewBroadcaster()
	reg := obs.NewRegistry()
	d := NewSignalDispatcher(cap, b, nil).WithRegistry(reg)
	d.tick = 5 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := d.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer d.Stop()

	// Saturate the 16-deep subscriber buffer to force drops.
	for i := 0; i < 64; i++ {
		b.Emit(obs.Event{Kind: "voice.speak", TS: time.Now().UTC()})
	}
	testutil.Eventually(t, time.Second, 5*time.Millisecond, func() bool {
		return b.Dropped() > 0
	})
	want := b.Dropped()
	testutil.Eventually(t, time.Second, 5*time.Millisecond, func() bool {
		return counterTotal(t, reg, "leah_signal_dispatcher_dropped_total") >= int64(want)
	})
}

// counterTotal sums all label-set values of name via Snapshot's JSON.
func counterTotal(t *testing.T, reg *obs.Registry, name string) int64 {
	t.Helper()
	path := t.TempDir() + "/snap.json"
	if err := reg.Snapshot(path); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	var doc struct {
		Counters map[string]int64 `json:"counters"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	var sum int64
	for k, v := range doc.Counters {
		if k == name || strings.HasPrefix(k, name+"|") {
			sum += v
		}
	}
	return sum
}

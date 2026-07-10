package memory

import (
	"context"
	"testing"
	"time"

	"github.com/trilam/leah/internal/obs"
)

// TestAddDecisionEmitsMemoryUpsert drives a real AddDecision and asserts the
// live SSE fan-out receives a memory.upsert frame, proving the write hook now
// feeds the dashboard subscription instead of only the metrics counter.
func TestAddDecisionEmitsMemoryUpsert(t *testing.T) {
	b := telemetry.NewBroadcaster()
	telemetry.SetDefaultBroadcaster(b)
	t.Cleanup(func() { telemetry.SetDefaultBroadcaster(nil) })
	sub, err := b.Subscribe(context.Background(), []string{"memory.upsert"})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	t.Cleanup(func() { _ = sub.Close() })

	s := newTestStore(t)
	if _, err := s.AddDecision(Decision{Topic: "tracker", Choice: "linear", DecidedAt: "2026-06-20"}); err != nil {
		t.Fatalf("add decision: %v", err)
	}

	select {
	case e := <-sub.Events():
		if e.Kind != "memory.upsert" {
			t.Fatalf("kind: got %q want memory.upsert", e.Kind)
		}
		if e.Target != "decision" {
			t.Fatalf("target: got %q want decision", e.Target)
		}
	case <-time.After(time.Second):
		t.Fatal("no memory.upsert frame on the live bus")
	}
}

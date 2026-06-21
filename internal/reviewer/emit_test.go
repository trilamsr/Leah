package reviewer

import (
	"context"
	"testing"
	"time"

	"github.com/trilam/leah/internal/obs"
)

// TestReviewEmitsSubagentSpawnAndComplete drives a real Review and asserts the
// live bus carries subagent.spawn (before the model call) and subagent.complete
// (after a parsed verdict) — the kinds events_timeline.js subscribes to but
// the review path never emitted before this wiring.
func TestReviewEmitsSubagentSpawnAndComplete(t *testing.T) {
	b := obs.NewBroadcaster()
	obs.SetDefaultBroadcaster(b)
	t.Cleanup(func() { obs.SetDefaultBroadcaster(nil) })
	sub, err := b.Subscribe(context.Background(), []string{"subagent.spawn", "subagent.complete"})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	t.Cleanup(func() { _ = sub.Close() })

	sa := &fakeSubagent{resp: "Reviewer-recommendation: APPROVE\nReviewer-agent-id: cavecrew-reviewer-x\n"}
	r := &Reviewer{Subagent: sa, SystemPrompt: "x"}
	if _, err := r.Review(context.Background(), "diff", "issue"); err != nil {
		t.Fatalf("review: %v", err)
	}

	want := map[string]bool{"subagent.spawn": false, "subagent.complete": false}
	deadline := time.After(2 * time.Second)
	for {
		done := true
		for _, seen := range want {
			if !seen {
				done = false
			}
		}
		if done {
			return
		}
		select {
		case e := <-sub.Events():
			if _, ok := want[e.Kind]; ok {
				want[e.Kind] = true
			}
		case <-deadline:
			for k, seen := range want {
				if !seen {
					t.Fatalf("no %s frame on the live bus", k)
				}
			}
		}
	}
}

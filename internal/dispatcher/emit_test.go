package dispatcher

import (
	"context"
	"testing"
	"time"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/budget"
	"github.com/trilam/leah/internal/obs"
	"github.com/trilam/leah/internal/regattaclient"
)

// drainKinds collects every kind seen on sub until timeout or n distinct kinds.
func drainKinds(t *testing.T, sub telemetry.SSESubscriber, want map[string]bool) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		remaining := false
		for _, seen := range want {
			if !seen {
				remaining = true
			}
		}
		if !remaining {
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
					t.Errorf("no %s frame on the live bus", k)
				}
			}
			return
		}
	}
}

// TestShipEmitsDispatchShipAndMerge drives a real ship+watch through to a
// regatta "merged" terminal state and asserts the live bus carries both
// dispatch.ship and dispatch.merge — the kinds events_timeline.js subscribes
// to but production never emitted before this wiring.
func TestShipEmitsDispatchShipAndMerge(t *testing.T) {
	b := telemetry.NewBroadcaster()
	telemetry.SetDefaultBroadcaster(b)
	t.Cleanup(func() { telemetry.SetDefaultBroadcaster(nil) })
	sub, err := b.Subscribe(context.Background(), []string{"dispatch.ship", "dispatch.merge"})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	t.Cleanup(func() { _ = sub.Close() })

	dir := t.TempDir()
	ship := &Ship{
		Reasoner:  &fakeShipReasoner{resp: "## Context\n\nx\n\n## What to do\n\n- y\n\n## Acceptance\n\n- z\n"},
		GH:        &fakeGh{createURL: "https://github.com/x/r/issues/1"},
		Audit:     &audit.Logger{Path: dir + "/audit.jsonl"},
		Budget:    &budget.Budget{Ceiling: 5.0},
		Out:       &noopWriter{},
		Repo:      "x/r",
		TmpDir:    dir,
		Watch:     true,
		Regatta:   &fakeRegatta{listResp: []regattaclient.Agent{{ID: "a1", State: "merged", PR: 1}}},
		PollEvery: time.Millisecond,
		MaxPolls:  2,
	}
	if err := ship.Run(context.Background(), "fix"); err != nil {
		t.Fatalf("run: %v", err)
	}

	drainKinds(t, sub, map[string]bool{"dispatch.ship": false, "dispatch.merge": false})
}

type noopWriter struct{}

func (noopWriter) Write(p []byte) (int, error) { return len(p), nil }

package operatormodel

import (
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
	"time"
)

// TestFeedbackObserverRecordsShipOutcome asserts a ship_dangling outcome
// lands as a Class=ship_outcome observation the Profile can fold in.
func TestFeedbackObserverRecordsShipOutcome(t *testing.T) {
	o := &FeedbackObserver{}
	o.Observe(ShipOutcome{Verdict: VerdictDangling, At: time.Date(2026, 6, 9, 10, 0, 0, 0, time.UTC)})

	obs := o.Drain()
	if len(obs) != 1 {
		t.Fatalf("want 1 observation, got %d", len(obs))
	}
	got := obs[0]
	if got.Class != "ship_outcome" {
		t.Errorf("class: want ship_outcome, got %q", got.Class)
	}
	if got.Slot != VerdictDangling {
		t.Errorf("slot: want %q, got %q", VerdictDangling, got.Slot)
	}
	if got.Count != 1 {
		t.Errorf("count: want 1, got %d", got.Count)
	}
}

// TestFeedbackObserverNoOutcomeDrainsEmpty is the negative test: no Observe
// call means nothing to fold into the Profile.
func TestFeedbackObserverNoOutcomeDrainsEmpty(t *testing.T) {
	o := &FeedbackObserver{}
	if got := o.Drain(); len(got) != 0 {
		t.Errorf("want 0 observations with no Observe call, got %d", len(got))
	}
}

// TestFeedbackObserverDrainClears asserts Drain transfers ownership — a
// second Drain after no new Observe returns empty (no double-count into
// successive Profile rebuilds).
func TestFeedbackObserverDrainClears(t *testing.T) {
	o := &FeedbackObserver{}
	o.Observe(ShipOutcome{Verdict: VerdictOK, At: time.Now()})
	if got := o.Drain(); len(got) != 1 {
		t.Fatalf("first drain: want 1, got %d", len(got))
	}
	if got := o.Drain(); len(got) != 0 {
		t.Errorf("second drain: want 0, got %d", len(got))
	}
}

// TestOperatormodelDoesNotImportLearn is the load-bearing import-arrow
// guard: the feedback edge MUST stay one-way (learn -> operatormodel via
// callback), never operatormodel -> learn, to avoid an import cycle.
func TestOperatormodelDoesNotImportLearn(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	fset := token.NewFileSet()
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, imp := range f.Imports {
			if strings.Contains(imp.Path.Value, "internal/learn") {
				t.Errorf("%s imports learn (%s) — breaks one-way feedback arrow", name, imp.Path.Value)
			}
		}
	}
}

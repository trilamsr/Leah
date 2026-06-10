package sources

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestSource_Name_Distinct: each impl must publish a unique Name so the
// recommend engine can attribute signals when sources land in Propose.
func TestSource_Name_Distinct(t *testing.T) {
	m := NewMacosMirrorSource(&fakeMirror{}, MacosOpts{})
	k := NewKnowledgeGraphSource(&fakeGraph{}, KnowledgeOpts{})
	if m.Name() == "" || k.Name() == "" {
		t.Fatalf("empty Name: macos=%q knowledge=%q", m.Name(), k.Name())
	}
	if m.Name() == k.Name() {
		t.Fatalf("Name collision: %q", m.Name())
	}
}

// TestSourcesIndependent: an erroring source MUST NOT block another. The
// adversarial framing: a hung knowledge graph kills calendar-driven recs in a
// naive aggregator.
func TestSourcesIndependent(t *testing.T) {
	base := time.Date(2026, 6, 1, 8, 30, 0, 0, time.UTC)
	var events []AppEvent
	for d := 0; d < 7; d++ {
		events = append(events, AppEvent{App: "Calendar", Timestamp: base.AddDate(0, 0, d)})
	}
	healthy := NewMacosMirrorSource(&fakeMirror{events: events}, MacosOpts{
		Now: func() time.Time { return base.AddDate(0, 0, 14) },
	})
	broken := NewKnowledgeGraphSource(&fakeGraph{err: errors.New("boom")}, KnowledgeOpts{})

	recs, errs := Gather(context.Background(), []Source{broken, healthy})
	if len(recs) == 0 {
		t.Fatal("healthy source produced 0 recs despite peer failure")
	}
	if len(errs) != 1 {
		t.Fatalf("want 1 error from broken source, got %d (%v)", len(errs), errs)
	}
}

package sources

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeMirror is an in-memory stub for MirrorSeam. Test wiring keeps the
// SQLite-mirror coupling out of the Source impl — Source consumes the seam.
type fakeMirror struct {
	events []AppEvent
	err    error
}

func (f *fakeMirror) AppOpens(ctx context.Context, since time.Time) ([]AppEvent, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.events, nil
}

// TestMacosMirror_RecommendationsFromCalendarPattern_OpenedDailyAt9 covers the
// happy path: Calendar.app opened ≥5 times within the 8–9am window over the
// past 14 days → propose CommitAtFocusEnd-style cadence rec.
func TestMacosMirror_RecommendationsFromCalendarPattern_OpenedDailyAt9(t *testing.T) {
	base := time.Date(2026, 6, 1, 8, 30, 0, 0, time.UTC)
	var events []AppEvent
	for d := 0; d < 7; d++ {
		events = append(events, AppEvent{
			App:       "Calendar",
			Timestamp: base.AddDate(0, 0, d),
		})
	}
	src := NewMacosMirrorSource(&fakeMirror{events: events}, MacosOpts{
		Now: func() time.Time { return base.AddDate(0, 0, 14) },
	})
	recs, err := src.Recommendations(context.Background())
	if err != nil {
		t.Fatalf("Recommendations: %v", err)
	}
	if len(recs) == 0 {
		t.Fatal("want >=1 rec from daily Calendar opens, got 0")
	}
	if recs[0].Source != "macos-mirror" {
		t.Errorf("Source = %q, want macos-mirror", recs[0].Source)
	}
	if recs[0].Pattern == "" {
		t.Errorf("Pattern empty")
	}
	if recs[0].Confidence <= 0 || recs[0].Confidence > 1 {
		t.Errorf("Confidence %v out of (0,1]", recs[0].Confidence)
	}
}

// TestMacosMirror_NoData_EmptyRecs guards the cold-start: no app events →
// nil rec slice, nil error. Engine.Propose merges multiple sources so empty
// must be benign.
func TestMacosMirror_NoData_EmptyRecs(t *testing.T) {
	src := NewMacosMirrorSource(&fakeMirror{events: nil}, MacosOpts{})
	recs, err := src.Recommendations(context.Background())
	if err != nil {
		t.Fatalf("Recommendations: %v", err)
	}
	if len(recs) != 0 {
		t.Errorf("want 0 recs on empty mirror, got %d", len(recs))
	}
}

// TestMacosMirror_SeamError_Propagates verifies the source surfaces seam
// errors so Aggregator can isolate them (TestSourcesIndependent).
func TestMacosMirror_SeamError_Propagates(t *testing.T) {
	sentinel := errors.New("mirror down")
	src := NewMacosMirrorSource(&fakeMirror{err: sentinel}, MacosOpts{})
	_, err := src.Recommendations(context.Background())
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want wraps %v", err, sentinel)
	}
}

// TestMacosMirror_BelowCadenceThreshold_NoRec — 2 opens in 14 days is noise,
// not a pattern; deletion-default keeps the candidate pool small.
func TestMacosMirror_BelowCadenceThreshold_NoRec(t *testing.T) {
	base := time.Date(2026, 6, 1, 8, 30, 0, 0, time.UTC)
	events := []AppEvent{
		{App: "Calendar", Timestamp: base},
		{App: "Calendar", Timestamp: base.AddDate(0, 0, 1)},
	}
	src := NewMacosMirrorSource(&fakeMirror{events: events}, MacosOpts{
		Now: func() time.Time { return base.AddDate(0, 0, 14) },
	})
	recs, err := src.Recommendations(context.Background())
	if err != nil {
		t.Fatalf("Recommendations: %v", err)
	}
	if len(recs) != 0 {
		t.Errorf("want 0 recs below threshold, got %d", len(recs))
	}
}

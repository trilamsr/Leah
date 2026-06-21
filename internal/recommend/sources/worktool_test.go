package sources

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeItems struct {
	items []WorkItem
	err   error
}

func (f *fakeItems) StaleItems(ctx context.Context, within time.Duration) ([]WorkItem, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.items, nil
}

func staleItemsAged(key string, ageDays int, now time.Time) []WorkItem {
	return []WorkItem{{Key: key, Title: "x", Updated: now.AddDate(0, 0, -ageDays)}}
}

// TestWorkToolSource_StaleItem_Recommends covers the happy path for each
// wired work tool: a stale operator-scoped item yields a confirm-tier rec
// tagged with the tool's Source name.
func TestWorkToolSource_StaleItem_Recommends(t *testing.T) {
	now := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	cases := []struct {
		name   string
		src    func(WorkItemSeam) Source
		source string
	}{
		{"jira", func(s WorkItemSeam) Source { return NewJiraSource(s, WorkToolOpts{Now: func() time.Time { return now }}) }, "jira"},
		{"linear", func(s WorkItemSeam) Source { return NewLinearSource(s, WorkToolOpts{Now: func() time.Time { return now }}) }, "linear"},
		{"confluence", func(s WorkItemSeam) Source { return NewConfluenceSource(s, WorkToolOpts{Now: func() time.Time { return now }}) }, "confluence"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := tc.src(&fakeItems{items: staleItemsAged("ENG-42", 5, now)})
			recs, err := src.Recommendations(context.Background())
			if err != nil {
				t.Fatalf("Recommendations: %v", err)
			}
			if len(recs) == 0 {
				t.Fatal("want >=1 rec from stale work item, got 0")
			}
			if recs[0].Source != tc.source {
				t.Errorf("Source = %q, want %q", recs[0].Source, tc.source)
			}
			if recs[0].Tier != "confirm" {
				t.Errorf("Tier = %q, want confirm", recs[0].Tier)
			}
			if recs[0].Pattern == "" {
				t.Error("Pattern empty")
			}
			if recs[0].Confidence <= 0 || recs[0].Confidence > 1 {
				t.Errorf("Confidence %v out of (0,1]", recs[0].Confidence)
			}
		})
	}
}

// TestWorkToolSource_NoItems_EmptyRecs guards cold-start: an empty read is a
// benign nil slice, nil error — Engine.Propose merges multiple sources.
func TestWorkToolSource_NoItems_EmptyRecs(t *testing.T) {
	src := NewJiraSource(&fakeItems{}, WorkToolOpts{})
	recs, err := src.Recommendations(context.Background())
	if err != nil {
		t.Fatalf("Recommendations: %v", err)
	}
	if len(recs) != 0 {
		t.Errorf("want 0 recs on empty read, got %d", len(recs))
	}
}

// TestWorkToolSource_FreshItem_NoRec — an item touched today is not stale;
// deletion-default keeps the candidate pool small.
func TestWorkToolSource_FreshItem_NoRec(t *testing.T) {
	now := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	src := NewLinearSource(&fakeItems{items: staleItemsAged("LEA-1", 0, now)}, WorkToolOpts{Now: func() time.Time { return now }})
	recs, err := src.Recommendations(context.Background())
	if err != nil {
		t.Fatalf("Recommendations: %v", err)
	}
	if len(recs) != 0 {
		t.Errorf("want 0 recs for fresh item, got %d", len(recs))
	}
}

// TestWorkToolSource_SeamError_Propagates mirrors macos/knowledge so the
// aggregator can isolate per-source failures.
func TestWorkToolSource_SeamError_Propagates(t *testing.T) {
	sentinel := errors.New("api down")
	src := NewConfluenceSource(&fakeItems{err: sentinel}, WorkToolOpts{})
	_, err := src.Recommendations(context.Background())
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want wraps %v", err, sentinel)
	}
}

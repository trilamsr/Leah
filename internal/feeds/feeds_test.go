package feeds_test

import (
	"context"
	"testing"
	"time"

	"github.com/trilam/leah/internal/feeds"
)

// stubFeed asserts the Feed interface compiles with a minimal value-type.
type stubFeed struct{}

func (stubFeed) Name() string                                   { return "stub" }
func (stubFeed) Fetch(_ context.Context) ([]feeds.Item, error)  { return nil, nil }

// TestFeed_InterfaceSatisfied compile-time guard: a trivial implementor satisfies Feed.
func TestFeed_InterfaceSatisfied(t *testing.T) {
	var f feeds.Feed = stubFeed{}
	if got := f.Name(); got != "stub" {
		t.Fatalf("Name() = %q, want stub", got)
	}
	items, err := f.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch returned err: %v", err)
	}
	if items != nil {
		t.Fatalf("Fetch returned non-nil items on stub")
	}
}

// TestItem_FieldsAddressable guards that Item exposes the spec-required fields.
func TestItem_FieldsAddressable(t *testing.T) {
	it := feeds.Item{
		ID:        "1",
		Title:     "t",
		Body:      "b",
		Source:    "s",
		Timestamp: time.Unix(0, 0),
		Tags:      []string{"a"},
	}
	if it.ID == "" || it.Title == "" || it.Body == "" || it.Source == "" || len(it.Tags) != 1 {
		t.Fatalf("Item fields not round-tripping: %+v", it)
	}
}

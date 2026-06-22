package source

import (
	"context"
	"testing"
	"time"
)

// fakeSource exists solely so the Source interface has at least one
// in-tree consumer; the three production implementations land in a
// later PR (git/voice/news) but the interface must compile against a
// satisfying type today.
type fakeSource struct{}

func (fakeSource) Name() string { return "fake" }
func (fakeSource) Candidates(_ context.Context, _ time.Time) ([]Fact, error) {
	return []Fact{{Slot: "commit", Summary: "ship", Origin: "abc1234"}}, nil
}

func TestSourceInterfaceShape(t *testing.T) {
	var s Source = fakeSource{}
	if s.Name() != "fake" {
		t.Fatalf("Name = %q, want fake", s.Name())
	}
	facts, err := s.Candidates(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("Candidates: %v", err)
	}
	if len(facts) != 1 || facts[0].Slot != "commit" {
		t.Fatalf("facts = %+v", facts)
	}
}

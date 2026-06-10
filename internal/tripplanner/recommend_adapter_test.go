package tripplanner_test

import (
	"context"
	"errors"
	"testing"

	"github.com/trilam/leah/internal/tripplanner"
)

// fakeRecommendSeam returns canned Propose results and records Accept/Reject
// calls. Stub-not-mock — tests assert observable reordering, not call shape.
type fakeRecommendSeam struct {
	proposeOut []tripplanner.Recommendation
	proposeErr error
	accepted   []string
	rejected   []string
	acceptErr  error
	rejectErr  error
}

func (f *fakeRecommendSeam) Propose(_ context.Context) ([]tripplanner.Recommendation, error) {
	return f.proposeOut, f.proposeErr
}

func (f *fakeRecommendSeam) Accept(_ context.Context, id string) error {
	if f.acceptErr != nil {
		return f.acceptErr
	}
	f.accepted = append(f.accepted, id)
	return nil
}

func (f *fakeRecommendSeam) Reject(_ context.Context, id string) error {
	if f.rejectErr != nil {
		return f.rejectErr
	}
	f.rejected = append(f.rejected, id)
	return nil
}

func TestRankPlaces_OperatorAcceptedSignal_RanksHigher(t *testing.T) {
	seam := &fakeRecommendSeam{proposeOut: []tripplanner.Recommendation{
		{Pattern: "place:park", Confidence: 0.9},
	}}
	adapter := tripplanner.NewRecommendAdapter(seam)

	in := []tripplanner.Place{
		{ID: "diner", Name: "Diner"},
		{ID: "park", Name: "Park"},
	}
	got, err := adapter.RankPlaces(context.Background(), in)
	if err != nil {
		t.Fatalf("RankPlaces: %v", err)
	}
	if len(got) != 2 || got[0].ID != "park" || got[1].ID != "diner" {
		t.Fatalf("RankPlaces = %+v; want park first", got)
	}
}

func TestRankPlaces_OperatorRejectedSignal_RanksLower(t *testing.T) {
	seam := &fakeRecommendSeam{proposeOut: []tripplanner.Recommendation{
		{Pattern: "place:diner", Confidence: -0.7},
	}}
	adapter := tripplanner.NewRecommendAdapter(seam)

	in := []tripplanner.Place{
		{ID: "diner", Name: "Diner"},
		{ID: "park", Name: "Park"},
	}
	got, err := adapter.RankPlaces(context.Background(), in)
	if err != nil {
		t.Fatalf("RankPlaces: %v", err)
	}
	if len(got) != 2 || got[0].ID != "park" || got[1].ID != "diner" {
		t.Fatalf("RankPlaces = %+v; want diner last", got)
	}
}

func TestRankPlaces_UnknownPlace_PreservesOrder(t *testing.T) {
	seam := &fakeRecommendSeam{proposeOut: nil}
	adapter := tripplanner.NewRecommendAdapter(seam)

	in := []tripplanner.Place{
		{ID: "a"}, {ID: "b"}, {ID: "c"},
	}
	got, err := adapter.RankPlaces(context.Background(), in)
	if err != nil {
		t.Fatalf("RankPlaces: %v", err)
	}
	if len(got) != 3 || got[0].ID != "a" || got[1].ID != "b" || got[2].ID != "c" {
		t.Fatalf("RankPlaces = %+v; want input order preserved", got)
	}
}

func TestRankPlaces_EngineError_ReturnsInputUnchanged(t *testing.T) {
	boom := errors.New("engine down")
	seam := &fakeRecommendSeam{proposeErr: boom}
	adapter := tripplanner.NewRecommendAdapter(seam)

	in := []tripplanner.Place{{ID: "a"}, {ID: "b"}}
	got, err := adapter.RankPlaces(context.Background(), in)
	if err != nil {
		t.Fatalf("RankPlaces: want nil err on engine failure, got %v", err)
	}
	if len(got) != 2 || got[0].ID != "a" || got[1].ID != "b" {
		t.Fatalf("RankPlaces = %+v; want input unchanged", got)
	}
}

func TestRecordAccept_PropagatesToEngine(t *testing.T) {
	seam := &fakeRecommendSeam{}
	adapter := tripplanner.NewRecommendAdapter(seam)

	if err := adapter.RecordAccept(context.Background(), "park"); err != nil {
		t.Fatalf("RecordAccept: %v", err)
	}
	if len(seam.accepted) != 1 || seam.accepted[0] != "park" {
		t.Fatalf("accepted = %+v; want [park]", seam.accepted)
	}
}

func TestRecordReject_PropagatesToEngine(t *testing.T) {
	seam := &fakeRecommendSeam{}
	adapter := tripplanner.NewRecommendAdapter(seam)

	if err := adapter.RecordReject(context.Background(), "diner"); err != nil {
		t.Fatalf("RecordReject: %v", err)
	}
	if len(seam.rejected) != 1 || seam.rejected[0] != "diner" {
		t.Fatalf("rejected = %+v; want [diner]", seam.rejected)
	}
}

func TestNewRecommendAdapter_NilSeam_Rejected(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("NewRecommendAdapter(nil) should panic or return error sentinel")
		}
	}()
	_ = tripplanner.NewRecommendAdapter(nil)
}

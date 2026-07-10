package tmdb

import (
	"context"
	"errors"
	"testing"
)

type fakeTransport struct {
	searchHits []SearchHit
	searchErr  error
	provSet    ProviderSet
	provErr    error
	lastType   string
	lastID     int
	lastRegion string
}

func (f *fakeTransport) Search(_ context.Context, _, _ string) ([]SearchHit, error) {
	return f.searchHits, f.searchErr
}

func (f *fakeTransport) WatchProviders(_ context.Context, _ string, mediaType string, id int, region string) (ProviderSet, error) {
	f.lastType, f.lastID, f.lastRegion = mediaType, id, region
	return f.provSet, f.provErr
}

type fakeTokenSource string

func (f fakeTokenSource) Token(_ context.Context) (string, error) { return string(f), nil }

type okAttestor struct{}

func (okAttestor) Attest(_ context.Context, _ string) error { return nil }

// TestAdapter_Find_HappyPath proves Search→WatchProviders compose into one merged Result.
func TestAdapter_Find_HappyPath(t *testing.T) {
	tr := &fakeTransport{
		searchHits: []SearchHit{{ID: 1399, MediaType: "tv", Title: "Game of Thrones", Year: 2011}},
		provSet: ProviderSet{
			Flatrate: []Provider{{Name: "HBO Max"}},
			Rent:     []Provider{{Name: "Apple TV"}},
			Buy:      []Provider{{Name: "Apple TV"}},
		},
	}
	ad, err := New(Config{Attestor: okAttestor{}, TokenSource: fakeTokenSource("tok"), Transport: tr, Region: "US"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	r, err := ad.Find(context.Background(), "game of thrones")
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if r.Title != "Game of Thrones" || r.Year != 2011 {
		t.Errorf("title/year = %q/%d", r.Title, r.Year)
	}
	if len(r.Flatrate) != 1 || r.Flatrate[0].Name != "HBO Max" {
		t.Errorf("flatrate = %+v", r.Flatrate)
	}
	if tr.lastRegion != "US" {
		t.Errorf("region = %q, want US", tr.lastRegion)
	}
}

// TestAdapter_Find_NoResults_ReturnsErr — empty search response is not silent.
func TestAdapter_Find_NoResults_ReturnsErr(t *testing.T) {
	tr := &fakeTransport{searchHits: nil}
	ad, _ := New(Config{Attestor: okAttestor{}, TokenSource: fakeTokenSource("t"), Transport: tr, Region: "US"})
	_, err := ad.Find(context.Background(), "asdfqwer")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

// TestAdapter_Find_FirstHit_IsTV_PicksTVProviders — media_type routes the providers RPC.
func TestAdapter_Find_FirstHit_IsTV_PicksTVProviders(t *testing.T) {
	tr := &fakeTransport{
		searchHits: []SearchHit{{ID: 42, MediaType: "tv", Title: "X", Year: 2020}},
	}
	ad, _ := New(Config{Attestor: okAttestor{}, TokenSource: fakeTokenSource("t"), Transport: tr, Region: "US"})
	if _, err := ad.Find(context.Background(), "x"); err != nil {
		t.Fatalf("Find: %v", err)
	}
	if tr.lastType != "tv" || tr.lastID != 42 {
		t.Errorf("provider call = %q/%d, want tv/42", tr.lastType, tr.lastID)
	}
}

// TestAdapter_Find_FirstHit_IsMovie_PicksMovieProviders — symmetric routing case.
func TestAdapter_Find_FirstHit_IsMovie_PicksMovieProviders(t *testing.T) {
	tr := &fakeTransport{
		searchHits: []SearchHit{{ID: 7, MediaType: "movie", Title: "Y", Year: 2019}},
	}
	ad, _ := New(Config{Attestor: okAttestor{}, TokenSource: fakeTokenSource("t"), Transport: tr, Region: "US"})
	if _, err := ad.Find(context.Background(), "y"); err != nil {
		t.Fatalf("Find: %v", err)
	}
	if tr.lastType != "movie" {
		t.Errorf("provider call = %q, want movie", tr.lastType)
	}
}

// TestAdapter_Find_InvalidRegion_RejectsBeforeRPC — region allowlist [A-Z]{2}.
func TestAdapter_Find_InvalidRegion_RejectsBeforeRPC(t *testing.T) {
	tr := &fakeTransport{searchHits: []SearchHit{{ID: 1, MediaType: "movie", Title: "Z"}}}
	ad, _ := New(Config{Attestor: okAttestor{}, TokenSource: fakeTokenSource("t"), Transport: tr, Region: "us"})
	_, err := ad.Find(context.Background(), "z")
	if !errors.Is(err, ErrInvalidRegion) {
		t.Errorf("err = %v, want ErrInvalidRegion", err)
	}
}

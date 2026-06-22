package tmdb

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestTransport_Search_HappyPath proves /search/multi parsing yields the first hit.
func TestTransport_Search_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search/multi" {
			t.Errorf("path = %q, want /search/multi", r.URL.Path)
		}
		if got := r.URL.Query().Get("query"); got != "game of thrones" {
			t.Errorf("query = %q, want %q", got, "game of thrones")
		}
		_, _ = w.Write([]byte(`{"results":[
			{"id":1399,"media_type":"tv","name":"Game of Thrones","first_air_date":"2011-04-17"},
			{"id":999,"media_type":"movie","title":"Other","release_date":"2020-01-01"}
		]}`))
	}))
	defer srv.Close()

	tr := NewHTTPTransport(srv.Client(), srv.URL)
	hits, err := tr.Search(context.Background(), "tok", "game of thrones")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("len = %d, want 2", len(hits))
	}
	h := hits[0]
	if h.ID != 1399 || h.MediaType != "tv" || h.Title != "Game of Thrones" || h.Year != 2011 {
		t.Errorf("first hit = %+v", h)
	}
}

// TestTransport_WatchProviders_FiltersByRegion asserts only the requested region's set is returned.
func TestTransport_WatchProviders_FiltersByRegion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/tv/1399/watch/providers") {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"results":{
			"US":{"flatrate":[{"provider_name":"HBO Max","logo_path":"/us.png"}],"rent":[{"provider_name":"Apple TV"}]},
			"UK":{"flatrate":[{"provider_name":"Sky"}]},
			"CA":{"flatrate":[{"provider_name":"Crave"}]}
		}}`))
	}))
	defer srv.Close()

	tr := NewHTTPTransport(srv.Client(), srv.URL)
	set, err := tr.WatchProviders(context.Background(), "tok", "tv", 1399, "US")
	if err != nil {
		t.Fatalf("WatchProviders: %v", err)
	}
	if len(set.Flatrate) != 1 || set.Flatrate[0].Name != "HBO Max" {
		t.Errorf("flatrate = %+v", set.Flatrate)
	}
	if len(set.Rent) != 1 || set.Rent[0].Name != "Apple TV" {
		t.Errorf("rent = %+v", set.Rent)
	}
}

// TestTransport_AuthHeader_BearerFormat catches a regression that would ship the raw v4 token.
func TestTransport_AuthHeader_BearerFormat(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"results":[]}`))
	}))
	defer srv.Close()

	tr := NewHTTPTransport(srv.Client(), srv.URL)
	if _, err := tr.Search(context.Background(), "eyJabc", "q"); err != nil {
		t.Fatalf("Search: %v", err)
	}
	if got != "Bearer eyJabc" {
		t.Errorf("Authorization = %q, want %q", got, "Bearer eyJabc")
	}
}

// TestTransport_Error_404_NotFound — classified ErrNotFound.
func TestTransport_Error_404_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	tr := NewHTTPTransport(srv.Client(), srv.URL)
	_, err := tr.WatchProviders(context.Background(), "tok", "movie", 1, "US")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

// TestTransport_Error_401_Unauth — classified ErrAuthFailed.
func TestTransport_Error_401_Unauth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	tr := NewHTTPTransport(srv.Client(), srv.URL)
	_, err := tr.Search(context.Background(), "tok", "q")
	if !errors.Is(err, ErrAuthFailed) {
		t.Errorf("err = %v, want ErrAuthFailed", err)
	}
}

// TestTransport_Error_DoesNotLeakToken — failure paths must NOT echo the bearer token.
func TestTransport_Error_DoesNotLeakToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	tr := NewHTTPTransport(srv.Client(), srv.URL)
	_, err := tr.Search(context.Background(), "supersecret-eyJ", "q")
	if err == nil {
		t.Fatal("want error")
	}
	if strings.Contains(err.Error(), "supersecret") || strings.Contains(err.Error(), "eyJ") {
		t.Errorf("error leaked token: %v", err)
	}
}

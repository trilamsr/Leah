package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeOSM serves both Nominatim (/search) and OSRM (/route/v1/...) shapes so
// the trip verb's geocode+route path is hermetic — no live OSM/flights calls.
func fakeOSM(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		// Return a deterministic single hit; lat/lng vary per query so a
		// degenerate "origin==dest" route is not accidentally produced.
		switch {
		case strings.Contains(strings.ToLower(q), "paris"):
			_, _ = w.Write([]byte(`[{"place_id":1,"display_name":"Paris, France","lat":"48.85","lon":"2.35","category":"city"}]`))
		case strings.Contains(strings.ToLower(q), "london"):
			_, _ = w.Write([]byte(`[{"place_id":2,"display_name":"London, UK","lat":"51.50","lon":"-0.12","category":"city"}]`))
		case strings.Contains(strings.ToLower(q), "nowhereville"):
			_, _ = w.Write([]byte(`[]`))
		default:
			_, _ = w.Write([]byte(`[{"place_id":3,"display_name":"Generic Place","lat":"40.0","lon":"-74.0","category":"city"}]`))
		}
	})
	mux.HandleFunc("/route/v1/driving/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":"Ok","routes":[{"geometry":"abc","distance":463000,"duration":18000,"legs":[{"steps":[]}]}]}`))
	})
	return httptest.NewServer(mux)
}

func TestRunTrip_EmptyArgs_ExitsTwo(t *testing.T) {
	var buf bytes.Buffer
	code := runTrip(context.Background(), nil, &buf)
	if code != 2 {
		t.Fatalf("empty args: got exit %d, want 2", code)
	}
}

func TestRunTrip_HappyPath_DestOnly_PrintsDriveOption(t *testing.T) {
	srv := fakeOSM(t)
	defer srv.Close()
	t.Setenv("LEAH_MAPS_AUTO_ATTEST", "1")
	t.Setenv("LEAH_TRIP_OSM_BASE", srv.URL)
	t.Setenv("LEAH_TRIP_DEFAULT_ORIGIN", "London")

	var buf bytes.Buffer
	code := runTrip(context.Background(), []string{"Paris"}, &buf)
	if code != 0 {
		t.Fatalf("happy path: got exit %d, want 0; output=%q", code, buf.String())
	}
	out := buf.String()
	// Drive option must surface distance (km) and duration (h/m) so the
	// operator sees the "rough cost" the spec promises.
	if !strings.Contains(strings.ToLower(out), "drive") {
		t.Errorf("output missing 'drive' line: %q", out)
	}
	if !strings.Contains(out, "463") && !strings.Contains(out, "km") {
		t.Errorf("output missing distance: %q", out)
	}
}

func TestRunTrip_ExplicitOrigin_ArrowSyntax(t *testing.T) {
	srv := fakeOSM(t)
	defer srv.Close()
	t.Setenv("LEAH_MAPS_AUTO_ATTEST", "1")
	t.Setenv("LEAH_TRIP_OSM_BASE", srv.URL)

	var buf bytes.Buffer
	code := runTrip(context.Background(), []string{"London", "->", "Paris"}, &buf)
	if code != 0 {
		t.Fatalf("arrow syntax: got exit %d, want 0; output=%q", code, buf.String())
	}
	out := buf.String()
	if !strings.Contains(out, "London") || !strings.Contains(out, "Paris") {
		t.Errorf("output missing origin/dest: %q", out)
	}
}

func TestRunTrip_NoOrigin_NoEnv_ExitsTwo(t *testing.T) {
	srv := fakeOSM(t)
	defer srv.Close()
	t.Setenv("LEAH_MAPS_AUTO_ATTEST", "1")
	t.Setenv("LEAH_TRIP_OSM_BASE", srv.URL)
	t.Setenv("LEAH_TRIP_DEFAULT_ORIGIN", "")

	var buf bytes.Buffer
	code := runTrip(context.Background(), []string{"Paris"}, &buf)
	if code != 2 {
		t.Fatalf("no origin: got exit %d, want 2", code)
	}
}

func TestRunTrip_UnknownDestination_GeocodeEmpty_ExitsOne(t *testing.T) {
	srv := fakeOSM(t)
	defer srv.Close()
	t.Setenv("LEAH_MAPS_AUTO_ATTEST", "1")
	t.Setenv("LEAH_TRIP_OSM_BASE", srv.URL)
	t.Setenv("LEAH_TRIP_DEFAULT_ORIGIN", "London")

	var buf bytes.Buffer
	code := runTrip(context.Background(), []string{"Nowhereville"}, &buf)
	if code != 1 {
		t.Fatalf("unknown dest: got exit %d, want 1", code)
	}
}

func TestRunTrip_HelpFlag_PrintsUsage(t *testing.T) {
	var buf bytes.Buffer
	code := runTrip(context.Background(), []string{"--help"}, &buf)
	if code != 0 {
		t.Fatalf("--help: got exit %d, want 0", code)
	}
	if !strings.Contains(buf.String(), "usage") {
		t.Errorf("--help: output missing 'usage': %q", buf.String())
	}
}

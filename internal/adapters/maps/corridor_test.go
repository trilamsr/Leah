package maps

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestPolylineDecode_KnownPolyline asserts the canonical Google example decodes
// to the published lat/lng tuples (spec §4 step 1).
func TestPolylineDecode_KnownPolyline(t *testing.T) {
	t.Parallel()
	// Canonical example from Google's encoded-polyline algorithm doc.
	pts, err := decodePolyline("_p~iF~ps|U_ulLnnqC_mqNvxq`@")
	if err != nil {
		t.Fatalf("decodePolyline: %v", err)
	}
	want := []latLng{
		{38.5, -120.2},
		{40.7, -120.95},
		{43.252, -126.453},
	}
	if len(pts) != len(want) {
		t.Fatalf("len(pts) = %d, want %d", len(pts), len(want))
	}
	for i, p := range pts {
		if math.Abs(p.lat-want[i].lat) > 1e-5 || math.Abs(p.lng-want[i].lng) > 1e-5 {
			t.Fatalf("pts[%d] = %+v, want %+v", i, p, want[i])
		}
	}
}

// TestSamplePolyline_RegularSpacing asserts anchors fall ~1km apart along a
// straight north-south synthetic polyline.
func TestSamplePolyline_RegularSpacing(t *testing.T) {
	t.Parallel()
	// Five points 1° apart in latitude ≈ 111km separations; sampling at 1km
	// should yield ~444 anchors (4 segments × ~111 each).
	pts := []latLng{{0, 0}, {1, 0}, {2, 0}, {3, 0}, {4, 0}}
	anchors := samplePolyline(pts, 1000)
	if len(anchors) < 440 || len(anchors) > 450 {
		t.Fatalf("len(anchors) = %d, want ~444", len(anchors))
	}
	// Adjacent anchors should be within 5% of 1km.
	for i := 1; i < len(anchors); i++ {
		d := haversineM(anchors[i-1], anchors[i])
		if d < 900 || d > 1100 {
			t.Fatalf("anchor[%d→%d] spacing = %.0fm, want ~1000m", i-1, i, d)
		}
	}
}

// TestDetourCost_StraightThrough_Zero: a POI on the segment line costs zero.
func TestDetourCost_StraightThrough_Zero(t *testing.T) {
	t.Parallel()
	prev := latLng{0, 0}
	next := latLng{0, 1} // ~111km east
	poi := latLng{0, 0.5}
	d := detourMeters(prev, poi, next)
	if d > 1.0 {
		t.Fatalf("detour = %.2fm, want ~0", d)
	}
}

// TestDetourCost_PerpendicularPOI_ApproxRoundTrip: a POI offset perpendicular
// to a tight anchor pair costs ~2× the perpendicular distance (round trip).
// Anchor pair sits at the POI's projection so the detour is dominated by the
// off-route excursion, not by segment length.
func TestDetourCost_PerpendicularPOI_ApproxRoundTrip(t *testing.T) {
	t.Parallel()
	prev := latLng{0, 0.5}
	next := latLng{0, 0.501} // ~111m east of prev
	poi := latLng{0.00904, 0.5} // 1km north of prev
	d := detourMeters(prev, poi, next)
	if d < 1800 || d > 2200 {
		t.Fatalf("detour = %.0fm, want ~2000m round trip", d)
	}
}

// TestPOIAlongRoute_HappyPath: fake nearby responses across anchors → ranked POIs.
func TestPOIAlongRoute_HappyPath(t *testing.T) {
	t.Parallel()
	// Two unique POIs across anchors; cheaper-detour-higher-rated ranks first.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/place/nearbysearch/json") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		// Return the same two POIs for every anchor — dedup must collapse them.
		_, _ = w.Write([]byte(`{
			"status": "OK",
			"results": [
				{"place_id": "near", "name": "Near", "geometry": {"location": {"lat": 0.0, "lng": 0.5}}, "types": ["restaurant"], "rating": 4.0, "opening_hours": {"open_now": true}},
				{"place_id": "far",  "name": "Far",  "geometry": {"location": {"lat": 0.009, "lng": 0.5}}, "types": ["restaurant"], "rating": 4.8, "opening_hours": {"open_now": true}}
			]
		}`))
	}))
	defer srv.Close()
	att := &fakeAttestor{}
	a := newTestAdapter(t, att, srv)
	// Synthetic ~111km east-west route; encode two endpoints so the adapter's
	// own decoder produces the anchors.
	rt := Route{Polyline: encodeTwoPoint(latLng{0, 0}, latLng{0, 1})}
	places, err := a.POIAlongRoute(context.Background(), rt, CorridorOpts{
		MaxDetourMinutes: 10,
		Categories:       []string{"restaurant"},
		OpenAt:           time.Now(),
	})
	if err != nil {
		t.Fatalf("POIAlongRoute: %v", err)
	}
	if len(places) != 2 {
		t.Fatalf("len(places) = %d, want 2 (deduped)", len(places))
	}
	// "near" has zero detour; "far" has ~2km detour. Even though "far" is
	// rated higher, the score formula must surface a positive ranking for
	// both; assert we got both unique IDs with "near" first (zero detour
	// dominates the rating multiplier).
	if places[0].ID != "near" {
		t.Fatalf("ranking wrong: got %q first, want near", places[0].ID)
	}
	if len(att.scopes) == 0 || att.scopes[0] != ScopePOIAlongRoute {
		t.Fatalf("scopes = %v, want first = %q", att.scopes, ScopePOIAlongRoute)
	}
}

// TestPOIAlongRoute_FiltersOutOverBudget: a POI whose detour exceeds the
// operator's MaxDetourMinutes must be excluded.
func TestPOIAlongRoute_FiltersOutOverBudget(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"status": "OK",
			"results": [
				{"place_id": "ok",  "name": "OK",  "geometry": {"location": {"lat": 0.0,  "lng": 0.5}}, "types": ["restaurant"], "rating": 4.0},
				{"place_id": "bad", "name": "Bad", "geometry": {"location": {"lat": 0.5,  "lng": 0.5}}, "types": ["restaurant"], "rating": 5.0}
			]
		}`))
	}))
	defer srv.Close()
	att := &fakeAttestor{}
	a := newTestAdapter(t, att, srv)
	rt := Route{Polyline: encodeTwoPoint(latLng{0, 0}, latLng{0, 1})}
	// 1 minute budget at default 50 km/h driving speed → ~833m one-way budget;
	// the "bad" POI is ~55km off-route round trip, must be filtered.
	places, err := a.POIAlongRoute(context.Background(), rt, CorridorOpts{
		MaxDetourMinutes: 1,
		Categories:       []string{"restaurant"},
	})
	if err != nil {
		t.Fatalf("POIAlongRoute: %v", err)
	}
	for _, p := range places {
		if p.ID == "bad" {
			t.Fatalf("over-budget POI not filtered: %+v", p)
		}
	}
	if len(places) == 0 {
		t.Fatalf("len(places) = 0, want >=1 (ok must survive)")
	}
}

// TestPOIAlongRoute_AttestationDenied_NoCall: gate-ordering — denial blocks HTTP.
func TestPOIAlongRoute_AttestationDenied_NoCall(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("HTTP MUST NOT be called when attestation denies")
	}))
	defer srv.Close()
	att := &fakeAttestor{err: errors.New("denied")}
	a := newTestAdapter(t, att, srv)
	_, err := a.POIAlongRoute(context.Background(), Route{Polyline: encodeTwoPoint(latLng{0, 0}, latLng{0, 1})}, CorridorOpts{})
	if !errors.Is(err, ErrAttestationDenied) {
		t.Fatalf("err = %v, want ErrAttestationDenied", err)
	}
}

// encodeTwoPoint emits a two-vertex encoded polyline for use in tests. It uses
// the same algorithm as decodePolyline so a round-trip test stays consistent.
func encodeTwoPoint(a, b latLng) string {
	var sb strings.Builder
	enc := func(prev, cur int32) {
		v := cur - prev
		v <<= 1
		if v < 0 {
			v = ^v
		}
		u := uint32(v)
		for u >= 0x20 {
			sb.WriteByte(byte((0x20 | (u & 0x1f)) + 63))
			u >>= 5
		}
		sb.WriteByte(byte(u + 63))
	}
	a0 := int32(math.Round(a.lat * 1e5))
	a1 := int32(math.Round(a.lng * 1e5))
	b0 := int32(math.Round(b.lat * 1e5))
	b1 := int32(math.Round(b.lng * 1e5))
	enc(0, a0)
	enc(0, a1)
	enc(a0, b0)
	enc(a1, b1)
	return sb.String()
}


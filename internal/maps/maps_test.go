package maps

import "testing"

// TestGeocodeZero documents the Geocode zero-value.
func TestGeocodeZero(t *testing.T) {
	var g Geocode
	if g.Lat != 0 || g.Lon != 0 || g.Label != "" {
		t.Fatalf("zero Geocode unexpected: %+v", g)
	}
}

func TestOptionsRequiresJWTPath(t *testing.T) {
	if err := (Options{}).validate(); err == nil {
		t.Fatal("expected error for empty Options")
	}
	if err := (Options{JWTPath: "/tmp/x.p8"}).validate(); err != nil {
		t.Fatalf("valid Options rejected: %v", err)
	}
}

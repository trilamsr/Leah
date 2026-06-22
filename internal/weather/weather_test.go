package weather

import "testing"

// TestForecastZero ensures the Forecast zero-value is meaningful.
func TestForecastZero(t *testing.T) {
	var f Forecast
	if f.Lat != 0 || f.Lon != 0 || f.Condition != "" {
		t.Fatalf("zero Forecast unexpected: %+v", f)
	}
}

// TestOptionsRequiresCoordinates documents the validation contract.
func TestOptionsRequiresCoordinates(t *testing.T) {
	if err := (Options{}).validate(); err == nil {
		t.Fatal("expected error for zero Options (no lat/lon)")
	}
	if err := (Options{Lat: 37.7749, Lon: -122.4194, JWTPath: "/tmp/x.p8"}).validate(); err != nil {
		t.Fatalf("valid Options rejected: %v", err)
	}
}

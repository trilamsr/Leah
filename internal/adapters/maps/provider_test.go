package maps

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSelectProvider_DefaultGoogle: absent config selects Google.
func TestSelectProvider_DefaultGoogle(t *testing.T) {
	t.Parallel()
	got, err := SelectProvider(filepath.Join(t.TempDir(), "absent.json"))
	if err != nil {
		t.Fatalf("SelectProvider: %v", err)
	}
	if got != ProviderGoogle {
		t.Fatalf("provider = %q, want %q", got, ProviderGoogle)
	}
}

// TestSelectProvider_OSMPick: maps.json {"provider":"osm"} selects OSM.
func TestSelectProvider_OSMPick(t *testing.T) {
	t.Parallel()
	p := filepath.Join(t.TempDir(), "maps.json")
	writeFile(t, p, `{"provider":"osm"}`)
	got, err := SelectProvider(p)
	if err != nil {
		t.Fatalf("SelectProvider: %v", err)
	}
	if got != ProviderOSM {
		t.Fatalf("provider = %q, want %q", got, ProviderOSM)
	}
}

// TestSelectProvider_UnknownDefaultsGoogle: a typo'd provider falls back to Google.
func TestSelectProvider_UnknownDefaultsGoogle(t *testing.T) {
	t.Parallel()
	p := filepath.Join(t.TempDir(), "maps.json")
	writeFile(t, p, `{"provider":"mapbox"}`)
	got, err := SelectProvider(p)
	if err != nil {
		t.Fatalf("SelectProvider: %v", err)
	}
	if got != ProviderGoogle {
		t.Fatalf("provider = %q, want %q", got, ProviderGoogle)
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

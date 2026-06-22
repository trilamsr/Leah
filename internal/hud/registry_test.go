package hud_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/trilam/leah/internal/hud"
)

func TestRegistry_ReadsWidgetRegistryJSON(t *testing.T) {
	dir := t.TempDir()
	regPath := filepath.Join(dir, "widget-registry.json")
	body := `{"market":{"refresh":60},"weather":{"refresh":900}}`
	if err := os.WriteFile(regPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := hud.ReadRegistry(regPath)
	if err != nil {
		t.Fatalf("ReadRegistry: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("entries: %d", len(got))
	}
	if got["market"].Refresh != 60 {
		t.Fatalf("market refresh: %d", got["market"].Refresh)
	}
}

func TestRegistry_MissingFileReturnsEmpty(t *testing.T) {
	got, err := hud.ReadRegistry(filepath.Join(t.TempDir(), "absent.json"))
	if err != nil {
		t.Fatalf("ReadRegistry on missing file should be nil err: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty map, got %d", len(got))
	}
}

func TestRegistry_RejectsMalformed(t *testing.T) {
	dir := t.TempDir()
	regPath := filepath.Join(dir, "widget-registry.json")
	if err := os.WriteFile(regPath, []byte(`{not json`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := hud.ReadRegistry(regPath); err == nil {
		t.Fatal("expected error on malformed JSON")
	}
}

// guard against accidentally importing encoding/json in test side only.
var _ = json.RawMessage(nil)

package plugin

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

const validManifestJSON = `{
 "schema_version": 1,
 "id": "com.example.weather",
 "name": "Weather Pro",
 "version": "1.0.0",
 "min_leah": "1.1.0",
 "author": {"name":"Ex","url":"https://example.com"},
 "capabilities": [{"kind":"widget","type":"weather","renderer":"schema-only"}],
 "permissions": {"network":["api.example.com"],"fs.read":[],"fs.write":[],"keychain":["com.example.weather.key"]},
 "ipc_quota": {"rpc_per_minute":60,"stream_bytes_per_minute":524288},
 "ui": {"icon":"Resources/icon.svg","settings_pane":"settings.html"}
}`

func TestParseManifest_Valid(t *testing.T) {
	m, err := ParseManifest([]byte(validManifestJSON))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if m.ID != "com.example.weather" || m.Name != "Weather Pro" || m.Version != "1.0.0" {
		t.Fatalf("identity round-trip lost: %+v", m)
	}
	if m.IPCQuota.RPCPerMinute != 60 || m.IPCQuota.StreamBytesPerMinute != 524288 {
		t.Fatalf("quota round-trip lost: %+v", m.IPCQuota)
	}
	if len(m.Capabilities) != 1 || m.Capabilities[0].Kind != "widget" {
		t.Fatalf("capabilities round-trip lost: %+v", m.Capabilities)
	}
}

func TestParseManifest_RejectsBadJSON(t *testing.T) {
	if _, err := ParseManifest([]byte("{not json")); err == nil {
		t.Fatal("expected error on malformed json")
	}
}

func TestParseManifest_RejectsBadSchemaVersion(t *testing.T) {
	_, err := ParseManifest([]byte(`{"schema_version":2,"id":"x","name":"x","version":"x"}`))
	if !errors.Is(err, ErrManifestUnsupportedSchema) {
		t.Fatalf("expected ErrManifestUnsupportedSchema, got %v", err)
	}
}

func TestParseManifest_RejectsMissingRequired(t *testing.T) {
	cases := []string{
		`{"schema_version":1,"name":"x","version":"x"}`, // no id
		`{"schema_version":1,"id":"x","version":"x"}`,   // no name
		`{"schema_version":1,"id":"x","name":"x"}`,      // no version
		`{"schema_version":1}`,                          // none
	}
	for i, c := range cases {
		if _, err := ParseManifest([]byte(c)); !errors.Is(err, ErrManifestRequiredField) {
			t.Fatalf("case %d: expected ErrManifestRequiredField, got %v", i, err)
		}
	}
}

func TestReadManifest_FromBundle(t *testing.T) {
	dir := t.TempDir()
	contents := filepath.Join(dir, "Contents")
	if err := os.MkdirAll(contents, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(contents, "manifest.json"), []byte(validManifestJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := ReadManifest(dir)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if m.ID != "com.example.weather" {
		t.Fatalf("got id %q", m.ID)
	}
}

func TestReadManifest_MissingFile(t *testing.T) {
	if _, err := ReadManifest(t.TempDir()); err == nil {
		t.Fatal("expected error on missing manifest.json")
	}
}

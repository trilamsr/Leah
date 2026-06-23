package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/trilam/leah/pkg/leahplugin"
)

// TestManifest_Parses guards the on-disk manifest.json against schema drift — the daemon's
// internal/plugin.ParseManifest will refuse anything that fails this same shape.
func TestManifest_Parses(t *testing.T) {
	raw, err := os.ReadFile("manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var mf leahplugin.Manifest
	if err := json.Unmarshal(raw, &mf); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if mf.SchemaVersion != leahplugin.SchemaVersion {
		t.Fatalf("schema_version: got %d want %d", mf.SchemaVersion, leahplugin.SchemaVersion)
	}
	if mf.ID != "com.maydow.weather-pro" {
		t.Fatalf("id: got %q", mf.ID)
	}
	if len(mf.Permissions.Network) != 1 || mf.Permissions.Network[0] != "api.open-meteo.com" {
		t.Fatalf("network ceiling drifted from spec: %v", mf.Permissions.Network)
	}
	if mf.IPCQuota.RPCPerMinute != 60 || mf.IPCQuota.StreamBytesPerMinute != 524288 {
		t.Fatalf("ipc_quota drifted: %+v", mf.IPCQuota)
	}
	var sawTool, sawWidget bool
	for _, c := range mf.Capabilities {
		switch c.Kind {
		case "mcp.tool":
			sawTool = c.Name == "weather.now"
		case "widget":
			sawWidget = c.Type == "weather"
		}
	}
	if !sawTool || !sawWidget {
		t.Fatalf("capabilities missing weather.now or widget: %+v", mf.Capabilities)
	}
}

// TestManifest_MatchesEmittedManifest pins the runtime-emitted manifest (from the Plugin interface)
// to the on-disk JSON — a divergence means the signed bundle would advertise different capabilities
// than the daemon installed.
func TestManifest_MatchesEmittedManifest(t *testing.T) {
	raw, err := os.ReadFile("manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var onDisk leahplugin.Manifest
	if err := json.Unmarshal(raw, &onDisk); err != nil {
		t.Fatal(err)
	}
	emitted := (&weatherPlugin{}).Manifest()
	if onDisk.ID != emitted.ID || onDisk.Version != emitted.Version {
		t.Fatalf("id/version drift: disk=%s/%s emit=%s/%s",
			onDisk.ID, onDisk.Version, emitted.ID, emitted.Version)
	}
}

// recordingHost captures EmitMCPTool + EmitWidget calls so Init's wiring is asserted, not assumed.
type recordingHost struct {
	tools   []leahplugin.MCPTool
	widgets []leahplugin.WidgetSchema
}

func (h *recordingHost) Log(leahplugin.LogLevel, string, ...any) {}
func (h *recordingHost) Keychain() leahplugin.KeychainAccessor   { return nil }
func (h *recordingHost) HTTP() *http.Client                      { return http.DefaultClient }
func (h *recordingHost) EmitMCPTool(t leahplugin.MCPTool) error {
	h.tools = append(h.tools, t)
	return nil
}
func (h *recordingHost) EmitWidget(w leahplugin.WidgetSchema) error {
	h.widgets = append(h.widgets, w)
	return nil
}
func (h *recordingHost) Bus() <-chan leahplugin.HostEvent { return nil }

func TestInit_RegistersToolAndWidget(t *testing.T) {
	h := &recordingHost{}
	if err := (&weatherPlugin{}).Init(context.Background(), h); err != nil {
		t.Fatalf("init: %v", err)
	}
	if len(h.tools) != 1 || h.tools[0].Name != "weather.now" {
		t.Fatalf("tool not registered: %+v", h.tools)
	}
	if len(h.widgets) != 1 || h.widgets[0].Type != "weather" {
		t.Fatalf("widget not registered: %+v", h.widgets)
	}
}

// TestFetchWeather_HappyPath uses httptest so the test does not depend on network reach to open-meteo.
// Pinning the request URL guards against silent param-shape changes (lat/lon → latitude/longitude).
func TestFetchWeather_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "latitude=37") || !strings.Contains(r.URL.RawQuery, "longitude=-122") {
			t.Errorf("query drift: %s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"current":{"temperature_2m":18.3}}`))
	}))
	defer srv.Close()
	orig := openMeteoEndpoint
	openMeteoEndpoint = srv.URL
	defer func() { openMeteoEndpoint = orig }()

	out, err := fetchWeather(context.Background(), srv.Client(), weatherToolInput{Lat: 37, Lon: -122})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := out["current"]; !ok {
		t.Fatalf("missing current: %+v", out)
	}
}

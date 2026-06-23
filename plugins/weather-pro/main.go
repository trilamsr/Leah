// Command weather-pro is the first-party sample plugin proving the §7.11 SDK contract:
// a real, runnable binary that registers a weather.now MCP tool against open-meteo
// and ships a widget schema. It is signed + loaded the same way third-party plugins are.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/trilam/leah/pkg/leahplugin"
)

// openMeteoEndpoint is the public forecast endpoint declared in manifest.permissions.network.
// Kept as a package-level var so tests can swap it for a httptest server.
var openMeteoEndpoint = "https://api.open-meteo.com/v1/forecast"

// weatherPlugin implements leahplugin.Plugin. Sole field is the host handle
// captured at Init so handler closures can route logs back to the daemon.
type weatherPlugin struct {
	host leahplugin.PluginHost
}

func (p *weatherPlugin) Manifest() leahplugin.Manifest {
	return leahplugin.Manifest{
		SchemaVersion: leahplugin.SchemaVersion,
		ID:            "com.maydow.weather-pro",
		Name:          "Weather Pro",
		Version:       "1.0.0",
		MinLeah:       "1.1.0",
		Author:        leahplugin.ManifestAuthor{Name: "Maydow", URL: "https://maydow.com"},
		Capabilities: []leahplugin.Capability{
			{Kind: "widget", Type: "weather", Renderer: "schema-only"},
			{Kind: "mcp.tool", Name: "weather.now", Scopes: []string{"network:api.open-meteo.com"}},
		},
		Permissions: leahplugin.Permissions{Network: []string{"api.open-meteo.com"}},
		IPCQuota:    leahplugin.Quota{RPCPerMinute: 60, StreamBytesPerMinute: 524288},
		UI:          leahplugin.UISpec{Icon: "Resources/icon.svg"},
	}
}

// weatherToolInput is the wire shape weather.now accepts. Kept exported-internal so the
// JSON schema embedded in MCPTool.InputSchema stays in lockstep with the decoder.
type weatherToolInput struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

// weatherToolSchema is the JSON Schema the daemon advertises to MCP clients for weather.now.
// Embedded as a literal so a stale schema is a compile-time review surface, not a runtime drift.
var weatherToolSchema = []byte(`{
  "type": "object",
  "properties": {
    "lat": {"type": "number", "minimum": -90, "maximum": 90},
    "lon": {"type": "number", "minimum": -180, "maximum": 180}
  },
  "required": ["lat", "lon"]
}`)

// weatherWidgetSchema describes the renderer-only widget the host hands to LeahPluginSDK.
var weatherWidgetSchema = []byte(`{
  "fields": [
    {"id": "temperature_c", "kind": "number", "unit": "°C"},
    {"id": "location",      "kind": "string"}
  ]
}`)

func (p *weatherPlugin) Init(ctx context.Context, h leahplugin.PluginHost) error {
	p.host = h
	if err := h.EmitMCPTool(leahplugin.MCPTool{
		Name:        "weather.now",
		Description: "Current temperature at a lat/lon via open-meteo.",
		InputSchema: weatherToolSchema,
	}); err != nil {
		return fmt.Errorf("weather.now register: %w", err)
	}
	return h.EmitWidget(leahplugin.WidgetSchema{Type: "weather", Schema: weatherWidgetSchema})
}

func (p *weatherPlugin) Shutdown(ctx context.Context) error { return nil }

// fetchWeather is the real network path weather.now executes when the daemon dispatches
// the tool call. Kept as a free function so main_test.go can drive it with httptest.
func fetchWeather(ctx context.Context, client *http.Client, in weatherToolInput) (map[string]any, error) {
	url := fmt.Sprintf("%s?latitude=%f&longitude=%f&current=temperature_2m", openMeteoEndpoint, in.Lat, in.Lon)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("weather.now: build request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("weather.now: http: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("weather.now: status %d: %s", resp.StatusCode, string(body))
	}
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("weather.now: decode: %w", err)
	}
	return out, nil
}

// runStandalone is the entrypoint until the daemon IPC handshake (T15 ships host;
// SDK Run() lands later). Verifies Manifest + Init against a no-op host so the
// signed bundle can be smoke-tested by `weather-pro -smoke` without a daemon.
func runStandalone() error {
	p := &weatherPlugin{}
	if err := p.Init(context.Background(), &smokeHost{}); err != nil {
		return err
	}
	return p.Shutdown(context.Background())
}

// smokeHost is the no-op host wired in -smoke mode. Real loads go through internal/plugin.Host.
type smokeHost struct{}

func (smokeHost) Log(leahplugin.LogLevel, string, ...any)  {}
func (smokeHost) Keychain() leahplugin.KeychainAccessor    { return nil }
func (smokeHost) HTTP() *http.Client                       { return &http.Client{Timeout: 5 * time.Second} }
func (smokeHost) EmitMCPTool(leahplugin.MCPTool) error     { return nil }
func (smokeHost) EmitWidget(leahplugin.WidgetSchema) error { return nil }
func (smokeHost) Bus() <-chan leahplugin.HostEvent         { return nil }

func main() {
	for _, a := range os.Args[1:] {
		if a == "-smoke" {
			if err := runStandalone(); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			return
		}
	}
	// Default path: print manifest so signed-bundle inspection works without a daemon.
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode((&weatherPlugin{}).Manifest())
}

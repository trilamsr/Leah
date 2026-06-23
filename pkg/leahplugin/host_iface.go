package leahplugin

import (
	"context"
	"net/http"
)

// LogLevel is the plugin log severity passed to PluginHost.Log.
type LogLevel int

// Plugin log levels mirror slog.Level steps so the host can re-emit unchanged.
const (
	LogDebug LogLevel = -4
	LogInfo  LogLevel = 0
	LogWarn  LogLevel = 4
	LogError LogLevel = 8
)

// MCPTool is the manifest+runtime descriptor a plugin emits via PluginHost.EmitMCPTool.
type MCPTool struct {
	Name        string
	Description string
	InputSchema []byte
}

// WidgetSchema describes a renderer-only widget the plugin contributes to the HUD.
type WidgetSchema struct {
	Type   string
	Schema []byte
}

// HostEvent is one signal on the bus the host pushes to the plugin (config change, shutdown, etc.).
type HostEvent struct {
	Kind    string
	Payload []byte
}

// KeychainAccessor brokers Keychain reads to declared service strings only.
type KeychainAccessor interface {
	Get(ctx context.Context, service string) (string, error)
}

// Plugin is the interface every plugin author implements.
type Plugin interface {
	Manifest() Manifest
	Init(ctx context.Context, h PluginHost) error
	Shutdown(ctx context.Context) error
}

// PluginHost is the SDK-side handle the daemon injects on Init.
type PluginHost interface {
	Log(level LogLevel, msg string, kv ...any)
	Keychain() KeychainAccessor
	HTTP() *http.Client
	EmitMCPTool(t MCPTool) error
	EmitWidget(w WidgetSchema) error
	Bus() <-chan HostEvent
}

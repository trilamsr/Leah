package leahplugin

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// Run is the canonical plugin entry — `func main() { leahplugin.Run(&MyPlugin{}) }`.
// Until the daemon IPC handshake is wired, it runs against a no-op host so signed
// bundles can smoke-test standalone; the daemon will swap the host in-place on the
// next iteration without touching the plugin-author surface.
func Run(p Plugin) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, p, defaultHost()); err != nil {
		os.Exit(1)
	}
}

// run is the testable core — caller owns ctx + host so tests don't fork signals.
func run(ctx context.Context, p Plugin, h PluginHost) error {
	if err := p.Init(ctx, h); err != nil {
		return err
	}
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return p.Shutdown(shutdownCtx)
}

// defaultHost is the standalone no-op host until the daemon IPC handshake replaces it.
// HTTP returns a real client so plugins can still hit declared endpoints in -smoke mode;
// emits succeed silently; bus is closed-empty so plugin selects unblock immediately.
func defaultHost() PluginHost { return noopHost{} }

type noopHost struct{}

func (noopHost) Log(LogLevel, string, ...any)  {}
func (noopHost) Keychain() KeychainAccessor    { return nil }
func (noopHost) HTTP() *http.Client            { return &http.Client{Timeout: 10 * time.Second} }
func (noopHost) EmitMCPTool(MCPTool) error     { return nil }
func (noopHost) EmitWidget(WidgetSchema) error { return nil }
func (noopHost) Bus() <-chan HostEvent {
	ch := make(chan HostEvent)
	close(ch)
	return ch
}

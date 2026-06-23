package leahplugin

import (
	"context"
	"errors"
	"testing"
)

type stubPlugin struct {
	initCalled, shutdownCalled bool
	order                      []string
	initErr                    error
	gotHost                    PluginHost
}

func (s *stubPlugin) Manifest() Manifest { return Manifest{ID: "test"} }
func (s *stubPlugin) Init(_ context.Context, h PluginHost) error {
	s.initCalled = true
	s.gotHost = h
	s.order = append(s.order, "init")
	return s.initErr
}
func (s *stubPlugin) Shutdown(context.Context) error {
	s.shutdownCalled = true
	s.order = append(s.order, "shutdown")
	return nil
}

// Run blocks until ctx is done; verifies clean Init→Shutdown ordering.
func TestRun_LifecycleOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	p := &stubPlugin{}
	done := make(chan error, 1)
	go func() { done <- run(ctx, p, defaultHost()) }()
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("run: %v", err)
	}
	if !p.initCalled || !p.shutdownCalled {
		t.Fatalf("init=%v shutdown=%v", p.initCalled, p.shutdownCalled)
	}
	if len(p.order) != 2 || p.order[0] != "init" || p.order[1] != "shutdown" {
		t.Fatalf("order=%v", p.order)
	}
}

// Init failure aborts before block — Shutdown must not run on a half-initialized plugin.
func TestRun_InitErrorSkipsShutdown(t *testing.T) {
	p := &stubPlugin{initErr: errors.New("boom")}
	err := run(context.Background(), p, defaultHost())
	if err == nil || err.Error() != "boom" {
		t.Fatalf("want boom, got %v", err)
	}
	if p.shutdownCalled {
		t.Fatal("shutdown ran on init failure")
	}
}

// defaultHost must be safe to call on every accessor — plugin authors will use it in -smoke.
func TestDefaultHost_NoopSafe(t *testing.T) {
	h := defaultHost()
	h.Log(LogInfo, "msg")
	if h.HTTP() == nil {
		t.Fatal("HTTP nil")
	}
	if err := h.EmitMCPTool(MCPTool{Name: "x"}); err != nil {
		t.Fatal(err)
	}
	if err := h.EmitWidget(WidgetSchema{Type: "y"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := <-h.Bus(); ok {
		t.Fatal("bus should be closed-empty")
	}
}

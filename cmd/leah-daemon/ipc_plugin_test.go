package main

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/trilam/leah/internal/ipc"
	plug "github.com/trilam/leah/internal/plugin"
	"github.com/trilam/leah/pkg/leahplugin"
)

type stubHost struct {
	listResult []leahplugin.PluginInfo
	logsResult []leahplugin.LogLine

	installErr   error
	uninstallErr error
	enableErr    error
	disableErr   error
	logsErr      error

	gotInstallPath string
	gotInstallID   leahplugin.PluginID
	gotUninstallID leahplugin.PluginID
	gotEnableID    leahplugin.PluginID
	gotDisableID   leahplugin.PluginID
	gotLogsID      leahplugin.PluginID
	gotLogsTail    int
}

func (s *stubHost) Install(_ context.Context, bundlePath string) (leahplugin.PluginID, error) {
	s.gotInstallPath = bundlePath
	return s.gotInstallID, s.installErr
}
func (s *stubHost) Uninstall(_ context.Context, id leahplugin.PluginID) error {
	s.gotUninstallID = id
	return s.uninstallErr
}
func (s *stubHost) Enable(_ context.Context, id leahplugin.PluginID) error {
	s.gotEnableID = id
	return s.enableErr
}
func (s *stubHost) Disable(_ context.Context, id leahplugin.PluginID) error {
	s.gotDisableID = id
	return s.disableErr
}
func (s *stubHost) Reload(_ context.Context, _ leahplugin.PluginID) error { return nil }
func (s *stubHost) List() []leahplugin.PluginInfo                         { return s.listResult }
func (s *stubHost) Logs(_ context.Context, id leahplugin.PluginID, tail int) ([]leahplugin.LogLine, error) {
	s.gotLogsID, s.gotLogsTail = id, tail
	return s.logsResult, s.logsErr
}

var _ plug.Host = (*stubHost)(nil)

func drainPluginFrames(ch <-chan ipc.Frame) []ipc.Frame {
	var out []ipc.Frame
	for f := range ch {
		out = append(out, f)
	}
	return out
}

func reqFrame(t *testing.T, kind string, payload any) ipc.Frame {
	t.Helper()
	var raw json.RawMessage
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		raw = b
	}
	return ipc.Frame{Kind: kind, TurnID: "tp", Seq: 1, Payload: raw}
}

func TestHandlePluginList_ReturnsHostSnapshot(t *testing.T) {
	h := &stubHost{listResult: []leahplugin.PluginInfo{
		{ID: "com.x.a", Name: "A", Version: "0.1", Enabled: true, AttestState: "ok"},
		{ID: "com.x.b", Name: "B", Version: "0.2", Enabled: false, AttestState: "stale"},
	}}
	got := drainPluginFrames(handlePluginList(reqFrame(t, ipc.KindPluginList, nil), h))
	if len(got) != 1 || got[0].Kind != ipc.KindPluginList {
		t.Fatalf("want one plugin.list frame: %+v", got)
	}
	var body struct {
		Plugins []struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			Version     string `json:"version"`
			Enabled     bool   `json:"enabled"`
			AttestState string `json:"attest_state"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal(got[0].Payload, &body); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if len(body.Plugins) != 2 || body.Plugins[0].ID != "com.x.a" || body.Plugins[1].Enabled {
		t.Fatalf("payload mismatch: %+v", body)
	}
}

func TestHandlePluginList_NilHost(t *testing.T) {
	got := drainPluginFrames(handlePluginList(reqFrame(t, ipc.KindPluginList, nil), nil))
	if len(got) != 1 || got[0].Kind != ipc.KindError {
		t.Fatalf("want error frame: %+v", got)
	}
}

func TestHandlePluginInstall_OK(t *testing.T) {
	h := &stubHost{gotInstallID: "com.x.a"}
	frame := reqFrame(t, ipc.KindPluginInstall, map[string]string{"bundle_path": "/tmp/a.leahplugin"})
	got := drainPluginFrames(handlePluginInstall(context.Background(), frame, h))
	if len(got) != 1 || got[0].Kind != ipc.KindPluginInstall {
		t.Fatalf("want plugin.install frame: %+v", got)
	}
	var body struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(got[0].Payload, &body); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if body.ID != "com.x.a" || h.gotInstallPath != "/tmp/a.leahplugin" {
		t.Fatalf("install mismatch: id=%q gotPath=%q", body.ID, h.gotInstallPath)
	}
}

func TestHandlePluginInstall_AttestFailedSurfacesAsError(t *testing.T) {
	h := &stubHost{installErr: plug.ErrPluginAttestFailed}
	frame := reqFrame(t, ipc.KindPluginInstall, map[string]string{"bundle_path": "/tmp/bad"})
	got := drainPluginFrames(handlePluginInstall(context.Background(), frame, h))
	if len(got) != 1 || got[0].Kind != ipc.KindError {
		t.Fatalf("want error frame: %+v", got)
	}
}

func TestHandlePluginInstall_BadPayload(t *testing.T) {
	frame := ipc.Frame{Kind: ipc.KindPluginInstall, TurnID: "tp", Seq: 1, Payload: json.RawMessage("{")}
	got := drainPluginFrames(handlePluginInstall(context.Background(), frame, &stubHost{}))
	if len(got) != 1 || got[0].Kind != ipc.KindError {
		t.Fatalf("want error frame: %+v", got)
	}
}

func TestHandlePluginEnable_OK(t *testing.T) {
	h := &stubHost{}
	frame := reqFrame(t, ipc.KindPluginEnable, map[string]string{"id": "com.x.a"})
	got := drainPluginFrames(handlePluginEnable(context.Background(), frame, h))
	if len(got) != 1 || got[0].Kind != ipc.KindPluginEnable {
		t.Fatalf("want plugin.enable frame: %+v", got)
	}
	if h.gotEnableID != "com.x.a" {
		t.Fatalf("id not routed: %q", h.gotEnableID)
	}
}

func TestHandlePluginEnable_NotFound(t *testing.T) {
	h := &stubHost{enableErr: plug.ErrPluginNotFound}
	frame := reqFrame(t, ipc.KindPluginEnable, map[string]string{"id": "nope"})
	got := drainPluginFrames(handlePluginEnable(context.Background(), frame, h))
	if len(got) != 1 || got[0].Kind != ipc.KindError {
		t.Fatalf("want error frame: %+v", got)
	}
}

func TestHandlePluginDisable_OK(t *testing.T) {
	h := &stubHost{}
	frame := reqFrame(t, ipc.KindPluginDisable, map[string]string{"id": "com.x.a"})
	got := drainPluginFrames(handlePluginDisable(context.Background(), frame, h))
	if len(got) != 1 || got[0].Kind != ipc.KindPluginDisable || h.gotDisableID != "com.x.a" {
		t.Fatalf("disable mismatch: %+v id=%q", got, h.gotDisableID)
	}
}

func TestHandlePluginUninstall_OK(t *testing.T) {
	h := &stubHost{}
	frame := reqFrame(t, ipc.KindPluginUninstall, map[string]string{"id": "com.x.a"})
	got := drainPluginFrames(handlePluginUninstall(context.Background(), frame, h))
	if len(got) != 1 || got[0].Kind != ipc.KindPluginUninstall || h.gotUninstallID != "com.x.a" {
		t.Fatalf("uninstall mismatch: %+v id=%q", got, h.gotUninstallID)
	}
}

func TestHandlePluginUninstall_Error(t *testing.T) {
	h := &stubHost{uninstallErr: errors.New("boom")}
	frame := reqFrame(t, ipc.KindPluginUninstall, map[string]string{"id": "com.x.a"})
	got := drainPluginFrames(handlePluginUninstall(context.Background(), frame, h))
	if len(got) != 1 || got[0].Kind != ipc.KindError {
		t.Fatalf("want error frame: %+v", got)
	}
}

func TestHandlePluginLogs_OK(t *testing.T) {
	h := &stubHost{logsResult: []leahplugin.LogLine{
		{At: 100, Level: leahplugin.LogInfo, Msg: "hello"},
		{At: 200, Level: leahplugin.LogError, Msg: "oops"},
	}}
	frame := reqFrame(t, ipc.KindPluginLogs, map[string]any{"id": "com.x.a", "tail": 50})
	got := drainPluginFrames(handlePluginLogs(context.Background(), frame, h))
	if len(got) != 1 || got[0].Kind != ipc.KindPluginLogs {
		t.Fatalf("want plugin.logs frame: %+v", got)
	}
	if h.gotLogsID != "com.x.a" || h.gotLogsTail != 50 {
		t.Fatalf("logs req mismatch: id=%q tail=%d", h.gotLogsID, h.gotLogsTail)
	}
	var body struct {
		Lines []struct {
			TS    int64  `json:"ts"`
			Level int    `json:"level"`
			Msg   string `json:"msg"`
		} `json:"lines"`
	}
	if err := json.Unmarshal(got[0].Payload, &body); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if len(body.Lines) != 2 || body.Lines[0].TS != 100 || body.Lines[1].Msg != "oops" {
		t.Fatalf("lines mismatch: %+v", body)
	}
}

func TestHandlePluginLogs_HostError(t *testing.T) {
	h := &stubHost{logsErr: errors.New("db gone")}
	frame := reqFrame(t, ipc.KindPluginLogs, map[string]any{"id": "com.x.a"})
	got := drainPluginFrames(handlePluginLogs(context.Background(), frame, h))
	if len(got) != 1 || got[0].Kind != ipc.KindError {
		t.Fatalf("want error frame: %+v", got)
	}
}

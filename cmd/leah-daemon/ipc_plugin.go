package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/trilam/leah/internal/ipc"
	plug "github.com/trilam/leah/internal/plugin"
	"github.com/trilam/leah/pkg/leahplugin"
)

// pluginInfoWire is the operator-facing snapshot wire form. Mirrors
// leahplugin.PluginInfo with explicit JSON tags so PluginsPane decoders
// don't depend on Go default field-name casing.
type pluginInfoWire struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Enabled     bool   `json:"enabled"`
	AttestState string `json:"attest_state"`
}

type pluginLogWire struct {
	TS    int64  `json:"ts"`
	Level int    `json:"level"`
	Msg   string `json:"msg"`
}

func handlePluginList(req ipc.Frame, host plug.Host) <-chan ipc.Frame {
	if host == nil {
		return errFrame(req, "plugin host unavailable")
	}
	infos := host.List()
	out := make([]pluginInfoWire, 0, len(infos))
	for _, p := range infos {
		out = append(out, pluginInfoWire{
			ID:          string(p.ID),
			Name:        p.Name,
			Version:     p.Version,
			Enabled:     p.Enabled,
			AttestState: p.AttestState,
		})
	}
	return okFrame(req, ipc.KindPluginList, map[string]any{"plugins": out})
}

func handlePluginInstall(ctx context.Context, req ipc.Frame, host plug.Host) <-chan ipc.Frame {
	if host == nil {
		return errFrame(req, "plugin host unavailable")
	}
	var p struct {
		BundlePath string `json:"bundle_path"`
	}
	if err := json.Unmarshal(req.Payload, &p); err != nil {
		return errFrame(req, fmt.Sprintf("bad payload: %v", err))
	}
	id, err := host.Install(ctx, p.BundlePath)
	if err != nil {
		return errFrame(req, err.Error())
	}
	return okFrame(req, ipc.KindPluginInstall, map[string]string{"id": string(id)})
}

func handlePluginEnable(ctx context.Context, req ipc.Frame, host plug.Host) <-chan ipc.Frame {
	return handlePluginToggle(ctx, req, host, ipc.KindPluginEnable, true)
}

func handlePluginDisable(ctx context.Context, req ipc.Frame, host plug.Host) <-chan ipc.Frame {
	return handlePluginToggle(ctx, req, host, ipc.KindPluginDisable, false)
}

func handlePluginToggle(ctx context.Context, req ipc.Frame, host plug.Host, kind string, enable bool) <-chan ipc.Frame {
	if host == nil {
		return errFrame(req, "plugin host unavailable")
	}
	var p struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(req.Payload, &p); err != nil {
		return errFrame(req, fmt.Sprintf("bad payload: %v", err))
	}
	var err error
	if enable {
		err = host.Enable(ctx, leahplugin.PluginID(p.ID))
	} else {
		err = host.Disable(ctx, leahplugin.PluginID(p.ID))
	}
	if err != nil {
		return errFrame(req, err.Error())
	}
	return okFrame(req, kind, map[string]any{"ok": true})
}

func handlePluginUninstall(ctx context.Context, req ipc.Frame, host plug.Host) <-chan ipc.Frame {
	if host == nil {
		return errFrame(req, "plugin host unavailable")
	}
	var p struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(req.Payload, &p); err != nil {
		return errFrame(req, fmt.Sprintf("bad payload: %v", err))
	}
	if err := host.Uninstall(ctx, leahplugin.PluginID(p.ID)); err != nil {
		return errFrame(req, err.Error())
	}
	return okFrame(req, ipc.KindPluginUninstall, map[string]any{"ok": true})
}

func handlePluginLogs(ctx context.Context, req ipc.Frame, host plug.Host) <-chan ipc.Frame {
	if host == nil {
		return errFrame(req, "plugin host unavailable")
	}
	var p struct {
		ID   string `json:"id"`
		Tail int    `json:"tail"`
	}
	if err := json.Unmarshal(req.Payload, &p); err != nil {
		return errFrame(req, fmt.Sprintf("bad payload: %v", err))
	}
	lines, err := host.Logs(ctx, leahplugin.PluginID(p.ID), p.Tail)
	if err != nil {
		return errFrame(req, err.Error())
	}
	wire := make([]pluginLogWire, 0, len(lines))
	for _, l := range lines {
		wire = append(wire, pluginLogWire{TS: l.At, Level: int(l.Level), Msg: l.Msg})
	}
	return okFrame(req, ipc.KindPluginLogs, map[string]any{"lines": wire})
}

func okFrame(req ipc.Frame, kind string, body any) <-chan ipc.Frame {
	out := make(chan ipc.Frame, 1)
	payload, _ := json.Marshal(body)
	out <- ipc.Frame{Kind: kind, TurnID: req.TurnID, Seq: 1, Payload: payload}
	close(out)
	return out
}

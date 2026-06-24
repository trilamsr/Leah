package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

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

// pluginIDRe constrains plugin IDs to reverse-DNS-style labels across IPC
// + sqlstore rows; rejects path-traversal, control chars, and SQL meta
// before host hits. Each dot-separated label is [a-z0-9][a-z0-9-]* and the
// overall length is capped at 64 to bound row size.
var pluginIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*(\.[a-z0-9][a-z0-9-]*)*$`)

// logTailMaxBytes caps the serialized log slice; an attacker passing
// tail=MaxInt would otherwise allocate unbounded memory in the host.
const logTailMaxBytes = 1 << 20 // 1 MiB

func validatePluginID(id string) error {
	if len(id) < 3 || len(id) > 64 {
		return fmt.Errorf("invalid plugin id")
	}
	if !pluginIDRe.MatchString(id) {
		return fmt.Errorf("invalid plugin id")
	}
	return nil
}

// validateBundlePath rejects paths outside the daemon's plugin root —
// without this an Install caller could pass `/etc/passwd` or `../../foo`
// and the host would resolve absolute + Stat it.
func validateBundlePath(bundlePath, pluginRoot string) (string, error) {
	if pluginRoot == "" {
		return "", errors.New("plugin root unconfigured")
	}
	abs, err := filepath.Abs(bundlePath)
	if err != nil {
		return "", fmt.Errorf("resolve bundle path: %w", err)
	}
	rootAbs, err := filepath.Abs(pluginRoot)
	if err != nil {
		return "", fmt.Errorf("resolve plugin root: %w", err)
	}
	// Trailing separator prevents `/root-evil` matching `/root`.
	prefix := rootAbs + string(filepath.Separator)
	if abs != rootAbs && !strings.HasPrefix(abs, prefix) {
		return "", errors.New("bundle path escapes plugin root")
	}
	return abs, nil
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

func handlePluginInstall(ctx context.Context, req ipc.Frame, host plug.Host, pluginRoot string) <-chan ipc.Frame {
	if host == nil {
		return errFrame(req, "plugin host unavailable")
	}
	var p struct {
		BundlePath string `json:"bundle_path"`
	}
	if err := json.Unmarshal(req.Payload, &p); err != nil {
		return errFrame(req, fmt.Sprintf("bad payload: %v", err))
	}
	abs, err := validateBundlePath(p.BundlePath, pluginRoot)
	if err != nil {
		return errFrame(req, err.Error())
	}
	id, err := host.Install(ctx, abs)
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
	if err := validatePluginID(p.ID); err != nil {
		return errFrame(req, err.Error())
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
	if err := validatePluginID(p.ID); err != nil {
		return errFrame(req, err.Error())
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
	if err := validatePluginID(p.ID); err != nil {
		return errFrame(req, err.Error())
	}
	lines, err := host.Logs(ctx, leahplugin.PluginID(p.ID), p.Tail)
	if err != nil {
		return errFrame(req, err.Error())
	}
	wire := make([]pluginLogWire, 0, len(lines))
	var bytes int
	for _, l := range lines {
		bytes += len(l.Msg) + 16 // approx ts+level overhead
		if bytes > logTailMaxBytes {
			break
		}
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

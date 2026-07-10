package main

import (
	"context"

	"github.com/trilam/leah/internal/platform/ipc"
)

type VisionIPC interface {
	Snap(ctx context.Context, req ipc.Frame) (<-chan ipc.Frame, error)
	StreamStart(ctx context.Context, req ipc.Frame) (<-chan ipc.Frame, error)
	StreamFrame(ctx context.Context, req ipc.Frame) (<-chan ipc.Frame, error)
}

type SyncIPC interface {
	PeerList(ctx context.Context, req ipc.Frame) (<-chan ipc.Frame, error)
	PairStart(ctx context.Context, req ipc.Frame) (<-chan ipc.Frame, error)
	PairAck(ctx context.Context, req ipc.Frame) (<-chan ipc.Frame, error)
}

type RecommendIPC interface {
	List(ctx context.Context, req ipc.Frame) (<-chan ipc.Frame, error)
	Apply(ctx context.Context, req ipc.Frame) (<-chan ipc.Frame, error)
	Dismiss(ctx context.Context, req ipc.Frame) (<-chan ipc.Frame, error)
	AntiAdd(ctx context.Context, req ipc.Frame) (<-chan ipc.Frame, error)
	AntiList(ctx context.Context, req ipc.Frame) (<-chan ipc.Frame, error)
}

type PluginIPC interface {
	List(ctx context.Context, req ipc.Frame) (<-chan ipc.Frame, error)
	Install(ctx context.Context, req ipc.Frame) (<-chan ipc.Frame, error)
	Enable(ctx context.Context, req ipc.Frame) (<-chan ipc.Frame, error)
	Disable(ctx context.Context, req ipc.Frame) (<-chan ipc.Frame, error)
	Uninstall(ctx context.Context, req ipc.Frame) (<-chan ipc.Frame, error)
	Logs(ctx context.Context, req ipc.Frame) (<-chan ipc.Frame, error)
}

type A2AIPC interface {
	PeerList(ctx context.Context, req ipc.Frame) (<-chan ipc.Frame, error)
	PairStart(ctx context.Context, req ipc.Frame) (<-chan ipc.Frame, error)
	PeerPause(ctx context.Context, req ipc.Frame) (<-chan ipc.Frame, error)
	PeerUnpair(ctx context.Context, req ipc.Frame) (<-chan ipc.Frame, error)
}

type IPCDeps struct {
	Vision    VisionIPC
	Sync      SyncIPC
	Recommend RecommendIPC
	Plugin    PluginIPC
	A2A       A2AIPC
}

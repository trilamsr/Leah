package main

import (
	"context"
	"database/sql"
	"errors"

	"github.com/trilam/leah/internal/platform/ipc"
	"github.com/trilam/leah/internal/platform/sync/discovery"
)

type a2aIPCAdapter struct {
	srv a2aServer
}

func (a *a2aIPCAdapter) PeerList(ctx context.Context, req ipc.Frame) (<-chan ipc.Frame, error) {
	return handleA2APeerList(ctx, req, a.srv)
}

func (a *a2aIPCAdapter) PairStart(ctx context.Context, req ipc.Frame) (<-chan ipc.Frame, error) {
	return handleA2APairStart(ctx, req, a.srv)
}

func (a *a2aIPCAdapter) PeerPause(ctx context.Context, req ipc.Frame) (<-chan ipc.Frame, error) {
	return handleA2APeerPause(ctx, req, a.srv)
}

func (a *a2aIPCAdapter) PeerUnpair(ctx context.Context, req ipc.Frame) (<-chan ipc.Frame, error) {
	return handleA2APeerUnpair(ctx, req, a.srv)
}

func newA2AIPCAdapter(srv a2aServer) A2AIPC {
	if srv == nil {
		return nil
	}
	return &a2aIPCAdapter{srv: srv}
}

type syncIPCAdapter struct {
	eng     syncEngine
	pending pendingPairs
	db      *sql.DB
}

func (s *syncIPCAdapter) PeerList(ctx context.Context, req ipc.Frame) (<-chan ipc.Frame, error) {
	return handleSyncPeerList(ctx, req, s.eng)
}

func (s *syncIPCAdapter) PairStart(ctx context.Context, req ipc.Frame) (<-chan ipc.Frame, error) {
	return handleSyncPairStart(ctx, req, s.eng, s.pending)
}

func (s *syncIPCAdapter) PairAck(ctx context.Context, req ipc.Frame) (<-chan ipc.Frame, error) {
	return handleSyncPairAck(ctx, req, s.db, s.pending)
}

func newSyncIPCAdapter(eng syncEngine, db *sql.DB) SyncIPC {
	if eng == nil || db == nil {
		return nil
	}
	return &syncIPCAdapter{eng: eng, pending: newPendingPairs(), db: db}
}

type discoveryBrowseEngine struct {
	disc discovery.Discovery
}

func (d *discoveryBrowseEngine) Browse(ctx context.Context) ([]discovery.Peer, error) {
	if d.disc == nil {
		return nil, errors.New("discovery not started")
	}
	ch, err := d.disc.Browse(ctx)
	if err != nil {
		return nil, err
	}
	var peers []discovery.Peer
	for {
		select {
		case <-ctx.Done():
			return peers, nil
		case p, ok := <-ch:
			if !ok {
				return peers, nil
			}
			peers = append(peers, p)
		}
	}
}

func (d *discoveryBrowseEngine) PairStart(_ context.Context, _ string) (PairResult, error) {
	return PairResult{}, errors.New("pair.start: mTLS handshake not yet wired")
}

func newDiscoveryEngine(disc discovery.Discovery) syncEngine {
	if disc == nil {
		return nil
	}
	return &discoveryBrowseEngine{disc: disc}
}

var _ syncEngine = (*discoveryBrowseEngine)(nil)

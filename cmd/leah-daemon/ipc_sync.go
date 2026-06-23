package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/trilam/leah/internal/ipc"
	"github.com/trilam/leah/internal/sync/discovery"
)

// syncEngine is the daemon-side surface the IPC sync handlers depend on.
// Browse returns the current snapshot of LAN peers; PairStart drives the
// mTLS+OTP handshake against the peer whose advertised OTP matches and
// returns the device id + (peer name, fingerprint) needed by PairAck;
// PairAck commits a sync_peer row. Split so tests can stub each leg
// independently and the composition root in main.go can wire the real
// discovery.Discovery + pair handshake.
type syncEngine interface {
	Browse(ctx context.Context) ([]discovery.Peer, error)
	PairStart(ctx context.Context, otp string) (PairResult, error)
}

// PairResult is the outcome of PairStart — handed back to the HUD on
// sync.pair.start and re-presented on sync.pair.ack so the daemon can
// persist the fingerprint without keeping per-OTP state across frames.
type PairResult struct {
	PeerID      string
	Name        string
	Fingerprint []byte
	Status      string
}

// browseTimeout caps how long sync.peer.list waits for the Browse channel
// before returning the snapshot. Bonjour browse is open-ended; the HUD
// needs a bounded response.
const browseTimeout = 250 * time.Millisecond

// handleSyncPeerList drains the current Browse snapshot and replies once.
// Errors close the conn — return them via an error frame instead so the
// HUD sees a structured failure rather than a dropped socket.
func handleSyncPeerList(ctx context.Context, req ipc.Frame, eng syncEngine) (<-chan ipc.Frame, error) {
	if eng == nil {
		return syncErr(req, "sync engine unavailable"), nil
	}
	browseCtx, cancel := context.WithTimeout(ctx, browseTimeout)
	defer cancel()
	peers, err := eng.Browse(browseCtx)
	if err != nil {
		return syncErr(req, fmt.Sprintf("browse: %v", err)), nil
	}
	out := make(chan ipc.Frame, 1)
	wire := make([]map[string]any, 0, len(peers))
	for _, p := range peers {
		wire = append(wire, map[string]any{
			"id":       string(p.ID()),
			"addr":     p.Endpoint().String(),
			"lastSeen": p.LastSeenAt().UnixMilli(),
			"status":   p.Status().String(),
		})
	}
	payload, _ := json.Marshal(map[string]any{"peers": wire})
	out <- ipc.Frame{Kind: ipc.KindSyncPeerList, TurnID: req.TurnID, Seq: 1, Payload: payload}
	close(out)
	return out, nil
}

// handleSyncPairStart runs the OTP handshake against the matching peer.
// Empty OTP and engine-reported lock-out both come back as error frames
// (operator UI surfaces them inline); success returns peer_id + status.
func handleSyncPairStart(ctx context.Context, req ipc.Frame, eng syncEngine, pending pendingPairs) (<-chan ipc.Frame, error) {
	if eng == nil {
		return syncErr(req, "sync engine unavailable"), nil
	}
	var p struct {
		OTP string `json:"otp_string"`
	}
	if len(req.Payload) > 0 && string(req.Payload) != "null" {
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return syncErr(req, fmt.Sprintf("bad payload: %v", err)), nil
		}
	}
	if p.OTP == "" {
		return syncErr(req, "otp_string required"), nil
	}
	res, err := eng.PairStart(ctx, p.OTP)
	if err != nil {
		return syncErr(req, fmt.Sprintf("pair start: %v", err)), nil
	}
	pending.put(res)
	out := make(chan ipc.Frame, 1)
	payload, _ := json.Marshal(map[string]string{"peer_id": res.PeerID, "status": res.Status})
	out <- ipc.Frame{Kind: ipc.KindSyncPairStart, TurnID: req.TurnID, Seq: 1, Payload: payload}
	close(out)
	return out, nil
}

// handleSyncPairAck commits the sync_peer row using the PairResult cached
// by handleSyncPairStart. Without the cache the daemon would either
// re-run the handshake (operator types OTP twice) or trust ack-supplied
// fingerprint bytes (defeats the pin). ON CONFLICT keeps re-ack idempotent.
func handleSyncPairAck(_ context.Context, req ipc.Frame, db *sql.DB, pending pendingPairs) (<-chan ipc.Frame, error) {
	var p struct {
		PeerID string `json:"peer_id"`
	}
	if len(req.Payload) > 0 && string(req.Payload) != "null" {
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return syncErr(req, fmt.Sprintf("bad payload: %v", err)), nil
		}
	}
	if p.PeerID == "" {
		return syncErr(req, "peer_id required"), nil
	}
	res, ok := pending.take(p.PeerID)
	if !ok {
		return syncErr(req, "no pending pair for peer_id (call sync.pair.start first)"), nil
	}
	if db == nil {
		return syncErr(req, "db unavailable"), nil
	}
	_, err := db.Exec(`INSERT INTO sync_peer(id,name,paired_at,paused,last_seen_at,fingerprint)
	    VALUES(?,?,?,0,?,?)
	    ON CONFLICT(id) DO UPDATE SET name=excluded.name, fingerprint=excluded.fingerprint, last_seen_at=excluded.last_seen_at`,
		res.PeerID, res.Name, time.Now().UnixMilli(), time.Now().UnixMilli(), res.Fingerprint)
	if err != nil {
		return syncErr(req, fmt.Sprintf("insert sync_peer: %v", err)), nil
	}
	out := make(chan ipc.Frame, 1)
	payload, _ := json.Marshal(map[string]bool{"ok": true})
	out <- ipc.Frame{Kind: ipc.KindSyncPairAck, TurnID: req.TurnID, Seq: 1, Payload: payload}
	close(out)
	return out, nil
}

func syncErr(req ipc.Frame, msg string) <-chan ipc.Frame {
	out := make(chan ipc.Frame, 1)
	payload, _ := json.Marshal(map[string]string{"error": msg})
	out <- ipc.Frame{Kind: ipc.KindError, TurnID: req.TurnID, Seq: 1, Payload: payload}
	close(out)
	return out
}

// pendingPairs holds PairResults between sync.pair.start and sync.pair.ack
// so the second frame can commit without re-running the handshake.
type pendingPairs interface {
	put(PairResult)
	take(peerID string) (PairResult, bool)
}

// ErrNoSyncEngine is returned by the composition root when sync handlers
// fire but the daemon booted without a discovery+pair stack (e.g. Linux CI).
var ErrNoSyncEngine = errors.New("sync engine unavailable")

// pendingPairsMap is the production pendingPairs — a process-local registry
// keyed by peer id. No TTL: handshake→ack is human-paced (operator hits
// "Confirm" within seconds) and the daemon restart drops the map anyway.
type pendingPairsMap struct {
	mu sync.Mutex
	m  map[string]PairResult
}

func newPendingPairs() *pendingPairsMap {
	return &pendingPairsMap{m: map[string]PairResult{}}
}

func (p *pendingPairsMap) put(r PairResult) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.m[r.PeerID] = r
}

func (p *pendingPairsMap) take(peerID string) (PairResult, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	r, ok := p.m[peerID]
	if ok {
		delete(p.m, peerID)
	}
	return r, ok
}

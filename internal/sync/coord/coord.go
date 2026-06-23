package coord

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/trilam/leah/internal/sync/crdt"
)

// SyncEventKind enumerates lifecycle events surfaced via Subscribe (§2.4.1).
type SyncEventKind int

const (
	EventDiscovered SyncEventKind = iota
	EventPaired
	EventDeltaApplied
	EventConflict
	EventDisconnected
)

// SyncEvent is one observable transition for HUD toasts + Settings ledger (§2.4.1).
type SyncEvent struct {
	Kind  SyncEventKind
	Peer  crdt.Peer
	Stats crdt.DeltaStats
}

// SyncCoordinator is the daemon-side multi-device sync surface (§2.4.1).
type SyncCoordinator interface {
	Pair(ctx context.Context, otp string) (crdt.Peer, error)
	Unpair(ctx context.Context, p crdt.Peer) error
	Pause(ctx context.Context, p crdt.Peer) error
	Resume(ctx context.Context, p crdt.Peer) error
	Subscribe() <-chan SyncEvent
}

// Pairer is the T10-side transport hook the coordinator delegates the OTP +
// mTLS handshake to. Coord owns CRDT semantics; transport owns network.
type Pairer interface {
	Pair(ctx context.Context, otp string) (crdt.Peer, error)
	Disconnect(ctx context.Context, p crdt.Peer) error
}

// Coord is the default SyncCoordinator. It composes:
//   - a Pairer (T10 transport),
//   - a CRDT log (this package),
//   - an Outbox for offline queueing.
//
// Pause flips an in-memory bit per peer; ApplyRemote and EmitFor short-circuit while
// the bit is set, so a kill-switch is one mutex-protected map write — no protocol
// renegotiation, no torn replication state on resume (§2.6 "Pause as kill-switch").
type Coord struct {
	pairer Pairer
	log    crdt.CRDT
	outbox *Outbox

	mu     sync.RWMutex
	paused map[crdt.DeviceID]bool
	peers  map[crdt.DeviceID]crdt.Peer

	subsMu sync.RWMutex
	subs   []chan SyncEvent
}

// NewCoord wires the three dependencies. Coord is safe for concurrent use.
func NewCoord(p Pairer, log crdt.CRDT, outbox *Outbox) *Coord {
	return &Coord{
		pairer: p,
		log:    log,
		outbox: outbox,
		paused: map[crdt.DeviceID]bool{},
		peers:  map[crdt.DeviceID]crdt.Peer{},
	}
}

// Pair delegates the OTP handshake to the transport then records the new peer so
// future Pause/Resume/Unpair are O(1) lookups.
func (c *Coord) Pair(ctx context.Context, otp string) (crdt.Peer, error) {
	if c.pairer == nil {
		return nil, errors.New("no pairer wired")
	}
	p, err := c.pairer.Pair(ctx, otp)
	if err != nil {
		return nil, fmt.Errorf("pair: %w", err)
	}
	c.mu.Lock()
	c.peers[p.ID()] = p
	delete(c.paused, p.ID())
	c.mu.Unlock()
	c.emit(SyncEvent{Kind: EventPaired, Peer: p})
	return p, nil
}

// Unpair tears down the transport session and forgets the peer. The shared keychain
// secret nuke (§2.6) lives in the transport layer, not here.
func (c *Coord) Unpair(ctx context.Context, p crdt.Peer) error {
	if c.pairer != nil {
		if err := c.pairer.Disconnect(ctx, p); err != nil {
			return fmt.Errorf("disconnect: %w", err)
		}
	}
	c.mu.Lock()
	delete(c.peers, p.ID())
	delete(c.paused, p.ID())
	c.mu.Unlock()
	c.emit(SyncEvent{Kind: EventDisconnected, Peer: p})
	return nil
}

// Pause flips the per-peer kill switch. Subsequent ApplyRemote + EmitFor calls
// return ErrPaused without touching the log or outbox.
func (c *Coord) Pause(ctx context.Context, p crdt.Peer) error {
	c.mu.Lock()
	c.paused[p.ID()] = true
	c.mu.Unlock()
	return nil
}

// Resume clears the kill switch. The next delta from this peer replays from the
// last lamport recorded in sync_clock (§2.6).
func (c *Coord) Resume(ctx context.Context, p crdt.Peer) error {
	c.mu.Lock()
	delete(c.paused, p.ID())
	c.mu.Unlock()
	return nil
}

// ErrPaused signals the per-peer kill switch is engaged.
var ErrPaused = errors.New("peer paused")

// ApplyRemote routes a remote batch through the CRDT log and surfaces the
// resulting stats as either EventDeltaApplied or EventConflict (§2.4.1).
func (c *Coord) ApplyRemote(ctx context.Context, p crdt.Peer, entries []crdt.LogEntry) (crdt.DeltaStats, error) {
	c.mu.RLock()
	paused := c.paused[p.ID()]
	c.mu.RUnlock()
	if paused {
		return crdt.DeltaStats{}, ErrPaused
	}
	stats, err := c.log.ApplyLog(ctx, entries)
	if err != nil {
		return stats, fmt.Errorf("apply: %w", err)
	}
	kind := EventDeltaApplied
	if stats.Conflicts > 0 {
		kind = EventConflict
	}
	c.emit(SyncEvent{Kind: kind, Peer: p, Stats: stats})
	return stats, nil
}

// EmitFor queues outbound deltas for a peer. Returns ErrPaused if the kill switch
// is engaged so the caller does not silently drop the deltas — the next Resume
// will pick them up on a fresh EmitLog call.
func (c *Coord) EmitFor(ctx context.Context, p crdt.Peer, since crdt.Lamport, limit int) error {
	c.mu.RLock()
	paused := c.paused[p.ID()]
	c.mu.RUnlock()
	if paused {
		return ErrPaused
	}
	entries, err := c.log.EmitLog(ctx, since, limit)
	if err != nil {
		return fmt.Errorf("emit: %w", err)
	}
	if len(entries) == 0 {
		return nil
	}
	return c.outbox.Enqueue(ctx, p.ID(), entries)
}

// Subscribe returns a channel that receives lifecycle events. Slow consumers drop
// events rather than block the dispatcher — UI surfaces are best-effort by design.
func (c *Coord) Subscribe() <-chan SyncEvent {
	ch := make(chan SyncEvent, 16)
	c.subsMu.Lock()
	c.subs = append(c.subs, ch)
	c.subsMu.Unlock()
	return ch
}

// emit fans out non-blocking. Buffered channel + select-default keeps a stalled
// subscriber from holding the CRDT writer mutex.
func (c *Coord) emit(ev SyncEvent) {
	c.subsMu.RLock()
	defer c.subsMu.RUnlock()
	for _, ch := range c.subs {
		select {
		case ch <- ev:
		default:
		}
	}
}

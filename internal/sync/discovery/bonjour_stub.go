//go:build !darwin

package discovery

import (
	"context"
	"net/netip"
	"sync"
	"time"
)

// stubDiscovery keeps published services in-process so Linux/CI runs can
// exercise the publish→browse round-trip without a real Bonjour daemon.
// Browsers receive every record currently in the registry plus any added
// while subscribed.
type stubDiscovery struct {
	mu      sync.Mutex
	records map[DeviceID]peerRecord
	subs    []chan Peer
	closed  bool
}

var stubRegistry = &stubDiscovery{records: map[DeviceID]peerRecord{}}

func newPlatform() Discovery { return stubRegistry }

func (d *stubDiscovery) Publish(_ context.Context, name string, port uint16) error {
	addr := netip.AddrPortFrom(netip.MustParseAddr("127.0.0.1"), port)
	rec := peerRecord{
		id:     DeviceID(name),
		addr:   addr,
		seenAt: time.Now(),
		status: StatusOnline,
	}
	d.mu.Lock()
	d.records[rec.id] = rec
	subs := append([]chan Peer{}, d.subs...)
	d.mu.Unlock()
	for _, s := range subs {
		select {
		case s <- rec:
		default:
		}
	}
	return nil
}

func (d *stubDiscovery) Browse(ctx context.Context) (<-chan Peer, error) {
	ch := make(chan Peer, 16)
	d.mu.Lock()
	for _, r := range d.records {
		select {
		case ch <- r:
		default:
		}
	}
	d.subs = append(d.subs, ch)
	d.mu.Unlock()
	go func() {
		<-ctx.Done()
		d.mu.Lock()
		for i, s := range d.subs {
			if s == ch {
				d.subs = append(d.subs[:i], d.subs[i+1:]...)
				break
			}
		}
		d.mu.Unlock()
		close(ch)
	}()
	return ch, nil
}

func (d *stubDiscovery) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.records = map[DeviceID]peerRecord{}
	d.subs = nil
}

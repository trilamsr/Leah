//go:build !darwin

package discovery

import (
	"context"
	"sync"
)

type stubDiscovery struct {
	mu      sync.Mutex
	records map[DeviceID]peerRecord
}

func newPlatform() Discovery { return &stubDiscovery{records: map[DeviceID]peerRecord{}} }

func (d *stubDiscovery) Publish(_ context.Context, name string, port uint16) error { return nil }

func (d *stubDiscovery) Browse(ctx context.Context) (<-chan Peer, error) {
	return make(chan Peer), nil
}

func (d *stubDiscovery) Stop() {}

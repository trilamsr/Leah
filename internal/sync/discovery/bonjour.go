package discovery

import (
	"context"
	"net/netip"
	"time"
)

type DeviceID string

type PeerStatus int

const (
	StatusOnline PeerStatus = iota
	StatusIdle
	StatusPaused
	StatusUnreachable
)

func (s PeerStatus) String() string {
	switch s {
	case StatusOnline:
		return "online"
	case StatusIdle:
		return "idle"
	case StatusPaused:
		return "paused"
	case StatusUnreachable:
		return "unreachable"
	}
	return "unknown"
}

type Peer interface {
	ID() DeviceID
	Endpoint() netip.AddrPort
	LastSeenAt() time.Time
	Status() PeerStatus
}

type Discovery interface {
	Publish(ctx context.Context, name string, port uint16) error
	Browse(ctx context.Context) (<-chan Peer, error)
	Stop()
}

// ServiceType is the Bonjour service name per spec §2.4.3.
const ServiceType = "_leah-sync._tcp"

// peerRecord backs the Peer interface so transport-layer code stays
// loose-coupled to the discovery layer.
type peerRecord struct {
	id     DeviceID
	addr   netip.AddrPort
	seenAt time.Time
	status PeerStatus
}

func (p peerRecord) ID() DeviceID             { return p.id }
func (p peerRecord) Endpoint() netip.AddrPort { return p.addr }
func (p peerRecord) LastSeenAt() time.Time    { return p.seenAt }
func (p peerRecord) Status() PeerStatus       { return p.status }

// NewPeer returns a Peer with the supplied fields. Exposed so
// platform-specific code in other build-tag files can synthesize a Peer
// without depending on the unexported struct.
func NewPeer(id DeviceID, addr netip.AddrPort, seenAt time.Time, status PeerStatus) Peer {
	return peerRecord{id: id, addr: addr, seenAt: seenAt, status: status}
}

// New returns the platform-appropriate Discovery. Darwin gets the NetService
// cgo bridge; everything else gets an in-memory stub so unit tests pass on
// CI Linux runners without losing the publish/browse contract.
func New() Discovery { return newPlatform() }

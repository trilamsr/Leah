// Package crdt models the replicated data layer for multi-device sync (phase4 §2.3).
//
// peer_iface.go ships a LOCAL stub of the T10 transport interfaces (Peer + MTLSConfig)
// because the T10 PR is blocked on its own gate. Same signatures, no implementation —
// final integration deletes this file when T10 lands and exports the real types from
// internal/sync (per spec §2.4.1).
package crdt

import (
	"net/netip"
	"time"
)

// DeviceID is a UUIDv7 minted at first launch; stable across daemon restarts (§2.4.1).
type DeviceID string

// PeerStatus enumerates the four states surfaced in Settings → Sync (§2.4.1).
type PeerStatus int

const (
	PeerOnline PeerStatus = iota
	PeerIdle
	PeerPaused
	PeerUnreachable
)

// Peer mirrors internal/sync.Peer (T10) so CRDT code compiles in isolation.
type Peer interface {
	ID() DeviceID
	Endpoint() netip.AddrPort
	LastSeenAt() time.Time
	Status() PeerStatus
}

// MTLSConfig mirrors internal/sync.MTLSConfig (T10); a fingerprint pin against the
// shared Curve25519 keychain secret (§2.6).
type MTLSConfig struct {
	Fingerprint []byte
}

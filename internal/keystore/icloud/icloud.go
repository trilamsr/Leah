// Package icloud is the iCloud-Keychain-backed key share for Phase 4 sync.
//
// Trust root for multi-device sync (§2.2): a Curve25519 secret stored with
// kSecAttrSynchronizable=true so paired Macs read the same value once the
// operator signs into iCloud. The same primitive opts API keys into sync
// (§2.6 — "Share API keys via iCloud Keychain"). The peer channel never
// carries raw keys.
package icloud

import (
	"context"
	"errors"
	"sort"
	"sync"
)

// ICloudKeystore is the Go-side handle on the iCloud-backed Keychain.
// Implementations: darwin SecItem backend (icloud_darwin.go) + non-darwin
// stub (icloud_stub.go) + in-memory fake for tests.
type ICloudKeystore interface {
	// Put stores data under (service, account). When sync=true the row is
	// marked kSecAttrSynchronizable, opting it into iCloud Keychain replication.
	Put(ctx context.Context, service, account string, data []byte, sync bool) error

	// Get returns the value previously stored under (service, account), or
	// ErrNotFound if no row exists.
	Get(ctx context.Context, service, account string) ([]byte, error)

	// Delete removes the row idempotently — no error if it never existed.
	Delete(ctx context.Context, service, account string) error

	// ListSynced returns "<service>/<account>" identifiers for every row
	// currently marked synchronizable. Used by the Sync pane to render the
	// per-key share state and by Unpair to wipe shared rows.
	ListSynced(ctx context.Context) ([]string, error)
}

// Sentinel errors surface decision-relevant conditions to UI + callers.
var (
	ErrUnsupported = errors.New("icloud: iCloud Keychain not available on this platform")
	ErrNotFound    = errors.New("icloud: keychain row not found")
	ErrEmptyData   = errors.New("icloud: empty data rejected")
	ErrBadKey      = errors.New("icloud: service and account are required")
)

// memKeystore is the cross-platform in-memory fake used by unit tests. The
// darwin SecItem backend is verified by manual/integration runs.
type memKeystore struct {
	mu   sync.Mutex
	rows map[string]memRow
}

type memRow struct {
	data []byte
	sync bool
}

func newMemKeystore() *memKeystore {
	return &memKeystore{rows: map[string]memRow{}}
}

func memKey(service, account string) string { return service + "/" + account }

func (m *memKeystore) Put(_ context.Context, service, account string, data []byte, syncFlag bool) error {
	if service == "" || account == "" {
		return ErrBadKey
	}
	if len(data) == 0 {
		return ErrEmptyData
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	// Copy to avoid aliasing caller buffer; SecItem persists bytes too.
	buf := append([]byte(nil), data...)
	m.rows[memKey(service, account)] = memRow{data: buf, sync: syncFlag}
	return nil
}

func (m *memKeystore) Get(_ context.Context, service, account string) ([]byte, error) {
	if service == "" || account == "" {
		return nil, ErrBadKey
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	row, ok := m.rows[memKey(service, account)]
	if !ok {
		return nil, ErrNotFound
	}
	return append([]byte(nil), row.data...), nil
}

func (m *memKeystore) Delete(_ context.Context, service, account string) error {
	if service == "" || account == "" {
		return ErrBadKey
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.rows, memKey(service, account))
	return nil
}

func (m *memKeystore) ListSynced(_ context.Context) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, 0, len(m.rows))
	for k, v := range m.rows {
		if v.sync {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out, nil
}

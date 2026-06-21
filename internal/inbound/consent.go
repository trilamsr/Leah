package inbound

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/trilam/leah/internal/attestation"
	"github.com/trilam/leah/internal/contracts"
)

// ScopeResolver maps a RecID to the attestation scope that gates Apply for
// that recommendation. A self-build rec returns ScopeSelfBuild; a generic
// TierConfirm rec returns ScopeInboundApply (spec §4.2). The router stays
// decoupled from internal/recommend — the daemon supplies the concrete
// resolver. Returning "" tells the gate to skip attestation, which is the
// correct null for a TierAuto, non-destructive rec.
type ScopeResolver interface {
	ScopeFor(recID string) string
}

// EnrollStore persists which (channel, peerID) pairs the operator has
// authorized to answer recommendations remotely. Enrollment is a one-time
// loopback act (spec §4.2 layer 1); the store survives daemon restarts so
// the trust grant isn't re-prompted on every boot.
type EnrollStore interface {
	Enrolled(channel, peerID string) (bool, error)
	Enroll(channel, peerID string) error
}

// Gate is the concrete two-layer ConsentGate (spec §4). Layer 1 checks an
// enrollment store; layer 2 attests against the rec's own scope. A self-build
// rec attests `self-build` regardless of remote origin — the load-bearing
// "remote origin never downgrades the gate" rule (§4.2).
type Gate struct {
	Attestor contracts.Attestor
	Resolver ScopeResolver
	Store    EnrollStore
}

func (g *Gate) Enrolled(channel, peerID string) (bool, error) {
	if g.Store == nil {
		return false, nil
	}
	return g.Store.Enrolled(channel, peerID)
}

// Authorize is called by the router AFTER Enrolled cleared and ONLY on the
// accept path. Reject/defer/unknown intents skip the gate — declining is
// always safe and gating it would only train the operator to dismiss prompts.
func (g *Gate) Authorize(ctx context.Context, p Pending, intent Intent) error {
	if intent != IntentAccept {
		return nil
	}
	if g.Attestor == nil || g.Resolver == nil {
		return errors.New("inbound: consent gate not configured")
	}
	scope := g.Resolver.ScopeFor(p.RecID)
	if scope == "" {
		// TierAuto non-destructive: enrollment alone suffices.
		return nil
	}
	return g.Attestor.Attest(ctx, scope)
}

// MemoryEnrollStore is the in-process default for tests and for daemon
// lifetimes where loss-on-restart is acceptable. FileEnrollStore is the
// production store the CLI persists into.
type MemoryEnrollStore struct {
	mu sync.RWMutex
	m  map[string]struct{}
}

func NewMemoryEnrollStore() *MemoryEnrollStore {
	return &MemoryEnrollStore{m: make(map[string]struct{})}
}

func (s *MemoryEnrollStore) Enrolled(channel, peerID string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.m[enrollKey(channel, peerID)]
	return ok, nil
}

func (s *MemoryEnrollStore) Enroll(channel, peerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[enrollKey(channel, peerID)] = struct{}{}
	return nil
}

// FileEnrollStore writes a single JSON file under ~/.leah-state/. The pattern
// matches every other on-disk secret-adjacent file: 0600 perms, atomic
// rename, parent dir 0700.
type FileEnrollStore struct {
	mu   sync.Mutex
	path string
	mem  map[string]struct{}
}

type enrollFile struct {
	Pairs []enrollPair `json:"pairs"`
}

type enrollPair struct {
	Channel string `json:"channel"`
	PeerID  string `json:"peer_id"`
}

// OpenFileEnrollStore loads (or initializes) the on-disk enrollment file.
// A missing file is not an error — it's the first-run state.
func OpenFileEnrollStore(path string) (*FileEnrollStore, error) {
	s := &FileEnrollStore{path: path, mem: make(map[string]struct{})}
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s, nil
		}
		return nil, fmt.Errorf("read enroll file: %w", err)
	}
	var f enrollFile
	if len(b) > 0 {
		if err := json.Unmarshal(b, &f); err != nil {
			return nil, fmt.Errorf("parse enroll file: %w", err)
		}
	}
	for _, p := range f.Pairs {
		s.mem[enrollKey(p.Channel, p.PeerID)] = struct{}{}
	}
	return s, nil
}

func (s *FileEnrollStore) Enrolled(channel, peerID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.mem[enrollKey(channel, peerID)]
	return ok, nil
}

// Enroll serializes to disk FIRST, then commits to the in-memory set. A flush
// failure (disk full, perms) must not leave mem/disk skew where Enrolled
// returns true now but false after restart — that would silently degrade the
// gate (spec §4.2: enrollment is the load-bearing layer-1 trust grant).
func (s *FileEnrollStore) Enroll(channel, peerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := enrollKey(channel, peerID)
	if _, ok := s.mem[k]; ok {
		return nil
	}
	if err := s.flushWithLocked(k); err != nil {
		return err
	}
	s.mem[k] = struct{}{}
	return nil
}

// flushWithLocked serializes (mem ∪ {newKey}) to disk via atomic rename.
// Caller commits newKey to mem only on success. Pairs sorted so repeated
// flushes don't churn the on-disk byte order.
func (s *FileEnrollStore) flushWithLocked(newKey string) error {
	keys := make([]string, 0, len(s.mem)+1)
	for k := range s.mem {
		keys = append(keys, k)
	}
	if _, exists := s.mem[newKey]; !exists {
		keys = append(keys, newKey)
	}
	sort.Strings(keys)
	var f enrollFile
	for _, k := range keys {
		ch, peer := splitEnrollKey(k)
		f.Pairs = append(f.Pairs, enrollPair{Channel: ch, PeerID: peer})
	}
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal enroll: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("mkdir enroll: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return fmt.Errorf("write enroll: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("rename enroll: %w", err)
	}
	return nil
}

func enrollKey(channel, peerID string) string { return channel + "\x00" + peerID }

func splitEnrollKey(k string) (string, string) {
	for i := 0; i < len(k); i++ {
		if k[i] == 0 {
			return k[:i], k[i+1:]
		}
	}
	return k, ""
}

// StaticScopeResolver returns the same scope for every RecID. Useful as a
// daemon default when no rec-tier metadata is wired yet — paired with
// ScopeInboundApply it gives TierConfirm-level gating to all remote accepts.
// Self-build / self-upgrade recs MUST use a richer resolver.
type StaticScopeResolver struct{ Scope string }

func (r StaticScopeResolver) ScopeFor(_ string) string { return r.Scope }

// Compile-time assertion that Gate satisfies the router's ConsentGate
// interface; the indirect coupling otherwise hides a signature drift.
var _ ConsentGate = (*Gate)(nil)

// DefaultApplyScope is re-exported so daemon wiring doesn't import attestation
// just to spell the constant. Authors of new resolvers can use this as the
// fallback when the rec carries no own-scope hint.
const DefaultApplyScope = attestation.ScopeInboundApply

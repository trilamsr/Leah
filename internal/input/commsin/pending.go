package commsin

import (
	"sync"
	"time"
)

type Pending struct {
	RecID   string
	Channel string
	ConvID  string
	PeerID  string
	SentAt  time.Time
}

type PendingStore interface {
	Put(Pending) error
	Take(channel, convID string) (Pending, bool)
}

// MemoryPendingStore is the v1 in-memory store. A daemon restart drops state
// by design — the next cron re-prompts (spec §3.2, no durability needed).
type MemoryPendingStore struct {
	mu sync.Mutex
	m  map[string]Pending
}

func NewMemoryPendingStore() *MemoryPendingStore {
	return &MemoryPendingStore{m: make(map[string]Pending)}
}

// Peek returns the entry without consuming it. Used by the router for
// binding checks (peer match, enrollment) that must not consume on failure.
func (s *MemoryPendingStore) Peek(channel, convID string) (Pending, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.m[key(channel, convID)]
	return p, ok
}

func (s *MemoryPendingStore) Put(p Pending) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[key(p.Channel, p.ConvID)] = p
	return nil
}

// Take is single-use: removes the entry on first hit so a duplicate/replayed
// reply cannot re-fire the recommendation (spec §3.2 replay defense).
func (s *MemoryPendingStore) Take(channel, convID string) (Pending, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := key(channel, convID)
	p, ok := s.m[k]
	if !ok {
		return Pending{}, false
	}
	delete(s.m, k)
	return p, true
}

func key(channel, convID string) string { return channel + "\x00" + convID }

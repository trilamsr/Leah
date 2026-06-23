package router

import "sync"

// ConsentScope is the lifetime of a Grant per spec §4.6. ScopeThisSession dies
// at daemon restart; ScopeUntilQuit dies on operator quit (HUD logout, not
// restart); ScopePersistent survives across restarts (DB-backed when a future
// store wraps memConsent).
type ConsentScope int

const (
	ScopeThisSession ConsentScope = iota
	ScopeUntilQuit
	ScopePersistent
)

// ConsentStore gates cloud frame uploads. mode is the spec §4.6 mode key —
// "screenshot", "live_screen", "live_camera" — NOT the VisionMode int.
type ConsentStore interface {
	Granted(mode string) bool
	Grant(mode string, scope ConsentScope)
	Revoke(mode string)
}

type memConsent struct {
	mu sync.Mutex
	m  map[string]ConsentScope
}

func newMemConsent() *memConsent { return &memConsent{m: map[string]ConsentScope{}} }

// NewMemConsent returns an in-process ConsentStore. Persistence is deferred
// to a DB-backed wrapper in T04.b — the in-memory store satisfies §4.6's
// "first-time-per-session" requirement on its own.
func NewMemConsent() ConsentStore { return newMemConsent() }

func (c *memConsent) Granted(mode string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.m[mode]
	return ok
}

func (c *memConsent) Grant(mode string, scope ConsentScope) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[mode] = scope
}

func (c *memConsent) Revoke(mode string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.m, mode)
}

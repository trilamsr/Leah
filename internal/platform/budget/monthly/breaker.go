package monthly

import (
	"sync"
	"time"

	"github.com/trilam/leah/internal/thinking/reasoner"
)

// OverrideTTL is the in-memory lifetime of an attested `leah cost override`
// (spec §7.3). Daemon restart drops the override so deny re-imposes —
// in-memory by design.
const OverrideTTL = time.Hour

// warnRatio is the spec §7.1 boundary at which non-merge kinds swap to Haiku.
const warnRatio = 0.80

// Breaker bridges a Store's spend/cap into the reasoner.Breaker interface
// the Router consults pre-call. Charge passes through to the store so the
// Router's post-call accounting remains a single mutex hop.
type Breaker struct {
	store         *Store
	mu            sync.Mutex
	overrideUntil time.Time
}

// NewBreaker wraps store; the override clock starts un-armed.
func NewBreaker(store *Store) *Breaker {
	return &Breaker{store: store}
}

// State reports the breaker tri-state against the wall clock.
func (b *Breaker) State() reasoner.BreakerState {
	return b.StateAt(time.Now())
}

// StateAt is State with the time injected — keeps the override-TTL test
// deterministic without monkey-patching time.Now.
func (b *Breaker) StateAt(now time.Time) reasoner.BreakerState {
	cap := b.store.Cap()
	spent := b.store.Spent()
	// Zero cap means "uncapped" — fail-open rather than divide-by-zero into deny.
	if cap <= 0 {
		return reasoner.StateOK
	}
	ratio := spent / cap
	if ratio < warnRatio {
		return reasoner.StateOK
	}
	if ratio < 1.0 {
		return reasoner.StateWarn
	}
	b.mu.Lock()
	overrideActive := !b.overrideUntil.IsZero() && now.Before(b.overrideUntil)
	b.mu.Unlock()
	if overrideActive {
		return reasoner.StateOK
	}
	return reasoner.StateDeny
}

// Spent and Cap forward to the store so the Router populates
// *BreakerDeniedError with the same numbers the operator sees on the HUD.
func (b *Breaker) Spent() float64 { return b.store.Spent() }
func (b *Breaker) Cap() float64   { return b.store.Cap() }

// Charge forwards to the store; the breaker keeps no shadow totals so a
// concurrent Spent() read on the store side cannot see a partial update.
func (b *Breaker) Charge(kind string, dollars float64) error {
	return b.store.Charge(kind, dollars)
}

// GrantOverride arms the 1-hour TTL from the wall clock (spec §7.3).
// Called by `leah cost override` after the attestation passes.
func (b *Breaker) GrantOverride() { b.GrantOverrideAt(time.Now()) }

// GrantOverrideAt is GrantOverride with the time injected; used by tests.
func (b *Breaker) GrantOverrideAt(now time.Time) {
	b.mu.Lock()
	b.overrideUntil = now.Add(OverrideTTL)
	b.mu.Unlock()
}

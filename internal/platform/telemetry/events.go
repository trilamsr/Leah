package telemetry

import (
	"context"
	"fmt"
	"hash/fnv"
	"strings"
	"sync"
	"time"
)

// EventQuery is a Query filter. Mutually-additive fields AND together.
type EventQuery struct {
	Since    time.Time
	Until    time.Time
	Kinds    []string
	Actors   []string
	Outcomes []string
	RefID    string
	Limit    int
}

// EventStore is the timeline backend; SQLite impl needs Close to drain.
type EventStore interface {
	Emit(ctx context.Context, e Event) error
	Query(ctx context.Context, q EventQuery) ([]Event, error)
	Close() error
}

var (
	defaultStoreMu sync.RWMutex
	defaultStore   EventStore
)

// SetDefaultEventStore wires EmitEvent to store; nil detaches (no-op mode).
func SetDefaultEventStore(store EventStore) {
	defaultStoreMu.Lock()
	defaultStore = store
	defaultStoreMu.Unlock()
}

// EmitEvent enqueues e against the default store AND fans it out to the
// default Broadcaster's live SSE subscribers. Either side is a no-op when
// its sink is unset, so this stays safe for callers that only wire one.
func EmitEvent(ctx context.Context, e Event) {
	if e.TS.IsZero() {
		e.TS = time.Now().UTC()
	}
	if e.RefID == "" {
		if id := RefID(ctx); id != "" {
			e.RefID = id
		}
	}
	Publish(e)
	defaultStoreMu.RLock()
	s := defaultStore
	defaultStoreMu.RUnlock()
	if s == nil {
		return
	}
	_ = s.Emit(ctx, e)
}

// KnownEventKinds — frozen enum; drift-gated by TestEventKinds_FrozenList.
var KnownEventKinds = []string{
	"dispatch.ship", "dispatch.review", "dispatch.merge",
	"attestation.attempt", "attestation.granted", "attestation.revoked",
	"audit.append", "audit.rotate",
	"connect.exchange", "connect.refresh", "connect.api_call",
	"voice.speak", "voice.fallback",
	"subagent.spawn", "subagent.complete",
	"reasoner.call", "reasoner.retry",
	"memory.query", "memory.upsert",
	"recommendation.propose", "recommendation.accept",
	"recommendation.reject", "recommendation.apply",
	"obs.snapshot", "obs.selfcheck", "obs.panic",
	"hud.state",
	"workspace.active_app_changed",
	"contact_store_changed",
	"messages_changed",
	"mail_changed",
	"notes_changed",
	"safari.history_changed",
	"calendar.store_changed",
	"focus.state_changed",
	"photos.library_changed",
	"reminders.store_changed",
}

// SafeDetail strips chars outside [\w\-\.:/], truncates 128r (spec §9 PII).
func SafeDetail(s string) string {
	const maxRunes = 128
	var b strings.Builder
	b.Grow(len(s))
	n := 0
	for _, r := range s {
		if n >= maxRunes {
			break
		}
		if isDetailRune(r) {
			b.WriteRune(r)
			n++
		}
	}
	return b.String()
}

func isDetailRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z':
		return true
	case r >= 'A' && r <= 'Z':
		return true
	case r >= '0' && r <= '9':
		return true
	case r == '_' || r == '-' || r == '.' || r == ':' || r == '/':
		return true
	}
	return false
}

// SafeDetailHashed returns "h:<FNV-1a hex>" — use for any PII-bearing value.
func SafeDetailHashed(s string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	return fmt.Sprintf("h:%x", h.Sum64())
}

// SSE transport for the structured-event stream (W77) + in-process Broadcaster
// fan-out so EmitEvent reaches live subscribers with no SQLite round trip
// (V2/W87). Canonical Event lives in events.go. Keep-alive comment every 15s
// per event-timeline.md §7; client disconnect (ctx done) tears down the sub.

package obs

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// SSESubscriber yields events as they arrive. Close releases writer-side state.
type SSESubscriber interface {
	Events() <-chan Event
	Close() error
}

// SSESubscribeFunc opens a subscription filtered to kinds (empty = any).
type SSESubscribeFunc func(ctx context.Context, kinds []string) (SSESubscriber, error)

// SSEHandler streams events to one SSE client; keep-alive defaults 15s.
type SSEHandler struct {
	Subscribe SSESubscribeFunc
	KeepAlive time.Duration
}

func (h *SSEHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.Subscribe == nil {
		http.Error(w, "event subscribe not wired", http.StatusServiceUnavailable)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	kinds := parseKinds(r.URL.Query().Get("kind"))
	sub, err := h.Subscribe(r.Context(), kinds)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer func() { _ = sub.Close() }()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	keepAlive := h.KeepAlive
	if keepAlive <= 0 {
		keepAlive = 15 * time.Second
	}
	tick := time.NewTicker(keepAlive)
	defer tick.Stop()

	events := sub.Events()
	for {
		select {
		case <-r.Context().Done():
			return
		case e, ok := <-events:
			if !ok {
				return
			}
			if err := writeSSEEvent(w, e); err != nil {
				return
			}
			flusher.Flush()
		case <-tick.C:
			if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func writeSSEEvent(w http.ResponseWriter, e Event) error {
	payload, err := json.Marshal(e)
	if err != nil {
		return err
	}
	kind := e.Kind
	if kind == "" {
		kind = "event"
	}
	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", kind, payload)
	return err
}

func parseKinds(csv string) []string {
	if csv == "" {
		return nil
	}
	parts := strings.Split(csv, ",")
	out := parts[:0]
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Broadcaster fan-outs Emit calls to every live SSE subscriber. Drops on a
// slow subscriber rather than blocking the producer.
type Broadcaster struct {
	mu   sync.Mutex
	subs map[*broadcasterSub]struct{}
}

// NewBroadcaster returns a Broadcaster with no subscribers.
func NewBroadcaster() *Broadcaster {
	return &Broadcaster{subs: map[*broadcasterSub]struct{}{}}
}

type broadcasterSub struct {
	ch     chan Event
	kinds  map[string]struct{}
	parent *Broadcaster
	once   sync.Once
}

func (s *broadcasterSub) Events() <-chan Event { return s.ch }

func (s *broadcasterSub) Close() error {
	s.once.Do(func() {
		s.parent.mu.Lock()
		delete(s.parent.subs, s)
		s.parent.mu.Unlock()
		close(s.ch)
	})
	return nil
}

// Subscribe returns an SSESubscriber wired to live Emit calls. kinds=nil → any.
func (b *Broadcaster) Subscribe(ctx context.Context, kinds []string) (SSESubscriber, error) {
	sub := &broadcasterSub{
		ch:     make(chan Event, 16),
		parent: b,
	}
	if len(kinds) > 0 {
		sub.kinds = make(map[string]struct{}, len(kinds))
		for _, k := range kinds {
			sub.kinds[k] = struct{}{}
		}
	}
	b.mu.Lock()
	b.subs[sub] = struct{}{}
	b.mu.Unlock()
	return sub, nil
}

// Emit fans e out to every matching subscriber non-blockingly.
func (b *Broadcaster) Emit(e Event) {
	b.mu.Lock()
	subs := make([]*broadcasterSub, 0, len(b.subs))
	for s := range b.subs {
		subs = append(subs, s)
	}
	b.mu.Unlock()
	for _, s := range subs {
		if s.kinds != nil {
			if _, ok := s.kinds[e.Kind]; !ok {
				continue
			}
		}
		select {
		case s.ch <- e:
		default:
		}
	}
}

var (
	defaultBroadcasterMu sync.RWMutex
	defaultBroadcaster   *Broadcaster
)

// SetDefaultBroadcaster wires Publish to b; nil detaches.
func SetDefaultBroadcaster(b *Broadcaster) {
	defaultBroadcasterMu.Lock()
	defaultBroadcaster = b
	defaultBroadcasterMu.Unlock()
}

// Publish fans e out to the default broadcaster's live subscribers. No-op
// when unset — keeps SQLite-only callers (W75 store path) untouched.
func Publish(e Event) {
	defaultBroadcasterMu.RLock()
	b := defaultBroadcaster
	defaultBroadcasterMu.RUnlock()
	if b == nil {
		return
	}
	b.Emit(e)
}

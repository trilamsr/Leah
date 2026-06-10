// SSE handler for the structured-event stream. W77 ships ahead of W75's
// SQLite EventStore by depending only on the local EventReader/Subscriber
// interfaces below; once W75 lands, its EventStore satisfies both contracts
// and the daemon wires it in. Keep-alive comment every 15s mirrors the
// event-timeline.md §7 contract; client disconnect (ctx done) tears down
// the goroutine without leaking the subscription.

package obs

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Event mirrors the schema from docs/engineer/specs/2026-06-10-event-timeline.md
// §2.1. W75 will move this to event.go alongside Query / EmitEvent; until then
// the SSE handler depends only on this struct shape, not on the storage layer.
type Event struct {
	TS        time.Time `json:"ts"`
	Kind      string    `json:"kind"`
	Actor     string    `json:"actor"`
	Target    string    `json:"target,omitempty"`
	Scope     string    `json:"scope,omitempty"`
	LatencyMS int64     `json:"latency_ms,omitempty"`
	Outcome   string    `json:"outcome"`
	RefID     string    `json:"ref_id,omitempty"`
	Detail    string    `json:"detail,omitempty"`
}

// SSESubscriber yields events as they arrive. Close() must stop the channel
// and release all writer-side resources.
type SSESubscriber interface {
	Events() <-chan Event
	Close() error
}

// SSESubscribeFunc opens a fresh subscription filtered to kinds (empty = any).
// The daemon wires this to the EventStore's pub-sub side once W75 lands.
type SSESubscribeFunc func(ctx context.Context, kinds []string) (SSESubscriber, error)

// SSEHandler streams events to a single SSE client. Keep-alive ping every
// keepAlive; defaults to 15s when zero. ctx done (client disconnect) tears
// down the subscription.
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

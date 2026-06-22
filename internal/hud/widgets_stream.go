package hud

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Stream is the server-side push channel that replaces widgets.js setInterval
// (B1 in the cross-surface UX audit). One goroutine per tile ticks at the
// spec-canonical TTL and fans pre-rendered HTML out to every live subscriber.
// Failure tiles ride the same channel so the client cannot conflate
// "couldn't-load" with "still-loading" — the symptom that made the original
// poll-and-swallow design hide errors as em-dashes.

type TileRenderer struct {
	ID     string
	TTL    time.Duration
	Render func(context.Context) string
}

type Frame struct {
	ID   string `json:"id"`
	HTML string `json:"html"`
}

type Stream struct {
	tiles []TileRenderer

	mu   sync.Mutex
	subs map[chan Frame]struct{}
	last map[string]Frame
}

func NewStream(tiles []TileRenderer) *Stream {
	return &Stream{
		tiles: tiles,
		subs:  make(map[chan Frame]struct{}),
		last:  make(map[string]Frame),
	}
}

// Run owns one ticker per tile and runs until ctx is cancelled. Each tile's
// first render fires immediately so a fresh subscriber doesn't wait a full TTL
// for the cache-replay path to populate.
func (s *Stream) Run(ctx context.Context) {
	var wg sync.WaitGroup
	for _, t := range s.tiles {
		wg.Add(1)
		go func(t TileRenderer) {
			defer wg.Done()
			s.tick(ctx, t)
			tk := time.NewTicker(t.TTL)
			defer tk.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-tk.C:
					s.tick(ctx, t)
				}
			}
		}(t)
	}
	wg.Wait()
}

func (s *Stream) tick(ctx context.Context, t TileRenderer) {
	html := t.Render(ctx)
	f := Frame{ID: t.ID, HTML: html}
	s.mu.Lock()
	s.last[t.ID] = f
	subs := make([]chan Frame, 0, len(s.subs))
	for c := range s.subs {
		subs = append(subs, c)
	}
	s.mu.Unlock()
	for _, c := range subs {
		select {
		case c <- f:
		default:
			// Slow subscriber drops a frame rather than blocking the ticker —
			// the next tick will catch them up; UX > strict delivery.
		}
	}
}

// Subscribe registers a buffered channel that receives all subsequent frames
// plus an immediate replay of the last-known frame per tile so a fresh
// subscriber doesn't stare at em-dashes until the next tick. Caller drains
// until ctx ends — the channel intentionally outlives ctx (no close) so
// concurrent senders never panic on send-to-closed; HandleSSE already selects
// on ctx.Done.
func (s *Stream) Subscribe(ctx context.Context) <-chan Frame {
	ch := make(chan Frame, 16)
	s.mu.Lock()
	s.subs[ch] = struct{}{}
	for _, f := range s.last {
		select {
		case ch <- f:
		default:
		}
	}
	s.mu.Unlock()

	go func() {
		<-ctx.Done()
		s.mu.Lock()
		delete(s.subs, ch)
		s.mu.Unlock()
	}()
	return ch
}

// TileIDs returns the canonical tile id list in declaration order. Single
// source of truth so widgets.js doesn't hardcode (and silently drift from) it.
func (s *Stream) TileIDs() []string {
	ids := make([]string, len(s.tiles))
	for i, t := range s.tiles {
		ids[i] = t.ID
	}
	return ids
}

// HandleSSE serves a long-lived text/event-stream that emits widget.refresh
// events. The first event is widget.init carrying the tile id list so the
// client can mark every tile offline on disconnect without hardcoding the set.
func (s *Stream) HandleSSE(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "stream unsupported", http.StatusInternalServerError)
		return
	}
	ctx := r.Context()
	ch := s.Subscribe(ctx)
	if init, err := json.Marshal(s.TileIDs()); err == nil {
		if _, err := fmt.Fprintf(w, "event: widget.init\ndata: %s\n\n", init); err != nil {
			return
		}
		fl.Flush()
	}
	keepalive := time.NewTicker(20 * time.Second)
	defer keepalive.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case f := <-ch:
			b, err := json.Marshal(f)
			if err != nil {
				continue
			}
			if _, err := fmt.Fprintf(w, "event: widget.refresh\ndata: %s\n\n", b); err != nil {
				return
			}
			fl.Flush()
		case <-keepalive.C:
			if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
				return
			}
			fl.Flush()
		}
	}
}

// StreamFromWidgets wires the canonical four-tile set off a Widgets instance.
// The wiring is here (not in cmd/leah-hud) so cmd stays a one-line bind and
// the tile set is unit-testable.
func StreamFromWidgets(wg *Widgets) *Stream {
	ttls := TTLs()
	return NewStream([]TileRenderer{
		{
			ID:  "weather",
			TTL: ttls["weather"],
			Render: func(ctx context.Context) string {
				h, _ := wg.Weather(ctx)
				return h
			},
		},
		{
			ID:  "market-AAPL",
			TTL: ttls["market"],
			Render: func(ctx context.Context) string {
				h, _ := wg.Market(ctx, "AAPL")
				return h
			},
		},
		{
			ID:  "news",
			TTL: ttls["news"],
			Render: func(ctx context.Context) string {
				h, _ := wg.News(ctx)
				return h
			},
		},
		{
			ID:  "calendar",
			TTL: ttls["calendar"],
			Render: func(ctx context.Context) string {
				h, _ := wg.CalendarNext(ctx)
				return h
			},
		},
	})
}

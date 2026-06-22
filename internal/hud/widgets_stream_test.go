package hud

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// B1: widget refresh must push from server, not poll from client. Stream wires
// every tile renderer onto one SSE channel and ticks server-side so widgets.js
// can drop setInterval. Failure tiles ride the same channel so the client
// renders "couldn't load" without inferring it from a missing 200.

func TestWidgetStream_PushesInitialFrameForEveryTile(t *testing.T) {
	calls := map[string]*int32{"a": new(int32), "b": new(int32)}
	s := NewStream([]TileRenderer{
		{ID: "a", TTL: time.Hour, Render: func(_ context.Context) string {
			atomic.AddInt32(calls["a"], 1)
			return `<div class="widget weather">a</div>`
		}},
		{ID: "b", TTL: time.Hour, Render: func(_ context.Context) string {
			atomic.AddInt32(calls["b"], 1)
			return `<div class="widget market">b</div>`
		}},
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go s.Run(ctx)

	srv := httptest.NewServer(http.HandlerFunc(s.HandleSSE))
	defer srv.Close()

	got := readFrames(t, srv.URL, 2, 800*time.Millisecond)
	if len(got) < 2 {
		t.Fatalf("want >=2 frames, got %d: %+v", len(got), got)
	}
	seen := map[string]bool{}
	for _, f := range got {
		seen[f.id] = true
	}
	if !seen["a"] || !seen["b"] {
		t.Fatalf("missing initial frame: seen=%v", seen)
	}
}

func TestWidgetStream_TickRePushesTile(t *testing.T) {
	var n int32
	s := NewStream([]TileRenderer{
		{ID: "weather", TTL: 30 * time.Millisecond, Render: func(_ context.Context) string {
			i := atomic.AddInt32(&n, 1)
			return fmt.Sprintf(`<div class="widget weather">v%d</div>`, i)
		}},
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go s.Run(ctx)
	srv := httptest.NewServer(http.HandlerFunc(s.HandleSSE))
	defer srv.Close()

	got := readFrames(t, srv.URL, 3, 500*time.Millisecond)
	if len(got) < 3 {
		t.Fatalf("want >=3 frames from re-ticks, got %d", len(got))
	}
	// frames must contain distinct payload versions (v1, v2, v3).
	versions := map[string]bool{}
	for _, f := range got {
		for _, v := range []string{"v1", "v2", "v3"} {
			if strings.Contains(f.html, v) {
				versions[v] = true
			}
		}
	}
	if len(versions) < 3 {
		t.Fatalf("want 3 distinct payload versions, got %v: %+v", versions, got)
	}
}

func TestWidgetStream_FailureFramePushedNotSwallowed(t *testing.T) {
	// Renderer returning an error tile must still reach the client so the
	// "couldn't load" state is visible (not stuck on em-dash forever).
	s := NewStream([]TileRenderer{
		{ID: "calendar", TTL: time.Hour, Render: func(_ context.Context) string {
			return errorTile("calendar")
		}},
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go s.Run(ctx)
	srv := httptest.NewServer(http.HandlerFunc(s.HandleSSE))
	defer srv.Close()

	got := readFrames(t, srv.URL, 1, 500*time.Millisecond)
	if len(got) == 0 || !strings.Contains(got[0].html, "widget-error") {
		t.Fatalf("expected widget-error frame, got %+v", got)
	}
}

func TestWidgetStream_FrameCarriesFreshnessHookForB5(t *testing.T) {
	// B5: every successful frame must carry the as-of timestamp the server
	// renders. Stream must not strip it.
	s := NewStream([]TileRenderer{
		{ID: "weather", TTL: time.Hour, Render: func(_ context.Context) string {
			return `<div class="widget weather"><span class="widget-as-of" data-ts="2026-01-01T00:00:00Z">as of 00:00</span></div>`
		}},
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go s.Run(ctx)
	srv := httptest.NewServer(http.HandlerFunc(s.HandleSSE))
	defer srv.Close()

	got := readFrames(t, srv.URL, 1, 500*time.Millisecond)
	if len(got) == 0 || !strings.Contains(got[0].html, "widget-as-of") {
		t.Fatalf("freshness span missing in pushed frame: %+v", got)
	}
}

func TestWidgetsStream_TileListServedToClient(t *testing.T) {
	// Yellow-flag follow-up to #321: widgets.js used to hardcode the tile id
	// list, which silently drifted if the server tile set changed. Server now
	// emits a widget.init event whose data is the JSON-encoded tile id slice in
	// declaration order; client reads it on connect instead of hardcoding.
	want := []string{"weather", "market-AAPL", "news", "calendar"}
	tiles := make([]TileRenderer, 0, len(want))
	for _, id := range want {
		id := id
		tiles = append(tiles, TileRenderer{
			ID:  id,
			TTL: time.Hour,
			Render: func(_ context.Context) string {
				return fmt.Sprintf(`<div class="widget">%s</div>`, id)
			},
		})
	}
	s := NewStream(tiles)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go s.Run(ctx)

	srv := httptest.NewServer(http.HandlerFunc(s.HandleSSE))
	defer srv.Close()

	got := readInitTiles(t, srv.URL, 500*time.Millisecond)
	if len(got) != len(want) {
		t.Fatalf("init tile count = %d, want %d (got=%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("init tile[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestWidgetStream_SetsSSEHeaders(t *testing.T) {
	s := NewStream([]TileRenderer{{ID: "x", TTL: time.Hour, Render: func(_ context.Context) string { return "<div></div>" }}})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)

	req := httptest.NewRequest(http.MethodGet, "/api/widgets/stream", nil)
	rec := httptest.NewRecorder()
	// run handler briefly then cancel via ctx so it returns.
	hctx, hcancel := context.WithTimeout(req.Context(), 80*time.Millisecond)
	defer hcancel()
	s.HandleSSE(rec, req.WithContext(hctx))
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("content-type %q", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc == "" {
		t.Fatalf("Cache-Control unset")
	}
}

// --- helpers ---

type pushedFrame struct {
	id   string
	html string
}

func readFrames(t *testing.T, url string, want int, timeout time.Duration) []pushedFrame {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	req.Header.Set("Accept", "text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var (
		frames []pushedFrame
		ev     string
		data   strings.Builder
	)
	flush := func() {
		if ev == "widget.refresh" && data.Len() > 0 {
			id, html := parseFrame(data.String())
			if id != "" {
				frames = append(frames, pushedFrame{id: id, html: html})
			}
		}
		ev = ""
		data.Reset()
	}

	r := bufio.NewReader(resp.Body)
	for len(frames) < want {
		line, err := r.ReadString('\n')
		if line != "" {
			line = strings.TrimRight(line, "\n")
			switch {
			case strings.HasPrefix(line, "event: "):
				ev = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				data.WriteString(strings.TrimPrefix(line, "data: "))
			case line == "":
				flush()
			}
		}
		if err != nil {
			if err == io.EOF || ctx.Err() != nil {
				break
			}
			break
		}
	}
	return frames
}

func readInitTiles(t *testing.T, url string, timeout time.Duration) []string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	req.Header.Set("Accept", "text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var ev string
	var data strings.Builder
	r := bufio.NewReader(resp.Body)
	for {
		line, err := r.ReadString('\n')
		if line != "" {
			line = strings.TrimRight(line, "\n")
			switch {
			case strings.HasPrefix(line, "event: "):
				ev = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				data.WriteString(strings.TrimPrefix(line, "data: "))
			case line == "":
				if ev == "widget.init" && data.Len() > 0 {
					var ids []string
					if jerr := json.Unmarshal([]byte(data.String()), &ids); jerr != nil {
						t.Fatalf("init payload not JSON array: %v (%q)", jerr, data.String())
					}
					return ids
				}
				ev = ""
				data.Reset()
			}
		}
		if err != nil {
			break
		}
	}
	return nil
}

func parseFrame(s string) (id, html string) {
	// Minimal JSON parse: {"id":"...","html":"..."} — keep test parser tiny so it
	// surfaces parse mismatch directly instead of hiding behind encoding/json.
	id = jsonField(s, `"id"`)
	html = jsonField(s, `"html"`)
	return
}

func jsonField(s, key string) string {
	i := strings.Index(s, key)
	if i < 0 {
		return ""
	}
	rest := s[i+len(key):]
	q1 := strings.Index(rest, `"`)
	if q1 < 0 {
		return ""
	}
	rest = rest[q1+1:]
	// scan until unescaped quote.
	var out strings.Builder
	for i := 0; i < len(rest); i++ {
		c := rest[i]
		if c == '\\' && i+1 < len(rest) {
			switch rest[i+1] {
			case '"':
				out.WriteByte('"')
			case '\\':
				out.WriteByte('\\')
			case 'n':
				out.WriteByte('\n')
			default:
				out.WriteByte(rest[i+1])
			}
			i++
			continue
		}
		if c == '"' {
			return out.String()
		}
		out.WriteByte(c)
	}
	return out.String()
}

package hud

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestIPC_PollMetrics_ParsesProm(t *testing.T) {
	body := strings.Join([]string{
		"# HELP leah_web_requests_total Total HTTP requests.",
		"# TYPE leah_web_requests_total counter",
		`leah_web_requests_total{path="/metrics"} 17`,
		`leah_web_requests_total{path="/api/state"} 4.5`,
		"# a comment",
		"",
		"leah_panics_total 0",
	}, "\n")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/metrics" {
			http.NotFound(w, r)
			return
		}
		_, _ = fmt.Fprint(w, body)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	m, err := c.PollMetrics(ctx)
	if err != nil {
		t.Fatalf("PollMetrics: %v", err)
	}
	if got := m["leah_panics_total"]; got != 0 {
		t.Errorf("leah_panics_total: got %v want 0", got)
	}
	// Labeled series flatten to "name{labels}" keys (last-value wins).
	wantKey := `leah_web_requests_total{path="/metrics"}`
	if got, ok := m[wantKey]; !ok || got != 17 {
		t.Errorf("missing or wrong %s: got %v ok=%v", wantKey, got, ok)
	}
}

func TestIPC_PollMetrics_NetworkErrorReturnsErr(t *testing.T) {
	c := NewClient("http://127.0.0.1:1") // unreachable port
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if _, err := c.PollMetrics(ctx); err == nil {
		t.Fatal("expected error from unreachable daemon, got nil")
	}
}

func TestIPC_PollEvents_StreamsEvents(t *testing.T) {
	// Server emits two SSE-flavored lines then closes.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/events" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fl, ok := w.(http.Flusher)
		if !ok {
			t.Errorf("ResponseWriter not Flusher")
			return
		}
		_, _ = fmt.Fprintln(w, "data: {\"kind\":\"weather\",\"text\":\"sunny\"}")
		fl.Flush()
		_, _ = fmt.Fprintln(w, "data: {\"kind\":\"news\",\"text\":\"hello\"}")
		fl.Flush()
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var (
		mu  sync.Mutex
		got []string
	)
	done := make(chan struct{})
	go func() {
		_ = c.StreamEvents(ctx, func(line string) {
			mu.Lock()
			got = append(got, line)
			if len(got) == 2 {
				cancel() // stop after both lines
			}
			mu.Unlock()
		})
		close(done)
	}()
	<-done

	mu.Lock()
	defer mu.Unlock()
	if len(got) < 2 {
		t.Fatalf("expected 2 events, got %d: %v", len(got), got)
	}
	if !strings.Contains(got[0], "sunny") {
		t.Errorf("event 0 missing payload: %q", got[0])
	}
	if !strings.Contains(got[1], "hello") {
		t.Errorf("event 1 missing payload: %q", got[1])
	}
}

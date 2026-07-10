package watchdog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestPingHitsURL(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	h := &Heartbeat{URL: srv.URL, HTTP: srv.Client()}
	if err := h.Ping(context.Background()); err != nil {
		t.Fatalf("ping: %v", err)
	}
	if hits != 1 {
		t.Errorf("hits: %d", hits)
	}
}

func TestPingSkipsWhenURLEmpty(t *testing.T) {
	h := &Heartbeat{URL: ""}
	if err := h.Ping(context.Background()); err != nil {
		t.Errorf("empty URL should skip silently, got %v", err)
	}
}

func TestPingReturnsErrorOn5xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()

	h := &Heartbeat{URL: srv.URL, HTTP: srv.Client()}
	if err := h.Ping(context.Background()); err == nil {
		t.Error("want error on 500")
	}
}

package hud

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFocus_Summon_OnHotkey(t *testing.T) {
	m := NewMachine()
	m.Show()
	f := NewFocus(m, "http://127.0.0.1:0")
	f.SummonHotkey()
	if got := m.State(); got != StateFocus {
		t.Fatalf("after SummonHotkey: got %v want %v", got, StateFocus)
	}
}

func TestFocus_AutoDismiss_30sIdle(t *testing.T) {
	m := NewMachine()
	m.Show()
	f := NewFocus(m, "http://127.0.0.1:0")
	start := time.Unix(1_700_000_000, 0)
	f.SummonAt(start)
	f.TickIdle(start.Add(29 * time.Second))
	if got := m.State(); got != StateFocus {
		t.Fatalf("at 29s idle: got %v want focus", got)
	}
	f.TickIdle(start.Add(31 * time.Second))
	if got := m.State(); got != StateAmbient {
		t.Fatalf("after 30s idle: got %v want ambient", got)
	}
}

func TestFocus_ReasonerError_ShowsFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	f := NewFocus(NewMachine(), srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	resp, err := f.Query(ctx, "what's the weather")
	if err == nil {
		t.Fatal("expected error from 500, got nil")
	}
	if resp.Text == "" || !strings.Contains(resp.Text, "unavailable") {
		t.Errorf("fallback text missing 'unavailable': %q", resp.Text)
	}
}

func TestFocus_EmptyQuery_NoSubmit(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		_ = json.NewEncoder(w).Encode(FocusResponse{Text: "ok"})
	}))
	defer srv.Close()

	f := NewFocus(NewMachine(), srv.URL)
	for _, q := range []string{"", "   ", "\t\n"} {
		_, err := f.Query(context.Background(), q)
		if !errors.Is(err, ErrEmptyQuery) {
			t.Errorf("Query(%q): err=%v want ErrEmptyQuery", q, err)
		}
	}
	if hits != 0 {
		t.Errorf("daemon hit on empty query: hits=%d", hits)
	}
}

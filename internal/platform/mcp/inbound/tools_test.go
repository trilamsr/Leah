package inbound

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestRegisterFirstParty_AllToolsMounted(t *testing.T) {
	srv := New(NewTokenStore(InMemory()))
	RegisterFirstParty(srv, FirstPartyDeps{})
	for _, want := range []string{
		"leah.memory.search",
		"leah.calendar.next",
		"leah.repo.cite",
		"leah.ask",
		"leah.widget.render",
	} {
		srv.mu.RLock()
		_, ok := srv.tools[want]
		srv.mu.RUnlock()
		if !ok {
			t.Errorf("tool %s not registered", want)
		}
	}
}

func TestAskHandler_ConsentDenied_SurfacesAsCode(t *testing.T) {
	srv := New(NewTokenStore(InMemory()))
	RegisterFirstParty(srv, FirstPartyDeps{
		Ask: func(_ context.Context, _ string) (string, error) {
			t.Fatal("Ask called despite consent deny")
			return "", nil
		},
		AskConsent: func(_ context.Context, _ string) error {
			return &ConsentDeniedError{Reason: "operator declined"}
		},
	})
	tok, _ := srv.Tokens().Issue(context.Background(), "n", []Scope{ScopeAskRun})
	tp := newPipe()
	tp.Push(Frame{ID: 1, Method: "tools/call", Token: tok.Plain, Params: mustJSON(map[string]any{"name": "leah.ask", "arguments": map[string]any{"prompt": "hi"}})})
	_ = tp.Close()
	if err := srv.Serve(context.Background(), tp); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	out := tp.Sent(0)
	if out.Error == nil || out.Error.Code != "consent.denied" {
		t.Fatalf("want consent.denied, got %+v", out.Error)
	}
}

func TestAskHandler_BudgetExhausted_BlocksUpstream(t *testing.T) {
	upstreamCalled := false
	srv := New(NewTokenStore(InMemory()))
	RegisterFirstParty(srv, FirstPartyDeps{
		Ask: func(_ context.Context, _ string) (string, error) {
			upstreamCalled = true
			return "", &BudgetExhaustedError{Bucket: "cloud.llm.tokens"}
		},
	})
	tok, _ := srv.Tokens().Issue(context.Background(), "n", []Scope{ScopeAskRun})
	tp := newPipe()
	tp.Push(Frame{ID: 1, Method: "tools/call", Token: tok.Plain, Params: mustJSON(map[string]any{"name": "leah.ask", "arguments": map[string]any{"prompt": "hi"}})})
	_ = tp.Close()
	if err := srv.Serve(context.Background(), tp); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	if !upstreamCalled {
		t.Fatal("Ask should be invoked to discover budget exhaustion")
	}
	out := tp.Sent(0)
	if out.Error == nil || out.Error.Code != "budget.exhausted" {
		t.Fatalf("want budget.exhausted, got %+v", out.Error)
	}
}

func TestMemorySearch_NoDeps_ReturnsInternal(t *testing.T) {
	srv := New(NewTokenStore(InMemory()))
	RegisterFirstParty(srv, FirstPartyDeps{})
	tok, _ := srv.Tokens().Issue(context.Background(), "n", []Scope{ScopeMemoryRead})
	tp := newPipe()
	tp.Push(Frame{ID: 1, Method: "tools/call", Token: tok.Plain, Params: mustJSON(map[string]any{"name": "leah.memory.search", "arguments": map[string]any{"query": "x"}})})
	_ = tp.Close()
	if err := srv.Serve(context.Background(), tp); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	out := tp.Sent(0)
	if out.Error == nil || out.Error.Code != "internal" {
		t.Fatalf("want internal, got %+v", out.Error)
	}
}

func TestIssueToken_RejectsUnknownScope(t *testing.T) {
	s := NewTokenStore(InMemory())
	if _, err := s.Issue(context.Background(), "x", []Scope{"made:up"}); !errors.Is(err, ErrUnknownScope) {
		t.Fatalf("err=%v want ErrUnknownScope", err)
	}
}

func TestMemorySearch_DefaultK(t *testing.T) {
	var seenK int
	srv := New(NewTokenStore(InMemory()))
	RegisterFirstParty(srv, FirstPartyDeps{
		MemorySearch: func(_ context.Context, _ string, k int) ([]MemoryHit, error) {
			seenK = k
			return []MemoryHit{{ID: "1", Text: "ok", Score: 0.5}}, nil
		},
	})
	tok, _ := srv.Tokens().Issue(context.Background(), "n", []Scope{ScopeMemoryRead})
	tp := newPipe()
	tp.Push(Frame{ID: 1, Method: "tools/call", Token: tok.Plain, Params: mustJSON(map[string]any{"name": "leah.memory.search", "arguments": map[string]any{"query": "ping"}})})
	_ = tp.Close()
	if err := srv.Serve(context.Background(), tp); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	if seenK != 5 {
		t.Fatalf("default k=5 not applied; saw k=%d", seenK)
	}
	var got map[string]any
	if err := json.Unmarshal(tp.Sent(0).Result, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["hits"] == nil {
		t.Fatal("hits missing from result")
	}
}

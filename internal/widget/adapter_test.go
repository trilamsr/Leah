package widget

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestPureAdapter_FetchReturnsPropsAsData(t *testing.T) {
	a := NewPureAdapter("stat")
	if a.Type() != "stat" {
		t.Fatalf("type: want stat got %q", a.Type())
	}
	props := json.RawMessage(`{"label":"PRs","value":5}`)
	p, err := a.Fetch(context.Background(), props)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if string(p.Data) != string(props) {
		t.Fatalf("data: want %s got %s", props, p.Data)
	}
	if p.StaleAfter != 0 {
		t.Fatalf("pure widgets never stale; got %v", p.StaleAfter)
	}
}

func TestRegistry_LookupUnknownReturnsError(t *testing.T) {
	r := NewRegistry()
	if _, ok := r.Lookup("hologram"); ok {
		t.Fatalf("expected miss")
	}
}

func TestAdapter_ContextCancellation(t *testing.T) {
	a := NewPureAdapter("stat")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := a.Fetch(ctx, json.RawMessage(`{}`))
	if err != context.Canceled {
		t.Fatalf("want canceled; got %v", err)
	}
}

func TestRefresh_EtagMatchReturnsPrev(t *testing.T) {
	a := NewPureAdapter("stat")
	prev := &Payload{Data: json.RawMessage(`{"v":1}`), Etag: "abc", FetchedAt: time.Now()}
	p, err := a.Refresh(context.Background(), "id1", json.RawMessage(`{}`), prev)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if p.Etag != "abc" || string(p.Data) != `{"v":1}` {
		t.Fatalf("pure refresh must echo prev; got %+v", p)
	}
}

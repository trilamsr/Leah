package inbound

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestServe_UnknownTool_ReturnsToolUnknown(t *testing.T) {
	srv := New(NewTokenStore(InMemory()))
	tok, _ := srv.Tokens().Issue(context.Background(), "n", []Scope{ScopeMemoryRead})
	tp := newPipe()
	tp.Push(Frame{ID: 1, Method: "tools/call", Token: tok.Plain, Params: mustJSON(map[string]any{"name": "leah.nope", "arguments": map[string]any{}})})
	_ = tp.Close()
	if err := srv.Serve(context.Background(), tp); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	out := tp.Sent(0)
	if out.Error == nil || out.Error.Code != "tool.unknown" {
		t.Fatalf("want error code tool.unknown, got %+v", out.Error)
	}
}

func TestServe_TokenMismatch_Returns401(t *testing.T) {
	srv := New(NewTokenStore(InMemory()))
	tp := newPipe()
	tp.Push(Frame{ID: 1, Method: "tools/call", Token: "bogus", Params: mustJSON(map[string]any{"name": "leah.memory.search"})})
	_ = tp.Close()
	if err := srv.Serve(context.Background(), tp); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	out := tp.Sent(0)
	if out.Error == nil || out.Error.Code != "unauthorized" {
		t.Fatalf("want unauthorized, got %+v", out.Error)
	}
}

func TestServe_ScopeMismatch_Forbidden(t *testing.T) {
	srv := New(NewTokenStore(InMemory()))
	srv.MustRegister(MCPTool{
		Name:           "leah.memory.search",
		RequiredScopes: []Scope{ScopeMemoryRead},
		Handler: func(_ context.Context, _ json.RawMessage) (any, error) {
			return map[string]any{"hits": []any{}}, nil
		},
	})
	tok, _ := srv.Tokens().Issue(context.Background(), "n", []Scope{ScopeAskRun})
	tp := newPipe()
	tp.Push(Frame{ID: 7, Method: "tools/call", Token: tok.Plain, Params: mustJSON(map[string]any{"name": "leah.memory.search", "arguments": map[string]any{}})})
	_ = tp.Close()
	if err := srv.Serve(context.Background(), tp); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	out := tp.Sent(0)
	if out.Error == nil || out.Error.Code != "forbidden" {
		t.Fatalf("want forbidden, got %+v", out.Error)
	}
}

func TestServe_DepthOverTwo_Rejected(t *testing.T) {
	srv := New(NewTokenStore(InMemory()))
	srv.MustRegister(MCPTool{
		Name:           "leah.ask",
		RequiredScopes: []Scope{ScopeAskRun},
		Handler: func(_ context.Context, _ json.RawMessage) (any, error) {
			return "ok", nil
		},
	})
	tok, _ := srv.Tokens().Issue(context.Background(), "n", []Scope{ScopeAskRun})
	tp := newPipe()
	tp.Push(Frame{ID: 1, Method: "tools/call", Token: tok.Plain, Depth: 3, Params: mustJSON(map[string]any{"name": "leah.ask", "arguments": map[string]any{}})})
	_ = tp.Close()
	if err := srv.Serve(context.Background(), tp); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	out := tp.Sent(0)
	if out.Error == nil || out.Error.Code != "depth.exceeded" {
		t.Fatalf("want depth.exceeded, got %+v", out.Error)
	}
}

func TestServe_BudgetExhausted_LeahAsk(t *testing.T) {
	srv := New(NewTokenStore(InMemory()))
	srv.MustRegister(MCPTool{
		Name:           "leah.ask",
		RequiredScopes: []Scope{ScopeAskRun},
		Handler: func(_ context.Context, _ json.RawMessage) (any, error) {
			return nil, &BudgetExhaustedError{Bucket: "cloud.llm.tokens"}
		},
	})
	tok, _ := srv.Tokens().Issue(context.Background(), "n", []Scope{ScopeAskRun})
	tp := newPipe()
	tp.Push(Frame{ID: 9, Method: "tools/call", Token: tok.Plain, Params: mustJSON(map[string]any{"name": "leah.ask", "arguments": map[string]any{"prompt": "hi"}})})
	_ = tp.Close()
	if err := srv.Serve(context.Background(), tp); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	out := tp.Sent(0)
	if out.Error == nil || out.Error.Code != "budget.exhausted" {
		t.Fatalf("want budget.exhausted, got %+v", out.Error)
	}
}

func TestServe_RateLimited_AfterFiveBadTokens(t *testing.T) {
	srv := New(NewTokenStore(InMemory()))
	tp := newPipe()
	for i := 0; i < 6; i++ {
		tp.Push(Frame{ID: int64(i), Method: "tools/call", Token: "bogus", Params: mustJSON(map[string]any{"name": "leah.memory.search"})})
	}
	_ = tp.Close()
	if err := srv.Serve(context.Background(), tp); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	for i := 0; i < 5; i++ {
		if c := tp.Sent(i).Error.Code; c != "unauthorized" {
			t.Fatalf("frame %d: want unauthorized, got %s", i, c)
		}
	}
	if c := tp.Sent(5).Error.Code; c != "rate.limited" {
		t.Fatalf("frame 5: want rate.limited, got %s", c)
	}
}

func TestServe_RegisterDuplicate_Errors(t *testing.T) {
	srv := New(NewTokenStore(InMemory()))
	tool := MCPTool{Name: "x", Handler: func(_ context.Context, _ json.RawMessage) (any, error) { return nil, nil }}
	if err := srv.Register(tool); err != nil {
		t.Fatalf("first register: %v", err)
	}
	if err := srv.Register(tool); !errors.Is(err, ErrDuplicateTool) {
		t.Fatalf("dup register: err=%v want ErrDuplicateTool", err)
	}
}

func TestServe_SuccessPath_RoundTripsResult(t *testing.T) {
	srv := New(NewTokenStore(InMemory()))
	srv.MustRegister(MCPTool{
		Name:           "leah.memory.search",
		RequiredScopes: []Scope{ScopeMemoryRead},
		Handler: func(_ context.Context, p json.RawMessage) (any, error) {
			var in struct {
				Query string `json:"query"`
			}
			if err := json.Unmarshal(p, &in); err != nil {
				return nil, err
			}
			return map[string]any{"echo": in.Query}, nil
		},
	})
	tok, _ := srv.Tokens().Issue(context.Background(), "n", []Scope{ScopeMemoryRead})
	tp := newPipe()
	tp.Push(Frame{ID: 42, Method: "tools/call", Token: tok.Plain, Params: mustJSON(map[string]any{"name": "leah.memory.search", "arguments": map[string]any{"query": "ping"}})})
	_ = tp.Close()
	if err := srv.Serve(context.Background(), tp); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	out := tp.Sent(0)
	if out.Error != nil {
		t.Fatalf("unexpected error: %+v", out.Error)
	}
	var got map[string]any
	if err := json.Unmarshal(out.Result, &got); err != nil {
		t.Fatalf("decode result: %v (raw=%s)", err, out.Result)
	}
	if got["echo"] != "ping" {
		t.Fatalf("round-trip lost payload: %v", got)
	}
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

// pipeTransport is an in-memory MCPTransport for tests. Push queues inbound
// frames; Sent reads what the server wrote back.
type pipeTransport struct {
	in     []Frame
	idx    int
	closed bool
	out    []Frame
}

func newPipe() *pipeTransport             { return &pipeTransport{} }
func (p *pipeTransport) Push(f Frame)     { p.in = append(p.in, f) }
func (p *pipeTransport) Close() error     { p.closed = true; return nil }
func (p *pipeTransport) Sent(i int) Frame { return p.out[i] }

func (p *pipeTransport) Read() (Frame, error) {
	if p.idx >= len(p.in) {
		return Frame{}, ErrTransportClosed
	}
	f := p.in[p.idx]
	p.idx++
	return f, nil
}

func (p *pipeTransport) Write(f Frame) error {
	p.out = append(p.out, f)
	return nil
}

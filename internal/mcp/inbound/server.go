package inbound

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
)

// Frame is the JSON-RPC-shaped envelope traded with peer MCP clients. The
// shape is a subset of MCP/1: ID + Method drive routing, Params/Result carry
// payloads, Error carries §5.8 error codes. Token is the per-call shared
// secret; Depth tracks caller-chain depth (§5.8 loop-protection).
type Frame struct {
	ID     int64           `json:"id"`
	Method string          `json:"method,omitempty"`
	Token  string          `json:"token,omitempty"`
	Depth  int             `json:"depth,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *FrameError     `json:"error,omitempty"`
}

// FrameError carries the §5.8 error code + a human message. Code is the
// stable identifier MCP peers branch on (tool.unknown, unauthorized,
// forbidden, budget.exhausted, depth.exceeded, consent.denied).
type FrameError struct {
	Code    string `json:"code"`
	Message string `json:"message,omitempty"`
}

// MCPTransport is the per-session read/write half. Implementations: stdio
// (CLI), local HTTP/SSE (loopback). Read returns ErrTransportClosed when the
// peer hangs up; Serve exits cleanly on that signal.
type MCPTransport interface {
	Read() (Frame, error)
	Write(Frame) error
	Close() error
}

// ErrTransportClosed signals normal session end; Serve treats it as EOF.
var ErrTransportClosed = errors.New("inbound: transport closed")

// ErrDuplicateTool is returned by Register when a tool with the same Name is
// already mounted. The §5.3 first-party set must register exactly once.
var ErrDuplicateTool = errors.New("inbound: duplicate tool")

// MaxCallerDepth caps the nested-ask chain per §5.8. Depth > MaxCallerDepth
// returns depth.exceeded; the proxy MUST increment Depth when forwarding.
const MaxCallerDepth = 2

// MCPTool is the registration record for a peer-callable tool. Handler is
// invoked only after token + scope + depth checks pass.
type MCPTool struct {
	Name           string
	Params         json.RawMessage
	Handler        func(ctx context.Context, p json.RawMessage) (any, error)
	RequiredScopes []Scope
}

// BudgetExhaustedError, when returned by a tool Handler, surfaces as the
// §5.8 budget.exhausted MCP error code instead of a generic internal error.
type BudgetExhaustedError struct {
	Bucket string
}

func (e *BudgetExhaustedError) Error() string {
	return "budget exhausted: " + e.Bucket
}

// ConsentDeniedError surfaces as the §5.8 consent.denied code.
type ConsentDeniedError struct{ Reason string }

func (e *ConsentDeniedError) Error() string { return "consent denied: " + e.Reason }

// Server hosts the inbound MCP surface. Construct via New; register tools
// up-front (registry is read-mostly so Serve stays lock-free per call).
type Server struct {
	tokens *TokenStore

	mu    sync.RWMutex
	tools map[string]MCPTool

	// SourceFn is the per-frame source-address provider (peer IP). When nil,
	// rate-limit recording uses a single shared bucket — acceptable for stdio
	// where there's no source distinction.
	SourceFn func() string
}

// New wires a Server. The token store is required; tools are added via
// Register or the spec-mandated MustRegisterFirstParty helper.
func New(tokens *TokenStore) *Server {
	return &Server{tokens: tokens, tools: map[string]MCPTool{}}
}

// Tokens exposes the underlying token store so the Settings/Connections pane
// can List/Issue/Revoke without round-tripping through the server.
func (s *Server) Tokens() *TokenStore { return s.tokens }

// Register mounts a tool. Returns ErrDuplicateTool on name collision so the
// composition root catches a double-wire at boot, not at first call.
func (s *Server) Register(t MCPTool) error {
	if t.Name == "" {
		return errors.New("inbound: tool name required")
	}
	if t.Handler == nil {
		return errors.New("inbound: tool handler required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.tools[t.Name]; ok {
		return fmt.Errorf("%w: %s", ErrDuplicateTool, t.Name)
	}
	s.tools[t.Name] = t
	return nil
}

// MustRegister panics on duplicate — used by composition root for the §5.3
// first-party set, where a duplicate is a coding bug, not a runtime error.
func (s *Server) MustRegister(t MCPTool) {
	if err := s.Register(t); err != nil {
		panic(err)
	}
}

// IssueToken delegates to the underlying store; satisfies the §5.5
// InboundMCP interface.
func (s *Server) IssueToken(ctx context.Context, name string, scopes []Scope) (Token, error) {
	return s.tokens.Issue(ctx, name, scopes)
}

// RevokeToken delegates to the underlying store.
func (s *Server) RevokeToken(ctx context.Context, id int64) error {
	return s.tokens.Revoke(ctx, id)
}

// Serve drains the transport until Read returns ErrTransportClosed. Per
// §5.7 every frame is auth+scope+depth checked before the tool runs.
func (s *Server) Serve(ctx context.Context, tp MCPTransport) error {
	defer func() { _ = tp.Close() }()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		f, err := tp.Read()
		if err != nil {
			if errors.Is(err, ErrTransportClosed) {
				return nil
			}
			return err
		}
		out := s.dispatch(ctx, f)
		if err := tp.Write(out); err != nil {
			return err
		}
	}
}

// dispatch validates a single frame and either invokes the tool or returns
// an error frame. Pulled out so unit tests can exercise the decision tree
// without a transport.
func (s *Server) dispatch(ctx context.Context, f Frame) Frame {
	resp := Frame{ID: f.ID}
	if f.Depth > MaxCallerDepth {
		resp.Error = &FrameError{Code: "depth.exceeded", Message: fmt.Sprintf("caller depth %d > %d", f.Depth, MaxCallerDepth)}
		return resp
	}
	tok, err := s.tokens.Verify(ctx, f.Token)
	if err != nil {
		src := "unknown"
		if s.SourceFn != nil {
			src = s.SourceFn()
		}
		_ = s.tokens.RecordFailure(ctx, src)
		resp.Error = &FrameError{Code: "unauthorized", Message: "token rejected"}
		return resp
	}
	name, params, perr := parseToolCall(f)
	if perr != nil {
		resp.Error = perr
		return resp
	}
	s.mu.RLock()
	tool, ok := s.tools[name]
	s.mu.RUnlock()
	if !ok {
		resp.Error = &FrameError{Code: "tool.unknown", Message: name}
		return resp
	}
	if !hasAllScopes(tok.Scopes, tool.RequiredScopes) {
		resp.Error = &FrameError{Code: "forbidden", Message: "scope insufficient"}
		return resp
	}
	res, herr := tool.Handler(ctx, params)
	if herr != nil {
		resp.Error = toFrameError(herr)
		return resp
	}
	b, merr := json.Marshal(res)
	if merr != nil {
		resp.Error = &FrameError{Code: "internal", Message: merr.Error()}
		return resp
	}
	resp.Result = b
	return resp
}

// parseToolCall extracts the {name, arguments} pair from a tools/call frame.
// Other Method values are accepted but treated as unknown for now — the
// §5.3 first-party set only uses tools/call.
func parseToolCall(f Frame) (string, json.RawMessage, *FrameError) {
	var p struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if len(f.Params) == 0 {
		return "", nil, &FrameError{Code: "invalid", Message: "missing params"}
	}
	if err := json.Unmarshal(f.Params, &p); err != nil {
		return "", nil, &FrameError{Code: "invalid", Message: err.Error()}
	}
	if p.Name == "" {
		return "", nil, &FrameError{Code: "invalid", Message: "tool name required"}
	}
	args := p.Arguments
	if len(args) == 0 {
		args = json.RawMessage("{}")
	}
	return p.Name, args, nil
}

func hasAllScopes(have, want []Scope) bool {
	if len(want) == 0 {
		return true
	}
	set := make(map[Scope]struct{}, len(have))
	for _, s := range have {
		set[s] = struct{}{}
	}
	for _, w := range want {
		if _, ok := set[w]; !ok {
			return false
		}
	}
	return true
}

// toFrameError maps known sentinel error types to stable MCP error codes.
// Unknown errors fall through as "internal" so a peer cannot fingerprint
// internal failure modes.
func toFrameError(err error) *FrameError {
	var be *BudgetExhaustedError
	if errors.As(err, &be) {
		return &FrameError{Code: "budget.exhausted", Message: be.Error()}
	}
	var ce *ConsentDeniedError
	if errors.As(err, &ce) {
		return &FrameError{Code: "consent.denied", Message: ce.Error()}
	}
	return &FrameError{Code: "internal", Message: err.Error()}
}

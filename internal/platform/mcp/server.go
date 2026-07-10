// Package mcp serves Leah's loopback MCP surface. Bind invariant: 127.0.0.1 only.
package mcp

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/trilam/leah/internal/platform/audit"
)

var (
	ErrNonLoopback = errors.New("mcp: non-loopback bind refused")
	ErrDisabled    = errors.New("mcp: disabled by LEAH_MCP_SERVER=0")
)

type ToolFunc func(body []byte) (any, int, error)

type Server struct {
	Addr             string
	AuditPath        string
	AuditLogger      *audit.Logger
	Tools            map[string]ToolFunc
	Now              func() time.Time
	RatePerMin       int // 0 → defaultRatePerMin
	GlobalPendingCap int // 0 → defaultGlobalPending

	// A2A agent-card + SelfBuild task endpoint. nil-safe: routes only
	// register when set.
	A2A *A2AHandler

	tokenMu sync.RWMutex
	token   string

	rateMu sync.Mutex
	rate   map[string][]time.Time

	pendingMu sync.Mutex
	pendingG  int
}

const (
	defaultRatePerMin    = 60
	defaultGlobalPending = 5
	envKillSwitch        = "LEAH_MCP_SERVER"
)

func NewServer(addr, token, auditPath string, logger *audit.Logger) *Server {
	return &Server{
		Addr:        addr,
		token:       token,
		AuditPath:   auditPath,
		AuditLogger: logger,
		Tools:       map[string]ToolFunc{},
	}
}

// SetToken rotates the bearer; old token rejected on the next call (no grace).
func (s *Server) SetToken(next string) {
	s.tokenMu.Lock()
	old := s.token
	s.token = next
	s.tokenMu.Unlock()
	s.audit("mcp_token_rotated", "old_sha="+peerHash(old), 0, "ok")
}

func (s *Server) Token() string {
	s.tokenMu.RLock()
	defer s.tokenMu.RUnlock()
	return s.token
}

// Register a tool handler under /tools/<name>. Must be called before Serve.
func (s *Server) Register(name string, fn ToolFunc) {
	if s.Tools == nil {
		s.Tools = map[string]ToolFunc{}
	}
	s.Tools[name] = fn
}

func (s *Server) Serve(ctx context.Context) error {
	if os.Getenv(envKillSwitch) == "0" {
		return ErrDisabled
	}
	host, _, err := net.SplitHostPort(s.Addr)
	if err != nil {
		return fmt.Errorf("mcp: parse addr: %w", err)
	}
	if !isLoopback(host) {
		return ErrNonLoopback
	}
	ln, err := net.Listen("tcp", s.Addr)
	if err != nil {
		return fmt.Errorf("mcp: listen: %w", err)
	}
	s.audit("mcp_server_start", "addr="+s.Addr, 0, "ok")
	mux := http.NewServeMux()
	for name, fn := range s.Tools {
		mux.HandleFunc("/tools/"+name, s.handle(name, fn))
	}
	if s.A2A != nil {
		mux.HandleFunc(AgentCardWellKnownPath, s.A2A.serveAgentCard(s.Addr))
		mux.HandleFunc("/a2a/tasks", s.handleA2ATask)
	}
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() { <-ctx.Done(); _ = srv.Close() }()
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func isLoopback(host string) bool {
	if host == "127.0.0.1" || host == "::1" || host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (s *Server) handle(name string, fn ToolFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tok := bearerFrom(r)
		if !s.tokenMatches(tok) {
			s.audit("mcp_auth_fail", "peer="+peerHash(tok), 0, "denied")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		peer := peerHash(tok)
		if !s.allowRate(peer) {
			s.audit("mcp_call", "peer="+peer+" tool="+name+" rate_limited", 0, "rate_limited")
			http.Error(w, "rate limited", http.StatusTooManyRequests)
			return
		}
		buf, _ := readBody(r)
		out, code, err := fn(buf)
		if err != nil {
			s.audit("mcp_call", "peer="+peer+" tool="+name+" err="+err.Error(), 0, "failed")
			http.Error(w, err.Error(), code)
			return
		}
		s.audit("mcp_call", "peer="+peer+" tool="+name, 0, "ok")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	}
}

func (s *Server) tokenMatches(got string) bool {
	s.tokenMu.RLock()
	want := s.token
	s.tokenMu.RUnlock()
	if want == "" || got == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(want), []byte(got)) == 1
}

func bearerFrom(r *http.Request) string {
	h := r.Header.Get("Authorization")
	const p = "Bearer "
	if !strings.HasPrefix(h, p) {
		return ""
	}
	return strings.TrimSpace(h[len(p):])
}

// PeerHash exposes the 8-hex peer fingerprint for audit details.
func PeerHash(token string) string { return peerHash(token) }

func peerHash(token string) string {
	if token == "" {
		return "anon"
	}
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:4])
}

// allowRate: per-peer rolling-60s req cap. Read = 60/min; write-tool
// attestation gets its own 1/min gate — not in this scope.
func (s *Server) allowRate(peer string) bool {
	cap := s.RatePerMin
	if cap == 0 {
		cap = defaultRatePerMin
	}
	now := s.now()
	cutoff := now.Add(-time.Minute)
	s.rateMu.Lock()
	defer s.rateMu.Unlock()
	if s.rate == nil {
		s.rate = map[string][]time.Time{}
	}
	hist := s.rate[peer]
	kept := hist[:0]
	for _, t := range hist {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= cap {
		s.rate[peer] = kept
		return false
	}
	s.rate[peer] = append(kept, now)
	return true
}

// EnqueuePending reserves a write-tool slot. capScope="global" when
// ok=false. The per-peer cap was dropped: per-peer attestation rate-limit
// (1/min) already prevents a single peer from holding >1 in-flight slot.
func (s *Server) EnqueuePending(peer string) (release func(), ok bool, capScope string) {
	gCap := s.GlobalPendingCap
	if gCap == 0 {
		gCap = defaultGlobalPending
	}
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	if s.pendingG >= gCap {
		return nil, false, "global"
	}
	s.pendingG++
	released := false
	return func() {
		s.pendingMu.Lock()
		defer s.pendingMu.Unlock()
		if released {
			return
		}
		released = true
		s.pendingG--
	}, true, ""
}

func (s *Server) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Server) audit(kind, detail string, br int, outcome string) {
	if s.AuditLogger == nil {
		return
	}
	_ = s.AuditLogger.Append(audit.Entry{Kind: kind, Detail: detail, BlastRadius: br, Outcome: outcome})
}

func readBody(r *http.Request) ([]byte, error) {
	defer func() { _ = r.Body.Close() }()
	const max = 1 << 20
	buf := make([]byte, 0, 512)
	tmp := make([]byte, 4096)
	for {
		n, err := r.Body.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
			if len(buf) > max {
				return nil, errors.New("body too large")
			}
		}
		if err != nil {
			break
		}
	}
	return buf, nil
}

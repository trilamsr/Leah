// Package inbound serves Leah's inbound MCP surface (phase4 §5.3 — third
// parties call Leah). Tokens are issued by the operator in Settings →
// Connections and scoped per call; the verifier is timing-safe + rate-limited
// per §5.7. The companion outbound MCP surface lives in `internal/mcp`.
package inbound

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Scope is one of the §5.3 capability strings. The set is closed — a token
// carrying a string outside this set must be rejected at issue time.
type Scope string

const (
	ScopeMemoryRead   Scope = "memory:read"
	ScopeCalendarRead Scope = "calendar:read"
	ScopeRepoRead     Scope = "repo:read"
	ScopeAskRun       Scope = "ask:run"
	ScopeWidgetRender Scope = "widget:render"
)

func validScopes() map[Scope]struct{} {
	return map[Scope]struct{}{
		ScopeMemoryRead: {}, ScopeCalendarRead: {}, ScopeRepoRead: {},
		ScopeAskRun: {}, ScopeWidgetRender: {},
	}
}

// Token is the per-client credential. Plain is populated ONLY by Issue —
// every subsequent read (List, lookup, audit) leaves it empty. Hash is the
// SHA-256 of (Salt || plain) per spec §5.7.
type Token struct {
	ID        int64
	Name      string
	Plain     string
	Hash      [32]byte
	Salt      [16]byte
	Scopes    []Scope
	IssuedAt  time.Time
	RevokedAt time.Time
}

// revoked reports whether RevokedAt has been set (non-zero).
func (t Token) revoked() bool { return !t.RevokedAt.IsZero() }

var (
	ErrUnauthorized  = errors.New("inbound: token unauthorized")
	ErrRateLimited   = errors.New("inbound: rate limited")
	ErrUnknownScope  = errors.New("inbound: unknown scope")
	ErrEmptyName     = errors.New("inbound: token name required")
	ErrTokenNotFound = errors.New("inbound: token not found")
)

// TokenBackend persists token rows. The mcp_token table from T05 is the
// production backend; tests inject InMemory().
type TokenBackend interface {
	Insert(ctx context.Context, t Token) (int64, error)
	List(ctx context.Context) ([]Token, error)
	Revoke(ctx context.Context, id int64, at time.Time) error
}

// TokenStore is the operator-facing token authority. It owns the in-memory
// lookup index keyed by plain-token-hash so Verify stays sub-ms; the backend
// is the source of truth at restart.
type TokenStore struct {
	backend TokenBackend
	now     func() time.Time

	mu     sync.RWMutex
	tokens map[int64]Token

	failMu sync.Mutex
	fails  map[string][]time.Time
}

// Option mutates a TokenStore at construction. WithClock is the test seam
// for deterministic rate-limit window tests; production passes time.Now.
type Option func(*TokenStore)

func WithClock(now func() time.Time) Option { return func(s *TokenStore) { s.now = now } }

// NewTokenStore wires the backend and seeds the in-memory index. Returning a
// concrete *TokenStore (not an interface) keeps the field surface explicit
// for ConnectionsPane wiring.
func NewTokenStore(b TokenBackend, opts ...Option) *TokenStore {
	s := &TokenStore{
		backend: b,
		now:     time.Now,
		tokens:  map[int64]Token{},
		fails:   map[string][]time.Time{},
	}
	for _, o := range opts {
		o(s)
	}
	if rows, err := b.List(context.Background()); err == nil {
		for _, r := range rows {
			s.tokens[r.ID] = r
		}
	}
	return s
}

// Issue mints a new token. The plain string is shown ONCE — caller is
// responsible for surfacing it to the operator. Subsequent reads return the
// row with Plain="". Scope set must be a subset of the §5.3 closed enum.
func (s *TokenStore) Issue(ctx context.Context, name string, scopes []Scope) (Token, error) {
	if name == "" {
		return Token{}, ErrEmptyName
	}
	valid := validScopes()
	for _, sc := range scopes {
		if _, ok := valid[sc]; !ok {
			return Token{}, fmt.Errorf("%w: %q", ErrUnknownScope, sc)
		}
	}
	var salt [16]byte
	if _, err := rand.Read(salt[:]); err != nil {
		return Token{}, err
	}
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return Token{}, err
	}
	plain := hex.EncodeToString(raw[:])
	hash := sha256.Sum256(append(salt[:], []byte(plain)...))
	row := Token{
		Name:     name,
		Plain:    plain,
		Hash:     hash,
		Salt:     salt,
		Scopes:   append([]Scope(nil), scopes...),
		IssuedAt: s.now(),
	}
	id, err := s.backend.Insert(ctx, row)
	if err != nil {
		return Token{}, err
	}
	row.ID = id
	s.mu.Lock()
	// Stored copy has no Plain — the only copy of Plain leaves via this return.
	stored := row
	stored.Plain = ""
	s.tokens[id] = stored
	s.mu.Unlock()
	return row, nil
}

// Verify resolves a plain token to its row. Comparison is constant-time
// across every active row; a wrong token costs the same wall-time as a right
// one, frustrating timing-oracle attacks per §5.7.
func (s *TokenStore) Verify(_ context.Context, plain string) (Token, error) {
	if plain == "" {
		return Token{}, ErrUnauthorized
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	var match Token
	found := 0
	for _, t := range s.tokens {
		if t.revoked() {
			continue
		}
		probe := sha256.Sum256(append(t.Salt[:], []byte(plain)...))
		if subtle.ConstantTimeCompare(probe[:], t.Hash[:]) == 1 {
			match = t
			found = 1
		}
	}
	if found == 0 {
		return Token{}, ErrUnauthorized
	}
	out := match
	out.Plain = ""
	return out, nil
}

// Revoke marks a token revoked at now(). Subsequent Verify rejects.
func (s *TokenStore) Revoke(ctx context.Context, id int64) error {
	now := s.now()
	if err := s.backend.Revoke(ctx, id, now); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tokens[id]
	if !ok {
		return ErrTokenNotFound
	}
	t.RevokedAt = now
	s.tokens[id] = t
	return nil
}

// List returns every issued token with Plain stripped. Used by
// ConnectionsPane to render the row table.
func (s *TokenStore) List(_ context.Context) ([]Token, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Token, 0, len(s.tokens))
	for _, t := range s.tokens {
		t.Plain = ""
		out = append(out, t)
	}
	return out, nil
}

// RecordFailure logs a 401 against a source key (IP). Returns ErrRateLimited
// once the source has crossed 5 fails in the last 60s. The window is rolling.
func (s *TokenStore) RecordFailure(_ context.Context, src string) error {
	s.failMu.Lock()
	defer s.failMu.Unlock()
	now := s.now()
	cutoff := now.Add(-time.Minute)
	hist := s.fails[src]
	pruned := hist[:0]
	for _, t := range hist {
		if t.After(cutoff) {
			pruned = append(pruned, t)
		}
	}
	if len(pruned) >= 5 {
		s.fails[src] = pruned
		return ErrRateLimited
	}
	pruned = append(pruned, now)
	s.fails[src] = pruned
	return nil
}

// InMemory is the test backend; production wires a sqlstore-backed backend.
func InMemory() TokenBackend { return &memBackend{} }

type memBackend struct {
	mu   sync.Mutex
	rows []Token
	next int64
}

func (m *memBackend) Insert(_ context.Context, t Token) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.next++
	t.ID = m.next
	m.rows = append(m.rows, t)
	return t.ID, nil
}

func (m *memBackend) List(_ context.Context) ([]Token, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Token, len(m.rows))
	copy(out, m.rows)
	return out, nil
}

func (m *memBackend) Revoke(_ context.Context, id int64, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, r := range m.rows {
		if r.ID == id {
			m.rows[i].RevokedAt = at
			return nil
		}
	}
	return ErrTokenNotFound
}

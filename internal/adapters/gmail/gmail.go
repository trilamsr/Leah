package gmail

import (
	"context"
	"errors"
	"fmt"
)

// Sentinel errors are kept exported so callers can switch on the failure mode
// without string-matching wrapped messages.
var (
	ErrAttestationDenied = errors.New("gmail: attestation denied")
	ErrMessageNotFound   = errors.New("gmail: message not found")
	ErrSendRejected      = errors.New("gmail: send rejected")
)

// Scopes the Attestor sees. Distinct per RPC so the operator-attestation log
// can attribute consent at the per-action grain (read vs. mutate vs. exfil).
const (
	ScopeList = "gmail:list_unread"
	ScopeMark = "gmail:mark_read"
	ScopeSend = "gmail:send"
)

// Message is the minimum send envelope the MVP supports; richer MIME (cc,
// bcc, attachments, threading) waits for the wiring wave so the surface stays
// stable while the daemon brief integration lands.
type Message struct {
	To      string
	Subject string
	Body    string
}

// Attestor is Leah's operator-attestation gate. Every secret-touching RPC
// calls Attest(scope) before the token leaves the TokenSource; a non-nil
// return aborts the RPC with ErrAttestationDenied (wrapped).
type Attestor interface {
	Attest(ctx context.Context, scope string) error
}

// TokenSource yields the OAuth bearer token. Production wiring reads it from
// $HOME/.leah-state/secrets/gmail-token.json (path documented in the spec);
// tests inject a fake to keep the suite hermetic.
type TokenSource interface {
	Token(ctx context.Context) (string, error)
}

// Transport is the seam between the adapter's policy layer (attestation,
// validation, error mapping) and the gmail/v1 SDK calls. Keeping it an
// interface defers the google.golang.org/api/gmail/v1 require until the
// wiring wave so MVP scaffolding does not block on a go.mod edit.
type Transport interface {
	ListUnread(ctx context.Context, token string) ([]string, error)
	MarkRead(ctx context.Context, token, id string) error
	Send(ctx context.Context, token string, msg Message) error
}

// Config carries the three collaborators the Client needs. All fields are
// required; New refuses to construct a half-wired Client because a missing
// Attestor would silently bypass the operator-consent gate.
type Config struct {
	Attestor    Attestor
	TokenSource TokenSource
	Transport   Transport
}

// Client is the Gmail adapter the rest of Leah depends on. Lifecycle (open
// network connections, refresh tokens, etc.) is owned by the caller — the
// constructor MUST NOT start background goroutines so daemon shutdown stays
// deterministic.
type Client struct {
	att Attestor
	ts  TokenSource
	tr  Transport
}

// New validates the wiring contract and returns a ready Client; no I/O.
func New(cfg Config) (*Client, error) {
	if cfg.Attestor == nil {
		return nil, errors.New("gmail: Config.Attestor required (operator-attestation gate)")
	}
	if cfg.TokenSource == nil {
		return nil, errors.New("gmail: Config.TokenSource required")
	}
	if cfg.Transport == nil {
		return nil, errors.New("gmail: Config.Transport required")
	}
	return &Client{att: cfg.Attestor, ts: cfg.TokenSource, tr: cfg.Transport}, nil
}

// ListUnread returns the IDs of unread messages in the operator's inbox.
func (c *Client) ListUnread(ctx context.Context) ([]string, error) {
	tok, err := c.gateAndToken(ctx, ScopeList)
	if err != nil {
		return nil, err
	}
	return c.tr.ListUnread(ctx, tok)
}

// MarkRead clears the UNREAD label on the given message id.
func (c *Client) MarkRead(ctx context.Context, id string) error {
	if id == "" {
		return ErrMessageNotFound
	}
	tok, err := c.gateAndToken(ctx, ScopeMark)
	if err != nil {
		return err
	}
	return c.tr.MarkRead(ctx, tok, id)
}

// Send sends msg. Validation runs before the attestation gate so the operator
// is not prompted to consent to a message that would be rejected anyway.
func (c *Client) Send(ctx context.Context, msg Message) error {
	if msg.To == "" {
		return fmt.Errorf("%w: missing recipient", ErrSendRejected)
	}
	tok, err := c.gateAndToken(ctx, ScopeSend)
	if err != nil {
		return err
	}
	return c.tr.Send(ctx, tok, msg)
}

// gateAndToken runs the attestation gate first; only on consent does the
// token leave the TokenSource. Ordering matters — token-load before
// attestation would leak the secret into a logger / panic trace even when
// the operator denies the action.
func (c *Client) gateAndToken(ctx context.Context, scope string) (string, error) {
	if err := c.att.Attest(ctx, scope); err != nil {
		return "", fmt.Errorf("%w: %v", ErrAttestationDenied, err)
	}
	tok, err := c.ts.Token(ctx)
	if err != nil {
		return "", fmt.Errorf("gmail: token load (%s): %w", scope, err)
	}
	return tok, nil
}

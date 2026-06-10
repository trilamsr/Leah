package confluence

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Sentinel errors so callers switch on failure mode without string-matching.
var (
	ErrAttestationDenied = errors.New("confluence: attestation denied")
	ErrAuthFailed        = errors.New("confluence: auth failed")
	ErrNotFound          = errors.New("confluence: not found")
	ErrRateLimited       = errors.New("confluence: rate limited")
)

// Per-RPC scopes so the operator-attestation audit log distinguishes read
// (list_recent, get_page) from search at the per-action grain.
const (
	ScopeListRecent = "confluence:list_recent"
	ScopeGetPage    = "confluence:get_page"
	ScopeSearch     = "confluence:search"
)

// Page is the read-side projection. Body is populated on GetPage only — List
// and Search return metadata to keep payload + audit-row size bounded.
type Page struct {
	ID       string
	Title    string
	SpaceKey string
	Body     string
	URL      string
	Updated  time.Time
}

// Attestor is Leah's operator-attestation gate. Every secret-touching RPC
// calls Attest(scope) before the token leaves the TokenSource; a non-nil
// return aborts the RPC with ErrAttestationDenied (wrapped).
type Attestor interface {
	Attest(ctx context.Context, scope string) error
}

// TokenSource yields the bearer (API-token mode in MVP, OAuth 3LO later).
type TokenSource interface {
	Token(ctx context.Context) (string, error)
}

// Transport is the seam between the adapter's policy layer (attestation,
// validation, error mapping) and the Atlassian REST / go-atlassian SDK calls.
// Keeping it an interface defers the SDK require to the W50 combined tidy.
type Transport interface {
	ListRecent(ctx context.Context, bearer, space string) ([]Page, error)
	GetPage(ctx context.Context, bearer, id string) (Page, error)
	SearchCQL(ctx context.Context, bearer, query string) ([]Page, error)
}

// Config carries the four collaborators the Client needs. All required; New
// refuses to construct a half-wired Client because a missing Attestor would
// silently bypass the operator-consent gate.
type Config struct {
	Attestor    Attestor
	TokenSource TokenSource
	Transport   Transport
	BaseURL     string
}

// Client is the Confluence adapter. Constructor performs no I/O so daemon
// boot stays deterministic.
type Client struct {
	att     Attestor
	ts      TokenSource
	tr      Transport
	baseURL string
}

// New validates the wiring contract and returns a ready Client; no I/O.
func New(cfg Config) (*Client, error) {
	if cfg.Attestor == nil {
		return nil, errors.New("confluence: Config.Attestor required (operator-attestation gate)")
	}
	if cfg.TokenSource == nil {
		return nil, errors.New("confluence: Config.TokenSource required")
	}
	if cfg.Transport == nil {
		return nil, errors.New("confluence: Config.Transport required")
	}
	if cfg.BaseURL == "" {
		return nil, errors.New("confluence: Config.BaseURL required (e.g. https://acme.atlassian.net/wiki)")
	}
	return &Client{att: cfg.Attestor, ts: cfg.TokenSource, tr: cfg.Transport, baseURL: cfg.BaseURL}, nil
}

// ListRecentPages returns pages recently updated in the given space.
func (c *Client) ListRecentPages(ctx context.Context, space string) ([]Page, error) {
	tok, err := c.gateAndToken(ctx, ScopeListRecent)
	if err != nil {
		return nil, err
	}
	return c.tr.ListRecent(ctx, tok, space)
}

// GetPage fetches a single page with storage-format body.
func (c *Client) GetPage(ctx context.Context, id string) (Page, error) {
	if id == "" {
		return Page{}, ErrNotFound
	}
	tok, err := c.gateAndToken(ctx, ScopeGetPage)
	if err != nil {
		return Page{}, err
	}
	return c.tr.GetPage(ctx, tok, id)
}

// SearchCQL runs a Confluence Query Language search. CQL escaping is the
// Transport's responsibility — raw-string concatenation into CQL is banned.
func (c *Client) SearchCQL(ctx context.Context, query string) ([]Page, error) {
	tok, err := c.gateAndToken(ctx, ScopeSearch)
	if err != nil {
		return nil, err
	}
	return c.tr.SearchCQL(ctx, tok, query)
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
		return "", fmt.Errorf("confluence: token load (%s): %w", scope, err)
	}
	return tok, nil
}

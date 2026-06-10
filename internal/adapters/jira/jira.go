package jira

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Sentinel errors are kept exported so callers can switch on the failure mode
// without string-matching wrapped messages.
var (
	ErrAttestationDenied = errors.New("jira: attestation denied")
	ErrAuthFailed        = errors.New("jira: auth failed")
	ErrNotFound          = errors.New("jira: not found")
	ErrRateLimited       = errors.New("jira: rate limited")
	ErrInvalidIssue      = errors.New("jira: invalid issue payload")
)

// Scopes the Attestor sees. Distinct per RPC so the operator-attestation log
// can attribute consent at the per-action grain (read vs. mutate).
const (
	ScopeListMy   = "jira:list_my_issues"
	ScopeGetIssue = "jira:get_issue"
	ScopeCreate   = "jira:create_issue"
	ScopeComment  = "jira:comment"
)

// Issue is the minimum projection of an Atlassian Cloud issue the MVP exposes.
// Richer fields (labels, priority, links) wait for the wiring wave so the
// surface stays stable while the daemon morning-brief integration lands.
type Issue struct {
	Key        string // public Jira key, e.g. "ENG-42"
	Summary    string
	Status     string
	Assignee   string
	ProjectKey string
	URL        string
	Updated    time.Time
}

// IssueReq is the create payload. ProjectKey + Summary + IssueType are
// required; missing fields short-circuit BEFORE the attestation gate so a
// typo does not consume an operator-consent prompt.
type IssueReq struct {
	ProjectKey  string
	Summary     string
	Description string
	IssueType   string // "Task", "Bug", "Story"
}

// Attestor is Leah's operator-attestation gate. Every secret-touching RPC
// calls Attest(scope) before the token leaves the TokenSource; a non-nil
// return aborts the RPC with ErrAttestationDenied (wrapped).
type Attestor interface {
	Attest(ctx context.Context, scope string) error
}

// TokenSource yields the Atlassian API-token bearer (email+token base64 pair
// at wiring time). Production wiring reads it from
// $HOME/.leah-state/secrets/jira-token.json (path documented in the spec);
// tests inject a fake to keep the suite hermetic.
type TokenSource interface {
	Token(ctx context.Context) (string, error)
}

// Transport is the seam between the adapter's policy layer (attestation,
// validation, error mapping) and the Atlassian Cloud REST v3 calls. Keeping
// it an interface defers the SDK require to the W50 combined tidy so MVP
// scaffolding does not block on a go.mod edit.
type Transport interface {
	ListMyIssues(ctx context.Context, bearer string) ([]Issue, error)
	GetIssue(ctx context.Context, bearer, key string) (Issue, error)
	CreateIssue(ctx context.Context, bearer string, req IssueReq) (Issue, error)
	Comment(ctx context.Context, bearer, key, body string) error
}

// Config carries the collaborators the Client needs. All fields are required;
// New refuses to construct a half-wired Client because a missing Attestor
// would silently bypass the operator-consent gate.
type Config struct {
	Attestor    Attestor
	TokenSource TokenSource
	Transport   Transport
	BaseURL     string // e.g. https://acme.atlassian.net
}

// Client is the Jira adapter the rest of Leah depends on. Lifecycle (HTTP
// pools, token refresh) is owned by the caller — the constructor MUST NOT
// start background goroutines so daemon shutdown stays deterministic.
type Client struct {
	att     Attestor
	ts      TokenSource
	tr      Transport
	baseURL string
}

// New validates the wiring contract and returns a ready Client; no I/O.
func New(cfg Config) (*Client, error) {
	if cfg.Attestor == nil {
		return nil, errors.New("jira: Config.Attestor required (operator-attestation gate)")
	}
	if cfg.TokenSource == nil {
		return nil, errors.New("jira: Config.TokenSource required")
	}
	if cfg.Transport == nil {
		return nil, errors.New("jira: Config.Transport required")
	}
	if cfg.BaseURL == "" {
		return nil, errors.New("jira: Config.BaseURL required")
	}
	return &Client{att: cfg.Attestor, ts: cfg.TokenSource, tr: cfg.Transport, baseURL: cfg.BaseURL}, nil
}

// ListMyIssues returns issues assigned to the operator. Default page size (50)
// is the Atlassian Cloud v3 default; pagination is a follow-up wave.
func (c *Client) ListMyIssues(ctx context.Context) ([]Issue, error) {
	tok, err := c.gateAndToken(ctx, ScopeListMy)
	if err != nil {
		return nil, err
	}
	return c.tr.ListMyIssues(ctx, tok)
}

// GetIssue resolves a single issue by its public Jira key (e.g. "ENG-42").
func (c *Client) GetIssue(ctx context.Context, key string) (Issue, error) {
	if key == "" {
		return Issue{}, fmt.Errorf("%w: empty key", ErrInvalidIssue)
	}
	tok, err := c.gateAndToken(ctx, ScopeGetIssue)
	if err != nil {
		return Issue{}, err
	}
	return c.tr.GetIssue(ctx, tok, key)
}

// CreateIssue creates an issue. Validation runs BEFORE attestation so a typo
// does not consume an operator-consent prompt.
func (c *Client) CreateIssue(ctx context.Context, req IssueReq) (Issue, error) {
	if req.ProjectKey == "" {
		return Issue{}, fmt.Errorf("%w: missing project key", ErrInvalidIssue)
	}
	if req.Summary == "" {
		return Issue{}, fmt.Errorf("%w: missing summary", ErrInvalidIssue)
	}
	if req.IssueType == "" {
		return Issue{}, fmt.Errorf("%w: missing issue type", ErrInvalidIssue)
	}
	tok, err := c.gateAndToken(ctx, ScopeCreate)
	if err != nil {
		return Issue{}, err
	}
	return c.tr.CreateIssue(ctx, tok, req)
}

// Comment posts a comment body on the issue identified by key. Validation runs
// BEFORE attestation (same rationale as CreateIssue).
func (c *Client) Comment(ctx context.Context, key, body string) error {
	if key == "" {
		return fmt.Errorf("%w: empty key", ErrInvalidIssue)
	}
	if body == "" {
		return fmt.Errorf("%w: empty body", ErrInvalidIssue)
	}
	tok, err := c.gateAndToken(ctx, ScopeComment)
	if err != nil {
		return err
	}
	return c.tr.Comment(ctx, tok, key, body)
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
		return "", fmt.Errorf("jira: token load (%s): %w", scope, err)
	}
	return tok, nil
}

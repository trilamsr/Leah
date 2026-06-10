package linear

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// Endpoint is Linear's GraphQL endpoint. Linear is GraphQL-only — no REST.
const Endpoint = "https://api.linear.app/graphql"

// Scopes the Attestor sees. Distinct per RPC so consent is attributed
// per-action (read vs. mutate).
const (
	ScopeListMy   = "linear:list_my"
	ScopeGetIssue = "linear:get_issue"
	ScopeCreate   = "linear:create_issue"
	ScopeComment  = "linear:comment"
)

// Conservative MVP rate budget: writes (create + comment) share 10 per 60s.
const (
	writeBudget = 10
	writeWindow = 60 * time.Second
)

var (
	ErrAttestationDenied = errors.New("linear: attestation denied")
	ErrAuthFailed        = errors.New("linear: auth failed")
	ErrNotFound          = errors.New("linear: not found")
	ErrRateLimited       = errors.New("linear: rate limited")
	ErrInvalidIssue      = errors.New("linear: invalid issue payload")
)

type Issue struct {
	ID         string
	Identifier string
	Title      string
	State      string
	TeamKey    string
	URL        string
	Updated    time.Time
}

type IssueReq struct {
	TeamID      string
	Title       string
	Description string
	Priority    int
}

type Attestor interface {
	Attest(ctx context.Context, scope string) error
}

type TokenSource interface {
	Token(ctx context.Context) (string, error)
}

type Transport interface {
	ListMyIssues(ctx context.Context, bearer string) ([]Issue, error)
	GetIssue(ctx context.Context, bearer, id string) (Issue, error)
	CreateIssue(ctx context.Context, bearer string, req IssueReq) (Issue, error)
	Comment(ctx context.Context, bearer, id, body string) error
}

type Config struct {
	Attestor    Attestor
	TokenSource TokenSource
	Transport   Transport
	Now         func() time.Time
}

type Client struct {
	att Attestor
	ts  TokenSource
	tr  Transport
	now func() time.Time

	mu     sync.Mutex
	writes []time.Time // sliding-window timestamps for create+comment
}

func New(cfg Config) (*Client, error) {
	if cfg.Attestor == nil {
		return nil, errors.New("linear: Config.Attestor required (operator-attestation gate)")
	}
	if cfg.TokenSource == nil {
		return nil, errors.New("linear: Config.TokenSource required")
	}
	if cfg.Transport == nil {
		return nil, errors.New("linear: Config.Transport required")
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &Client{att: cfg.Attestor, ts: cfg.TokenSource, tr: cfg.Transport, now: now}, nil
}

func (c *Client) ListMyIssues(ctx context.Context) ([]Issue, error) {
	tok, err := c.gateAndToken(ctx, ScopeListMy)
	if err != nil {
		return nil, err
	}
	return c.tr.ListMyIssues(ctx, tok)
}

func (c *Client) GetIssue(ctx context.Context, id string) (Issue, error) {
	if id == "" {
		return Issue{}, fmt.Errorf("%w: empty id", ErrInvalidIssue)
	}
	tok, err := c.gateAndToken(ctx, ScopeGetIssue)
	if err != nil {
		return Issue{}, err
	}
	return c.tr.GetIssue(ctx, tok, id)
}

func (c *Client) CreateIssue(ctx context.Context, req IssueReq) (Issue, error) {
	if req.TeamID == "" {
		return Issue{}, fmt.Errorf("%w: missing TeamID", ErrInvalidIssue)
	}
	if req.Title == "" {
		return Issue{}, fmt.Errorf("%w: missing Title", ErrInvalidIssue)
	}
	if err := c.reserveWrite(); err != nil {
		return Issue{}, err
	}
	tok, err := c.gateAndToken(ctx, ScopeCreate)
	if err != nil {
		return Issue{}, err
	}
	return c.tr.CreateIssue(ctx, tok, req)
}

func (c *Client) Comment(ctx context.Context, id, body string) error {
	if id == "" {
		return fmt.Errorf("%w: empty id", ErrInvalidIssue)
	}
	if body == "" {
		return fmt.Errorf("%w: empty body", ErrInvalidIssue)
	}
	if err := c.reserveWrite(); err != nil {
		return err
	}
	tok, err := c.gateAndToken(ctx, ScopeComment)
	if err != nil {
		return err
	}
	return c.tr.Comment(ctx, tok, id, body)
}

// gateAndToken: attestation runs before TokenSource.Token so a denied call
// never materializes the bearer; ordering is the load-bearing invariant.
func (c *Client) gateAndToken(ctx context.Context, scope string) (string, error) {
	if err := c.att.Attest(ctx, scope); err != nil {
		return "", fmt.Errorf("%w: %v", ErrAttestationDenied, err)
	}
	tok, err := c.ts.Token(ctx)
	if err != nil {
		return "", fmt.Errorf("linear: token load (%s): %w", scope, err)
	}
	return tok, nil
}

// reserveWrite enforces the shared create+comment budget before attestation
// so a burst-rejected call never consumes an operator consent prompt.
func (c *Client) reserveWrite() error {
	now := c.now()
	cutoff := now.Add(-writeWindow)
	c.mu.Lock()
	defer c.mu.Unlock()
	kept := c.writes[:0]
	for _, t := range c.writes {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	c.writes = kept
	if len(c.writes) >= writeBudget {
		return ErrRateLimited
	}
	c.writes = append(c.writes, now)
	return nil
}

// HTTPTransport is the default Transport implementation: direct GraphQL POSTs
// to Linear's endpoint, no genqlient. Adopt-over-build defers the codegen
// dependency until wiring-wave demand.
type HTTPTransport struct {
	hc       *http.Client
	endpoint string
}

func NewHTTPTransport(hc *http.Client, endpoint string) *HTTPTransport {
	if hc == nil {
		hc = http.DefaultClient
	}
	if endpoint == "" {
		endpoint = Endpoint
	}
	return &HTTPTransport{hc: hc, endpoint: endpoint}
}

const listMyIssuesQuery = `query LeahListMyIssues {
  viewer {
    assignedIssues(first: 50, filter: {state: {type: {nin: ["completed","canceled"]}}}) {
      nodes { id identifier title state { name } team { key } url updatedAt }
    }
  }
}`

const getIssueQuery = `query LeahGetIssue($id: String!) {
  issue(id: $id) { id identifier title state { name } team { key } url updatedAt }
}`

const createIssueMutation = `mutation LeahCreateIssue($teamId: String!, $title: String!, $description: String, $priority: Int) {
  issueCreate(input: {teamId: $teamId, title: $title, description: $description, priority: $priority}) {
    success
    issue { id identifier title state { name } team { key } url updatedAt }
  }
}`

const commentCreateMutation = `mutation LeahComment($issueId: String!, $body: String!) {
  commentCreate(input: {issueId: $issueId, body: $body}) { success }
}`

type issueNode struct {
	ID         string `json:"id"`
	Identifier string `json:"identifier"`
	Title      string `json:"title"`
	State      struct {
		Name string `json:"name"`
	} `json:"state"`
	Team struct {
		Key string `json:"key"`
	} `json:"team"`
	URL       string    `json:"url"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (n issueNode) toIssue() Issue {
	return Issue{
		ID:         n.ID,
		Identifier: n.Identifier,
		Title:      n.Title,
		State:      n.State.Name,
		TeamKey:    n.Team.Key,
		URL:        n.URL,
		Updated:    n.UpdatedAt,
	}
}

type gqlError struct {
	Message    string         `json:"message"`
	Extensions map[string]any `json:"extensions"`
}

func (h *HTTPTransport) ListMyIssues(ctx context.Context, bearer string) ([]Issue, error) {
	var resp struct {
		Data struct {
			Viewer struct {
				AssignedIssues struct {
					Nodes []issueNode `json:"nodes"`
				} `json:"assignedIssues"`
			} `json:"viewer"`
		} `json:"data"`
		Errors []gqlError `json:"errors"`
	}
	if err := h.post(ctx, bearer, listMyIssuesQuery, nil, &resp); err != nil {
		return nil, err
	}
	if err := classifyErrors(resp.Errors); err != nil {
		return nil, err
	}
	out := make([]Issue, 0, len(resp.Data.Viewer.AssignedIssues.Nodes))
	for _, n := range resp.Data.Viewer.AssignedIssues.Nodes {
		out = append(out, n.toIssue())
	}
	return out, nil
}

func (h *HTTPTransport) GetIssue(ctx context.Context, bearer, id string) (Issue, error) {
	var resp struct {
		Data struct {
			Issue *issueNode `json:"issue"`
		} `json:"data"`
		Errors []gqlError `json:"errors"`
	}
	if err := h.post(ctx, bearer, getIssueQuery, map[string]any{"id": id}, &resp); err != nil {
		return Issue{}, err
	}
	if err := classifyErrors(resp.Errors); err != nil {
		return Issue{}, err
	}
	if resp.Data.Issue == nil {
		return Issue{}, ErrNotFound
	}
	return resp.Data.Issue.toIssue(), nil
}

func (h *HTTPTransport) CreateIssue(ctx context.Context, bearer string, req IssueReq) (Issue, error) {
	vars := map[string]any{
		"teamId":      req.TeamID,
		"title":       req.Title,
		"description": req.Description,
		"priority":    req.Priority,
	}
	var resp struct {
		Data struct {
			IssueCreate struct {
				Success bool       `json:"success"`
				Issue   *issueNode `json:"issue"`
			} `json:"issueCreate"`
		} `json:"data"`
		Errors []gqlError `json:"errors"`
	}
	if err := h.post(ctx, bearer, createIssueMutation, vars, &resp); err != nil {
		return Issue{}, err
	}
	if err := classifyErrors(resp.Errors); err != nil {
		return Issue{}, err
	}
	if !resp.Data.IssueCreate.Success || resp.Data.IssueCreate.Issue == nil {
		return Issue{}, fmt.Errorf("%w: server reported no success", ErrInvalidIssue)
	}
	return resp.Data.IssueCreate.Issue.toIssue(), nil
}

func (h *HTTPTransport) Comment(ctx context.Context, bearer, id, body string) error {
	vars := map[string]any{"issueId": id, "body": body}
	var resp struct {
		Data struct {
			CommentCreate struct {
				Success bool `json:"success"`
			} `json:"commentCreate"`
		} `json:"data"`
		Errors []gqlError `json:"errors"`
	}
	if err := h.post(ctx, bearer, commentCreateMutation, vars, &resp); err != nil {
		return err
	}
	if err := classifyErrors(resp.Errors); err != nil {
		return err
	}
	if !resp.Data.CommentCreate.Success {
		return fmt.Errorf("linear: comment not accepted")
	}
	return nil
}

// post issues a GraphQL POST with Linear's API-key Authorization header
// (no "Bearer " prefix; raw key by Linear convention). The bearer MUST NOT
// appear in any returned error string.
func (h *HTTPTransport) post(ctx context.Context, bearer, query string, vars map[string]any, out any) error {
	payload, err := json.Marshal(struct {
		Query     string         `json:"query"`
		Variables map[string]any `json:"variables,omitempty"`
	}{Query: query, Variables: vars})
	if err != nil {
		return fmt.Errorf("linear: marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("linear: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", bearer)

	resp, err := h.hc.Do(req)
	if err != nil {
		return fmt.Errorf("linear: post: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("linear: read body: %w", err)
	}
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return ErrAuthFailed
	case http.StatusTooManyRequests:
		return ErrRateLimited
	case http.StatusNotFound:
		return ErrNotFound
	}
	if resp.StatusCode >= 500 {
		return fmt.Errorf("linear: server error %d", resp.StatusCode)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("linear: decode: %w", err)
	}
	return nil
}

func classifyErrors(errs []gqlError) error {
	if len(errs) == 0 {
		return nil
	}
	for _, e := range errs {
		if t, _ := e.Extensions["type"].(string); t == "not_found" {
			return ErrNotFound
		}
	}
	return fmt.Errorf("linear: graphql error: %s", errs[0].Message)
}

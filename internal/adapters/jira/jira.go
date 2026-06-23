package jira

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/trilam/leah/internal/adapters/atlassian"
	"github.com/trilam/leah/internal/obs/connectadapter"
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
	// Metrics is optional — nil is a no-op (connectadapter contract), so
	// existing callers keep working without a registry.
	Metrics *connectadapter.Metrics
}

// Client is the Jira adapter the rest of Leah depends on. Lifecycle (HTTP
// pools, token refresh) is owned by the caller — the constructor MUST NOT
// start background goroutines so daemon shutdown stays deterministic.
type Client struct {
	att     Attestor
	ts      TokenSource
	tr      Transport
	baseURL string
	m       *connectadapter.Metrics
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
	return &Client{att: cfg.Attestor, ts: cfg.TokenSource, tr: cfg.Transport, baseURL: cfg.BaseURL, m: cfg.Metrics}, nil
}

// ListMyIssues returns issues assigned to the operator. Default page size (50)
// is the Atlassian Cloud v3 default; pagination is a follow-up wave.
func (c *Client) ListMyIssues(ctx context.Context) ([]Issue, error) {
	tok, err := c.gateAndToken(ctx, ScopeListMy)
	if err != nil {
		return nil, err
	}
	start := time.Now()
	out, err := c.tr.ListMyIssues(ctx, tok)
	c.m.ObserveAPI("list_my_issues", time.Since(start).Seconds())
	return out, err
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
	start := time.Now()
	out, err := c.tr.GetIssue(ctx, tok, key)
	c.m.ObserveAPI("get_issue", time.Since(start).Seconds())
	return out, err
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
	start := time.Now()
	out, err := c.tr.CreateIssue(ctx, tok, req)
	c.m.ObserveAPI("create_issue", time.Since(start).Seconds())
	return out, err
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
	start := time.Now()
	err = c.tr.Comment(ctx, tok, key, body)
	c.m.ObserveAPI("comment", time.Since(start).Seconds())
	return err
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

// HTTPTransport is the default Transport: hand-rolled Atlassian Cloud REST v3 (go-atlassian SDK deferred to W50, adopt-over-build).
type HTTPTransport struct {
	hc      *http.Client
	baseURL string
}

func NewHTTPTransport(hc *http.Client, baseURL string) *HTTPTransport {
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second}
	}
	return &HTTPTransport{hc: hc, baseURL: strings.TrimRight(baseURL, "/")}
}

type issueResp struct {
	Key    string `json:"key"`
	Fields struct {
		Summary string `json:"summary"`
		Status  struct {
			Name string `json:"name"`
		} `json:"status"`
		Assignee struct {
			DisplayName string `json:"displayName"`
		} `json:"assignee"`
		Project struct {
			Key string `json:"key"`
		} `json:"project"`
		Updated jiraTime `json:"updated"`
	} `json:"fields"`
}

// jiraTime decodes Jira Cloud's "...-0700" stamp; the colon-less zone is not RFC3339, so time.Time's default unmarshal rejects it.
type jiraTime struct{ time.Time }

func (j *jiraTime) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		return nil
	}
	t, err := time.Parse("2006-01-02T15:04:05.000-0700", s)
	if err != nil {
		return err
	}
	j.Time = t
	return nil
}

func (h *HTTPTransport) toIssue(r issueResp) Issue {
	return Issue{
		Key:        r.Key,
		Summary:    r.Fields.Summary,
		Status:     r.Fields.Status.Name,
		Assignee:   r.Fields.Assignee.DisplayName,
		ProjectKey: r.Fields.Project.Key,
		URL:        h.baseURL + "/browse/" + r.Key,
		Updated:    r.Fields.Updated.Time,
	}
}

func (h *HTTPTransport) ListMyIssues(ctx context.Context, bearer string) ([]Issue, error) {
	var out struct {
		Issues []issueResp `json:"issues"`
	}
	q := url.Values{
		"jql":    {"assignee = currentUser() ORDER BY updated DESC"},
		"fields": {"summary,status,assignee,project,updated"},
	}
	if err := h.do(ctx, http.MethodGet, "/rest/api/3/search/jql?"+q.Encode(), bearer, nil, &out); err != nil {
		return nil, err
	}
	issues := make([]Issue, len(out.Issues))
	for i, r := range out.Issues {
		issues[i] = h.toIssue(r)
	}
	return issues, nil
}

func (h *HTTPTransport) GetIssue(ctx context.Context, bearer, key string) (Issue, error) {
	var out issueResp
	q := url.Values{"fields": {"summary,status,assignee,project,updated"}}
	if err := h.do(ctx, http.MethodGet, "/rest/api/3/issue/"+url.PathEscape(key)+"?"+q.Encode(), bearer, nil, &out); err != nil {
		return Issue{}, err
	}
	return h.toIssue(out), nil
}

func (h *HTTPTransport) CreateIssue(ctx context.Context, bearer string, req IssueReq) (Issue, error) {
	body := map[string]any{"fields": map[string]any{
		"project":     map[string]string{"key": req.ProjectKey},
		"summary":     req.Summary,
		"issuetype":   map[string]string{"name": req.IssueType},
		"description": adf(req.Description),
	}}
	var out struct {
		Key string `json:"key"`
	}
	if err := h.do(ctx, http.MethodPost, "/rest/api/3/issue", bearer, body, &out); err != nil {
		return Issue{}, err
	}
	return Issue{Key: out.Key, Summary: req.Summary, ProjectKey: req.ProjectKey, URL: h.baseURL + "/browse/" + out.Key}, nil
}

func (h *HTTPTransport) Comment(ctx context.Context, bearer, key, body string) error {
	payload := map[string]any{"body": adf(body)}
	return h.do(ctx, http.MethodPost, "/rest/api/3/issue/"+url.PathEscape(key)+"/comment", bearer, payload, nil)
}

// adf wraps plain text in a minimal Atlassian Document Format doc; v3 description / comment fields reject raw strings.
func adf(text string) map[string]any {
	return map[string]any{
		"type":    "doc",
		"version": 1,
		"content": []map[string]any{{
			"type":    "paragraph",
			"content": []map[string]any{{"type": "text", "text": text}},
		}},
	}
}

func (h *HTTPTransport) do(ctx context.Context, method, path, bearer string, body, out any) error {
	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("jira: encode request: %w", err)
		}
		rdr = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, h.baseURL+path, rdr)
	if err != nil {
		return fmt.Errorf("jira: build request: %w", err)
	}
	req.Header.Set("Authorization", bearer)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := h.hc.Do(req)
	if err != nil {
		return fmt.Errorf("jira: request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := mapStatus(resp.StatusCode); err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("jira: read body: %w", err)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("jira: decode: %w", err)
	}
	return nil
}

// mapStatus re-labels shared atlassian sentinels as jira.Err* so callers switch on this package's errors without importing atlassian.
func mapStatus(code int) error {
	switch atlassian.MapStatus(code) {
	case nil:
		return nil
	case atlassian.ErrAuthFailed:
		return ErrAuthFailed
	case atlassian.ErrNotFound:
		return ErrNotFound
	case atlassian.ErrRateLimited:
		return ErrRateLimited
	default:
		return fmt.Errorf("jira: upstream status %d", code)
	}
}

// AuditDetail emits issue_key + project_key only, never Summary/Description, so user-authored text cannot leak into the audit log.
func AuditDetail(i Issue) string {
	return fmt.Sprintf("issue_key=%s project_key=%s", i.Key, i.ProjectKey)
}

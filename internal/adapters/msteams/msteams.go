package msteams

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
	"unicode/utf8"

	"github.com/trilam/leah/internal/contracts"
)

const graphBase = "https://graph.microsoft.com/v1.0"

// Scopes are per-RPC so the operator-attestation log can attribute consent at
// the action grain (list vs. mutate vs. exfil).
const (
	ScopeListTeams    = "msteams:list_teams"
	ScopeListChannels = "msteams:list_channels"
	ScopePost         = "msteams:post_message"
	ScopeSearch       = "msteams:search"
)

var (
	ErrAttestationDenied = errors.New("msteams: attestation denied")
	ErrAuthFailed        = errors.New("msteams: auth failed")
	ErrNotFound          = errors.New("msteams: not found")
	ErrRateLimited       = errors.New("msteams: rate limited")
	ErrInvalidChannel    = errors.New("msteams: invalid channel")
)

type Team struct {
	ID          string
	DisplayName string
}

type Channel struct {
	ID          string
	TeamID      string
	DisplayName string
}

type Result struct {
	TeamID    string
	ChannelID string
	MessageID string
	Snippet   string
	URL       string
}

// Attestor is Leah's operator-attestation gate; mirror of internal/contracts.
type Attestor interface {
	Attest(ctx context.Context, scope string) error
}

// AuditRow omits plaintext channel ID, message body, query, and snippets by
// design (spec §7). TeamID is plaintext (tenant-scoped GUID, not human-readable).
type AuditRow struct {
	Kind        string
	Success     bool
	TeamID      string
	ChannelHash string
	TextLen     int
	QueryHash   string
	Reason      string
}

type AuditSink interface {
	Record(row AuditRow)
}

type Config struct {
	Attestor    Attestor
	TokenSource contracts.TokenSource
	HTTPClient  contracts.HTTPClient
	Audit       AuditSink
	Now         func() time.Time
}

type Client struct {
	att   Attestor
	ts    contracts.TokenSource
	http  contracts.HTTPClient
	audit AuditSink
	now   func() time.Time
}

// New refuses construction when any policy collaborator is missing; the
// audit sink stays optional because not every wiring path needs structured
// rows yet (W51 connects audit.jsonl).
func New(cfg Config) (*Client, error) {
	if cfg.Attestor == nil {
		return nil, errors.New("msteams: Config.Attestor required (operator-attestation gate)")
	}
	if cfg.TokenSource == nil {
		return nil, errors.New("msteams: Config.TokenSource required")
	}
	if cfg.HTTPClient == nil {
		return nil, errors.New("msteams: Config.HTTPClient required")
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &Client{
		att:   cfg.Attestor,
		ts:    cfg.TokenSource,
		http:  cfg.HTTPClient,
		audit: cfg.Audit,
		now:   now,
	}, nil
}

func (c *Client) ListTeams(ctx context.Context) ([]Team, error) {
	tok, err := c.gateAndToken(ctx, ScopeListTeams)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Value []struct {
			ID          string `json:"id"`
			DisplayName string `json:"displayName"`
		} `json:"value"`
	}
	if err := c.get(ctx, tok, graphBase+"/me/joinedTeams", &resp); err != nil {
		return nil, err
	}
	teams := make([]Team, 0, len(resp.Value))
	for _, t := range resp.Value {
		teams = append(teams, Team{ID: t.ID, DisplayName: t.DisplayName})
	}
	return teams, nil
}

func (c *Client) ListChannels(ctx context.Context, teamID string) ([]Channel, error) {
	if teamID == "" {
		return nil, ErrInvalidChannel
	}
	tok, err := c.gateAndToken(ctx, ScopeListChannels)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Value []struct {
			ID          string `json:"id"`
			DisplayName string `json:"displayName"`
		} `json:"value"`
	}
	endpoint := fmt.Sprintf("%s/teams/%s/channels", graphBase, url.PathEscape(teamID))
	if err := c.get(ctx, tok, endpoint, &resp); err != nil {
		return nil, err
	}
	chs := make([]Channel, 0, len(resp.Value))
	for _, ch := range resp.Value {
		chs = append(chs, Channel{ID: ch.ID, TeamID: teamID, DisplayName: ch.DisplayName})
	}
	return chs, nil
}

// PostMessage order is load-bearing: validate -> attest -> token -> POST.
// Empty-arg validation runs first so a typo cannot consume an attestation
// prompt (spec §4).
func (c *Client) PostMessage(ctx context.Context, teamID, channelID, text string) error {
	hash := shortHash(channelID)
	textLen := utf8.RuneCountInString(text)
	row := AuditRow{Kind: "msteams_post_message", TeamID: teamID, ChannelHash: hash, TextLen: textLen}

	if teamID == "" || channelID == "" || text == "" {
		row.Reason = "invalid_channel"
		c.record(row)
		return ErrInvalidChannel
	}
	if err := c.att.Attest(ctx, ScopePost); err != nil {
		row.Reason = "attestation_denied"
		c.record(row)
		return fmt.Errorf("%w: %v", ErrAttestationDenied, err)
	}
	tok, err := c.ts.Token(ctx)
	if err != nil {
		row.Reason = "token_load_failed"
		c.record(row)
		return fmt.Errorf("msteams: token load (%s): %w", ScopePost, err)
	}

	body := map[string]any{
		"body": map[string]string{"contentType": "text", "content": text},
	}
	endpoint := fmt.Sprintf("%s/teams/%s/channels/%s/messages", graphBase, url.PathEscape(teamID), url.PathEscape(channelID))
	if err := c.post(ctx, tok, endpoint, body, nil); err != nil {
		row.Reason = errReason(err)
		c.record(row)
		return err
	}
	row.Success = true
	c.record(row)
	return nil
}

func (c *Client) Search(ctx context.Context, query string) ([]Result, error) {
	queryHash := shortHash(query)
	row := AuditRow{Kind: "msteams_search", QueryHash: queryHash}

	if query == "" {
		row.Reason = "invalid_channel"
		c.record(row)
		return nil, ErrInvalidChannel
	}
	if err := c.att.Attest(ctx, ScopeSearch); err != nil {
		row.Reason = "attestation_denied"
		c.record(row)
		return nil, fmt.Errorf("%w: %v", ErrAttestationDenied, err)
	}
	tok, err := c.ts.Token(ctx)
	if err != nil {
		row.Reason = "token_load_failed"
		c.record(row)
		return nil, fmt.Errorf("msteams: token load (%s): %w", ScopeSearch, err)
	}

	// Graph /search/query envelope; entityTypes=chatMessage hits Teams content.
	reqBody := map[string]any{
		"requests": []map[string]any{{
			"entityTypes": []string{"chatMessage"},
			"query":       map[string]string{"queryString": query},
		}},
	}
	var resp struct {
		Value []struct {
			HitsContainers []struct {
				Hits []struct {
					HitID    string `json:"hitId"`
					Summary  string `json:"summary"`
					Resource struct {
						ChannelIdentity struct {
							TeamID    string `json:"teamId"`
							ChannelID string `json:"channelId"`
						} `json:"channelIdentity"`
						WebURL string `json:"webUrl"`
					} `json:"resource"`
				} `json:"hits"`
			} `json:"hitsContainers"`
		} `json:"value"`
	}
	if err := c.post(ctx, tok, graphBase+"/search/query", reqBody, &resp); err != nil {
		row.Reason = errReason(err)
		c.record(row)
		return nil, err
	}

	var out []Result
	for _, v := range resp.Value {
		for _, hc := range v.HitsContainers {
			for _, h := range hc.Hits {
				out = append(out, Result{
					TeamID:    h.Resource.ChannelIdentity.TeamID,
					ChannelID: h.Resource.ChannelIdentity.ChannelID,
					MessageID: h.HitID,
					Snippet:   h.Summary,
					URL:       h.Resource.WebURL,
				})
			}
		}
	}
	row.Success = true
	c.record(row)
	return out, nil
}

// gateAndToken runs the attestation gate first; token load happens only on
// consent so the secret never leaks into a panic trace when denied.
func (c *Client) gateAndToken(ctx context.Context, scope string) (string, error) {
	if err := c.att.Attest(ctx, scope); err != nil {
		return "", fmt.Errorf("%w: %v", ErrAttestationDenied, err)
	}
	tok, err := c.ts.Token(ctx)
	if err != nil {
		return "", fmt.Errorf("msteams: token load (%s): %w", scope, err)
	}
	return tok, nil
}

func (c *Client) get(ctx context.Context, bearer, endpoint string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	return c.do(req, bearer, out)
}

func (c *Client) post(ctx context.Context, bearer, endpoint string, body any, out any) error {
	buf, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, bearer, out)
}

func (c *Client) do(req *http.Request, bearer string, out any) error {
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("msteams: http: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return mapStatus(resp)
	}
	if out == nil {
		// Drain so the connection can be reused without leaking bytes.
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// mapStatus collapses Graph HTTP errors onto the package's sentinel set so
// callers switch on Is(err, Err*) without parsing JSON.
func mapStatus(resp *http.Response) error {
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("%w: status %d", ErrAuthFailed, resp.StatusCode)
	case http.StatusNotFound:
		return fmt.Errorf("%w: status %d", ErrNotFound, resp.StatusCode)
	case http.StatusTooManyRequests:
		return fmt.Errorf("%w: status %d", ErrRateLimited, resp.StatusCode)
	default:
		return fmt.Errorf("msteams: graph status %d", resp.StatusCode)
	}
}

func errReason(err error) string {
	switch {
	case errors.Is(err, ErrAuthFailed):
		return "auth_failed"
	case errors.Is(err, ErrRateLimited):
		return "rate_limited"
	case errors.Is(err, ErrNotFound):
		return "not_found"
	default:
		return "http_error"
	}
}

func (c *Client) record(r AuditRow) {
	if c.audit == nil {
		return
	}
	c.audit.Record(r)
}

func shortHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:8]
}

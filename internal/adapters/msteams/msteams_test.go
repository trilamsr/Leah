package msteams

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/trilam/leah/internal/contracts"
)

type fakeAttestor struct {
	called int
	scopes []string
	err    error
}

func (f *fakeAttestor) Attest(_ context.Context, scope string) error {
	f.called++
	f.scopes = append(f.scopes, scope)
	if !strings.HasPrefix(scope, "msteams:") {
		return errors.New("unexpected scope: " + scope)
	}
	return f.err
}

type fakeTokenSource struct {
	tok    string
	err    error
	called int
}

func (f *fakeTokenSource) Token(_ context.Context) (string, error) {
	f.called++
	return f.tok, f.err
}

// failingTokenSource fails the test if Token() is invoked.
type failingTokenSource struct{ t *testing.T }

func (f *failingTokenSource) Token(_ context.Context) (string, error) {
	f.t.Fatal("TokenSource.Token() must NOT be called when validation/attestation denies")
	return "", nil
}

// fakeHTTP records the last request and replies with canned body/status.
type fakeHTTP struct {
	lastReq    *http.Request
	lastBody   string
	lastBearer string
	status     int
	body       string
	err        error
	calls      int
}

func (f *fakeHTTP) Do(req *http.Request) (*http.Response, error) {
	f.calls++
	f.lastReq = req
	f.lastBearer = req.Header.Get("Authorization")
	if req.Body != nil {
		b, _ := io.ReadAll(req.Body)
		f.lastBody = string(b)
	}
	if f.err != nil {
		return nil, f.err
	}
	status := f.status
	if status == 0 {
		status = 200
	}
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(f.body)),
		Header:     make(http.Header),
	}, nil
}

type failingHTTP struct{ t *testing.T }

func (f *failingHTTP) Do(_ *http.Request) (*http.Response, error) {
	f.t.Fatal("HTTPClient.Do must NOT be called when validation/attestation denies")
	return nil, nil
}

type captureAudit struct {
	rows []AuditRow
}

func (c *captureAudit) Record(r AuditRow) { c.rows = append(c.rows, r) }

func newClient(t *testing.T, att Attestor, ts contracts.TokenSource, h contracts.HTTPClient, audit AuditSink) *Client {
	t.Helper()
	c, err := New(Config{Attestor: att, TokenSource: ts, HTTPClient: h, Audit: audit})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func TestListTeams_HappyPath(t *testing.T) {
	t.Parallel()
	h := &fakeHTTP{body: `{"value":[{"id":"t1","displayName":"Alpha"},{"id":"t2","displayName":"Beta"}]}`}
	att := &fakeAttestor{}
	c := newClient(t, att, &fakeTokenSource{tok: "bearer-xyz"}, h, nil)

	teams, err := c.ListTeams(context.Background())
	if err != nil {
		t.Fatalf("ListTeams: %v", err)
	}
	if len(teams) != 2 || teams[0].ID != "t1" || teams[0].DisplayName != "Alpha" {
		t.Fatalf("teams = %+v", teams)
	}
	if att.called != 1 || att.scopes[0] != ScopeListTeams {
		t.Fatalf("attestor not gated: %+v", att)
	}
	if h.lastBearer != "Bearer bearer-xyz" {
		t.Fatalf("bearer header = %q", h.lastBearer)
	}
}

func TestListTeams_AttestationDenied(t *testing.T) {
	t.Parallel()
	att := &fakeAttestor{err: errors.New("denied")}
	c, err := New(Config{
		Attestor:    att,
		TokenSource: &failingTokenSource{t: t},
		HTTPClient:  &failingHTTP{t: t},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := c.ListTeams(context.Background()); !errors.Is(err, ErrAttestationDenied) {
		t.Fatalf("err = %v, want ErrAttestationDenied", err)
	}
}

func TestPostMessage_AttestationDenied_NoCall(t *testing.T) {
	t.Parallel()
	att := &fakeAttestor{err: errors.New("denied")}
	audit := &captureAudit{}
	c, err := New(Config{
		Attestor:    att,
		TokenSource: &failingTokenSource{t: t},
		HTTPClient:  &failingHTTP{t: t},
		Audit:       audit,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	err = c.PostMessage(context.Background(), "t1", "c1", "hello")
	if !errors.Is(err, ErrAttestationDenied) {
		t.Fatalf("err = %v, want ErrAttestationDenied", err)
	}
	if len(audit.rows) != 1 || audit.rows[0].Reason != "attestation_denied" {
		t.Fatalf("audit rows = %+v", audit.rows)
	}
}

func TestPostMessage_AuditRowHashesChannel(t *testing.T) {
	t.Parallel()
	h := &fakeHTTP{status: 201, body: `{"id":"m99"}`}
	att := &fakeAttestor{}
	audit := &captureAudit{}
	c := newClient(t, att, &fakeTokenSource{tok: "tok"}, h, audit)

	const channelID = "19:abc@thread.tacv2"
	const text = "héllo"
	if err := c.PostMessage(context.Background(), "team-1", channelID, text); err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	if len(audit.rows) != 1 {
		t.Fatalf("audit rows = %d, want 1", len(audit.rows))
	}
	row := audit.rows[0]
	if row.Kind != "msteams_post_message" || !row.Success {
		t.Fatalf("row = %+v", row)
	}
	if row.TeamID != "team-1" {
		t.Fatalf("team_id = %q", row.TeamID)
	}
	if row.ChannelHash == channelID || len(row.ChannelHash) != 8 {
		t.Fatalf("channel_hash = %q (must be 8-char sha256 prefix, not plaintext)", row.ChannelHash)
	}
	if strings.Contains(row.ChannelHash, "@") || strings.Contains(row.ChannelHash, "thread") {
		t.Fatalf("channel_hash leaks plaintext: %q", row.ChannelHash)
	}
	if row.TextLen != 5 { // 5 runes (é counts once)
		t.Fatalf("text_len = %d, want 5", row.TextLen)
	}
	// Body bytes MUST not appear in audit row.
	if strings.Contains(row.Reason, text) {
		t.Fatalf("reason leaks text: %q", row.Reason)
	}
}

func TestSearch_HappyPath(t *testing.T) {
	t.Parallel()
	// Graph /search/query response shape: {"value":[{"hitsContainers":[{"hits":[{"hitId":"m1","resource":{"channelIdentity":{"teamId":"t1","channelId":"c1"},"webUrl":"https://teams/x"},"summary":"hello world"}]}]}]}
	h := &fakeHTTP{body: `{"value":[{"hitsContainers":[{"hits":[{"hitId":"m1","summary":"hello world","resource":{"channelIdentity":{"teamId":"t1","channelId":"c1"},"webUrl":"https://teams.microsoft.com/x"}}]}]}]}`}
	att := &fakeAttestor{}
	audit := &captureAudit{}
	c := newClient(t, att, &fakeTokenSource{tok: "tok"}, h, audit)

	results, err := c.Search(context.Background(), "incident")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}
	r := results[0]
	if r.MessageID != "m1" || r.TeamID != "t1" || r.ChannelID != "c1" || r.Snippet != "hello world" {
		t.Fatalf("result = %+v", r)
	}
	// Audit row MUST omit plaintext query / snippet.
	if len(audit.rows) != 1 {
		t.Fatalf("audit rows = %d", len(audit.rows))
	}
	row := audit.rows[0]
	if row.Kind != "msteams_search" || !row.Success {
		t.Fatalf("row = %+v", row)
	}
	if row.QueryHash == "incident" || len(row.QueryHash) != 8 {
		t.Fatalf("query_hash leaks plaintext or wrong shape: %q", row.QueryHash)
	}
	// Spec §7: audit row records query_hash, NEVER query or snippet.
	for _, leak := range []string{"incident", "hello world"} {
		if strings.Contains(row.Reason, leak) {
			t.Fatalf("reason leaks %q: %q", leak, row.Reason)
		}
	}
}

func TestListChannels_TeamIDEmpty_NoAttest(t *testing.T) {
	t.Parallel()
	att := &fakeAttestor{}
	c := newClient(t, att, &fakeTokenSource{tok: "tok"}, &failingHTTP{t: t}, nil)
	if _, err := c.ListChannels(context.Background(), ""); !errors.Is(err, ErrInvalidChannel) {
		t.Fatalf("err = %v, want ErrInvalidChannel", err)
	}
	if att.called != 0 {
		t.Fatalf("attestor invoked on empty teamID (%d calls)", att.called)
	}
}

func TestPostMessage_EmptyArgs_NoAttest(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, team, ch, text string
	}{
		{"empty team", "", "c", "t"},
		{"empty channel", "t", "", "t"},
		{"empty text", "t", "c", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			att := &fakeAttestor{}
			c := newClient(t, att, &fakeTokenSource{tok: "tok"}, &failingHTTP{t: t}, nil)
			if err := c.PostMessage(context.Background(), tc.team, tc.ch, tc.text); !errors.Is(err, ErrInvalidChannel) {
				t.Fatalf("err = %v, want ErrInvalidChannel", err)
			}
			if att.called != 0 {
				t.Fatalf("attestor invoked despite invalid args")
			}
		})
	}
}

func TestSearch_QueryEmpty_NoAttest(t *testing.T) {
	t.Parallel()
	att := &fakeAttestor{}
	c := newClient(t, att, &fakeTokenSource{tok: "tok"}, &failingHTTP{t: t}, nil)
	if _, err := c.Search(context.Background(), ""); !errors.Is(err, ErrInvalidChannel) {
		t.Fatalf("err = %v, want ErrInvalidChannel", err)
	}
	if att.called != 0 {
		t.Fatalf("attestor invoked on empty query")
	}
}

func TestNewRejectsMissingDeps(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		cfg  Config
	}{
		{"no attestor", Config{TokenSource: &fakeTokenSource{}, HTTPClient: &fakeHTTP{}}},
		{"no token source", Config{Attestor: &fakeAttestor{}, HTTPClient: &fakeHTTP{}}},
		{"no http client", Config{Attestor: &fakeAttestor{}, TokenSource: &fakeTokenSource{}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := New(tc.cfg); err == nil {
				t.Fatal("New: expected error for missing dep")
			}
		})
	}
}

func TestPostMessage_AuthFailure_MapsErrAuthFailed(t *testing.T) {
	t.Parallel()
	h := &fakeHTTP{status: 401, body: `{"error":{"code":"InvalidAuthenticationToken"}}`}
	c := newClient(t, &fakeAttestor{}, &fakeTokenSource{tok: "tok"}, h, nil)
	err := c.PostMessage(context.Background(), "t", "c", "x")
	if !errors.Is(err, ErrAuthFailed) {
		t.Fatalf("err = %v, want ErrAuthFailed", err)
	}
}

func TestListTeams_RateLimited_MapsErrRateLimited(t *testing.T) {
	t.Parallel()
	h := &fakeHTTP{status: 429, body: `{"error":{"code":"TooManyRequests"}}`}
	c := newClient(t, &fakeAttestor{}, &fakeTokenSource{tok: "tok"}, h, nil)
	_, err := c.ListTeams(context.Background())
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("err = %v, want ErrRateLimited", err)
	}
}

package linear

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fakeAttestor struct {
	called int
	err    error
}

func (f *fakeAttestor) Attest(_ context.Context, scope string) error {
	f.called++
	if !strings.HasPrefix(scope, "linear:") {
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

type fakeTransport struct {
	listOut    []Issue
	listErr    error
	getOut     Issue
	getErr     error
	createOut  Issue
	createErr  error
	commentErr error

	lastBearer    string
	lastGetID     string
	lastCreateReq IssueReq
	lastCommentID string
	lastBody      string
}

func (f *fakeTransport) ListMyIssues(_ context.Context, bearer string) ([]Issue, error) {
	f.lastBearer = bearer
	return f.listOut, f.listErr
}

func (f *fakeTransport) GetIssue(_ context.Context, bearer, id string) (Issue, error) {
	f.lastBearer = bearer
	f.lastGetID = id
	return f.getOut, f.getErr
}

func (f *fakeTransport) CreateIssue(_ context.Context, bearer string, req IssueReq) (Issue, error) {
	f.lastBearer = bearer
	f.lastCreateReq = req
	return f.createOut, f.createErr
}

func (f *fakeTransport) Comment(_ context.Context, bearer, id, body string) error {
	f.lastBearer = bearer
	f.lastCommentID = id
	f.lastBody = body
	return f.commentErr
}

type failingTokenSource struct{ t *testing.T }

func (f *failingTokenSource) Token(_ context.Context) (string, error) {
	f.t.Fatal("TokenSource.Token() must NOT be called before attestation passes")
	return "", nil
}

func newTestClient(t *testing.T, att Attestor, ts TokenSource, tr Transport) *Client {
	t.Helper()
	c, err := New(Config{Attestor: att, TokenSource: ts, Transport: tr})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func TestListMyIssues_HappyPath(t *testing.T) {
	t.Parallel()
	tr := &fakeTransport{listOut: []Issue{
		{ID: "i1", Identifier: "ENG-1", Title: "first"},
		{ID: "i2", Identifier: "ENG-2", Title: "second"},
	}}
	c := newTestClient(t, &fakeAttestor{}, &fakeTokenSource{tok: "key"}, tr)
	got, err := c.ListMyIssues(context.Background())
	if err != nil {
		t.Fatalf("ListMyIssues: %v", err)
	}
	if len(got) != 2 || got[0].Identifier != "ENG-1" || got[1].Title != "second" {
		t.Fatalf("unexpected issues: %+v", got)
	}
	if tr.lastBearer != "key" {
		t.Fatalf("bearer = %q, want key", tr.lastBearer)
	}
}

func TestListMyIssues_AttestationDenied_NoTokenLoad(t *testing.T) {
	t.Parallel()
	ts := &failingTokenSource{t: t}
	c, err := New(Config{
		Attestor:    &fakeAttestor{err: errors.New("denied")},
		TokenSource: ts,
		Transport:   &fakeTransport{},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := c.ListMyIssues(context.Background()); !errors.Is(err, ErrAttestationDenied) {
		t.Fatalf("err = %v, want ErrAttestationDenied", err)
	}
}

func TestGetIssue_IDEmpty_NoAttest(t *testing.T) {
	t.Parallel()
	att := &fakeAttestor{}
	c := newTestClient(t, att, &fakeTokenSource{tok: "k"}, &fakeTransport{})
	if _, err := c.GetIssue(context.Background(), ""); !errors.Is(err, ErrInvalidIssue) {
		t.Fatalf("err = %v, want ErrInvalidIssue", err)
	}
	if att.called != 0 {
		t.Fatalf("attestor called %d times, want 0", att.called)
	}
}

func TestGetIssue_NotFound(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, &fakeAttestor{}, &fakeTokenSource{tok: "k"}, &fakeTransport{getErr: ErrNotFound})
	if _, err := c.GetIssue(context.Background(), "id"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestCreateIssue_TeamIDEmpty_NoAttest(t *testing.T) {
	t.Parallel()
	att := &fakeAttestor{}
	c := newTestClient(t, att, &fakeTokenSource{tok: "k"}, &fakeTransport{})
	_, err := c.CreateIssue(context.Background(), IssueReq{Title: "x"})
	if !errors.Is(err, ErrInvalidIssue) {
		t.Fatalf("err = %v, want ErrInvalidIssue", err)
	}
	if att.called != 0 {
		t.Fatalf("attestor called %d times, want 0", att.called)
	}
}

func TestCreateIssue_TitleEmpty_NoAttest(t *testing.T) {
	t.Parallel()
	att := &fakeAttestor{}
	c := newTestClient(t, att, &fakeTokenSource{tok: "k"}, &fakeTransport{})
	_, err := c.CreateIssue(context.Background(), IssueReq{TeamID: "t"})
	if !errors.Is(err, ErrInvalidIssue) {
		t.Fatalf("err = %v, want ErrInvalidIssue", err)
	}
	if att.called != 0 {
		t.Fatalf("attestor called %d times, want 0", att.called)
	}
}

func TestComment_BodyEmpty_NoAttest(t *testing.T) {
	t.Parallel()
	att := &fakeAttestor{}
	c := newTestClient(t, att, &fakeTokenSource{tok: "k"}, &fakeTransport{})
	if err := c.Comment(context.Background(), "id", ""); !errors.Is(err, ErrInvalidIssue) {
		t.Fatalf("err = %v, want ErrInvalidIssue", err)
	}
	if att.called != 0 {
		t.Fatalf("attestor called %d times, want 0", att.called)
	}
}

func TestComment_IDEmpty_NoAttest(t *testing.T) {
	t.Parallel()
	att := &fakeAttestor{}
	c := newTestClient(t, att, &fakeTokenSource{tok: "k"}, &fakeTransport{})
	if err := c.Comment(context.Background(), "", "body"); !errors.Is(err, ErrInvalidIssue) {
		t.Fatalf("err = %v, want ErrInvalidIssue", err)
	}
}

func TestRateLimit_RejectsBurst(t *testing.T) {
	t.Parallel()
	now := time.Unix(1_700_000_000, 0)
	c, err := New(Config{
		Attestor:    &fakeAttestor{},
		TokenSource: &fakeTokenSource{tok: "k"},
		Transport:   &fakeTransport{createOut: Issue{ID: "i", Identifier: "ENG-1"}},
		Now:         func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	for i := 0; i < 10; i++ {
		if _, err := c.CreateIssue(context.Background(), IssueReq{TeamID: "t", Title: "x"}); err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
	}
	if _, err := c.CreateIssue(context.Background(), IssueReq{TeamID: "t", Title: "x"}); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("11th err = %v, want ErrRateLimited", err)
	}
	// Comments share the budget.
	if err := c.Comment(context.Background(), "id", "b"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("comment after burst err = %v, want ErrRateLimited", err)
	}
	// Advance past 60s — budget resets.
	now = now.Add(61 * time.Second)
	if _, err := c.CreateIssue(context.Background(), IssueReq{TeamID: "t", Title: "x"}); err != nil {
		t.Fatalf("post-window create: %v", err)
	}
}

func TestRateLimit_DoesNotCountReads(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, &fakeAttestor{}, &fakeTokenSource{tok: "k"},
		&fakeTransport{listOut: []Issue{}, getOut: Issue{ID: "x"}})
	for i := 0; i < 50; i++ {
		if _, err := c.ListMyIssues(context.Background()); err != nil {
			t.Fatalf("list %d: %v", i, err)
		}
		if _, err := c.GetIssue(context.Background(), "id"); err != nil {
			t.Fatalf("get %d: %v", i, err)
		}
	}
}

func TestRateLimit_BurstRejectsBeforeAttest(t *testing.T) {
	t.Parallel()
	att := &fakeAttestor{}
	now := time.Unix(1_700_000_000, 0)
	c, err := New(Config{
		Attestor:    att,
		TokenSource: &fakeTokenSource{tok: "k"},
		Transport:   &fakeTransport{createOut: Issue{Identifier: "X"}},
		Now:         func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	for i := 0; i < 10; i++ {
		if _, err := c.CreateIssue(context.Background(), IssueReq{TeamID: "t", Title: "x"}); err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
	}
	prior := att.called
	if _, err := c.CreateIssue(context.Background(), IssueReq{TeamID: "t", Title: "x"}); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("want ErrRateLimited, got %v", err)
	}
	if att.called != prior {
		t.Fatal("attestor invoked on rate-limited call — burst MUST reject before consent prompt")
	}
}

func TestNewRejectsMissingDeps(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		cfg  Config
	}{
		{"no attestor", Config{TokenSource: &fakeTokenSource{}, Transport: &fakeTransport{}}},
		{"no token source", Config{Attestor: &fakeAttestor{}, Transport: &fakeTransport{}}},
		{"no transport", Config{Attestor: &fakeAttestor{}, TokenSource: &fakeTokenSource{}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := New(tc.cfg); err == nil {
				t.Fatal("New: expected error for missing dep")
			}
		})
	}
}

func TestBearer_NoPrefix_InHeader(t *testing.T) {
	t.Parallel()
	const key = "lin_api_secret"
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = io.WriteString(w, `{"data":{"viewer":{"assignedIssues":{"nodes":[]}}}}`)
	}))
	defer srv.Close()

	tr := NewHTTPTransport(srv.Client(), srv.URL)
	if _, err := tr.ListMyIssues(context.Background(), key); err != nil {
		t.Fatalf("ListMyIssues: %v", err)
	}
	if gotAuth != key {
		t.Fatalf("Authorization = %q, want %q (no Bearer prefix)", gotAuth, key)
	}
}

func TestHTTPTransport_ListMyIssues_ParsesNodes(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		_, _ = io.WriteString(w, `{"data":{"viewer":{"assignedIssues":{"nodes":[
			{"id":"i1","identifier":"ENG-1","title":"a","state":{"name":"Todo"},"team":{"key":"ENG"},"url":"https://linear.app/x/issue/ENG-1","updatedAt":"2026-06-10T00:00:00Z"},
			{"id":"i2","identifier":"ENG-2","title":"b","state":{"name":"In Progress"},"team":{"key":"ENG"},"url":"https://linear.app/x/issue/ENG-2","updatedAt":"2026-06-10T00:00:00Z"}
		]}}}}`)
	}))
	defer srv.Close()

	tr := NewHTTPTransport(srv.Client(), srv.URL)
	got, err := tr.ListMyIssues(context.Background(), "key")
	if err != nil {
		t.Fatalf("ListMyIssues: %v", err)
	}
	if len(got) != 2 || got[0].Identifier != "ENG-1" || got[1].TeamKey != "ENG" {
		t.Fatalf("unexpected: %+v", got)
	}
	if got[0].State != "Todo" {
		t.Fatalf("State = %q, want Todo", got[0].State)
	}
}

func TestHTTPTransport_GetIssue_NotFound(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"data":{"issue":null},"errors":[{"message":"Entity not found","extensions":{"type":"not_found"}}]}`)
	}))
	defer srv.Close()

	tr := NewHTTPTransport(srv.Client(), srv.URL)
	if _, err := tr.GetIssue(context.Background(), "k", "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestHTTPTransport_GetIssue_AuthFailed(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"errors":[{"message":"bad token"}]}`)
	}))
	defer srv.Close()
	tr := NewHTTPTransport(srv.Client(), srv.URL)
	if _, err := tr.GetIssue(context.Background(), "k", "id"); !errors.Is(err, ErrAuthFailed) {
		t.Fatalf("err = %v, want ErrAuthFailed", err)
	}
}

func TestHTTPTransport_RateLimited(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"errors":[{"message":"slow down"}]}`)
	}))
	defer srv.Close()
	tr := NewHTTPTransport(srv.Client(), srv.URL)
	if _, err := tr.ListMyIssues(context.Background(), "k"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("err = %v, want ErrRateLimited", err)
	}
}

func TestHTTPTransport_CreateIssue_SendsVariables(t *testing.T) {
	t.Parallel()
	var gotBody struct {
		Query     string                 `json:"query"`
		Variables map[string]interface{} `json:"variables"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode: %v", err)
		}
		_, _ = io.WriteString(w, `{"data":{"issueCreate":{"success":true,"issue":{"id":"new","identifier":"ENG-99","title":"hi","state":{"name":"Triage"},"team":{"key":"ENG"},"url":"u","updatedAt":"2026-06-10T00:00:00Z"}}}}`)
	}))
	defer srv.Close()
	tr := NewHTTPTransport(srv.Client(), srv.URL)
	got, err := tr.CreateIssue(context.Background(), "k", IssueReq{TeamID: "team-uuid", Title: "hi", Description: "d", Priority: 2})
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	if got.Identifier != "ENG-99" {
		t.Fatalf("identifier = %q, want ENG-99", got.Identifier)
	}
	if gotBody.Variables["teamId"] != "team-uuid" || gotBody.Variables["title"] != "hi" {
		t.Fatalf("variables = %+v", gotBody.Variables)
	}
	// Priority is float in JSON-decoded interface{}.
	if p, ok := gotBody.Variables["priority"].(float64); !ok || p != 2 {
		t.Fatalf("priority = %v, want 2", gotBody.Variables["priority"])
	}
}

func TestHTTPTransport_Comment_SendsBody(t *testing.T) {
	t.Parallel()
	var gotBody struct {
		Variables map[string]interface{} `json:"variables"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = io.WriteString(w, `{"data":{"commentCreate":{"success":true}}}`)
	}))
	defer srv.Close()
	tr := NewHTTPTransport(srv.Client(), srv.URL)
	if err := tr.Comment(context.Background(), "k", "issue-uuid", "hello"); err != nil {
		t.Fatalf("Comment: %v", err)
	}
	if gotBody.Variables["issueId"] != "issue-uuid" || gotBody.Variables["body"] != "hello" {
		t.Fatalf("variables = %+v", gotBody.Variables)
	}
}

func TestHTTPTransport_GraphQLError_NonNotFound(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"errors":[{"message":"validation failed"}]}`)
	}))
	defer srv.Close()
	tr := NewHTTPTransport(srv.Client(), srv.URL)
	_, err := tr.GetIssue(context.Background(), "k", "id")
	if err == nil {
		t.Fatal("want error")
	}
	if errors.Is(err, ErrNotFound) || errors.Is(err, ErrAuthFailed) {
		t.Fatalf("unexpected sentinel: %v", err)
	}
}

package jira

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/trilam/leah/internal/adapters/atlassian"
)

// fakeAttestor records whether the attestation gate was invoked. The Jira
// adapter MUST route every token load through Attest() so the operator-
// attestation flow gates secret access at the per-RPC scope grain.
type fakeAttestor struct {
	called int
	err    error
}

func (f *fakeAttestor) Attest(_ context.Context, scope string) error {
	f.called++
	if !strings.HasPrefix(scope, "jira:") {
		return errors.New("unexpected scope: " + scope)
	}
	return f.err
}

// fakeTokenSource returns a canned token without touching disk; lets tests run
// hermetically without $HOME/.leah-state/secrets/jira-token.json.
type fakeTokenSource struct {
	tok string
	err error
}

func (f *fakeTokenSource) Token(_ context.Context) (string, error) {
	return f.tok, f.err
}

// fakeTransport stands in for the Atlassian REST layer; the adapter MVP routes
// RPCs through a swappable Transport so tests don't hit the real API.
type fakeTransport struct {
	listIssues []Issue
	listErr    error

	getIssue Issue
	getErr   error

	createIssue Issue
	createReq   IssueReq
	createErr   error

	commentKey  string
	commentBody string
	commentErr  error
}

func (f *fakeTransport) ListMyIssues(_ context.Context, _ string) ([]Issue, error) {
	return f.listIssues, f.listErr
}

func (f *fakeTransport) GetIssue(_ context.Context, _, _ string) (Issue, error) {
	return f.getIssue, f.getErr
}

func (f *fakeTransport) CreateIssue(_ context.Context, _ string, req IssueReq) (Issue, error) {
	f.createReq = req
	return f.createIssue, f.createErr
}

func (f *fakeTransport) Comment(_ context.Context, _, key, body string) error {
	f.commentKey, f.commentBody = key, body
	return f.commentErr
}

func newTestClient(t *testing.T, att *fakeAttestor, ts *fakeTokenSource, tr *fakeTransport) *Client {
	t.Helper()
	c, err := New(Config{
		Attestor:    att,
		TokenSource: ts,
		Transport:   tr,
		BaseURL:     "https://acme.atlassian.net",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

// failingTokenSource fails the test if Token() is invoked; used to prove the
// attestation gate runs BEFORE the token materializes (gate-ordering invariant).
type failingTokenSource struct{ t *testing.T }

func (f *failingTokenSource) Token(_ context.Context) (string, error) {
	f.t.Fatal("TokenSource.Token() must NOT be called when attestation denies")
	return "", nil
}

func TestListMyIssues_HappyPath(t *testing.T) {
	t.Parallel()
	att := &fakeAttestor{}
	want := []Issue{
		{Key: "ENG-1", Summary: "fix", Status: "Open", Assignee: "me", ProjectKey: "ENG", Updated: time.Unix(1, 0)},
		{Key: "ENG-2", Summary: "ship", Status: "In Progress", Assignee: "me", ProjectKey: "ENG"},
	}
	c := newTestClient(t, att, &fakeTokenSource{tok: "t"}, &fakeTransport{listIssues: want})

	got, err := c.ListMyIssues(context.Background())
	if err != nil {
		t.Fatalf("ListMyIssues: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].Key != want[i].Key || got[i].Status != want[i].Status {
			t.Fatalf("issue[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
	if att.called == 0 {
		t.Fatal("attestor never invoked — token load bypassed the gate")
	}
}

func TestListMyIssues_AttestationDenied(t *testing.T) {
	t.Parallel()
	c, err := New(Config{
		Attestor:    &fakeAttestor{err: errors.New("denied")},
		TokenSource: &failingTokenSource{t: t},
		Transport:   &fakeTransport{},
		BaseURL:     "https://acme.atlassian.net",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := c.ListMyIssues(context.Background()); !errors.Is(err, ErrAttestationDenied) {
		t.Fatalf("err = %v, want ErrAttestationDenied", err)
	}
}

func TestGetIssue_NotFound(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, &fakeAttestor{}, &fakeTokenSource{tok: "t"}, &fakeTransport{getErr: ErrNotFound})
	_, err := c.GetIssue(context.Background(), "ENG-404")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestGetIssue_EmptyKey(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, &fakeAttestor{}, &fakeTokenSource{tok: "t"}, &fakeTransport{})
	if _, err := c.GetIssue(context.Background(), ""); !errors.Is(err, ErrInvalidIssue) {
		t.Fatalf("err = %v, want ErrInvalidIssue", err)
	}
}

func TestCreateIssue_HappyPath(t *testing.T) {
	t.Parallel()
	att := &fakeAttestor{}
	tr := &fakeTransport{createIssue: Issue{Key: "ENG-42", Summary: "new", ProjectKey: "ENG"}}
	c := newTestClient(t, att, &fakeTokenSource{tok: "t"}, tr)

	req := IssueReq{ProjectKey: "ENG", Summary: "new", Description: "details", IssueType: "Task"}
	got, err := c.CreateIssue(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	if got.Key != "ENG-42" {
		t.Fatalf("key = %q, want ENG-42", got.Key)
	}
	if tr.createReq != req {
		t.Fatalf("transport saw req = %+v, want %+v", tr.createReq, req)
	}
	if att.called == 0 {
		t.Fatal("attestor never invoked")
	}
}

func TestCreateIssue_AttestationDenied_NoCall(t *testing.T) {
	t.Parallel()
	tr := &fakeTransport{createIssue: Issue{Key: "X"}}
	c, err := New(Config{
		Attestor:    &fakeAttestor{err: errors.New("denied")},
		TokenSource: &failingTokenSource{t: t},
		Transport:   tr,
		BaseURL:     "https://acme.atlassian.net",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	req := IssueReq{ProjectKey: "ENG", Summary: "new", IssueType: "Task"}
	if _, err := c.CreateIssue(context.Background(), req); !errors.Is(err, ErrAttestationDenied) {
		t.Fatalf("err = %v, want ErrAttestationDenied", err)
	}
	// Transport MUST NOT have been called when attestation denies.
	if (tr.createReq != IssueReq{}) {
		t.Fatalf("transport saw req %+v; expected no call", tr.createReq)
	}
}

func TestCreateIssue_InvalidReq_NoAttest(t *testing.T) {
	t.Parallel()
	// A typo MUST NOT consume an attestation prompt — validation runs BEFORE
	// the gate. This is the inverse of the gate-ordering invariant.
	cases := []struct {
		name string
		req  IssueReq
	}{
		{name: "missing project", req: IssueReq{Summary: "s", IssueType: "Task"}},
		{name: "missing summary", req: IssueReq{ProjectKey: "ENG", IssueType: "Task"}},
		{name: "missing type", req: IssueReq{ProjectKey: "ENG", Summary: "s"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			att := &fakeAttestor{}
			c := newTestClient(t, att, &fakeTokenSource{tok: "t"}, &fakeTransport{})
			if _, err := c.CreateIssue(context.Background(), tc.req); !errors.Is(err, ErrInvalidIssue) {
				t.Fatalf("err = %v, want ErrInvalidIssue", err)
			}
			if att.called != 0 {
				t.Fatalf("attestor called %d times on invalid req; want 0", att.called)
			}
		})
	}
}

func TestComment_AttestationDenied_NoCall(t *testing.T) {
	t.Parallel()
	tr := &fakeTransport{}
	c, err := New(Config{
		Attestor:    &fakeAttestor{err: errors.New("denied")},
		TokenSource: &failingTokenSource{t: t},
		Transport:   tr,
		BaseURL:     "https://acme.atlassian.net",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := c.Comment(context.Background(), "ENG-1", "lgtm"); !errors.Is(err, ErrAttestationDenied) {
		t.Fatalf("err = %v, want ErrAttestationDenied", err)
	}
	if tr.commentKey != "" || tr.commentBody != "" {
		t.Fatalf("transport saw key=%q body=%q; expected no call", tr.commentKey, tr.commentBody)
	}
}

func TestComment_InvalidReq_NoAttest(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		key, body string
	}{
		{name: "empty key", key: "", body: "lgtm"},
		{name: "empty body", key: "ENG-1", body: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			att := &fakeAttestor{}
			c := newTestClient(t, att, &fakeTokenSource{tok: "t"}, &fakeTransport{})
			if err := c.Comment(context.Background(), tc.key, tc.body); !errors.Is(err, ErrInvalidIssue) {
				t.Fatalf("err = %v, want ErrInvalidIssue", err)
			}
			if att.called != 0 {
				t.Fatalf("attestor called %d times on invalid req; want 0", att.called)
			}
		})
	}
}

func TestComment_HappyPath(t *testing.T) {
	t.Parallel()
	tr := &fakeTransport{}
	c := newTestClient(t, &fakeAttestor{}, &fakeTokenSource{tok: "t"}, tr)
	if err := c.Comment(context.Background(), "ENG-1", "lgtm"); err != nil {
		t.Fatalf("Comment: %v", err)
	}
	if tr.commentKey != "ENG-1" || tr.commentBody != "lgtm" {
		t.Fatalf("transport saw key=%q body=%q, want ENG-1 / lgtm", tr.commentKey, tr.commentBody)
	}
}

func TestNewRejectsMissingDeps(t *testing.T) {
	t.Parallel()
	good := Config{
		Attestor:    &fakeAttestor{},
		TokenSource: &fakeTokenSource{},
		Transport:   &fakeTransport{},
		BaseURL:     "https://acme.atlassian.net",
	}
	cases := []struct {
		name string
		mut  func(*Config)
	}{
		{name: "no attestor", mut: func(c *Config) { c.Attestor = nil }},
		{name: "no token source", mut: func(c *Config) { c.TokenSource = nil }},
		{name: "no transport", mut: func(c *Config) { c.Transport = nil }},
		{name: "empty base url", mut: func(c *Config) { c.BaseURL = "" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := good
			tc.mut(&cfg)
			if _, err := New(cfg); err == nil {
				t.Fatal("New: expected error for missing dep")
			}
		})
	}
}

func TestHTTPTransport_ListMyIssues_ParsesAndBasicAuth(t *testing.T) {
	t.Parallel()
	var gotAuth, gotPath, gotJQL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotPath, gotJQL = r.Header.Get("Authorization"), r.URL.Path, r.URL.Query().Get("jql")
		_, _ = w.Write([]byte(`{"issues":[{"key":"ENG-1","self":"https://acme.atlassian.net/rest/api/3/issue/10001","fields":{"summary":"fix","status":{"name":"Open"},"assignee":{"displayName":"Me"},"project":{"key":"ENG"},"updated":"2026-06-19T08:00:00.000+0000"}}]}`))
	}))
	defer srv.Close()
	tr := NewHTTPTransport(srv.Client(), srv.URL)
	bearer := atlassian.BasicAuthHeader("ops@acme.com", "tok")
	got, err := tr.ListMyIssues(context.Background(), bearer)
	if err != nil {
		t.Fatalf("ListMyIssues: %v", err)
	}
	if gotAuth != bearer {
		t.Fatalf("Authorization = %q, want Basic header %q", gotAuth, bearer)
	}
	if !strings.HasSuffix(gotPath, "/rest/api/3/search/jql") {
		t.Fatalf("path = %q, want .../rest/api/3/search/jql", gotPath)
	}
	if !strings.Contains(gotJQL, "currentUser()") {
		t.Fatalf("jql = %q, want assignee=currentUser()", gotJQL)
	}
	if len(got) != 1 || got[0].Key != "ENG-1" || got[0].Status != "Open" || got[0].Assignee != "Me" || got[0].ProjectKey != "ENG" {
		t.Fatalf("parse mismatch: %+v", got)
	}
	if got[0].URL != srv.URL+"/browse/ENG-1" {
		t.Fatalf("URL = %q, want %s/browse/ENG-1", got[0].URL, srv.URL)
	}
}

func TestHTTPTransport_GetIssue_ParsesAndPath(t *testing.T) {
	t.Parallel()
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"key":"ENG-42","fields":{"summary":"S","status":{"name":"Done"},"project":{"key":"ENG"},"updated":"2026-06-19T08:00:00.000+0000"}}`))
	}))
	defer srv.Close()
	tr := NewHTTPTransport(srv.Client(), srv.URL)
	got, err := tr.GetIssue(context.Background(), "auth", "ENG-42")
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}
	if !strings.HasSuffix(gotPath, "/rest/api/3/issue/ENG-42") {
		t.Fatalf("path = %q", gotPath)
	}
	if got.Key != "ENG-42" || got.Status != "Done" {
		t.Fatalf("parse mismatch: %+v", got)
	}
}

func TestHTTPTransport_CreateIssue_PostsADFAndParsesKey(t *testing.T) {
	t.Parallel()
	var gotMethod, gotCT, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotCT = r.Method, r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"key":"ENG-99"}`))
	}))
	defer srv.Close()
	tr := NewHTTPTransport(srv.Client(), srv.URL)
	got, err := tr.CreateIssue(context.Background(), "auth", IssueReq{ProjectKey: "ENG", Summary: "boom", Description: "the body", IssueType: "Bug"})
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	if got.Key != "ENG-99" {
		t.Fatalf("key = %q, want ENG-99", got.Key)
	}
	if gotMethod != http.MethodPost || gotCT != "application/json" {
		t.Fatalf("method=%q ct=%q", gotMethod, gotCT)
	}
	if !strings.Contains(gotBody, `"type":"doc"`) || !strings.Contains(gotBody, "the body") {
		t.Fatalf("body not ADF-wrapped: %q", gotBody)
	}
	if !strings.Contains(gotBody, `"issuetype"`) || !strings.Contains(gotBody, `"Bug"`) {
		t.Fatalf("body missing issuetype: %q", gotBody)
	}
}

func TestHTTPTransport_Comment_PostsADFToCommentPath(t *testing.T) {
	t.Parallel()
	var gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()
	tr := NewHTTPTransport(srv.Client(), srv.URL)
	if err := tr.Comment(context.Background(), "auth", "ENG-1", "lgtm"); err != nil {
		t.Fatalf("Comment: %v", err)
	}
	if !strings.HasSuffix(gotPath, "/rest/api/3/issue/ENG-1/comment") {
		t.Fatalf("path = %q", gotPath)
	}
	if !strings.Contains(gotBody, `"type":"doc"`) || !strings.Contains(gotBody, "lgtm") {
		t.Fatalf("comment body not ADF-wrapped: %q", gotBody)
	}
}

func TestHTTPTransport_MapsStatusToSentinels(t *testing.T) {
	t.Parallel()
	cases := []struct {
		code int
		want error
	}{
		{http.StatusUnauthorized, ErrAuthFailed},
		{http.StatusForbidden, ErrAuthFailed},
		{http.StatusNotFound, ErrNotFound},
		{http.StatusTooManyRequests, ErrRateLimited},
	}
	for _, tc := range cases {
		t.Run(http.StatusText(tc.code), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.code)
			}))
			defer srv.Close()
			tr := NewHTTPTransport(srv.Client(), srv.URL)
			if _, err := tr.GetIssue(context.Background(), "auth", "ENG-1"); !errors.Is(err, tc.want) {
				t.Fatalf("GetIssue(%d) err = %v, want %v", tc.code, err, tc.want)
			}
		})
	}
}

func TestAuditDetail_ExcludesBody(t *testing.T) {
	t.Parallel()
	d := AuditDetail(Issue{Key: "ENG-42", ProjectKey: "ENG", Summary: "SECRET SUMMARY"})
	if !strings.Contains(d, "ENG-42") || !strings.Contains(d, "ENG") {
		t.Fatalf("audit detail missing issue_key/project_key: %q", d)
	}
	if strings.Contains(d, "SECRET SUMMARY") {
		t.Fatalf("audit detail leaked summary: %q", d)
	}
}

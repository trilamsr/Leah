package jira

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
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

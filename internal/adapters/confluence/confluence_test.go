package confluence

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// fakeAttestor records gate invocations; the adapter MUST route every token
// load through Attest() so operator-attestation gates secret access.
type fakeAttestor struct {
	called int
	err    error
}

func (f *fakeAttestor) Attest(_ context.Context, scope string) error {
	f.called++
	if !strings.HasPrefix(scope, "confluence:") {
		return errors.New("unexpected scope: " + scope)
	}
	return f.err
}

// fakeTokenSource returns a canned bearer; lets tests run hermetically.
type fakeTokenSource struct {
	tok string
	err error
}

func (f *fakeTokenSource) Token(_ context.Context) (string, error) {
	return f.tok, f.err
}

// fakeTransport stands in for the Atlassian REST seam.
type fakeTransport struct {
	listPages []Page
	listErr   error
	getPage   Page
	getErr    error
	searchOut []Page
	searchErr error
	lastQuery string
}

func (f *fakeTransport) ListRecent(_ context.Context, _, _ string) ([]Page, error) {
	return f.listPages, f.listErr
}

func (f *fakeTransport) GetPage(_ context.Context, _, _ string) (Page, error) {
	return f.getPage, f.getErr
}

func (f *fakeTransport) SearchCQL(_ context.Context, _, q string) ([]Page, error) {
	f.lastQuery = q
	return f.searchOut, f.searchErr
}

func newTestClient(t *testing.T, att *fakeAttestor, ts *fakeTokenSource, tr *fakeTransport) *Client {
	t.Helper()
	c, err := New(Config{
		Attestor:    att,
		TokenSource: ts,
		Transport:   tr,
		BaseURL:     "https://acme.atlassian.net/wiki",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func TestListRecentPages_HappyPath(t *testing.T) {
	t.Parallel()
	want := []Page{
		{ID: "1", Title: "Overnight notes", SpaceKey: "ENG", URL: "https://acme.atlassian.net/wiki/x/1", Updated: time.Unix(1717891200, 0)},
		{ID: "2", Title: "Roadmap", SpaceKey: "ENG", URL: "https://acme.atlassian.net/wiki/x/2", Updated: time.Unix(1717977600, 0)},
	}
	att := &fakeAttestor{}
	c := newTestClient(t, att, &fakeTokenSource{tok: "tok"}, &fakeTransport{listPages: want})
	got, err := c.ListRecentPages(context.Background(), "ENG")
	if err != nil {
		t.Fatalf("ListRecentPages: %v", err)
	}
	if len(got) != len(want) || got[0].Title != "Overnight notes" || got[1].SpaceKey != "ENG" {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	if att.called != 1 {
		t.Fatalf("attestor called %d times, want 1", att.called)
	}
}

// failingTokenSource fails the test if Token() runs; proves gate ordering.
type failingTokenSource struct{ t *testing.T }

func (f *failingTokenSource) Token(_ context.Context) (string, error) {
	f.t.Fatal("TokenSource.Token() must NOT be called when attestation denies")
	return "", nil
}

func TestListRecentPages_AttestationDenied_NoTokenLoad(t *testing.T) {
	t.Parallel()
	c, err := New(Config{
		Attestor:    &fakeAttestor{err: errors.New("denied")},
		TokenSource: &failingTokenSource{t: t},
		Transport:   &fakeTransport{},
		BaseURL:     "https://acme.atlassian.net/wiki",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := c.ListRecentPages(context.Background(), "ENG"); !errors.Is(err, ErrAttestationDenied) {
		t.Fatalf("err = %v, want ErrAttestationDenied", err)
	}
}

func TestGetPage_NotFound(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, &fakeAttestor{}, &fakeTokenSource{tok: "tok"}, &fakeTransport{getErr: ErrNotFound})
	if _, err := c.GetPage(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestSearchCQL_HappyPath(t *testing.T) {
	t.Parallel()
	want := []Page{{ID: "42", Title: "Q3 plan", SpaceKey: "ENG"}}
	tr := &fakeTransport{searchOut: want}
	c := newTestClient(t, &fakeAttestor{}, &fakeTokenSource{tok: "tok"}, tr)
	got, err := c.SearchCQL(context.Background(), "type=page AND space=ENG")
	if err != nil {
		t.Fatalf("SearchCQL: %v", err)
	}
	if len(got) != 1 || got[0].ID != "42" {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	if tr.lastQuery != "type=page AND space=ENG" {
		t.Fatalf("transport query = %q, want passthrough", tr.lastQuery)
	}
}

func TestAllRPCs_RateLimited_PassThrough(t *testing.T) {
	t.Parallel()
	tr := &fakeTransport{listErr: ErrRateLimited, getErr: ErrRateLimited, searchErr: ErrRateLimited}
	c := newTestClient(t, &fakeAttestor{}, &fakeTokenSource{tok: "tok"}, tr)
	if _, err := c.ListRecentPages(context.Background(), "ENG"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("List err = %v, want ErrRateLimited", err)
	}
	if _, err := c.GetPage(context.Background(), "1"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("Get err = %v, want ErrRateLimited", err)
	}
	if _, err := c.SearchCQL(context.Background(), "q"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("Search err = %v, want ErrRateLimited", err)
	}
}

func TestNewRejectsMissingDeps(t *testing.T) {
	t.Parallel()
	base := "https://acme.atlassian.net/wiki"
	cases := []struct {
		name string
		cfg  Config
	}{
		{name: "no attestor", cfg: Config{TokenSource: &fakeTokenSource{}, Transport: &fakeTransport{}, BaseURL: base}},
		{name: "no token source", cfg: Config{Attestor: &fakeAttestor{}, Transport: &fakeTransport{}, BaseURL: base}},
		{name: "no transport", cfg: Config{Attestor: &fakeAttestor{}, TokenSource: &fakeTokenSource{}, BaseURL: base}},
		{name: "no base url", cfg: Config{Attestor: &fakeAttestor{}, TokenSource: &fakeTokenSource{}, Transport: &fakeTransport{}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := New(tc.cfg); err == nil {
				t.Fatal("New: expected error for missing dep")
			}
		})
	}
}

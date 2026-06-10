package notion

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeAttestor records gate invocations and per-scope shape so the test
// suite can assert per-RPC consent labeling.
type fakeAttestor struct {
	called    int
	lastScope string
	err       error
}

func (f *fakeAttestor) Attest(_ context.Context, scope string) error {
	f.called++
	f.lastScope = scope
	if !strings.HasPrefix(scope, "notion:") {
		return errors.New("unexpected scope: " + scope)
	}
	return f.err
}

// fakeTokenSource yields a canned integration secret without touching
// $HOME/.leah-state/secrets/notion-token.json.
type fakeTokenSource struct {
	tok string
	err error
}

func (f *fakeTokenSource) Token(_ context.Context) (string, error) {
	return f.tok, f.err
}

// fakeTransport stands in for the api.notion.com/v1 REST layer; the adapter
// MVP routes RPCs through a swappable Transport so tests stay hermetic.
type fakeTransport struct {
	dbs           []DB
	pages         []Page
	page          Page
	createReturns Page

	listErr   error
	queryErr  error
	getErr    error
	createErr error

	lastBearer string
	lastDBID   string
	lastFilter Filter
	lastProps  Props
}

func (f *fakeTransport) ListDatabases(_ context.Context, bearer string) ([]DB, error) {
	f.lastBearer = bearer
	return f.dbs, f.listErr
}

func (f *fakeTransport) QueryDatabase(_ context.Context, bearer, dbID string, fl Filter) ([]Page, error) {
	f.lastBearer = bearer
	f.lastDBID = dbID
	f.lastFilter = fl
	return f.pages, f.queryErr
}

func (f *fakeTransport) GetPage(_ context.Context, bearer, id string) (Page, error) {
	f.lastBearer = bearer
	return f.page, f.getErr
}

func (f *fakeTransport) CreatePage(_ context.Context, bearer, dbID string, p Props) (Page, error) {
	f.lastBearer = bearer
	f.lastDBID = dbID
	f.lastProps = p
	return f.createReturns, f.createErr
}

// failingTokenSource fails the test if Token() runs — used to prove gate
// ordering (attestation BEFORE token load).
type failingTokenSource struct {
	t *testing.T
}

func (f *failingTokenSource) Token(_ context.Context) (string, error) {
	f.t.Fatal("TokenSource.Token() must NOT be called when validation/attestation fails")
	return "", nil
}

func newTestAdapter(t *testing.T, att *fakeAttestor, ts *fakeTokenSource, tr *fakeTransport) *Adapter {
	t.Helper()
	a, err := New(Config{Attestor: att, TokenSource: ts, Transport: tr})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return a
}

func TestListDatabases_HappyPath(t *testing.T) {
	t.Parallel()
	att := &fakeAttestor{}
	tr := &fakeTransport{dbs: []DB{{ID: "db1", Title: "Notes", URL: "https://notion.so/db1"}}}
	a := newTestAdapter(t, att, &fakeTokenSource{tok: "secret_x"}, tr)

	got, err := a.ListDatabases(context.Background())
	if err != nil {
		t.Fatalf("ListDatabases: %v", err)
	}
	if len(got) != 1 || got[0].ID != "db1" || got[0].Title != "Notes" {
		t.Fatalf("got %+v", got)
	}
	if att.lastScope != ScopeListDatabases {
		t.Fatalf("scope = %q, want %q", att.lastScope, ScopeListDatabases)
	}
	if tr.lastBearer != "secret_x" {
		t.Fatalf("bearer = %q, want secret_x", tr.lastBearer)
	}
}

func TestListDatabases_AttestationDenied_NoTokenLoad(t *testing.T) {
	t.Parallel()
	a, err := New(Config{
		Attestor:    &fakeAttestor{err: errors.New("denied")},
		TokenSource: &failingTokenSource{t: t},
		Transport:   &fakeTransport{},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := a.ListDatabases(context.Background()); !errors.Is(err, ErrAttestationDenied) {
		t.Fatalf("err = %v, want ErrAttestationDenied", err)
	}
}

func TestQueryDatabase_DBIDEmpty_NoAttest(t *testing.T) {
	t.Parallel()
	att := &fakeAttestor{}
	a, err := New(Config{
		Attestor:    att,
		TokenSource: &failingTokenSource{t: t},
		Transport:   &fakeTransport{},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := a.QueryDatabase(context.Background(), "", Filter{}); !errors.Is(err, ErrInvalidPage) {
		t.Fatalf("err = %v, want ErrInvalidPage", err)
	}
	if att.called != 0 {
		t.Fatalf("attestor called %d times; must not run before id-validation", att.called)
	}
}

func TestQueryDatabase_HappyPath(t *testing.T) {
	t.Parallel()
	att := &fakeAttestor{}
	tr := &fakeTransport{pages: []Page{{ID: "p1", Title: "Page1", DB: "db1"}}}
	a := newTestAdapter(t, att, &fakeTokenSource{tok: "tok"}, tr)

	got, err := a.QueryDatabase(context.Background(), "db1", Filter{PropertyName: "Status", PropertyValue: "Open"})
	if err != nil {
		t.Fatalf("QueryDatabase: %v", err)
	}
	if len(got) != 1 || got[0].ID != "p1" {
		t.Fatalf("got %+v", got)
	}
	if tr.lastDBID != "db1" {
		t.Fatalf("transport saw dbID=%q, want db1", tr.lastDBID)
	}
	if tr.lastFilter.PropertyName != "Status" {
		t.Fatalf("filter property = %q, want Status", tr.lastFilter.PropertyName)
	}
	if att.lastScope != ScopeQueryDatabase {
		t.Fatalf("scope = %q, want %q", att.lastScope, ScopeQueryDatabase)
	}
}

func TestGetPage_IDEmpty_NoAttest(t *testing.T) {
	t.Parallel()
	att := &fakeAttestor{}
	a, err := New(Config{
		Attestor:    att,
		TokenSource: &failingTokenSource{t: t},
		Transport:   &fakeTransport{},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := a.GetPage(context.Background(), ""); !errors.Is(err, ErrInvalidPage) {
		t.Fatalf("err = %v, want ErrInvalidPage", err)
	}
	if att.called != 0 {
		t.Fatal("attestor invoked despite id-validation failure")
	}
}

func TestGetPage_NotFound(t *testing.T) {
	t.Parallel()
	tr := &fakeTransport{getErr: ErrNotFound}
	a := newTestAdapter(t, &fakeAttestor{}, &fakeTokenSource{tok: "tok"}, tr)
	if _, err := a.GetPage(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestCreatePage_TitleEmpty_NoAttest(t *testing.T) {
	t.Parallel()
	att := &fakeAttestor{}
	a, err := New(Config{
		Attestor:    att,
		TokenSource: &failingTokenSource{t: t},
		Transport:   &fakeTransport{},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := a.CreatePage(context.Background(), "db1", Props{Title: ""}); !errors.Is(err, ErrInvalidPage) {
		t.Fatalf("err = %v, want ErrInvalidPage", err)
	}
	if att.called != 0 {
		t.Fatal("attestor invoked despite title-validation failure")
	}
}

func TestCreatePage_DBIDEmpty_NoAttest(t *testing.T) {
	t.Parallel()
	att := &fakeAttestor{}
	a, err := New(Config{
		Attestor:    att,
		TokenSource: &failingTokenSource{t: t},
		Transport:   &fakeTransport{},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := a.CreatePage(context.Background(), "", Props{Title: "x"}); !errors.Is(err, ErrInvalidPage) {
		t.Fatalf("err = %v, want ErrInvalidPage", err)
	}
}

func TestCreatePage_HappyPath(t *testing.T) {
	t.Parallel()
	att := &fakeAttestor{}
	tr := &fakeTransport{createReturns: Page{ID: "new1", Title: "Hi", DB: "db1", URL: "https://notion.so/new1"}}
	a := newTestAdapter(t, att, &fakeTokenSource{tok: "tok"}, tr)

	got, err := a.CreatePage(context.Background(), "db1", Props{Title: "Hi", Fields: map[string]string{"Status": "Open"}})
	if err != nil {
		t.Fatalf("CreatePage: %v", err)
	}
	if got.ID != "new1" {
		t.Fatalf("got %+v", got)
	}
	if tr.lastProps.Title != "Hi" {
		t.Fatalf("transport saw title=%q, want Hi", tr.lastProps.Title)
	}
	if att.lastScope != ScopeCreatePage {
		t.Fatalf("scope = %q, want %q", att.lastScope, ScopeCreatePage)
	}
}

func TestGateOrdering_AttestationDeniedSkipsTokenAndTransport(t *testing.T) {
	t.Parallel()
	tr := &fakeTransport{}
	a, err := New(Config{
		Attestor:    &fakeAttestor{err: errors.New("denied")},
		TokenSource: &failingTokenSource{t: t},
		Transport:   tr,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := a.CreatePage(context.Background(), "db1", Props{Title: "x"}); !errors.Is(err, ErrAttestationDenied) {
		t.Fatalf("err = %v, want ErrAttestationDenied", err)
	}
	if tr.lastBearer != "" {
		t.Fatal("transport invoked despite attestation denial")
	}
}

func TestNewRejectsMissingDeps(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		cfg  Config
	}{
		{name: "no attestor", cfg: Config{TokenSource: &fakeTokenSource{}, Transport: &fakeTransport{}}},
		{name: "no token source", cfg: Config{Attestor: &fakeAttestor{}, Transport: &fakeTransport{}}},
		{name: "no transport", cfg: Config{Attestor: &fakeAttestor{}, TokenSource: &fakeTokenSource{}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := New(tc.cfg); err == nil {
				t.Fatal("New: expected error for missing dep")
			}
		})
	}
}

func TestScopeConstants_DistinctPerRPC(t *testing.T) {
	t.Parallel()
	scopes := []string{ScopeListDatabases, ScopeQueryDatabase, ScopeGetPage, ScopeCreatePage}
	seen := map[string]bool{}
	for _, s := range scopes {
		if !strings.HasPrefix(s, "notion:") {
			t.Errorf("scope %q lacks notion: prefix", s)
		}
		if seen[s] {
			t.Errorf("scope %q duplicated — per-RPC consent grain lost", s)
		}
		seen[s] = true
	}
}

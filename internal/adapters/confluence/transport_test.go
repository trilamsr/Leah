package confluence

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/trilam/leah/internal/adapters/atlassian"
)

func TestAuditDetail_ExcludesBody(t *testing.T) {
	t.Parallel()
	p := Page{ID: "42", SpaceKey: "ENG", Body: "SECRET BODY TEXT"}
	d := AuditDetail(p)
	if !strings.Contains(d, "42") || !strings.Contains(d, "ENG") {
		t.Fatalf("audit detail missing page_id/space_key: %q", d)
	}
	if strings.Contains(d, "SECRET BODY TEXT") {
		t.Fatalf("audit detail leaked page body: %q", d)
	}
}

func TestHTTPTransport_GetPage_ParsesAndBasicAuth(t *testing.T) {
	t.Parallel()
	var gotAuth, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"id":"42","title":"Q3 Plan","space":{"key":"ENG"},"body":{"storage":{"value":"<p>hi</p>"}},"_links":{"webui":"/spaces/ENG/pages/42","base":"https://acme.atlassian.net/wiki"},"version":{"when":"2026-06-19T08:00:00Z"}}`))
	}))
	defer srv.Close()
	tr := NewHTTPTransport(srv.Client(), srv.URL)
	bearer := atlassian.BasicAuthHeader("ops@acme.com", "tok")
	p, err := tr.GetPage(context.Background(), bearer, "42")
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if gotAuth != bearer {
		t.Fatalf("Authorization = %q, want Basic header %q", gotAuth, bearer)
	}
	if !strings.HasSuffix(gotPath, "/content/42") {
		t.Fatalf("path = %q, want .../content/42", gotPath)
	}
	if p.Title != "Q3 Plan" || p.SpaceKey != "ENG" || p.Body != "<p>hi</p>" {
		t.Fatalf("parse mismatch: %+v", p)
	}
	if p.URL != "https://acme.atlassian.net/wiki/spaces/ENG/pages/42" {
		t.Fatalf("URL = %q", p.URL)
	}
}

func TestHTTPTransport_ListRecent_Parses(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"results":[{"id":"1","title":"A","space":{"key":"ENG"},"_links":{"webui":"/x/1","base":"https://acme.atlassian.net/wiki"},"version":{"when":"2026-06-19T08:00:00Z"}}]}`))
	}))
	defer srv.Close()
	tr := NewHTTPTransport(srv.Client(), srv.URL)
	pages, err := tr.ListRecent(context.Background(), "auth", "ENG")
	if err != nil {
		t.Fatalf("ListRecent: %v", err)
	}
	if len(pages) != 1 || pages[0].SpaceKey != "ENG" || pages[0].Body != "" {
		t.Fatalf("list parse: %+v", pages)
	}
}

func TestHTTPTransport_SearchCQL_Parses(t *testing.T) {
	t.Parallel()
	var gotCQL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCQL = r.URL.Query().Get("cql")
		_, _ = w.Write([]byte(`{"results":[{"content":{"id":"7","title":"B","space":{"key":"ENG"},"_links":{"webui":"/x/7"}}},{"content":{"id":"8","title":"C","space":{"key":"ENG"},"_links":{"webui":"/x/8"}}}],"_links":{"base":"https://acme.atlassian.net/wiki"}}`))
	}))
	defer srv.Close()
	tr := NewHTTPTransport(srv.Client(), srv.URL)
	pages, err := tr.SearchCQL(context.Background(), "auth", "type=page AND space=ENG")
	if err != nil {
		t.Fatalf("SearchCQL: %v", err)
	}
	if gotCQL != "type=page AND space=ENG" {
		t.Fatalf("cql param = %q, want passthrough (escaped, not concatenated)", gotCQL)
	}
	if len(pages) != 2 || pages[0].ID != "7" || pages[1].ID != "8" {
		t.Fatalf("search parse: %+v", pages)
	}
}

func TestHTTPTransport_ErrorMapping(t *testing.T) {
	t.Parallel()
	cases := []struct {
		code int
		want error
	}{
		{401, ErrAuthFailed},
		{403, ErrAuthFailed},
		{404, ErrNotFound},
		{429, ErrRateLimited},
	}
	for _, tc := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(tc.code)
		}))
		tr := NewHTTPTransport(srv.Client(), srv.URL)
		if _, err := tr.GetPage(context.Background(), "auth", "1"); !errors.Is(err, tc.want) {
			t.Fatalf("status %d -> %v, want %v", tc.code, err, tc.want)
		}
		srv.Close()
	}
}

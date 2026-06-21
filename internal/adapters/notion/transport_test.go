package notion

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPTransport_HeadersOnEveryCall(t *testing.T) {
	t.Parallel()
	calls := []struct {
		name string
		fn   func(*HTTPTransport) error
		body string
	}{
		{"ListDatabases", func(tr *HTTPTransport) error {
			_, err := tr.ListDatabases(context.Background(), "secret_x")
			return err
		}, `{"results":[]}`},
		{"QueryDatabase", func(tr *HTTPTransport) error {
			_, err := tr.QueryDatabase(context.Background(), "secret_x", "db1", Filter{})
			return err
		}, `{"results":[]}`},
		{"GetPage", func(tr *HTTPTransport) error {
			_, err := tr.GetPage(context.Background(), "secret_x", "p1")
			return err
		}, `{"id":"p1","properties":{}}`},
		{"CreatePage", func(tr *HTTPTransport) error {
			_, err := tr.CreatePage(context.Background(), "secret_x", "db1", Props{Title: "Hi"})
			return err
		}, `{"id":"new1","properties":{}}`},
	}
	for _, tc := range calls {
		var gotAuth, gotVersion string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotAuth = r.Header.Get("Authorization")
			gotVersion = r.Header.Get("Notion-Version")
			_, _ = w.Write([]byte(tc.body))
		}))
		tr := NewHTTPTransport(srv.Client(), srv.URL)
		if err := tc.fn(tr); err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if gotAuth != "Bearer secret_x" {
			t.Fatalf("%s: Authorization = %q, want %q", tc.name, gotAuth, "Bearer secret_x")
		}
		if gotVersion != notionVersion {
			t.Fatalf("%s: Notion-Version = %q, want %q", tc.name, gotVersion, notionVersion)
		}
		srv.Close()
	}
}

func TestHTTPTransport_ListDatabases_Parses(t *testing.T) {
	t.Parallel()
	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		_, _ = w.Write([]byte(`{"results":[{"id":"db1","url":"https://notion.so/db1","title":[{"plain_text":"Notes"}]}]}`))
	}))
	defer srv.Close()
	tr := NewHTTPTransport(srv.Client(), srv.URL)
	dbs, err := tr.ListDatabases(context.Background(), "tok")
	if err != nil {
		t.Fatalf("ListDatabases: %v", err)
	}
	if gotMethod != http.MethodPost || !strings.HasSuffix(gotPath, "/search") {
		t.Fatalf("want POST .../search, got %s %s", gotMethod, gotPath)
	}
	if len(dbs) != 1 || dbs[0].ID != "db1" || dbs[0].Title != "Notes" || dbs[0].URL != "https://notion.so/db1" {
		t.Fatalf("parse mismatch: %+v", dbs)
	}
}

func TestHTTPTransport_QueryDatabase_FilterAndParse(t *testing.T) {
	t.Parallel()
	var gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		b := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(b)
		gotBody = string(b)
		_, _ = w.Write([]byte(`{"results":[{"id":"p1","url":"https://notion.so/p1","parent":{"database_id":"db1"},"properties":{"Name":{"title":[{"plain_text":"Page1"}]}}}]}`))
	}))
	defer srv.Close()
	tr := NewHTTPTransport(srv.Client(), srv.URL)
	pages, err := tr.QueryDatabase(context.Background(), "tok", "db1", Filter{PropertyName: "Status", PropertyValue: "Open"})
	if err != nil {
		t.Fatalf("QueryDatabase: %v", err)
	}
	if !strings.HasSuffix(gotPath, "/databases/db1/query") {
		t.Fatalf("path = %q, want .../databases/db1/query", gotPath)
	}
	if !strings.Contains(gotBody, "Status") || !strings.Contains(gotBody, "Open") {
		t.Fatalf("filter not in body: %q", gotBody)
	}
	if len(pages) != 1 || pages[0].ID != "p1" || pages[0].Title != "Page1" || pages[0].DB != "db1" {
		t.Fatalf("parse mismatch: %+v", pages)
	}
}

func TestHTTPTransport_GetPage_Parses(t *testing.T) {
	t.Parallel()
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"id":"p1","url":"https://notion.so/p1","parent":{"database_id":"db1"},"properties":{"Name":{"title":[{"plain_text":"Hello"}]}}}`))
	}))
	defer srv.Close()
	tr := NewHTTPTransport(srv.Client(), srv.URL)
	p, err := tr.GetPage(context.Background(), "tok", "p1")
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if !strings.HasSuffix(gotPath, "/pages/p1") {
		t.Fatalf("path = %q, want .../pages/p1", gotPath)
	}
	if p.ID != "p1" || p.Title != "Hello" || p.DB != "db1" {
		t.Fatalf("parse mismatch: %+v", p)
	}
}

func TestHTTPTransport_CreatePage_BodyAndParse(t *testing.T) {
	t.Parallel()
	var gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		b := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(b)
		gotBody = string(b)
		_, _ = w.Write([]byte(`{"id":"new1","url":"https://notion.so/new1","parent":{"database_id":"db1"},"properties":{"Name":{"title":[{"plain_text":"Hi"}]}}}`))
	}))
	defer srv.Close()
	tr := NewHTTPTransport(srv.Client(), srv.URL)
	p, err := tr.CreatePage(context.Background(), "tok", "db1", Props{Title: "Hi", Fields: map[string]string{"Status": "Open"}})
	if err != nil {
		t.Fatalf("CreatePage: %v", err)
	}
	if !strings.HasSuffix(gotPath, "/pages") {
		t.Fatalf("path = %q, want .../pages", gotPath)
	}
	if !strings.Contains(gotBody, "db1") || !strings.Contains(gotBody, "Hi") {
		t.Fatalf("parent/title not in body: %q", gotBody)
	}
	if p.ID != "new1" || p.DB != "db1" {
		t.Fatalf("parse mismatch: %+v", p)
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
		if _, err := tr.GetPage(context.Background(), "tok", "1"); !errors.Is(err, tc.want) {
			t.Fatalf("status %d -> %v, want %v", tc.code, err, tc.want)
		}
		srv.Close()
	}
}

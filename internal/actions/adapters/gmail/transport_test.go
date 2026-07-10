package gmail

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPTransportListUnread(t *testing.T) {
	var gotAuth, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"messages":[{"id":"a","threadId":"t1"},{"id":"b","threadId":"t2"}]}`))
	}))
	defer srv.Close()

	tr := newHTTPTransport(srv.Client(), srv.URL)
	ids, err := tr.ListUnread(context.Background(), "tok-123")
	if err != nil {
		t.Fatalf("ListUnread err = %v", err)
	}
	if len(ids) != 2 || ids[0] != "a" || ids[1] != "b" {
		t.Fatalf("ids = %v, want [a b]", ids)
	}
	if gotAuth != "Bearer tok-123" {
		t.Fatalf("auth header = %q, want Bearer tok-123", gotAuth)
	}
	if !strings.Contains(gotQuery, "q=is%3Aunread") {
		t.Fatalf("query = %q, want is:unread filter", gotQuery)
	}
}

func TestHTTPTransportListUnreadEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"resultSizeEstimate":0}`))
	}))
	defer srv.Close()

	tr := newHTTPTransport(srv.Client(), srv.URL)
	ids, err := tr.ListUnread(context.Background(), "tok")
	if err != nil {
		t.Fatalf("ListUnread err = %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("ids = %v, want empty", ids)
	}
}

func TestHTTPTransportListUnreadAuthRequired(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	tr := newHTTPTransport(srv.Client(), srv.URL)
	_, err := tr.ListUnread(context.Background(), "stale")
	if !errors.Is(err, ErrAuthRequired) {
		t.Fatalf("err = %v, want ErrAuthRequired", err)
	}
}

package notify

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestPushoverPostsExpectedForm(t *testing.T) {
	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got, _ = url.ParseQuery(string(body))
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"status":1}`))
	}))
	defer srv.Close()

	p := &Pushover{
		User:   "uuser",
		Token:  "tToken",
		HTTP:   srv.Client(),
		APIURL: srv.URL,
	}

	if err := p.Notify(context.Background(), "title!", "body!"); err != nil {
		t.Fatalf("notify: %v", err)
	}
	if got.Get("user") != "uuser" {
		t.Errorf("user: %v", got.Get("user"))
	}
	if got.Get("token") != "tToken" {
		t.Errorf("token: %v", got.Get("token"))
	}
	if got.Get("title") != "title!" || got.Get("message") != "body!" {
		t.Errorf("payload: %v", got)
	}
}

func TestPushoverSkipsWhenCredsMissing(t *testing.T) {
	p := &Pushover{User: "", Token: ""}
	err := p.Notify(context.Background(), "t", "b")
	if err == nil || !strings.Contains(err.Error(), "credentials") {
		t.Errorf("want credentials error, got %v", err)
	}
}

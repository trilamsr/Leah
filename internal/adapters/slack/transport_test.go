package slack

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// slackServer replies with body at HTTP 200 (Slack's convention even on logical
// errors), recording the path and Authorization header it saw.
func slackServer(t *testing.T, body string, gotAuth, gotPath *string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if gotAuth != nil {
			*gotAuth = r.Header.Get("Authorization")
		}
		if gotPath != nil {
			*gotPath = r.URL.Path
		}
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestHTTPTransport_ListChannels_ParsesAndSetsBearer(t *testing.T) {
	t.Parallel()
	var gotAuth, gotPath string
	srv := slackServer(t, `{"ok":true,"channels":[
		{"id":"C1","name":"general","is_im":false,"is_group":false},
		{"id":"D1","name":"","is_im":true,"is_group":false},
		{"id":"G1","name":"private","is_im":false,"is_group":true}
	]}`, &gotAuth, &gotPath)
	tr := NewHTTPTransport(srv.Client(), srv.URL)

	chs, err := tr.ListChannels(context.Background(), "xoxb-bot")
	if err != nil {
		t.Fatalf("ListChannels: %v", err)
	}
	if len(chs) != 3 {
		t.Fatalf("len = %d, want 3", len(chs))
	}
	if chs[0] != (Channel{ID: "C1", Name: "general"}) {
		t.Fatalf("ch0 = %+v", chs[0])
	}
	if !chs[1].IsIM || !chs[2].IsGroup {
		t.Fatalf("type flags lost: %+v", chs)
	}
	if gotAuth != "Bearer xoxb-bot" {
		t.Fatalf("Authorization = %q, want Bearer xoxb-bot", gotAuth)
	}
	if gotPath != "/conversations.list" {
		t.Fatalf("path = %q, want /conversations.list", gotPath)
	}
}

// The load-bearing Slack gotcha: HTTP 200 + ok:false MUST surface as a typed
// error, never a silent empty success.
func TestHTTPTransport_OkFalse_OnHTTP200_MapsTypedError(t *testing.T) {
	t.Parallel()
	cases := []struct {
		slackErr string
		want     error
	}{
		{"invalid_auth", ErrAuthFailed},
		{"not_authed", ErrAuthFailed},
		{"token_revoked", ErrAuthFailed},
		{"channel_not_found", ErrNotFound},
		{"thread_not_found", ErrNotFound},
		{"ratelimited", ErrRateLimited},
		{"rate_limited", ErrRateLimited},
		{"is_archived", ErrInvalidChannel},
		{"msg_too_long", ErrInvalidChannel},
	}
	for _, tc := range cases {
		t.Run(tc.slackErr, func(t *testing.T) {
			srv := slackServer(t, `{"ok":false,"error":"`+tc.slackErr+`"}`, nil, nil)
			tr := NewHTTPTransport(srv.Client(), srv.URL)
			err := tr.PostMessage(context.Background(), "xoxb", "C1", "hi")
			if !errors.Is(err, tc.want) {
				t.Fatalf("error %q -> %v, want %v", tc.slackErr, err, tc.want)
			}
		})
	}
}

func TestHTTPTransport_OkFalse_UnknownError(t *testing.T) {
	t.Parallel()
	srv := slackServer(t, `{"ok":false,"error":"something_weird"}`, nil, nil)
	tr := NewHTTPTransport(srv.Client(), srv.URL)
	err := tr.PostMessage(context.Background(), "xoxb", "C1", "hi")
	if err == nil {
		t.Fatal("ok:false with unknown error must still be a non-nil error")
	}
	// Unknown codes are NOT silently swallowed and NOT a known sentinel.
	if errors.Is(err, ErrNotFound) || errors.Is(err, ErrAuthFailed) {
		t.Fatalf("unexpected sentinel for unknown error: %v", err)
	}
	if !strings.Contains(err.Error(), "something_weird") {
		t.Fatalf("error should carry the Slack code: %v", err)
	}
}

func TestHTTPTransport_PostMessage_OkTrue(t *testing.T) {
	t.Parallel()
	var gotPath, gotAuth string
	srv := slackServer(t, `{"ok":true}`, &gotAuth, &gotPath)
	tr := NewHTTPTransport(srv.Client(), srv.URL)
	if err := tr.PostMessage(context.Background(), "xoxb-bot", "C1", "hi"); err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	if gotPath != "/chat.postMessage" {
		t.Fatalf("path = %q, want /chat.postMessage", gotPath)
	}
	if gotAuth != "Bearer xoxb-bot" {
		t.Fatalf("auth = %q", gotAuth)
	}
}

func TestHTTPTransport_GetThread_Parses(t *testing.T) {
	t.Parallel()
	var gotPath string
	srv := slackServer(t, `{"ok":true,"messages":[
		{"user":"U1","ts":"1.1","text":"root"},
		{"user":"U2","ts":"2.2","text":"reply"}
	]}`, nil, &gotPath)
	tr := NewHTTPTransport(srv.Client(), srv.URL)

	th, err := tr.GetThread(context.Background(), "xoxb", "C1", "1.1")
	if err != nil {
		t.Fatalf("GetThread: %v", err)
	}
	if th.Channel != "C1" || th.TS != "1.1" || len(th.Replies) != 2 {
		t.Fatalf("thread = %+v", th)
	}
	if th.Replies[1] != (Message{User: "U2", TS: "2.2", Text: "reply"}) {
		t.Fatalf("reply lost: %+v", th.Replies[1])
	}
	if gotPath != "/conversations.replies" {
		t.Fatalf("path = %q, want /conversations.replies", gotPath)
	}
}

func TestHTTPTransport_GetThread_OkFalse_NotFound(t *testing.T) {
	t.Parallel()
	srv := slackServer(t, `{"ok":false,"error":"thread_not_found"}`, nil, nil)
	tr := NewHTTPTransport(srv.Client(), srv.URL)
	if _, err := tr.GetThread(context.Background(), "xoxb", "C1", "1.1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestHTTPTransport_Search_ParsesAndUsesUserToken(t *testing.T) {
	t.Parallel()
	var gotAuth, gotPath string
	srv := slackServer(t, `{"ok":true,"messages":{"matches":[
		{"channel":{"id":"C1"},"ts":"1.1","text":"snippet here","permalink":"https://x/p1"}
	]}}`, &gotAuth, &gotPath)
	tr := NewHTTPTransport(srv.Client(), srv.URL)

	res, err := tr.Search(context.Background(), "xoxp-user", "incident")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res) != 1 || res[0] != (Result{Channel: "C1", TS: "1.1", Snippet: "snippet here", URL: "https://x/p1"}) {
		t.Fatalf("result = %+v", res)
	}
	if gotAuth != "Bearer xoxp-user" {
		t.Fatalf("Authorization = %q, want Bearer xoxp-user", gotAuth)
	}
	if gotPath != "/search.messages" {
		t.Fatalf("path = %q, want /search.messages", gotPath)
	}
}

func TestHTTPTransport_HTTPError_MapsSentinel(t *testing.T) {
	t.Parallel()
	cases := []struct {
		status int
		want   error
	}{
		{http.StatusUnauthorized, ErrAuthFailed},
		{http.StatusForbidden, ErrAuthFailed},
		{http.StatusTooManyRequests, ErrRateLimited},
	}
	for _, tc := range cases {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
			}))
			t.Cleanup(srv.Close)
			tr := NewHTTPTransport(srv.Client(), srv.URL)
			if _, err := tr.ListChannels(context.Background(), "xoxb"); !errors.Is(err, tc.want) {
				t.Fatalf("status %d -> %v, want %v", tc.status, err, tc.want)
			}
		})
	}
}

func TestNewHTTPTransport_DefaultsBaseURL(t *testing.T) {
	t.Parallel()
	tr := NewHTTPTransport(nil, "")
	if tr.base != apiBase {
		t.Fatalf("base = %q, want %q", tr.base, apiBase)
	}
	if tr.hc == nil {
		t.Fatal("nil HTTP client not defaulted")
	}
}

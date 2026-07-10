package gmail

import (
	"context"
	"errors"
	"testing"

	"github.com/trilam/leah/internal/platform/telemetry"
	"github.com/trilam/leah/internal/platform/telemetry/obstest"
)

// TestRegisterMetrics_AddsSeries asserts the shared leah_connect_* series surface bound to gmail.
func TestRegisterMetrics_AddsSeries(t *testing.T) {
	r := telemetry.NewRegistry()
	RegisterMetrics(r)
	keys := obstest.SnapshotKeys(t, r)
	for _, want := range []string{
		"leah_connect_api_call_total",
		"leah_connect_exchange_total",
		"leah_connect_refresh_total",
		"leah_connect_token_age_seconds",
		"leah_connect_api_latency_seconds",
	} {
		if !obstest.ContainsPrefix(keys, want) {
			t.Fatalf("series %q missing from %v", want, keys)
		}
	}
}

// TestObserveAPI_OnRPC asserts each Transport-backed RPC bumps api_call_total on success and failure.
func TestObserveAPI_OnRPC(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		endpoint string
		run      func(*testing.T, *Client)
	}{
		{
			name:     "list_unread success",
			endpoint: "list_unread",
			run: func(t *testing.T, c *Client) {
				if _, err := c.ListUnread(context.Background()); err != nil {
					t.Fatalf("ListUnread: %v", err)
				}
			},
		},
		{
			name:     "mark_read success",
			endpoint: "mark_read",
			run: func(t *testing.T, c *Client) {
				if err := c.MarkRead(context.Background(), "msg-1"); err != nil {
					t.Fatalf("MarkRead: %v", err)
				}
			},
		},
		{
			name:     "send success",
			endpoint: "send",
			run: func(t *testing.T, c *Client) {
				if err := c.Send(context.Background(), Message{To: "x@y.z"}); err != nil {
					t.Fatalf("Send: %v", err)
				}
			},
		},
		{
			name:     "send failure still observed",
			endpoint: "send",
			run: func(t *testing.T, c *Client) {
				err := c.Send(context.Background(), Message{To: "x@y.z"})
				if err == nil {
					t.Fatal("Send: expected transport error")
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := telemetry.NewRegistry()
			RegisterMetrics(r)
			tr := &fakeTransport{}
			if tc.name == "send failure still observed" {
				tr.sendErr = errors.New("boom")
			}
			c, err := New(Config{
				Attestor:    &fakeAttestor{},
				TokenSource: &fakeTokenSource{tok: "t"},
				Transport:   tr,
				Metrics:     Metrics(r),
			})
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			tc.run(t, c)
			keys := obstest.SnapshotKeys(t, r)
			want := "leah_connect_api_call_total|endpoint=" + tc.endpoint + ",provider=gmail"
			if !obstest.ContainsExact(keys, want) {
				t.Fatalf("missing %q in %v", want, keys)
			}
			if !obstest.ContainsPrefix(keys, "leah_connect_api_latency_seconds") {
				t.Fatalf("latency histogram not observed: %v", keys)
			}
		})
	}
}

// TestObserveAPI_NilMetricsNoop proves the legacy Config caller shape still works when Metrics is omitted.
func TestObserveAPI_NilMetricsNoop(t *testing.T) {
	t.Parallel()
	c, err := New(Config{
		Attestor:    &fakeAttestor{},
		TokenSource: &fakeTokenSource{tok: "t"},
		Transport:   &fakeTransport{listIDs: []string{"a"}},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := c.ListUnread(context.Background()); err != nil {
		t.Fatalf("ListUnread: %v", err)
	}
}

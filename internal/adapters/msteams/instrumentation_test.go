package msteams

import (
	"context"
	"testing"

	"github.com/trilam/leah/internal/obs"
	"github.com/trilam/leah/internal/obs/obstest"
)

// TestRegisterMetrics_AddsSeries asserts the shared leah_connect_* series surface bound to msteams.
func TestRegisterMetrics_AddsSeries(t *testing.T) {
	r := obs.NewRegistry()
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

// TestObserveAPI_OnRPC asserts each Graph-backed RPC bumps api_call_total on success and failure.
func TestObserveAPI_OnRPC(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		endpoint string
		status   int
		body     string
		run      func(*testing.T, *Client)
	}{
		{
			name:     "list_teams success",
			endpoint: "list_teams",
			body:     `{"value":[{"id":"t1","displayName":"Alpha"}]}`,
			run: func(t *testing.T, c *Client) {
				if _, err := c.ListTeams(context.Background()); err != nil {
					t.Fatalf("ListTeams: %v", err)
				}
			},
		},
		{
			name:     "list_teams failure still observed",
			endpoint: "list_teams",
			status:   401,
			body:     `{"error":{"code":"InvalidAuthenticationToken"}}`,
			run: func(t *testing.T, c *Client) {
				if _, err := c.ListTeams(context.Background()); err == nil {
					t.Fatal("ListTeams: expected auth error")
				}
			},
		},
		{
			name:     "list_channels success",
			endpoint: "list_channels",
			body:     `{"value":[{"id":"c1","displayName":"general"}]}`,
			run: func(t *testing.T, c *Client) {
				if _, err := c.ListChannels(context.Background(), "t1"); err != nil {
					t.Fatalf("ListChannels: %v", err)
				}
			},
		},
		{
			name:     "post_message success",
			endpoint: "post_message",
			status:   201,
			body:     `{"id":"m1"}`,
			run: func(t *testing.T, c *Client) {
				if err := c.PostMessage(context.Background(), "t1", "c1", "hi"); err != nil {
					t.Fatalf("PostMessage: %v", err)
				}
			},
		},
		{
			name:     "post_message failure still observed",
			endpoint: "post_message",
			status:   429,
			body:     `{"error":{"code":"TooManyRequests"}}`,
			run: func(t *testing.T, c *Client) {
				if err := c.PostMessage(context.Background(), "t1", "c1", "hi"); err == nil {
					t.Fatal("PostMessage: expected rate-limit error")
				}
			},
		},
		{
			name:     "search success",
			endpoint: "search",
			body:     `{"value":[]}`,
			run: func(t *testing.T, c *Client) {
				if _, err := c.Search(context.Background(), "incident"); err != nil {
					t.Fatalf("Search: %v", err)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := obs.NewRegistry()
			RegisterMetrics(r)
			h := &fakeHTTP{status: tc.status, body: tc.body}
			c, err := New(Config{
				Attestor:    &fakeAttestor{},
				TokenSource: &fakeTokenSource{tok: "t"},
				HTTPClient:  h,
				Metrics:     Metrics(r),
			})
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			tc.run(t, c)
			keys := obstest.SnapshotKeys(t, r)
			want := "leah_connect_api_call_total|endpoint=" + tc.endpoint + ",provider=msteams"
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
		HTTPClient:  &fakeHTTP{body: `{"value":[]}`},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := c.ListTeams(context.Background()); err != nil {
		t.Fatalf("ListTeams: %v", err)
	}
}

package confluence

import (
	"context"
	"errors"
	"testing"

	"github.com/trilam/leah/internal/obs"
	"github.com/trilam/leah/internal/obs/obstest"
)

// TestRegisterMetrics_AddsSeries asserts the shared leah_connect_* series surface bound to confluence.
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

// TestObserveAPI_OnRPC asserts each Transport-backed RPC bumps api_call_total on success and failure.
func TestObserveAPI_OnRPC(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		endpoint string
		run      func(*testing.T, *Client)
		setupTr  func(*fakeTransport)
	}{
		{
			name:     "list_recent success",
			endpoint: "list_recent",
			run: func(t *testing.T, c *Client) {
				if _, err := c.ListRecentPages(context.Background(), "ENG"); err != nil {
					t.Fatalf("ListRecentPages: %v", err)
				}
			},
		},
		{
			name:     "get_page success",
			endpoint: "get_page",
			run: func(t *testing.T, c *Client) {
				if _, err := c.GetPage(context.Background(), "1"); err != nil {
					t.Fatalf("GetPage: %v", err)
				}
			},
		},
		{
			name:     "search success",
			endpoint: "search",
			run: func(t *testing.T, c *Client) {
				if _, err := c.SearchCQL(context.Background(), "type=page"); err != nil {
					t.Fatalf("SearchCQL: %v", err)
				}
			},
		},
		{
			name:     "search failure still observed",
			endpoint: "search",
			setupTr: func(tr *fakeTransport) {
				tr.searchErr = errors.New("boom")
			},
			run: func(t *testing.T, c *Client) {
				if _, err := c.SearchCQL(context.Background(), "q"); err == nil {
					t.Fatal("SearchCQL: expected transport error")
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := obs.NewRegistry()
			RegisterMetrics(r)
			tr := &fakeTransport{}
			if tc.setupTr != nil {
				tc.setupTr(tr)
			}
			c, err := New(Config{
				Attestor:    &fakeAttestor{},
				TokenSource: &fakeTokenSource{tok: "t"},
				Transport:   tr,
				BaseURL:     "https://acme.atlassian.net/wiki",
				Metrics:     Metrics(r),
			})
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			tc.run(t, c)
			keys := obstest.SnapshotKeys(t, r)
			want := "leah_connect_api_call_total|endpoint=" + tc.endpoint + ",provider=confluence"
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
		Transport:   &fakeTransport{},
		BaseURL:     "https://acme.atlassian.net/wiki",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := c.ListRecentPages(context.Background(), "ENG"); err != nil {
		t.Fatalf("ListRecentPages: %v", err)
	}
}

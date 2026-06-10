package jira

import (
	"context"
	"errors"
	"testing"

	"github.com/trilam/leah/internal/obs"
	"github.com/trilam/leah/internal/obs/obstest"
)

// TestRegisterMetrics_AddsSeries asserts the shared leah_connect_* series surface bound to this provider.
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
		failure  bool
		run      func(*testing.T, *Client)
	}{
		{
			name:     "list_my_issues success",
			endpoint: "list_my_issues",
			run: func(t *testing.T, c *Client) {
				if _, err := c.ListMyIssues(context.Background()); err != nil {
					t.Fatalf("ListMyIssues: %v", err)
				}
			},
		},
		{
			name:     "get_issue success",
			endpoint: "get_issue",
			run: func(t *testing.T, c *Client) {
				if _, err := c.GetIssue(context.Background(), "ENG-1"); err != nil {
					t.Fatalf("GetIssue: %v", err)
				}
			},
		},
		{
			name:     "create_issue success",
			endpoint: "create_issue",
			run: func(t *testing.T, c *Client) {
				req := IssueReq{ProjectKey: "ENG", Summary: "s", IssueType: "Task"}
				if _, err := c.CreateIssue(context.Background(), req); err != nil {
					t.Fatalf("CreateIssue: %v", err)
				}
			},
		},
		{
			name:     "comment success",
			endpoint: "comment",
			run: func(t *testing.T, c *Client) {
				if err := c.Comment(context.Background(), "ENG-1", "lgtm"); err != nil {
					t.Fatalf("Comment: %v", err)
				}
			},
		},
		{
			name:     "comment failure still observed",
			endpoint: "comment",
			failure:  true,
			run: func(t *testing.T, c *Client) {
				if err := c.Comment(context.Background(), "ENG-1", "lgtm"); err == nil {
					t.Fatal("Comment: expected transport error")
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := obs.NewRegistry()
			RegisterMetrics(r)
			tr := &fakeTransport{}
			if tc.failure {
				tr.commentErr = errors.New("boom")
			}
			c, err := New(Config{
				Attestor:    &fakeAttestor{},
				TokenSource: &fakeTokenSource{tok: "t"},
				Transport:   tr,
				BaseURL:     "https://acme.atlassian.net",
				Metrics:     Metrics(r),
			})
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			tc.run(t, c)
			keys := obstest.SnapshotKeys(t, r)
			want := "leah_connect_api_call_total|endpoint=" + tc.endpoint + ",provider=jira"
			if !obstest.ContainsExact(keys, want) {
				t.Fatalf("missing %q in %v", want, keys)
			}
			if !obstest.ContainsPrefix(keys, "leah_connect_api_latency_seconds") {
				t.Fatalf("latency histogram not observed: %v", keys)
			}
		})
	}
}

// TestObserveAPI_NilMetricsNoop proves legacy Config callers still work when Metrics is omitted.
func TestObserveAPI_NilMetricsNoop(t *testing.T) {
	t.Parallel()
	c, err := New(Config{
		Attestor:    &fakeAttestor{},
		TokenSource: &fakeTokenSource{tok: "t"},
		Transport:   &fakeTransport{listIssues: []Issue{{Key: "ENG-1"}}},
		BaseURL:     "https://acme.atlassian.net",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := c.ListMyIssues(context.Background()); err != nil {
		t.Fatalf("ListMyIssues: %v", err)
	}
}

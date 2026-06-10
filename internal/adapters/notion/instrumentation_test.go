package notion

import (
	"context"
	"errors"
	"testing"

	"github.com/trilam/leah/internal/obs"
	"github.com/trilam/leah/internal/obs/obstest"
)

// TestRegisterMetrics_AddsSeries asserts the shared leah_connect_* series surface bound to notion.
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
		run      func(*testing.T, *Adapter)
		fail     bool
	}{
		{
			name:     "list_databases success",
			endpoint: "list_databases",
			run: func(t *testing.T, a *Adapter) {
				if _, err := a.ListDatabases(context.Background()); err != nil {
					t.Fatalf("ListDatabases: %v", err)
				}
			},
		},
		{
			name:     "query_database success",
			endpoint: "query_database",
			run: func(t *testing.T, a *Adapter) {
				if _, err := a.QueryDatabase(context.Background(), "db1", Filter{}); err != nil {
					t.Fatalf("QueryDatabase: %v", err)
				}
			},
		},
		{
			name:     "get_page success",
			endpoint: "get_page",
			run: func(t *testing.T, a *Adapter) {
				if _, err := a.GetPage(context.Background(), "p1"); err != nil {
					t.Fatalf("GetPage: %v", err)
				}
			},
		},
		{
			name:     "create_page success",
			endpoint: "create_page",
			run: func(t *testing.T, a *Adapter) {
				if _, err := a.CreatePage(context.Background(), "db1", Props{Title: "Hi"}); err != nil {
					t.Fatalf("CreatePage: %v", err)
				}
			},
		},
		{
			name:     "create_page failure still observed",
			endpoint: "create_page",
			fail:     true,
			run: func(t *testing.T, a *Adapter) {
				_, err := a.CreatePage(context.Background(), "db1", Props{Title: "Hi"})
				if err == nil {
					t.Fatal("CreatePage: expected transport error")
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := obs.NewRegistry()
			RegisterMetrics(r)
			tr := &fakeTransport{}
			if tc.fail {
				tr.createErr = errors.New("boom")
			}
			a, err := New(Config{
				Attestor:    &fakeAttestor{},
				TokenSource: &fakeTokenSource{tok: "secret_x"},
				Transport:   tr,
				Metrics:     Metrics(r),
			})
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			tc.run(t, a)
			keys := obstest.SnapshotKeys(t, r)
			want := "leah_connect_api_call_total|endpoint=" + tc.endpoint + ",provider=notion"
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
	a, err := New(Config{
		Attestor:    &fakeAttestor{},
		TokenSource: &fakeTokenSource{tok: "tok"},
		Transport:   &fakeTransport{dbs: []DB{{ID: "db1"}}},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := a.ListDatabases(context.Background()); err != nil {
		t.Fatalf("ListDatabases: %v", err)
	}
}

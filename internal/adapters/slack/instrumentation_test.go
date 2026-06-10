package slack

import (
	"context"
	"errors"
	"testing"

	"github.com/trilam/leah/internal/obs"
	"github.com/trilam/leah/internal/obs/obstest"
)

// TestRegisterMetrics_AddsSeries asserts the shared leah_connect_* series
// surface bound to this provider.
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
		trMut    func(*fakeTransport)
		run      func(*testing.T, *Adapter)
	}{
		{
			name:     "list_channels success",
			endpoint: "list_channels",
			run: func(t *testing.T, a *Adapter) {
				if _, err := a.ListChannels(context.Background()); err != nil {
					t.Fatalf("ListChannels: %v", err)
				}
			},
		},
		{
			name:     "post_message success",
			endpoint: "post_message",
			run: func(t *testing.T, a *Adapter) {
				if err := a.PostMessage(context.Background(), "C1", "hi"); err != nil {
					t.Fatalf("PostMessage: %v", err)
				}
			},
		},
		{
			name:     "post_message failure still observed",
			endpoint: "post_message",
			trMut:    func(tr *fakeTransport) { tr.postErr = errors.New("boom") },
			run: func(t *testing.T, a *Adapter) {
				if err := a.PostMessage(context.Background(), "C1", "hi"); err == nil {
					t.Fatal("PostMessage: expected transport error")
				}
			},
		},
		{
			name:     "get_thread success",
			endpoint: "get_thread",
			run: func(t *testing.T, a *Adapter) {
				if _, err := a.GetThread(context.Background(), "C1", "1.0"); err != nil {
					t.Fatalf("GetThread: %v", err)
				}
			},
		},
		{
			name:     "search success",
			endpoint: "search",
			run: func(t *testing.T, a *Adapter) {
				if _, err := a.Search(context.Background(), "needle"); err != nil {
					t.Fatalf("Search: %v", err)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := obs.NewRegistry()
			RegisterMetrics(r)
			tr := &fakeTransport{}
			if tc.trMut != nil {
				tc.trMut(tr)
			}
			a, err := New(Config{
				Attestor:    &fakeAttestor{},
				TokenSource: &fakeTokenSource{bot: "t", user: "u"},
				Transport:   tr,
				Metrics:     Metrics(r),
			})
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			tc.run(t, a)
			keys := obstest.SnapshotKeys(t, r)
			want := "leah_connect_api_call_total|endpoint=" + tc.endpoint + ",provider=slack"
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
		TokenSource: &fakeTokenSource{bot: "t", user: "u"},
		Transport:   &fakeTransport{},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := a.ListChannels(context.Background()); err != nil {
		t.Fatalf("ListChannels: %v", err)
	}
}

package gcal

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/trilam/leah/internal/obs"
	"github.com/trilam/leah/internal/obs/obstest"
)

// TestRegisterMetrics_AddsSeries asserts the shared leah_connect_* series surface bound to gcal.
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

// TestObserveAPI_OnRPC asserts each calendarService-backed RPC bumps api_call_total on success and failure.
func TestObserveAPI_OnRPC(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		endpoint string
		svc      *fakeService
		run      func(*testing.T, *Adapter)
	}{
		{
			name:     "list_today success",
			endpoint: "list_today",
			svc:      &fakeService{listEvents: []Event{{ID: "e1"}}},
			run: func(t *testing.T, a *Adapter) {
				if _, err := a.ListToday(context.Background()); err != nil {
					t.Fatalf("ListToday: %v", err)
				}
			},
		},
		{
			name:     "list_today failure still observed",
			endpoint: "list_today",
			svc:      &fakeService{listErr: errors.New("boom")},
			run: func(t *testing.T, a *Adapter) {
				if _, err := a.ListToday(context.Background()); err == nil {
					t.Fatal("ListToday: expected service error")
				}
			},
		},
		{
			name:     "create_event success",
			endpoint: "create_event",
			svc:      &fakeService{},
			run: func(t *testing.T, a *Adapter) {
				if _, err := a.CreateEvent(context.Background(), Event{Summary: "x"}); err != nil {
					t.Fatalf("CreateEvent: %v", err)
				}
			},
		},
		{
			name:     "create_event failure still observed",
			endpoint: "create_event",
			svc:      &fakeService{createErr: ErrInvalidEvent},
			run: func(t *testing.T, a *Adapter) {
				if _, err := a.CreateEvent(context.Background(), Event{}); err == nil {
					t.Fatal("CreateEvent: expected validation error")
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := telemetry.NewRegistry()
			RegisterMetrics(r)
			a := &Adapter{
				svc: tc.svc,
				att: &fakeAttestor{},
				ts:  &fakeTokenSource{tok: "t"},
				now: func() time.Time { return time.Now() },
				m:   Metrics(r),
			}
			tc.run(t, a)
			keys := obstest.SnapshotKeys(t, r)
			want := "leah_connect_api_call_total|endpoint=" + tc.endpoint + ",provider=gcal"
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
		TokenPath: "/tmp/leah-test-gcal-token-noop",
		Attestor:  &fakeAttestor{},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	a.svc = &fakeService{listEvents: []Event{{ID: "e1"}}}
	if _, err := a.ListToday(context.Background()); err != nil {
		t.Fatalf("ListToday: %v", err)
	}
}

package facetime

import (
	"context"
	"errors"
	"testing"

	"github.com/trilam/leah/internal/obs"
	"github.com/trilam/leah/internal/obs/obstest"
)

// TestRegisterMetrics_AddsSeries asserts the shared leah_connect_* series surface bound to facetime.
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

// TestObserveAPI_OnRPC asserts each Initiate path bumps api_call_total on success and failure.
func TestObserveAPI_OnRPC(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		endpoint string
		execErr  error
		run      func(*testing.T, *Adapter)
	}{
		{
			name:     "initiate_video success",
			endpoint: "initiate_video",
			run: func(t *testing.T, a *Adapter) {
				if err := a.InitiateVideo(context.Background(), "alice@example.com"); err != nil {
					t.Fatalf("InitiateVideo: %v", err)
				}
			},
		},
		{
			name:     "initiate_audio success",
			endpoint: "initiate_audio",
			run: func(t *testing.T, a *Adapter) {
				if err := a.InitiateAudio(context.Background(), "+15551234567"); err != nil {
					t.Fatalf("InitiateAudio: %v", err)
				}
			},
		},
		{
			name:     "initiate_video failure still observed",
			endpoint: "initiate_video",
			execErr:  errors.New("open boom"),
			run: func(t *testing.T, a *Adapter) {
				err := a.InitiateVideo(context.Background(), "alice@example.com")
				if !errors.Is(err, ErrLaunchFailed) {
					t.Fatalf("InitiateVideo err=%v, want ErrLaunchFailed", err)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := obs.NewRegistry()
			RegisterMetrics(r)
			a, err := New(Config{
				Attestor: &fakeAttestor{},
				OSExec:   &fakeOSExec{err: tc.execErr},
				Metrics:  Metrics(r),
			})
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			tc.run(t, a)
			keys := obstest.SnapshotKeys(t, r)
			want := "leah_connect_api_call_total|endpoint=" + tc.endpoint + ",provider=facetime"
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
		Attestor: &fakeAttestor{},
		OSExec:   &fakeOSExec{},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := a.InitiateVideo(context.Background(), "alice@example.com"); err != nil {
		t.Fatalf("InitiateVideo: %v", err)
	}
}

package imessage

import (
	"context"
	"errors"
	"testing"

	"github.com/trilam/leah/internal/platform/telemetry"
	"github.com/trilam/leah/internal/platform/telemetry/obstest"
)

// TestRegisterMetrics_AddsSeries asserts the shared leah_connect_* series bound to imessage.
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

// TestObserveAPI_OnRPC asserts Send bumps api_call_total on success and failure.
func TestObserveAPI_OnRPC(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		ex   *fakeOSExec
	}{
		{name: "send success", ex: &fakeOSExec{}},
		{name: "send failure still observed", ex: &fakeOSExec{err: errors.New("boom")}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := telemetry.NewRegistry()
			RegisterMetrics(r)
			a, err := New(Config{
				Attestor: &fakeAttestor{},
				OSExec:   tc.ex,
				Metrics:  Metrics(r),
			})
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			_ = a.Send(context.Background(), Message{To: "+15551234567", Body: "x"})
			keys := obstest.SnapshotKeys(t, r)
			want := "leah_connect_api_call_total|endpoint=send,provider=imessage"
			if !obstest.ContainsExact(keys, want) {
				t.Fatalf("missing %q in %v", want, keys)
			}
			if !obstest.ContainsPrefix(keys, "leah_connect_api_latency_seconds") {
				t.Fatalf("latency histogram not observed: %v", keys)
			}
		})
	}
}

// TestObserveAPI_NilMetricsNoop proves the legacy Config shape still works when Metrics is omitted.
func TestObserveAPI_NilMetricsNoop(t *testing.T) {
	t.Parallel()
	a, err := New(Config{Attestor: &fakeAttestor{}, OSExec: &fakeOSExec{}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := a.Send(context.Background(), Message{To: "+15551234567", Body: "x"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
}

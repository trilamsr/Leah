package discord

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/trilam/leah/internal/platform/telemetry"
	"github.com/trilam/leah/internal/platform/telemetry/obstest"
)

// TestRegisterMetrics_AddsSeries asserts the shared leah_connect_* series surface bound to this provider.
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

// errRT short-circuits the HTTP round trip so a.http.Do returns an error path.
type errRT struct{ err error }

func (e *errRT) RoundTrip(*http.Request) (*http.Response, error) { return nil, e.err }

// TestObserveAPI_OnRPC asserts each RPC bumps api_call_total on success and failure.
func TestObserveAPI_OnRPC(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		endpoint string
		run      func(*testing.T, *Adapter)
		failHTTP bool
	}{
		{
			name:     "post_message success",
			endpoint: "post_message",
			run: func(t *testing.T, a *Adapter) {
				if err := a.PostMessage(context.Background(), "c1", "hi"); err != nil {
					t.Fatalf("PostMessage: %v", err)
				}
			},
		},
		{
			name:     "post_message http failure still observed",
			endpoint: "post_message",
			failHTTP: true,
			run: func(t *testing.T, a *Adapter) {
				if err := a.PostMessage(context.Background(), "c1", "hi"); err == nil {
					t.Fatal("PostMessage: expected http error")
				}
			},
		},
		{
			name:     "list_channels success",
			endpoint: "list_channels",
			run: func(t *testing.T, a *Adapter) {
				a.guildAllowlist = []string{"g1"}
				if _, err := a.ListChannels(context.Background(), "g1"); err != nil {
					t.Fatalf("ListChannels: %v", err)
				}
			},
		},
		{
			name:     "post_voice success",
			endpoint: "post_voice",
			run: func(t *testing.T, a *Adapter) {
				if err := a.PostVoice(context.Background(), "c1", []byte("audio")); err != nil {
					t.Fatalf("PostVoice: %v", err)
				}
			},
		},
		{
			name:     "fetch_voice success",
			endpoint: "fetch_voice",
			run: func(t *testing.T, a *Adapter) {
				_ = a.fetchVoiceAttachment(context.Background(), a.baseURL+"/anything")
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`[]`))
			}))
			defer srv.Close()

			r := telemetry.NewRegistry()
			RegisterMetrics(r)

			cfg := Config{
				Attestor:    &fakeAttestor{},
				TokenSource: &fakeTokenSource{},
				HTTPClient:  srv.Client(),
				BaseURL:     srv.URL,
				Metrics:     Metrics(r),
			}
			if tc.failHTTP {
				cfg.HTTPClient = &http.Client{Transport: &errRT{err: errors.New("boom")}}
			}
			a, err := New(cfg)
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			tc.run(t, a)

			keys := obstest.SnapshotKeys(t, r)
			want := "leah_connect_api_call_total|endpoint=" + tc.endpoint + ",provider=discord"
			if !obstest.ContainsExact(keys, want) {
				t.Fatalf("missing %q in %v", want, keys)
			}
			if !obstest.ContainsPrefix(keys, "leah_connect_api_latency_seconds") {
				t.Fatalf("latency histogram not observed: %v", keys)
			}
		})
	}
}

// TestObserveAPI_SubscribeDial asserts the gateway-dial RPC is observed on dial failure.
func TestObserveAPI_SubscribeDial(t *testing.T) {
	t.Parallel()
	r := telemetry.NewRegistry()
	RegisterMetrics(r)
	a, err := New(Config{
		Attestor:        &fakeAttestor{},
		TokenSource:     &fakeTokenSource{},
		WebSocketDialer: &errDialer{err: errors.New("dial boom")},
		Metrics:         Metrics(r),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	err = a.Subscribe(context.Background(), nil, func(Message) {})
	if err == nil {
		t.Fatal("Subscribe: expected dial error")
	}
	keys := obstest.SnapshotKeys(t, r)
	want := "leah_connect_api_call_total|endpoint=subscribe_dial,provider=discord"
	if !obstest.ContainsExact(keys, want) {
		t.Fatalf("missing %q in %v", want, keys)
	}
}

// TestObserveAPI_NilMetricsNoop proves the legacy Config caller shape still works when Metrics is omitted.
func TestObserveAPI_NilMetricsNoop(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"x"}`))
	}))
	defer srv.Close()
	a, err := New(Config{
		Attestor:    &fakeAttestor{},
		TokenSource: &fakeTokenSource{},
		HTTPClient:  srv.Client(),
		BaseURL:     srv.URL,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := a.PostMessage(context.Background(), "c1", "hi"); err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
}

type errDialer struct{ err error }

func (e *errDialer) Dial(context.Context, string, string) (WebSocketConn, error) {
	return nil, e.err
}

package whatsapp

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/trilam/leah/internal/contracts"
	"github.com/trilam/leah/internal/obs"
	"github.com/trilam/leah/internal/obs/obstest"
)

// TestRegisterMetrics_AddsSeries asserts the shared leah_connect_* series surface bound to whatsapp.
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

// errHTTP returns a network error so the failure-still-observed case exercises the err branch.
type errHTTP struct{}

func (errHTTP) Do(*http.Request) (*http.Response, error) { return nil, errors.New("boom") }

// okHTTP returns a 200 echo so the success case avoids the http-error audit branch.
type okHTTP struct{}

func (okHTTP) Do(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader([]byte(`{}`))), Header: http.Header{}}, nil
}

// TestObserveAPI_OnRPC asserts SendText's HTTP RPC bumps api_call_total on success and failure.
func TestObserveAPI_OnRPC(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		http    contracts.HTTPClient
		wantErr bool
	}{
		{"send_text success", okHTTP{}, false},
		{"send_text failure still observed", errHTTP{}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := obs.NewRegistry()
			RegisterMetrics(r)
			a, err := New(Config{
				Attestor:           &fakeAttestor{},
				TokenSource:        &fakeTokenSource{tok: "bearer", phoneID: "PNID"},
				HTTP:               tc.http,
				RecipientAllowlist: []string{"+14155551234"},
				Metrics:            Metrics(r),
			})
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			err = a.SendText(context.Background(), "+14155551234", "hi")
			if tc.wantErr && err == nil {
				t.Fatal("SendText: expected transport error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("SendText: %v", err)
			}
			keys := obstest.SnapshotKeys(t, r)
			want := "leah_connect_api_call_total|endpoint=send_text,provider=whatsapp"
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
		Attestor:           &fakeAttestor{},
		TokenSource:        &fakeTokenSource{tok: "bearer", phoneID: "PNID"},
		HTTP:               okHTTP{},
		RecipientAllowlist: []string{"+14155551234"},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := a.SendText(context.Background(), "+14155551234", "hi"); err != nil {
		t.Fatalf("SendText: %v", err)
	}
}

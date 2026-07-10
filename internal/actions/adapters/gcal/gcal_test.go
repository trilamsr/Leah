package gcal

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/trilam/leah/internal/platform/contracts"
)

// fakeService is a hand-rolled stub of calendarService so tests do not need
// the google-api transport. Each method returns a canned result the table
// row pins; missing canned data is treated as a test-author bug, not a
// production code path.
type fakeService struct {
	listEvents []Event
	listErr    error
	createErr  error
	created    *Event
}

func (f *fakeService) ListToday(_ context.Context, _ string, _ time.Time) ([]Event, error) {
	return f.listEvents, f.listErr
}

func (f *fakeService) Create(_ context.Context, _ string, ev Event) (*Event, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	f.created = &ev
	return &ev, nil
}

// fakeAttestor records whether the attestation gate was invoked. The gcal
// adapter MUST route every token load through Attest() so the operator-
// attestation flow gates secret access at per-action grain.
type fakeAttestor struct {
	called int
	err    error
}

func (f *fakeAttestor) Attest(_ context.Context, scope string) error {
	f.called++
	if !strings.HasPrefix(scope, "gcal:") {
		return errors.New("unexpected scope: " + scope)
	}
	return f.err
}

// fakeTokenSource returns a canned token without touching disk.
type fakeTokenSource struct {
	tok    string
	err    error
	called int
}

func (f *fakeTokenSource) Token(_ context.Context) (string, error) {
	f.called++
	return f.tok, f.err
}

// failingTokenSource fails the test if Token() is invoked; proves the
// attestation gate runs BEFORE token materialization — a denied action
// must NOT leak the bearer into a logger / panic trace.
type failingTokenSource struct {
	t *testing.T
}

func (f *failingTokenSource) Token(_ context.Context) (string, error) {
	f.t.Fatal("TokenSource.Token() must NOT be called when attestation denies")
	return "", nil
}

// TestListTodayTable pins list-today contracts including the attest_denied
// gate-ordering invariant (no token materialization on deny).
func TestListTodayTable(t *testing.T) {
	now := time.Date(2026, 6, 9, 9, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		svc       *fakeService
		att       contracts.Attestor
		ts        contracts.TokenSource
		wantLen   int
		wantErrIs error
	}{
		{
			name:    "happy",
			svc:     &fakeService{listEvents: []Event{{ID: "e1", Summary: "standup"}}},
			att:     &fakeAttestor{},
			ts:      &fakeTokenSource{tok: "t"},
			wantLen: 1,
		},
		{
			name:      "auth_fail",
			svc:       &fakeService{listErr: ErrAuthRequired},
			att:       &fakeAttestor{},
			ts:        &fakeTokenSource{tok: "t"},
			wantErrIs: ErrAuthRequired,
		},
		{
			name:      "attest_denied",
			svc:       &fakeService{},
			att:       &fakeAttestor{err: errors.New("operator denied")},
			ts:        &failingTokenSource{t: t},
			wantErrIs: ErrAttestationDenied,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := &Adapter{svc: tc.svc, att: tc.att, ts: tc.ts, now: func() time.Time { return now }}
			got, err := a.ListToday(context.Background())
			if tc.wantErrIs != nil {
				if !errors.Is(err, tc.wantErrIs) {
					t.Fatalf("want errors.Is(%v), got %v", tc.wantErrIs, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if len(got) != tc.wantLen {
				t.Fatalf("want %d events, got %d", tc.wantLen, len(got))
			}
		})
	}
}

// TestCreateEventTable pins create contracts including the attest_denied
// gate-ordering invariant (no token materialization on deny).
func TestCreateEventTable(t *testing.T) {
	tests := []struct {
		name      string
		in        Event
		svc       *fakeService
		att       contracts.Attestor
		ts        contracts.TokenSource
		wantErrIs error
	}{
		{
			name: "happy",
			in:   Event{Summary: "1:1", Start: time.Now(), End: time.Now().Add(time.Hour)},
			svc:  &fakeService{},
			att:  &fakeAttestor{},
			ts:   &fakeTokenSource{tok: "t"},
		},
		{
			name:      "validation_400",
			in:        Event{Summary: ""},
			svc:       &fakeService{createErr: ErrInvalidEvent},
			att:       &fakeAttestor{},
			ts:        &fakeTokenSource{tok: "t"},
			wantErrIs: ErrInvalidEvent,
		},
		{
			name:      "attest_denied",
			in:        Event{Summary: "1:1"},
			svc:       &fakeService{},
			att:       &fakeAttestor{err: errors.New("operator denied")},
			ts:        &failingTokenSource{t: t},
			wantErrIs: ErrAttestationDenied,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := &Adapter{svc: tc.svc, att: tc.att, ts: tc.ts}
			got, err := a.CreateEvent(context.Background(), tc.in)
			if tc.wantErrIs != nil {
				if !errors.Is(err, tc.wantErrIs) {
					t.Fatalf("want errors.Is(%v), got %v", tc.wantErrIs, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got == nil || got.Summary != tc.in.Summary {
				t.Fatalf("want echo of %q, got %+v", tc.in.Summary, got)
			}
		})
	}
}

// TestNewRejectsEmptyTokenPath pins the constructor contract: a missing
// token path is a programmer error caught at startup, not at first API call.
func TestNewRejectsEmptyTokenPath(t *testing.T) {
	if _, err := New(Config{TokenPath: "x", Attestor: &fakeAttestor{}}); err != nil {
		t.Fatalf("baseline New: %v", err)
	}
	if _, err := New(Config{TokenPath: "", Attestor: &fakeAttestor{}}); err == nil {
		t.Fatal("want error on empty TokenPath, got nil")
	}
}

// TestNewRejectsMissingAttestor mirrors gmail.TestNewRejectsMissingDeps —
// a half-wired Adapter would silently bypass the operator-consent gate, so
// New refuses construction without an Attestor.
func TestNewRejectsMissingAttestor(t *testing.T) {
	if _, err := New(Config{TokenPath: "x"}); err == nil {
		t.Fatal("want error on missing Attestor, got nil")
	}
}

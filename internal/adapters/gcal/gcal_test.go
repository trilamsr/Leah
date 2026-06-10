package gcal

import (
	"context"
	"errors"
	"testing"
	"time"
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

func (f *fakeService) ListToday(_ context.Context, _ time.Time) ([]Event, error) {
	return f.listEvents, f.listErr
}

func (f *fakeService) Create(_ context.Context, ev Event) (*Event, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	f.created = &ev
	return &ev, nil
}

// TestListTodayTable pins the two list-today contracts: a happy-path row
// returns the canned events untouched, and an auth-fail row surfaces
// ErrAuthRequired so the dispatcher can re-prompt operator attestation.
func TestListTodayTable(t *testing.T) {
	now := time.Date(2026, 6, 9, 9, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		svc       *fakeService
		wantLen   int
		wantErrIs error
	}{
		{
			name:    "happy",
			svc:     &fakeService{listEvents: []Event{{ID: "e1", Summary: "standup"}}},
			wantLen: 1,
		},
		{
			name:      "auth_fail",
			svc:       &fakeService{listErr: ErrAuthRequired},
			wantErrIs: ErrAuthRequired,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := &Adapter{svc: tc.svc, now: func() time.Time { return now }}
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

// TestCreateEventTable pins the two create contracts: a happy-path row
// echoes the created event back, and a 400-validation row surfaces
// ErrInvalidEvent so the caller can show the validation message instead
// of retrying the same payload.
func TestCreateEventTable(t *testing.T) {
	tests := []struct {
		name      string
		in        Event
		svc       *fakeService
		wantErrIs error
	}{
		{
			name: "happy",
			in:   Event{Summary: "1:1", Start: time.Now(), End: time.Now().Add(time.Hour)},
			svc:  &fakeService{},
		},
		{
			name:      "validation_400",
			in:        Event{Summary: ""},
			svc:       &fakeService{createErr: ErrInvalidEvent},
			wantErrIs: ErrInvalidEvent,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := &Adapter{svc: tc.svc}
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
// token path is a programmer error caught at startup, not at first API
// call — operator should see the failure during daemon boot.
func TestNewRejectsEmptyTokenPath(t *testing.T) {
	if _, err := New(Config{TokenPath: ""}); err == nil {
		t.Fatal("want error on empty TokenPath, got nil")
	}
}

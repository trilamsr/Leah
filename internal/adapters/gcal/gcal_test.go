package gcal

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// fakeService stubs calendarService so tests do not pull in the
// google-api transport.
type fakeService struct {
	listEvents []Event
	listErr    error
	createID   string
	createErr  error
}

func (f *fakeService) ListToday(_ context.Context, _ time.Time) ([]Event, error) {
	return f.listEvents, f.listErr
}

func (f *fakeService) CreateEvent(_ context.Context, _ Event) (string, error) {
	return f.createID, f.createErr
}

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

type fakeTokenSource struct {
	tok    string
	err    error
	called int
}

func (f *fakeTokenSource) Token(_ context.Context) (string, error) {
	f.called++
	return f.tok, f.err
}

// failingTokenSource proves the attestation gate runs BEFORE token load —
// any Token() call on a denied path is a test failure (spec §5).
type failingTokenSource struct{ t *testing.T }

func (f *failingTokenSource) Token(_ context.Context) (string, error) {
	f.t.Fatal("TokenSource.Token() must NOT be called when attestation denies")
	return "", nil
}

// TestListTodayTable pins list-today contracts incl. attest-before-token.
func TestListTodayTable(t *testing.T) {
	now := time.Date(2026, 6, 9, 9, 0, 0, 0, time.UTC)
	t.Run("happy", func(t *testing.T) {
		att := &fakeAttestor{}
		ts := &fakeTokenSource{tok: "t"}
		c := &Client{
			cfg: Config{TokenPath: "x", Attestor: att, TokenSource: ts, CalendarID: "primary"},
			svc: &fakeService{listEvents: []Event{{Title: "standup"}}},
			now: func() time.Time { return now },
		}
		got, err := c.ListToday(context.Background())
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if len(got) != 1 || got[0].Title != "standup" {
			t.Fatalf("want 1 event titled standup, got %+v", got)
		}
		if att.called != 1 {
			t.Fatalf("want 1 Attest call, got %d", att.called)
		}
	})
	t.Run("attest_denied", func(t *testing.T) {
		att := &fakeAttestor{err: errors.New("operator denied")}
		c := &Client{
			cfg: Config{TokenPath: "x", Attestor: att, TokenSource: &failingTokenSource{t: t}, CalendarID: "primary"},
			svc: &fakeService{},
			now: func() time.Time { return now },
		}
		_, err := c.ListToday(context.Background())
		if !errors.Is(err, ErrAttestationDenied) {
			t.Fatalf("want ErrAttestationDenied, got %v", err)
		}
	})
	t.Run("auth_required", func(t *testing.T) {
		c := &Client{
			cfg: Config{TokenPath: "x", Attestor: &fakeAttestor{}, TokenSource: &fakeTokenSource{tok: "t"}, CalendarID: "primary"},
			svc: nil,
			now: func() time.Time { return now },
		}
		_, err := c.ListToday(context.Background())
		if !errors.Is(err, ErrAuthRequired) {
			t.Fatalf("want ErrAuthRequired, got %v", err)
		}
	})
}

// TestCreateEventTable pins create contracts incl. attest-before-token and
// caller-side invalid-event detection.
func TestCreateEventTable(t *testing.T) {
	valid := Event{Title: "1:1", Start: time.Now(), End: time.Now().Add(time.Hour)}
	t.Run("happy", func(t *testing.T) {
		c := &Client{
			cfg: Config{TokenPath: "x", Attestor: &fakeAttestor{}, TokenSource: &fakeTokenSource{tok: "t"}, CalendarID: "primary"},
			svc: &fakeService{createID: "evt-123"},
		}
		id, err := c.CreateEvent(context.Background(), valid)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if id != "evt-123" {
			t.Fatalf("want id evt-123, got %q", id)
		}
	})
	t.Run("attest_denied", func(t *testing.T) {
		c := &Client{
			cfg: Config{TokenPath: "x", Attestor: &fakeAttestor{err: errors.New("denied")}, TokenSource: &failingTokenSource{t: t}, CalendarID: "primary"},
			svc: &fakeService{},
		}
		_, err := c.CreateEvent(context.Background(), valid)
		if !errors.Is(err, ErrAttestationDenied) {
			t.Fatalf("want ErrAttestationDenied, got %v", err)
		}
	})
	t.Run("auth_required", func(t *testing.T) {
		c := &Client{
			cfg: Config{TokenPath: "x", Attestor: &fakeAttestor{}, TokenSource: &fakeTokenSource{tok: "t"}, CalendarID: "primary"},
			svc: nil,
		}
		_, err := c.CreateEvent(context.Background(), valid)
		if !errors.Is(err, ErrAuthRequired) {
			t.Fatalf("want ErrAuthRequired, got %v", err)
		}
	})
	t.Run("invalid_event", func(t *testing.T) {
		c := &Client{
			cfg: Config{TokenPath: "x", Attestor: &fakeAttestor{}, TokenSource: &fakeTokenSource{tok: "t"}, CalendarID: "primary"},
			svc: &fakeService{createID: "should-not-reach"},
		}
		_, err := c.CreateEvent(context.Background(), Event{Title: "", Start: time.Now(), End: time.Now().Add(time.Hour)})
		if !errors.Is(err, ErrInvalidEvent) {
			t.Fatalf("want ErrInvalidEvent, got %v", err)
		}
	})
}

// TestNewRejectsMissingDeps pins constructor preconditions — TokenPath and
// Attestor are load-bearing; missing either bypasses the operator-consent gate.
func TestNewRejectsMissingDeps(t *testing.T) {
	att := &fakeAttestor{}
	if _, err := New(Config{TokenPath: "", Attestor: att}); err == nil {
		t.Fatal("want error on empty TokenPath, got nil")
	}
	if _, err := New(Config{TokenPath: "x", Attestor: nil}); err == nil {
		t.Fatal("want error on nil Attestor, got nil")
	}
	if _, err := New(Config{TokenPath: "x", Attestor: att}); err != nil {
		t.Fatalf("baseline New: %v", err)
	}
}

// TestCalendarIDDefaultsToPrimary pins the default-calendar contract; an
// unset CalendarID must not silently target a wrong calendar (spec §5
// confused-deputy).
func TestCalendarIDDefaultsToPrimary(t *testing.T) {
	c, err := New(Config{TokenPath: "x", Attestor: &fakeAttestor{}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if c.cfg.CalendarID != "primary" {
		t.Fatalf("want CalendarID=primary, got %q", c.cfg.CalendarID)
	}
}

package gcal

import (
	"context"
	"errors"
	"time"
)

// ErrAuthRequired signals the operator-attestation gate has not produced
// a valid token at TokenPath. Callers should surface a re-attest prompt
// rather than retry — silent retry would hammer Google with bad creds and
// trip a rate-limit lockout.
var ErrAuthRequired = errors.New("gcal: auth required (token missing or revoked)")

// ErrInvalidEvent signals Google rejected the create payload with a 400.
// Caller should show the validation message; retrying the same body is
// guaranteed to fail.
var ErrInvalidEvent = errors.New("gcal: event payload rejected by API (400)")

// Event is the minimal calendar-event shape Leah's morning-brief flow
// consumes. Wider Google fields (attendees, recurrence, reminders) stay
// out of MVP — adding a field is cheaper than removing one once callers
// depend on it.
type Event struct {
	ID       string
	Summary  string
	Start    time.Time
	End      time.Time
	Location string
}

// Config is the constructor input. TokenPath is the only required field;
// production wiring sets the other knobs and tests leave them zero.
type Config struct {
	// TokenPath is the absolute path to the OAuth token JSON managed by
	// Leah's operator-attestation gate (see internal/dispatcher/selfbuild.go
	// + internal/audit). The adapter never writes this file — the
	// attestation flow owns refresh.
	TokenPath string

	// CalendarID is the target calendar; "primary" is the operator's
	// default calendar when empty.
	CalendarID string
}

// calendarService is the seam the adapter talks through so tests can
// inject a fake without dragging in the google-api transport. The real
// implementation lives in a later wiring wave (see spec §4).
type calendarService interface {
	ListToday(ctx context.Context, now time.Time) ([]Event, error)
	Create(ctx context.Context, ev Event) (*Event, error)
}

// Adapter is the gcal facade Leah's morning-brief + meeting-create flows
// depend on. Construct via New so the TokenPath precondition is checked
// at startup, not at first API call.
type Adapter struct {
	cfg Config
	svc calendarService
	now func() time.Time
}

// New constructs an Adapter and validates the TokenPath precondition.
// Returns a typed error so daemon boot can fail fast rather than
// surfacing a confusing nil-deref at first morning-brief tick.
func New(cfg Config) (*Adapter, error) {
	if cfg.TokenPath == "" {
		return nil, errors.New("gcal.New: Config.TokenPath required")
	}
	if cfg.CalendarID == "" {
		cfg.CalendarID = "primary"
	}
	return &Adapter{cfg: cfg, now: time.Now}, nil
}

// ListToday returns events whose start falls in the operator's local
// "today" window. The window is computed by the injected now() so tests
// pin a deterministic boundary.
func (a *Adapter) ListToday(ctx context.Context) ([]Event, error) {
	if a.svc == nil {
		return nil, ErrAuthRequired
	}
	return a.svc.ListToday(ctx, a.now())
}

// CreateEvent inserts an event into the configured calendar and returns
// the server's echo (ID + canonical timestamps). A 400 validation reply
// surfaces as ErrInvalidEvent so the caller can show the message instead
// of looping on the same payload.
func (a *Adapter) CreateEvent(ctx context.Context, ev Event) (*Event, error) {
	if a.svc == nil {
		return nil, ErrAuthRequired
	}
	return a.svc.Create(ctx, ev)
}

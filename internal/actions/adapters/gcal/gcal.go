package gcal

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/trilam/leah/internal/platform/contracts"
	"github.com/trilam/leah/internal/platform/telemetry/connectadapter"
)

// Sentinel errors are exported so callers can switch on the failure mode
// without string-matching wrapped messages.
var (
	// ErrAuthRequired signals the operator-attestation gate has not produced
	// a valid token at TokenPath. Callers should surface a re-attest prompt
	// rather than retry — silent retry would hammer Google with bad creds and
	// trip a rate-limit lockout.
	ErrAuthRequired = errors.New("gcal: auth required (token missing or revoked)")

	// ErrInvalidEvent signals Google rejected the create payload with a 400.
	// Caller should show the validation message; retrying the same body is
	// guaranteed to fail.
	ErrInvalidEvent = errors.New("gcal: event payload rejected by API (400)")

	// ErrAttestationDenied wraps the Attestor's rejection so callers can
	// distinguish operator-denied actions from auth-missing / API failures.
	ErrAttestationDenied = errors.New("gcal: attestation denied")
)

// Scopes the Attestor sees. Distinct per RPC so the operator-attestation log
// can attribute consent at the per-action grain (read vs. mutate).
const (
	ScopeListToday   = "gcal:list_today"
	ScopeCreateEvent = "gcal:create_event"
)

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

// Config is the constructor input. TokenPath + Attestor are required;
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

	// Attestor is the operator-consent gate; missing → New refuses
	// construction because a nil gate would silently bypass attestation.
	Attestor contracts.Attestor

	// TokenSource yields the (auto-refreshing) bearer. When set, New builds
	// the production HTTP calendarService; when nil the adapter stays a
	// gate-only stub (tests inject svc directly).
	TokenSource contracts.TokenSource

	// BaseURL overrides the Google Calendar host for tests (httptest). Empty
	// → the production v3 endpoint.
	BaseURL string

	// Metrics is optional — nil is a no-op (connectadapter contract), so
	// existing callers keep working without a registry.
	Metrics *connectadapter.Metrics
}

// calendarService is the seam the adapter talks through so tests can inject a
// fake without dragging in the google-api SDK. token is passed per-call so the
// service stays stateless (mirrors gmail.Transport).
type calendarService interface {
	ListToday(ctx context.Context, token string, now time.Time) ([]Event, error)
	Create(ctx context.Context, token string, ev Event) (*Event, error)
}

// Adapter is the gcal facade Leah's morning-brief + meeting-create flows
// depend on. Construct via New so the TokenPath + Attestor preconditions
// are checked at startup, not at first API call.
type Adapter struct {
	cfg Config
	svc calendarService
	att contracts.Attestor
	ts  contracts.TokenSource
	now func() time.Time
	m   *connectadapter.Metrics
}

// New constructs an Adapter and validates the TokenPath + Attestor
// preconditions. Returns a typed error so daemon boot can fail fast rather
// than surfacing a confusing nil-deref at first morning-brief tick.
func New(cfg Config) (*Adapter, error) {
	if cfg.TokenPath == "" {
		return nil, errors.New("gcal.New: Config.TokenPath required")
	}
	if cfg.Attestor == nil {
		return nil, errors.New("gcal.New: Config.Attestor required (operator-attestation gate)")
	}
	if cfg.CalendarID == "" {
		cfg.CalendarID = "primary"
	}
	a := &Adapter{cfg: cfg, att: cfg.Attestor, ts: cfg.TokenSource, now: time.Now, m: cfg.Metrics}
	if cfg.TokenSource != nil {
		a.svc = newHTTPService(nil, cfg.BaseURL, cfg.CalendarID)
	}
	return a, nil
}

// ListToday returns events whose start falls in the operator's local
// "today" window. Attestation runs BEFORE any token load — a denied
// action must not materialize the bearer.
func (a *Adapter) ListToday(ctx context.Context) ([]Event, error) {
	tok, err := a.gateAndToken(ctx, ScopeListToday)
	if err != nil {
		return nil, err
	}
	if a.svc == nil {
		return nil, ErrAuthRequired
	}
	start := time.Now()
	evs, err := a.svc.ListToday(ctx, tok, a.now())
	a.m.ObserveAPI("list_today", time.Since(start).Seconds())
	return evs, err
}

// CreateEvent inserts an event into the configured calendar and returns
// the server's echo (ID + canonical timestamps). A 400 validation reply
// surfaces as ErrInvalidEvent so the caller can show the message instead
// of looping on the same payload.
func (a *Adapter) CreateEvent(ctx context.Context, ev Event) (*Event, error) {
	tok, err := a.gateAndToken(ctx, ScopeCreateEvent)
	if err != nil {
		return nil, err
	}
	if a.svc == nil {
		return nil, ErrAuthRequired
	}
	start := time.Now()
	out, err := a.svc.Create(ctx, tok, ev)
	a.m.ObserveAPI("create_event", time.Since(start).Seconds())
	return out, err
}

// gateAndToken runs the attestation gate first; only on consent does the
// token leave the TokenSource. Ordering matters — token-load before
// attestation would leak the secret into a logger / panic trace even when
// the operator denies the action. Nil TokenSource is tolerated for tests —
// production wiring fails earlier at New.
func (a *Adapter) gateAndToken(ctx context.Context, scope string) (string, error) {
	if err := a.att.Attest(ctx, scope); err != nil {
		return "", fmt.Errorf("%w: %v", ErrAttestationDenied, err)
	}
	if a.ts == nil {
		return "", nil
	}
	tok, err := a.ts.Token(ctx)
	if err != nil {
		return "", fmt.Errorf("gcal: token load (%s): %w", scope, err)
	}
	return tok, nil
}

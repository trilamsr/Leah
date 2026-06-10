package gcal

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Sentinel errors are exported so callers can switch on the failure mode
// without string-matching wrapped messages.
var (
	// ErrAttestationDenied wraps the Attestor's rejection so callers can
	// distinguish operator-denied actions from auth-missing / API failures.
	ErrAttestationDenied = errors.New("gcal: attestation denied")

	// ErrAuthRequired signals the operator-attestation gate has not yet
	// produced a usable calendarService. Surface a re-attest prompt — silent
	// retry would hammer Google's auth endpoint and trip rate-limit lockout.
	ErrAuthRequired = errors.New("gcal: auth required (token missing or revoked)")

	// ErrInvalidEvent guards the caller from a 400-validation loop: an empty
	// Title (and any future invariant the API rejects deterministically) is
	// caught before the round trip.
	ErrInvalidEvent = errors.New("gcal: invalid event payload")
)

// Scopes are per-action so the audit log attributes consent at the grain
// the operator actually sees (read vs. mutate).
const (
	ScopeListToday   = "gcal:list_today"
	ScopeCreateEvent = "gcal:create_event"
)

// Event is the minimal calendar shape Leah's morning-brief and meeting-create
// flows consume. Recurrence / attendees / reminders stay deferred until a
// real caller files an issue citing the gap (spec §1, §6).
type Event struct {
	Title string
	Start time.Time
	End   time.Time
}

// Attestor is Leah's operator-consent gate. Every secret-touching RPC calls
// Attest(scope) BEFORE the bearer leaves TokenSource — a denied action must
// not materialize the token (spec §5).
type Attestor interface {
	Attest(ctx context.Context, scope string) error
}

// TokenSource yields the OAuth bearer. Production wiring reads the path
// managed by the attestation flow; tests inject a fake to stay hermetic.
type TokenSource interface {
	Token(ctx context.Context) (string, error)
}

// Config is the New() input. TokenPath + Attestor are required; CalendarID
// defaults to "primary"; TokenSource is wired by the daemon-boot follow-up.
type Config struct {
	TokenPath   string
	CalendarID  string
	Attestor    Attestor
	TokenSource TokenSource
}

// calendarService is the seam the Client talks through so tests skip the
// google-api transport. The real implementation lands with the W9 wiring
// wave (spec §4).
type calendarService interface {
	ListToday(ctx context.Context, now time.Time) ([]Event, error)
	CreateEvent(ctx context.Context, ev Event) (string, error)
}

// Client is the gcal facade. Construct via New so TokenPath + Attestor
// preconditions fail at startup, not at first morning-brief tick.
type Client struct {
	cfg Config
	svc calendarService
	now func() time.Time
}

// New constructs a Client and validates the load-bearing preconditions.
func New(cfg Config) (*Client, error) {
	if cfg.TokenPath == "" {
		return nil, errors.New("gcal.New: Config.TokenPath required")
	}
	if cfg.Attestor == nil {
		return nil, errors.New("gcal.New: Config.Attestor required (operator-consent gate)")
	}
	if cfg.CalendarID == "" {
		cfg.CalendarID = "primary"
	}
	return &Client{cfg: cfg, now: time.Now}, nil
}

// ListToday returns events whose start falls in the operator's local today
// window. Attestation runs BEFORE any token load (spec §5).
func (c *Client) ListToday(ctx context.Context) ([]Event, error) {
	if _, err := c.gateAndToken(ctx, ScopeListToday); err != nil {
		return nil, err
	}
	if c.svc == nil {
		return nil, ErrAuthRequired
	}
	return c.svc.ListToday(ctx, c.now())
}

// CreateEvent inserts an event and returns the server-assigned id. Empty
// Title is rejected client-side to avoid a guaranteed 400 round trip.
func (c *Client) CreateEvent(ctx context.Context, ev Event) (string, error) {
	if ev.Title == "" {
		return "", fmt.Errorf("%w: Title required", ErrInvalidEvent)
	}
	if _, err := c.gateAndToken(ctx, ScopeCreateEvent); err != nil {
		return "", err
	}
	if c.svc == nil {
		return "", ErrAuthRequired
	}
	return c.svc.CreateEvent(ctx, ev)
}

// gateAndToken runs the attestation gate first; only on consent does the
// token leave TokenSource. Reversing the order would leak the bearer into a
// logger / panic trace even when the operator denies the action (spec §5).
// Nil TokenSource is tolerated until the wiring wave injects one — production
// wiring fails earlier at New.
func (c *Client) gateAndToken(ctx context.Context, scope string) (string, error) {
	if err := c.cfg.Attestor.Attest(ctx, scope); err != nil {
		return "", fmt.Errorf("%w: %v", ErrAttestationDenied, err)
	}
	if c.cfg.TokenSource == nil {
		return "", nil
	}
	tok, err := c.cfg.TokenSource.Token(ctx)
	if err != nil {
		return "", fmt.Errorf("gcal: token load (%s): %w", scope, err)
	}
	return tok, nil
}

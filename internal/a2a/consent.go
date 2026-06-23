package a2a

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ConsentScope mirrors the §5.7 prompt buttons: [Once] [This session] [Always].
// Persisted in a2a_consent.scope; the CHECK constraint is the upstream source.
type ConsentScope string

const (
	ScopeOnce    ConsentScope = "once"
	ScopeSession ConsentScope = "session"
	ScopeAlways  ConsentScope = "always"
)

// ConsentKind names the action being consented to. §5.7 today gates
// "leah.ask" (token-spending) explicitly; memory.search + task.offer follow
// the same gate.
type ConsentKind string

const (
	ConsentAsk    ConsentKind = "leah.ask"
	ConsentMemory ConsentKind = "leah.memory.search"
	ConsentTask   ConsentKind = "leah.task"
)

// ErrConsentNotGranted is returned when no live a2a_consent row exists for
// the (peer, kind) tuple — caller must surface the §5.7 HUD prompt.
var ErrConsentNotGranted = errors.New("a2a: consent not granted")

// ConsentStore persists operator decisions per (peer, kind). Backed by the
// a2a_consent table shipped by T05's 2026-06-22-007-a2a.sql migration.
type ConsentStore struct {
	DB  *sql.DB
	Now func() time.Time
}

// NewConsentStore wraps db.
func NewConsentStore(db *sql.DB) *ConsentStore {
	return &ConsentStore{DB: db, Now: time.Now}
}

// Grant records the operator's decision. Per-peer-per-kind UNIQUE upstream —
// re-granting upserts. ScopeOnce sets expires_at = granted_at so the next
// Check rejects it.
func (s *ConsentStore) Grant(ctx context.Context, peer PeerID, kind ConsentKind, scope ConsentScope) error {
	now := s.now().Unix()
	var expires sql.NullInt64
	switch scope {
	case ScopeOnce:
		// expires_at NULL — Check consumes the row on first hit instead of
		// relying on a time gate.
	case ScopeSession:
		// Session lifetime = 24 hours — daemon restart drops it via Sweep.
		expires = sql.NullInt64{Int64: now + 24*3600, Valid: true}
	case ScopeAlways:
		// expires_at NULL — survives until Revoke.
	default:
		return fmt.Errorf("a2a: bad consent scope %q", scope)
	}
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO a2a_consent (peer_id, kind, scope, granted_at, expires_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(peer_id, kind) DO UPDATE SET
			scope = excluded.scope,
			granted_at = excluded.granted_at,
			expires_at = excluded.expires_at
	`, string(peer), string(kind), string(scope), now, expires)
	if err != nil {
		return fmt.Errorf("a2a: grant consent: %w", err)
	}
	return nil
}

// Deny clears any standing consent. Equivalent to "no record present" on the
// next Check call — distinct row not needed; §5.7's HUD path covers it.
func (s *ConsentStore) Deny(ctx context.Context, peer PeerID, kind ConsentKind) error {
	_, err := s.DB.ExecContext(ctx,
		`DELETE FROM a2a_consent WHERE peer_id = ? AND kind = ?`,
		string(peer), string(kind))
	return err
}

// Check returns nil when (peer, kind) has a live consent row. ScopeOnce
// consents are atomically consumed inside a transaction — concurrent Checks
// against the same once-row see exactly one success and one ErrConsentNotGranted.
func (s *ConsentStore) Check(ctx context.Context, peer PeerID, kind ConsentKind) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("a2a: consent tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	row := tx.QueryRowContext(ctx, `
		SELECT id, scope, expires_at FROM a2a_consent
		WHERE peer_id = ? AND kind = ?
	`, string(peer), string(kind))
	var id int64
	var scope string
	var expires sql.NullInt64
	if err := row.Scan(&id, &scope, &expires); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrConsentNotGranted
		}
		return err
	}
	now := s.now().Unix()
	if expires.Valid && now > expires.Int64 {
		if _, err := tx.ExecContext(ctx, `DELETE FROM a2a_consent WHERE id = ?`, id); err != nil {
			return fmt.Errorf("a2a: gc stale-consent: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("a2a: commit gc: %w", err)
		}
		return ErrConsentNotGranted
	}
	if ConsentScope(scope) == ScopeOnce {
		// Atomic one-shot: DELETE WHERE scope='once' so a concurrent racer
		// either wins (rows=1, commit succeeds) or loses (rows=0, treat as
		// not-granted). The scope check makes the predicate idempotent.
		res, err := tx.ExecContext(ctx,
			`DELETE FROM a2a_consent WHERE id = ? AND scope = 'once'`, id)
		if err != nil {
			return fmt.Errorf("a2a: consume once-consent: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("a2a: consume once-consent rows: %w", err)
		}
		if n == 0 {
			return ErrConsentNotGranted
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("a2a: consent commit: %w", err)
	}
	return nil
}

func (s *ConsentStore) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

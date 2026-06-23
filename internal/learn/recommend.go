package learn

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

type Recommender interface {
	Observe(ctx context.Context, ev Observation) error
	NextBatch(ctx context.Context, surface Surface, maxN int) ([]Recommendation, error)
	Record(ctx context.Context, id int64, out Outcome) error
}

type recommender struct {
	db    *sql.DB
	store *store
	now   func() time.Time
}

func New(db *sql.DB) *recommender {
	return &recommender{db: db, store: newStore(db), now: time.Now}
}

func (r *recommender) Observe(ctx context.Context, ev Observation) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO learn_observation(at, kind, ctx_hash) VALUES(?,?,?)`,
		ev.Ts.Unix(), string(ev.Kind), int64(ev.CtxHash))
	return err
}

// NextBatch — pacing caps gate before ranking so a flood-day never reorders the floor (§3.4).
func (r *recommender) NextBatch(ctx context.Context, surface Surface, maxN int) ([]Recommendation, error) {
	_ = surface // reserved for per-surface routing
	now := r.now()
	dayCount, err := r.store.surfacedSince(ctx, now.Add(-24*time.Hour))
	if err != nil {
		return nil, err
	}
	if dayCount >= PacingPerDay {
		return nil, nil
	}
	hourCount, err := r.store.surfacedSince(ctx, now.Add(-time.Hour))
	if err != nil {
		return nil, err
	}
	if hourCount >= PacingPerHour {
		return nil, nil
	}
	return r.store.topQueued(ctx, now, maxN)
}

func (r *recommender) Record(ctx context.Context, id int64, out Outcome) error {
	state := "ignored"
	switch out.Kind {
	case Accepted:
		state = "accepted"
	case Dismissed:
		state = "dismissed"
	case Applied:
		state = "applied"
	}
	_, err := r.db.ExecContext(ctx,
		`UPDATE learn_recommendation SET state=? WHERE id=?`, state, id)
	return err
}

// insertCandidate — *testing.T gate keeps this off prod call sites.
func (r *recommender) insertCandidate(t *testing.T, rec Recommendation) {
	t.Helper()
	if _, err := r.db.Exec(
		`INSERT INTO learn_recommendation(kind, body, action_ref, score, confidence, decay_id, expires_at, state)
		 VALUES(?,?,?,?,?,1,?,'queued')`,
		string(rec.Kind), "test", "noop", rec.Score, rec.Confidence,
		time.Now().Add(time.Hour).Unix()); err != nil {
		t.Fatalf("insertCandidate: %v", err)
	}
}

func (r *recommender) insertSurfaced(t *testing.T, at time.Time) {
	t.Helper()
	if _, err := r.db.Exec(
		`INSERT INTO learn_recommendation(kind, body, action_ref, score, confidence, decay_id, expires_at, state, surfaced_at)
		 VALUES('pin-widget','t','n',0.9,0.9,1,?, 'surfaced', ?)`,
		time.Now().Add(time.Hour).Unix(), at.Unix()); err != nil {
		t.Fatalf("insertSurfaced: %v", err)
	}
}

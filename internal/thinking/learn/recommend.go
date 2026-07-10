package learn

import (
	"context"
	"database/sql"
	"time"
)

type Recommender interface {
	Observe(ctx context.Context, ev Observation) error
	NextBatch(ctx context.Context, surface Surface, maxN int) ([]Recommendation, error)
	Record(ctx context.Context, id RecommendationID, out Outcome) error
	AntiAdd(ctx context.Context, kind RecommendKind, reason string) error
	AntiList(ctx context.Context) ([]AntiRule, error)
}

type recommender struct {
	db  *sql.DB
	now func() time.Time
}

func New(db *sql.DB) *recommender {
	return &recommender{db: db, now: time.Now}
}

func (r *recommender) Observe(ctx context.Context, ev Observation) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO learn_observation(at, kind, ctx_hash) VALUES(?,?,?)`,
		ev.Ts.Unix(), string(ev.Kind), int64(ev.CtxHash))
	return err
}

// NextBatch — pacing caps gate before ranking so a flood-day never reorders the floor (§3.4).
// Returned rows are atomically transitioned queued→surfaced inside one tx so the next
// caller sees them in the pacing window, not as still-queued candidates.
func (r *recommender) NextBatch(ctx context.Context, surface Surface, maxN int) ([]Recommendation, error) {
	_ = surface // reserved for per-surface routing
	now := r.now()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	dayCount, err := txSurfacedSince(ctx, tx, now.Add(-24*time.Hour))
	if err != nil {
		return nil, err
	}
	if dayCount >= PacingPerDay {
		return nil, nil
	}
	hourCount, err := txSurfacedSince(ctx, tx, now.Add(-time.Hour))
	if err != nil {
		return nil, err
	}
	if hourCount >= PacingPerHour {
		return nil, nil
	}
	rows, err := txTopQueued(ctx, tx, now, maxN)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, tx.Commit()
	}
	for i := range rows {
		if _, err := tx.ExecContext(ctx,
			`UPDATE learn_recommendation SET state='surfaced', surfaced_at=? WHERE id=?`,
			now.Unix(), int64(rows[i].ID)); err != nil {
			return nil, err
		}
		rows[i].SurfacedAt = now
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *recommender) Record(ctx context.Context, id RecommendationID, out Outcome) error {
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
		`UPDATE learn_recommendation SET state=? WHERE id=?`, state, int64(id))
	return err
}

// AntiAdd registers an operator-source anti-rule (spec/auto sources route through antilist.go).
func (r *recommender) AntiAdd(ctx context.Context, kind RecommendKind, reason string) error {
	return newAntiList(r.db, r.now).Add(ctx, kind, reason, AntiOperator)
}

func (r *recommender) AntiList(ctx context.Context) ([]AntiRule, error) {
	return newAntiList(r.db, r.now).List(ctx)
}

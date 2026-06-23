package learn

import (
	"context"
	"database/sql"
	"time"
)

// dbq abstracts *sql.DB and *sql.Tx so the pacing read + surfaced UPDATE
// can run in one tx without duplicating the query shape.
type dbq interface {
	QueryRowContext(ctx context.Context, q string, args ...any) *sql.Row
	QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error)
}

func txSurfacedSince(ctx context.Context, q dbq, since time.Time) (int, error) {
	var n int
	err := q.QueryRowContext(ctx,
		`SELECT count(*) FROM learn_recommendation
		 WHERE surfaced_at >= ?
		 AND state IN ('surfaced','accepted','dismissed','ignored','applied')`,
		since.Unix()).Scan(&n)
	return n, err
}

func txTopQueued(ctx context.Context, q dbq, now time.Time, n int) ([]Recommendation, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT id, kind, body, score, confidence, action_ref, expires_at
		 FROM learn_recommendation
		 WHERE state='queued' AND confidence >= ? AND expires_at > ?
		 ORDER BY score DESC LIMIT ?`,
		ConfidenceFloor, now.Unix(), n)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Recommendation
	for rows.Next() {
		var r Recommendation
		var exp int64
		if err := rows.Scan(&r.ID, &r.Kind, &r.Body, &r.Score, &r.Confidence, &r.ActionRef, &exp); err != nil {
			return nil, err
		}
		r.ExpiresAt = time.Unix(exp, 0)
		out = append(out, r)
	}
	return out, rows.Err()
}

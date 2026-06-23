package learn

import (
	"context"
	"database/sql"
	"time"
)

type store struct{ db *sql.DB }

func newStore(db *sql.DB) *store { return &store{db: db} }

// surfacedSince — 'queued' excluded so only surfaced/terminal rows burn pacing.
func (s *store) surfacedSince(ctx context.Context, since time.Time) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM learn_recommendation
		 WHERE surfaced_at >= ?
		 AND state IN ('surfaced','accepted','dismissed','ignored','applied')`,
		since.Unix()).Scan(&n)
	return n, err
}

func (s *store) topQueued(ctx context.Context, now time.Time, n int) ([]Recommendation, error) {
	rows, err := s.db.QueryContext(ctx,
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

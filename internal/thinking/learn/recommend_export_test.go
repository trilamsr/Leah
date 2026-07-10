package learn

import (
	"testing"
	"time"
)

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

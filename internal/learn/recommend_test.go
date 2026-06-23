package learn

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

const testSchema = `
CREATE TABLE learn_observation (
    id          INTEGER PRIMARY KEY,
    at          INTEGER NOT NULL,
    kind        TEXT NOT NULL,
    payload     BLOB,
    ctx_hash    BLOB
);
CREATE TABLE learn_decay (
    id            INTEGER PRIMARY KEY,
    kind          TEXT NOT NULL,
    half_life_s   INTEGER NOT NULL,
    hard_expire_s INTEGER NOT NULL
);
CREATE TABLE learn_recommendation (
    id          INTEGER PRIMARY KEY,
    kind        TEXT NOT NULL,
    body        TEXT NOT NULL,
    action_ref  TEXT NOT NULL,
    score       REAL NOT NULL,
    confidence  REAL NOT NULL,
    decay_id    INTEGER NOT NULL REFERENCES learn_decay(id),
    surfaced_at INTEGER,
    expires_at  INTEGER NOT NULL,
    state       TEXT NOT NULL
);
INSERT INTO learn_decay(id, kind, half_life_s, hard_expire_s) VALUES(1, 'pin-widget', 604800, 1814400);
`

func newTestDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "learn.db")
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := db.Exec(testSchema); err != nil {
		_ = db.Close()
		t.Fatalf("schema: %v", err)
	}
	return db, func() { _ = db.Close() }
}

func TestNextBatch_FiltersBelowFloor(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()
	r := New(db)
	r.insertCandidate(t, Recommendation{Kind: "pin-widget", Confidence: 0.1, Score: 0.9})
	r.insertCandidate(t, Recommendation{Kind: "pin-widget", Confidence: 0.8, Score: 0.5})
	batch, err := r.NextBatch(context.Background(), SurfaceNotification, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch) != 1 {
		t.Fatalf("want 1 above-floor candidate, got %d", len(batch))
	}
}

func TestNextBatch_PacingCapsAt3PerDay(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()
	r := New(db)
	for i := 0; i < 5; i++ {
		r.insertSurfaced(t, time.Now().Add(-time.Duration(i)*time.Hour))
	}
	batch, err := r.NextBatch(context.Background(), SurfaceNotification, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch) != 0 {
		t.Fatalf("pacing cap should have blocked all, got %d", len(batch))
	}
}

func TestNextBatch_PacingCapsAt1PerHour(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()
	r := New(db)
	r.insertSurfaced(t, time.Now().Add(-10*time.Minute))
	r.insertCandidate(t, Recommendation{Kind: "pin-widget", Confidence: 0.9, Score: 0.9})
	batch, err := r.NextBatch(context.Background(), SurfaceNotification, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch) != 0 {
		t.Fatalf("hourly pacing cap should block, got %d", len(batch))
	}
}

func TestObserve_WritesRow(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()
	r := New(db)
	if err := r.Observe(context.Background(), Observation{Kind: "pin-widget", CtxHash: 42, Ts: time.Now()}); err != nil {
		t.Fatalf("observe: %v", err)
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM learn_observation`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("want 1 observation row, got %d", n)
	}
}

func TestNextBatch_TransitionsQueuedToSurfaced(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()
	r := New(db)
	r.insertCandidate(t, Recommendation{Kind: "pin-widget", Confidence: 0.9, Score: 0.9})
	batch, err := r.NextBatch(context.Background(), SurfaceNotification, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch) != 1 {
		t.Fatalf("want 1 returned row, got %d", len(batch))
	}
	var state string
	var surfacedAt sql.NullInt64
	if err := db.QueryRow(`SELECT state, surfaced_at FROM learn_recommendation WHERE id=?`, int64(batch[0].ID)).Scan(&state, &surfacedAt); err != nil {
		t.Fatal(err)
	}
	if state != "surfaced" {
		t.Fatalf("post-NextBatch state: want 'surfaced', got %q", state)
	}
	if !surfacedAt.Valid {
		t.Fatalf("surfaced_at must be set after NextBatch returns the row")
	}
}

func TestNextBatch_SecondCallHitsPacingCap(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()
	r := New(db)
	r.insertCandidate(t, Recommendation{Kind: "pin-widget", Confidence: 0.9, Score: 0.9})
	r.insertCandidate(t, Recommendation{Kind: "pin-widget", Confidence: 0.9, Score: 0.8})
	first, err := r.NextBatch(context.Background(), SurfaceNotification, 1)
	if err != nil || len(first) != 1 {
		t.Fatalf("first call: got %d rows, err=%v", len(first), err)
	}
	second, err := r.NextBatch(context.Background(), SurfaceNotification, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 0 {
		t.Fatalf("second call within the hour must burn pacing slot from the first; got %d rows", len(second))
	}
}

func TestRecord_TransitionsState(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()
	r := New(db)
	r.insertCandidate(t, Recommendation{Kind: "pin-widget", Confidence: 0.9, Score: 0.9})
	var id int64
	if err := db.QueryRow(`SELECT id FROM learn_recommendation LIMIT 1`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if err := r.Record(context.Background(), RecommendationID(id), Outcome{Kind: Accepted}); err != nil {
		t.Fatalf("record: %v", err)
	}
	var state string
	if err := db.QueryRow(`SELECT state FROM learn_recommendation WHERE id=?`, id).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "accepted" {
		t.Fatalf("want accepted, got %q", state)
	}
}

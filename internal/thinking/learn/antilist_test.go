package learn

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

const antiSchema = testSchema + `
CREATE TABLE anti_recommend (
    kind        TEXT NOT NULL,
    reason      TEXT NOT NULL,
    added_at    INTEGER NOT NULL,
    source      TEXT NOT NULL CHECK(source IN ('operator','auto','spec')),
    PRIMARY KEY(kind, source)
);
INSERT INTO anti_recommend(kind, reason, added_at, source) VALUES
  ('wake-word-on', 'spec §3.6: operator must opt in', 1, 'spec');
`

func newAntiDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()
	dir := t.TempDir()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dir, "anti.db")+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := db.Exec(antiSchema); err != nil {
		_ = db.Close()
		t.Fatalf("schema: %v", err)
	}
	return db, func() { _ = db.Close() }
}

func TestAntiList_SpecCannotBeRemovedByOperator(t *testing.T) {
	db, cleanup := newAntiDB(t)
	defer cleanup()
	a := newAntiList(db, time.Now)
	err := a.Remove(context.Background(), "wake-word-on", AntiOperator)
	if !errors.Is(err, ErrSpecLocked) {
		t.Fatalf("want ErrSpecLocked removing spec row via operator; got %v", err)
	}
	blocked, err := a.IsBlocked(context.Background(), "wake-word-on")
	if err != nil {
		t.Fatal(err)
	}
	if !blocked {
		t.Fatal("spec row must still block after rejected removal")
	}
}

func TestAntiList_OperatorAddAndRemove(t *testing.T) {
	db, cleanup := newAntiDB(t)
	defer cleanup()
	a := newAntiList(db, time.Now)
	ctx := context.Background()
	if err := a.Add(ctx, "pin-widget", "noisy", AntiOperator); err != nil {
		t.Fatal(err)
	}
	blocked, err := a.IsBlocked(ctx, "pin-widget")
	if err != nil || !blocked {
		t.Fatalf("operator add should block; blocked=%v err=%v", blocked, err)
	}
	if err := a.Remove(ctx, "pin-widget", AntiOperator); err != nil {
		t.Fatal(err)
	}
	blocked, _ = a.IsBlocked(ctx, "pin-widget")
	if blocked {
		t.Fatal("post-remove must not block")
	}
}

func TestAntiList_AutoAddAfter3Dismissed(t *testing.T) {
	db, cleanup := newAntiDB(t)
	defer cleanup()
	frozen := time.Unix(1_700_000_000, 0)
	a := newAntiList(db, func() time.Time { return frozen })
	ctx := context.Background()

	for i := 0; i < AutoDismissThreshold; i++ {
		if _, err := db.Exec(
			`INSERT INTO learn_recommendation(kind, body, action_ref, score, confidence, decay_id, expires_at, state, surfaced_at)
			 VALUES('pin-widget','t','n',0.5,0.5,1,?,?, ?)`,
			frozen.Add(time.Hour).Unix(), "dismissed", frozen.Add(-time.Duration(i)*time.Hour).Unix()); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	if err := a.AutoSweep(ctx); err != nil {
		t.Fatalf("AutoSweep: %v", err)
	}
	blocked, err := a.IsBlocked(ctx, "pin-widget")
	if err != nil {
		t.Fatal(err)
	}
	if !blocked {
		t.Fatal("3 consecutive Dismissed in 30 d must auto-add to anti-list")
	}
	rules, err := a.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var hit AntiRule
	for _, r := range rules {
		if r.Kind == "pin-widget" {
			hit = r
		}
	}
	if hit.Source != AntiAuto {
		t.Fatalf("auto-added rule must carry AntiAuto source; got %q", hit.Source)
	}
}

func TestAntiList_AutoIgnoresOldDismissed(t *testing.T) {
	db, cleanup := newAntiDB(t)
	defer cleanup()
	frozen := time.Unix(1_700_000_000, 0)
	a := newAntiList(db, func() time.Time { return frozen })
	ctx := context.Background()

	// All Dismissed sit outside the 30 d window — must NOT trigger an auto-add.
	for i := 0; i < AutoDismissThreshold; i++ {
		if _, err := db.Exec(
			`INSERT INTO learn_recommendation(kind, body, action_ref, score, confidence, decay_id, expires_at, state, surfaced_at)
			 VALUES('pin-widget','t','n',0.5,0.5,1,?,?, ?)`,
			frozen.Add(time.Hour).Unix(), "dismissed", frozen.Add(-31*24*time.Hour-time.Duration(i)*time.Hour).Unix()); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	if err := a.AutoSweep(ctx); err != nil {
		t.Fatal(err)
	}
	blocked, _ := a.IsBlocked(ctx, "pin-widget")
	if blocked {
		t.Fatal("Dismissed outside the 30 d window must not auto-add")
	}
}

func TestAntiList_OperatorRemovesAutoRule(t *testing.T) {
	db, cleanup := newAntiDB(t)
	defer cleanup()
	a := newAntiList(db, time.Now)
	ctx := context.Background()
	if err := a.Add(ctx, "pin-widget", "auto", AntiAuto); err != nil {
		t.Fatal(err)
	}
	if err := a.Remove(ctx, "pin-widget", AntiAuto); err != nil {
		t.Fatalf("operator-routed Remove(AntiAuto) must drop the auto row; got %v", err)
	}
	blocked, _ := a.IsBlocked(ctx, "pin-widget")
	if blocked {
		t.Fatal("post-Remove must not block")
	}
}

func TestAntiList_AutoSweepRePromotesAfterRemove(t *testing.T) {
	db, cleanup := newAntiDB(t)
	defer cleanup()
	frozen := time.Unix(1_700_000_000, 0)
	a := newAntiList(db, func() time.Time { return frozen })
	ctx := context.Background()

	for i := 0; i < AutoDismissThreshold; i++ {
		if _, err := db.Exec(
			`INSERT INTO learn_recommendation(kind, body, action_ref, score, confidence, decay_id, expires_at, state, surfaced_at)
			 VALUES('pin-widget','t','n',0.5,0.5,1,?,?, ?)`,
			frozen.Add(time.Hour).Unix(), "dismissed", frozen.Add(-time.Duration(i)*time.Hour).Unix()); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	if err := a.AutoSweep(ctx); err != nil {
		t.Fatal(err)
	}
	if err := a.Remove(ctx, "pin-widget", AntiAuto); err != nil {
		t.Fatal(err)
	}
	// Dismissals still in the rolling window — next sweep must re-promote.
	if err := a.AutoSweep(ctx); err != nil {
		t.Fatal(err)
	}
	blocked, _ := a.IsBlocked(ctx, "pin-widget")
	if !blocked {
		t.Fatal("AutoSweep must re-promote a kind whose Dismissed window remains hot after manual removal")
	}
}

func TestAntiList_AutoSweepIsIdempotent(t *testing.T) {
	db, cleanup := newAntiDB(t)
	defer cleanup()
	frozen := time.Unix(1_700_000_000, 0)
	a := newAntiList(db, func() time.Time { return frozen })
	ctx := context.Background()
	for i := 0; i < AutoDismissThreshold; i++ {
		if _, err := db.Exec(
			`INSERT INTO learn_recommendation(kind, body, action_ref, score, confidence, decay_id, expires_at, state, surfaced_at)
			 VALUES('pin-widget','t','n',0.5,0.5,1,?,?, ?)`,
			frozen.Add(time.Hour).Unix(), "dismissed", frozen.Add(-time.Duration(i)*time.Hour).Unix()); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	for i := 0; i < 3; i++ {
		if err := a.AutoSweep(ctx); err != nil {
			t.Fatalf("AutoSweep iter %d: %v", i, err)
		}
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM anti_recommend WHERE kind='pin-widget' AND source='auto'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("AutoSweep must coalesce — want 1 auto row, got %d", n)
	}
}

package learn

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

const abSchema = `
CREATE TABLE learn_experiment (
    id          INTEGER PRIMARY KEY,
    kind        TEXT NOT NULL,
    arm_a       TEXT NOT NULL,
    arm_b       TEXT NOT NULL,
    impressions_a INTEGER NOT NULL DEFAULT 0,
    impressions_b INTEGER NOT NULL DEFAULT 0,
    wins_a      INTEGER NOT NULL DEFAULT 0,
    wins_b      INTEGER NOT NULL DEFAULT 0,
    locked      INTEGER NOT NULL DEFAULT 0,
    locked_arm  TEXT
);
`

func newABDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()
	dir := t.TempDir()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dir, "ab.db")+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := db.Exec(abSchema); err != nil {
		_ = db.Close()
		t.Fatalf("schema: %v", err)
	}
	return db, func() { _ = db.Close() }
}

func seedExp(t *testing.T, db *sql.DB, kind, a, b string) int64 {
	t.Helper()
	res, err := db.Exec(
		`INSERT INTO learn_experiment(kind, arm_a, arm_b) VALUES(?,?,?)`,
		kind, a, b)
	if err != nil {
		t.Fatalf("seedExp: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

func TestAB_AssignDeterministicByCoin(t *testing.T) {
	db, cleanup := newABDB(t)
	defer cleanup()
	id := seedExp(t, db, "pin-widget", "Pin the agenda?", "Want me to pin it?")
	headsThenTails := func() float64 {
		return 0.1 // < 0.5 always picks arm A
	}
	ab := newABKernel(db, headsThenTails)
	arm, expID, err := ab.Assign(context.Background(), "pin-widget")
	if err != nil {
		t.Fatal(err)
	}
	if expID != id {
		t.Fatalf("want exp %d, got %d", id, expID)
	}
	if arm != "Pin the agenda?" {
		t.Fatalf("coin<0.5 must pick arm A; got %q", arm)
	}
}

func TestAB_LockAfter50Impressions(t *testing.T) {
	db, cleanup := newABDB(t)
	defer cleanup()
	id := seedExp(t, db, "pin-widget", "A", "B")
	if _, err := db.Exec(
		`UPDATE learn_experiment SET impressions_a=60, impressions_b=55, wins_a=40, wins_b=20 WHERE id=?`,
		id); err != nil {
		t.Fatal(err)
	}
	ab := newABKernel(db, nil)
	if err := ab.Lock(context.Background(), id); err != nil {
		t.Fatalf("Lock: %v", err)
	}
	var locked int
	var arm string
	if err := db.QueryRow(`SELECT locked, locked_arm FROM learn_experiment WHERE id=?`, id).Scan(&locked, &arm); err != nil {
		t.Fatal(err)
	}
	if locked != 1 {
		t.Fatalf("must lock once both arms ≥ %d impressions and Wilson-LB differs", ABLockThreshold)
	}
	if arm != "A" {
		t.Fatalf("Wilson-LB winner is A (40/60 vs 20/55); got %q", arm)
	}
}

func TestAB_LockRejectsBelowThreshold(t *testing.T) {
	db, cleanup := newABDB(t)
	defer cleanup()
	id := seedExp(t, db, "pin-widget", "A", "B")
	if _, err := db.Exec(
		`UPDATE learn_experiment SET impressions_a=49, impressions_b=80, wins_a=40, wins_b=20 WHERE id=?`,
		id); err != nil {
		t.Fatal(err)
	}
	ab := newABKernel(db, nil)
	if err := ab.Lock(context.Background(), id); err == nil {
		t.Fatal("Lock must reject when either arm is below threshold")
	}
}

func TestAB_LockTieStaysOpen(t *testing.T) {
	db, cleanup := newABDB(t)
	defer cleanup()
	id := seedExp(t, db, "pin-widget", "A", "B")
	if _, err := db.Exec(
		`UPDATE learn_experiment SET impressions_a=50, impressions_b=50, wins_a=25, wins_b=25 WHERE id=?`,
		id); err != nil {
		t.Fatal(err)
	}
	ab := newABKernel(db, nil)
	if err := ab.Lock(context.Background(), id); err == nil {
		t.Fatal("ties must stay 50/50 (spec §3.7)")
	}
}

func TestAB_RecordIncrementsCorrectArm(t *testing.T) {
	db, cleanup := newABDB(t)
	defer cleanup()
	id := seedExp(t, db, "pin-widget", "A", "B")
	ab := newABKernel(db, nil)
	if err := ab.Record(context.Background(), id, "A", true); err != nil {
		t.Fatal(err)
	}
	if err := ab.Record(context.Background(), id, "A", false); err != nil {
		t.Fatal(err)
	}
	if err := ab.Record(context.Background(), id, "B", true); err != nil {
		t.Fatal(err)
	}
	var iA, iB, wA, wB int
	if err := db.QueryRow(
		`SELECT impressions_a, impressions_b, wins_a, wins_b FROM learn_experiment WHERE id=?`,
		id).Scan(&iA, &iB, &wA, &wB); err != nil {
		t.Fatal(err)
	}
	if iA != 2 || iB != 1 || wA != 1 || wB != 1 {
		t.Fatalf("counters wrong: iA=%d iB=%d wA=%d wB=%d", iA, iB, wA, wB)
	}
}

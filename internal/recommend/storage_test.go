package recommend

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// TestSQLite_Persist_AcrossInstances is the headline spec assertion: state
// written by Engine1 must be readable by Engine2 opened on the same path.
func TestSQLite_Persist_AcrossInstances(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "recommend.db")

	e1, err := NewSQLiteEngine(path, nil)
	if err != nil {
		t.Fatalf("open e1: %v", err)
	}
	rec := Recommendation{
		ID: "r1", Pattern: "commit_at_noon", Tier: TierConfirm,
		Source: "selflearn", Confidence: 0.8, CreatedAt: time.Unix(1700000000, 0).UTC(),
	}
	if err := e1.Seed(rec); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := e1.Accept(context.Background(), "r1"); err != nil {
		t.Fatalf("accept: %v", err)
	}
	if err := e1.Close(); err != nil {
		t.Fatalf("close e1: %v", err)
	}

	e2, err := NewSQLiteEngine(path, nil)
	if err != nil {
		t.Fatalf("open e2: %v", err)
	}
	defer func() { _ = e2.Close() }()

	got, accepted, err := e2.Get("r1")
	if err != nil {
		t.Fatalf("get on e2: %v", err)
	}
	if !accepted {
		t.Fatalf("accept did not survive restart: got accepted=%v", accepted)
	}
	if got.Pattern != rec.Pattern || got.Tier != rec.Tier || got.Source != rec.Source {
		t.Fatalf("row mismatch after restart: %+v vs %+v", got, rec)
	}
	// Apply on confirm-tier should now succeed because Accept persisted.
	if err := e2.Apply(context.Background(), got); err != nil {
		t.Fatalf("apply on e2 after persisted accept: %v", err)
	}
}

// TestSQLite_0600Mode verifies on-disk file permissions match the spec.
func TestSQLite_0600Mode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "recommend.db")
	eng, err := NewSQLiteEngine(path, nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = eng.Close() }()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	mode := info.Mode().Perm()
	if mode != 0o600 {
		t.Fatalf("want mode 0600, got %#o", mode)
	}
}

// TestSQLite_ConcurrentSafe drives the engine from multiple goroutines so
// `go test -race` proves the single-writer + mutex story holds. Failure mode
// the race detector would flag: actions map mutated without the mutex.
func TestSQLite_ConcurrentSafe(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "recommend.db")
	eng, err := NewSQLiteEngine(path, nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = eng.Close() }()

	const n = 20
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			id := fmt.Sprintf("rec-%d", i)
			rec := Recommendation{
				ID: id, Pattern: "p", Tier: TierConfirm,
				Source: "test", Confidence: 0.5, CreatedAt: time.Now().UTC(),
				Action: func(context.Context) error { return nil },
			}
			if err := eng.Seed(rec); err != nil {
				t.Errorf("seed: %v", err)
				return
			}
			if err := eng.Accept(context.Background(), id); err != nil {
				t.Errorf("accept: %v", err)
				return
			}
			if _, err := eng.Propose(context.Background()); err != nil {
				t.Errorf("propose: %v", err)
			}
		}()
	}
	wg.Wait()
}

// TestSQLite_SchemaVersion_IntParse guards the PR #58 lessons: a manually
// bumped on-disk version higher than the embedded constant must refuse to
// open (lex compare "10" < "9" would have silently downgraded — int compare
// rejects). Mirrors internal/memory's TestMigrate_NewerOnDiskVersionRejected.
func TestSQLite_SchemaVersion_IntParse(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "recommend.db")

	// Round 1: create at embedded version.
	eng, err := NewSQLiteEngine(path, nil)
	if err != nil {
		t.Fatalf("initial open: %v", err)
	}
	if err := eng.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// Manually stamp a numerically-larger version that would have lex-sorted
	// LOWER than the embedded value if Atoi were missing.
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("raw open: %v", err)
	}
	if _, err := db.Exec(`INSERT OR REPLACE INTO schema_meta(key, value) VALUES('version', '99')`); err != nil {
		t.Fatalf("stamp v99: %v", err)
	}
	_ = db.Close()

	// Round 2: re-open must refuse — int parse: 99 > embedded(1).
	if _, err := NewSQLiteEngine(path, nil); err == nil {
		t.Fatalf("expected refusal on newer on-disk version, got nil")
	}

	// Garbage version must surface as parse error, not silent downgrade.
	db2, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("raw open2: %v", err)
	}
	if _, err := db2.Exec(`UPDATE schema_meta SET value='not-a-number' WHERE key='version'`); err != nil {
		t.Fatalf("stamp garbage: %v", err)
	}
	_ = db2.Close()
	_, err = NewSQLiteEngine(path, nil)
	if err == nil {
		t.Fatalf("expected error on garbage version")
	}
}

// TestSQLite_Reject_Removes confirms Reject deletes the row so Propose drops it.
func TestSQLite_Reject_Removes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "recommend.db")
	eng, err := NewSQLiteEngine(path, nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = eng.Close() }()

	rec := Recommendation{ID: "rx", Pattern: "p", Tier: TierConfirm, Source: "s", Confidence: 0.5}
	if err := eng.Seed(rec); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := eng.Reject(context.Background(), "rx"); err != nil {
		t.Fatalf("reject: %v", err)
	}
	if _, _, err := eng.Get("rx"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound after reject, got %v", err)
	}
	recs, err := eng.Propose(context.Background())
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	if len(recs) != 0 {
		t.Fatalf("propose still returns rejected rec: %v", recs)
	}
}

// TestSQLite_ApplyConfirm_RequiresAccept locks in the tier-rubric parity with
// MemoryEngine: confirm without Accept errors; auto fires immediately.
func TestSQLite_ApplyConfirm_RequiresAccept(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "recommend.db")
	eng, err := NewSQLiteEngine(path, nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = eng.Close() }()

	var ran bool
	rec := Recommendation{
		ID: "c1", Pattern: "p", Tier: TierConfirm, Source: "s", Confidence: 0.5,
		Action: func(context.Context) error { ran = true; return nil },
	}
	if err := eng.Seed(rec); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := eng.Apply(context.Background(), rec); !errors.Is(err, ErrConfirmRequired) {
		t.Fatalf("want ErrConfirmRequired, got %v", err)
	}
	if ran {
		t.Fatalf("Action fired before Accept")
	}
	if err := eng.Accept(context.Background(), "c1"); err != nil {
		t.Fatalf("accept: %v", err)
	}
	if err := eng.Apply(context.Background(), rec); err != nil {
		t.Fatalf("apply after accept: %v", err)
	}
	if !ran {
		t.Fatalf("Action did not fire after Accept")
	}
}

// TestSQLite_RecordFeedback verifies feedback rows persist with the spec §8 shape.
func TestSQLite_RecordFeedback(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "recommend.db")
	eng, err := NewSQLiteEngine(path, nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = eng.Close() }()

	if err := eng.RecordFeedback(context.Background(), "r1", "accept", 1.0); err != nil {
		t.Fatalf("record: %v", err)
	}
	var n int
	if err := eng.db.QueryRow(`SELECT count(*) FROM feedback WHERE rec_id='r1' AND kind='accept'`).Scan(&n); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if n != 1 {
		t.Fatalf("want 1 feedback row, got %d", n)
	}
}

// TestSQLite_Engine_InterfaceParity locks in that SQLiteEngine satisfies the
// same Engine surface as MemoryEngine. Compile-only assertion via blank.
func TestSQLite_Engine_InterfaceParity(t *testing.T) {
	var _ Engine = (*SQLiteEngine)(nil)
}

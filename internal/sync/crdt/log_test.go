package crdt

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/trilam/leah/internal/sync/discovery"
	_ "modernc.org/sqlite"
)

const testSchema = `
CREATE TABLE sync_clock (
    table_name TEXT NOT NULL,
    row_id     INTEGER NOT NULL,
    node_uuid  TEXT NOT NULL,
    lamport    INTEGER NOT NULL,
    PRIMARY KEY(table_name, row_id, node_uuid)
);
CREATE TABLE sync_tombstone (
    table_name TEXT NOT NULL,
    row_id     INTEGER NOT NULL,
    deleted_at INTEGER NOT NULL,
    deleted_by TEXT NOT NULL,
    PRIMARY KEY(table_name, row_id)
);`

// testStore wraps *sql.DB to satisfy Store; we keep the same pool semantics as
// sqlstore.OpenWAL (single writer) to surface the same lock contention bugs.
type testStore struct{ db *sql.DB }

func (t *testStore) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	return t.db.BeginTx(ctx, opts)
}

func newLog(t *testing.T, self discovery.DeviceID) (*Log, *sql.DB) {
	t.Helper()
	dsn := "file:" + filepath.Join(t.TempDir(), "sync.db") + "?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(testSchema); err != nil {
		t.Fatalf("schema: %v", err)
	}
	return NewLog(&testStore{db: db}, self), db
}

// Replaying the same LogEntry twice must leave on-disk state unchanged and
// surface as Skipped, not Applied.
func TestApplyLog_IdempotentReplay(t *testing.T) {
	l, _ := newLog(t, "self")
	ctx := context.Background()
	entry := LogEntry{Table: "memory", RowID: 1, Node: "node-a", Lamport: 5, Op: OpUpdate, Payload: []byte("v1")}

	stats, err := l.ApplyLog(ctx, []LogEntry{entry})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if stats.Applied != 1 || stats.Skipped != 0 {
		t.Fatalf("first apply stats = %+v, want Applied=1", stats)
	}

	stats, err = l.ApplyLog(ctx, []LogEntry{entry})
	if err != nil {
		t.Fatalf("re-apply: %v", err)
	}
	if stats.Skipped != 1 || stats.Applied != 0 {
		t.Fatalf("replay stats = %+v, want Skipped=1", stats)
	}
}

// A tombstone replay (OpDelete) must be idempotent.
func TestApplyLog_TombstoneIdempotent(t *testing.T) {
	l, db := newLog(t, "self")
	ctx := context.Background()
	del := LogEntry{Table: "memory", RowID: 7, Node: "node-a", Lamport: 3, Op: OpDelete}

	if _, err := l.ApplyLog(ctx, []LogEntry{del}); err != nil {
		t.Fatalf("apply 1: %v", err)
	}
	if _, err := l.ApplyLog(ctx, []LogEntry{del}); err != nil {
		t.Fatalf("apply 2: %v", err)
	}

	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sync_tombstone WHERE table_name='memory' AND row_id=7`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Fatalf("tombstone rows = %d, want 1 after idempotent replay", n)
	}
}

// When a remote write loses LWW resolution against an existing higher-lamport
// local write, ApplyLog reports Conflict.
func TestApplyLog_LWWConflict(t *testing.T) {
	l, _ := newLog(t, "self")
	ctx := context.Background()
	local := LogEntry{Table: "settings", RowID: 1, Node: "node-z", Lamport: 10, Op: OpUpdate, Payload: []byte("L")}
	if _, err := l.ApplyLog(ctx, []LogEntry{local}); err != nil {
		t.Fatalf("local: %v", err)
	}
	remote := LogEntry{Table: "settings", RowID: 1, Node: "node-a", Lamport: 5, Op: OpUpdate, Payload: []byte("R")}
	stats, err := l.ApplyLog(ctx, []LogEntry{remote})
	if err != nil {
		t.Fatalf("remote: %v", err)
	}
	if stats.Conflicts != 1 {
		t.Fatalf("stats = %+v, want Conflicts=1", stats)
	}
}

// EmitLog returns rows with lamport > since, sorted (lamport ASC, node ASC),
// and marks tombstoned rows as OpDelete.
func TestEmitLog_OrderingAndTombstoneMark(t *testing.T) {
	l, _ := newLog(t, "self")
	ctx := context.Background()
	in := []LogEntry{
		{Table: "memory", RowID: 1, Node: "node-b", Lamport: 2, Op: OpUpdate},
		{Table: "memory", RowID: 2, Node: "node-a", Lamport: 3, Op: OpDelete},
		{Table: "memory", RowID: 3, Node: "node-a", Lamport: 1, Op: OpInsert}, // below since=1 cutoff
	}
	if _, err := l.ApplyLog(ctx, in); err != nil {
		t.Fatalf("seed: %v", err)
	}
	out, err := l.EmitLog(ctx, 1, 10)
	if err != nil {
		t.Fatalf("emit: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("emit returned %d entries, want 2 (since=1 excludes lamport=1)", len(out))
	}
	if out[0].Lamport >= out[1].Lamport {
		t.Fatalf("not sorted asc: %+v", out)
	}
	for _, e := range out {
		if e.RowID == 2 && e.Op != OpDelete {
			t.Fatalf("rowid=2 emitted as %s, want OpDelete", e.Op)
		}
	}
}

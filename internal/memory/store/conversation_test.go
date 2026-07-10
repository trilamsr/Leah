package memory

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/trilam/leah/internal/memory/sqlstore"
)

// newTurnDB opens a fresh WAL DB with just the conversation_turn schema —
// avoids dragging in the full memory.NewStore migration for tests that only
// exercise this single table.
func newTurnDB(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := sqlstore.OpenWAL(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`CREATE TABLE conversation_turn(
		id              TEXT PRIMARY KEY,
		user_text       TEXT NOT NULL,
		assistant_text  TEXT NOT NULL,
		created_at      TEXT NOT NULL
	);`); err != nil {
		t.Fatalf("schema: %v", err)
	}
	return db
}

func TestRecordAndRecentTurns(t *testing.T) {
	db := newTurnDB(t)
	if err := RecordTurn(db, "t1", "hello", "hi there"); err != nil {
		t.Fatalf("record t1: %v", err)
	}
	if err := RecordTurn(db, "t2", "what's up", "all good"); err != nil {
		t.Fatalf("record t2: %v", err)
	}
	turns, err := RecentTurns(db, 10)
	if err != nil {
		t.Fatalf("recent: %v", err)
	}
	if len(turns) != 2 {
		t.Fatalf("want 2 turns, got %d", len(turns))
	}
	if turns[0].ID != "t2" {
		t.Fatalf("expected t2 first (newest), got %s", turns[0].ID)
	}
	if turns[0].UserText != "what's up" {
		t.Errorf("UserText mismatch: %q", turns[0].UserText)
	}
	if turns[0].AssistantText != "all good" {
		t.Errorf("AssistantText mismatch: %q", turns[0].AssistantText)
	}
	if turns[0].CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
}

func TestRecordTurn_Idempotent(t *testing.T) {
	db := newTurnDB(t)
	if err := RecordTurn(db, "dup", "first", "resp1"); err != nil {
		t.Fatalf("record first: %v", err)
	}
	if err := RecordTurn(db, "dup", "second", "resp2"); err != nil {
		t.Fatalf("record second: %v", err)
	}
	turns, err := RecentTurns(db, 10)
	if err != nil {
		t.Fatalf("recent: %v", err)
	}
	if len(turns) != 1 {
		t.Fatalf("INSERT OR REPLACE broken: want 1 row, got %d", len(turns))
	}
}

func TestRecentTurns_Limit(t *testing.T) {
	db := newTurnDB(t)
	for i := 0; i < 5; i++ {
		id := string(rune('a' + i))
		if err := RecordTurn(db, id, "q"+id, "a"+id); err != nil {
			t.Fatalf("record %s: %v", id, err)
		}
	}
	turns, err := RecentTurns(db, 2)
	if err != nil {
		t.Fatalf("recent: %v", err)
	}
	if len(turns) != 2 {
		t.Fatalf("want 2 (limit), got %d", len(turns))
	}
}

func TestRecentTurns_BadTimestampSurfaceError(t *testing.T) {
	db := newTurnDB(t)
	if _, err := db.Exec(
		`INSERT INTO conversation_turn(id, user_text, assistant_text, created_at) VALUES('x','q','a','not-a-timestamp')`,
	); err != nil {
		t.Fatalf("insert raw: %v", err)
	}
	if _, err := RecentTurns(db, 10); err == nil {
		t.Fatal("expected parse error for malformed created_at, got nil")
	}
}

func TestNewStore_HasConversationTurnTable(t *testing.T) {
	s := newTestStore(t)
	var name string
	err := s.db.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='table' AND name='conversation_turn'`,
	).Scan(&name)
	if err != nil {
		t.Fatalf("conversation_turn table missing after schema v9 migration: %v", err)
	}
}

package memory

import (
	"path/filepath"
	"testing"

	"github.com/trilam/leah/internal/sqlstore"
)

func TestRecordAndRecentTurns(t *testing.T) {
	dir := t.TempDir()
	db, err := sqlstore.OpenWAL(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE conversation_turn(
		id              TEXT PRIMARY KEY,
		user_text       TEXT NOT NULL,
		assistant_text  TEXT NOT NULL,
		created_at      TEXT NOT NULL
	);`); err != nil {
		t.Fatalf("schema: %v", err)
	}
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
	// Most recent first.
	if turns[0].ID != "t2" {
		t.Fatalf("expected t2 first, got %s", turns[0].ID)
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
	dir := t.TempDir()
	db, err := sqlstore.OpenWAL(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE conversation_turn(
		id              TEXT PRIMARY KEY,
		user_text       TEXT NOT NULL,
		assistant_text  TEXT NOT NULL,
		created_at      TEXT NOT NULL
	);`); err != nil {
		t.Fatalf("schema: %v", err)
	}
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
	dir := t.TempDir()
	db, err := sqlstore.OpenWAL(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE conversation_turn(
		id              TEXT PRIMARY KEY,
		user_text       TEXT NOT NULL,
		assistant_text  TEXT NOT NULL,
		created_at      TEXT NOT NULL
	);`); err != nil {
		t.Fatalf("schema: %v", err)
	}
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

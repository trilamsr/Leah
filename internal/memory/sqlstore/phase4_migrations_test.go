package sqlstore

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// newTestDB opens a fresh WAL DB in a temp dir.
func newTestDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "leah.db")
	db, err := OpenWAL(p)
	if err != nil {
		t.Fatalf("OpenWAL: %v", err)
	}
	return db, func() { _ = db.Close() }
}

func TestPhase4Migrations_AllTablesCreated(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()
	if err := MigrateUp(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"voice_session", "voice_turn", "voice_suppression",
		"vision_event", "vision_consent",
		"learn_observation", "learn_recommendation", "learn_decay", "learn_experiment", "anti_recommend",
		"budget_bucket", "budget_sample",
		"attest_record",
		"sync_peer", "sync_clock", "sync_tombstone", "sync_outbox",
		"a2a_peer", "a2a_call", "mcp_token", "a2a_consent",
		"plugin", "plugin_log", "plugin_quota",
		"supervisor_event", "supervisor_rss",
	}
	for _, table := range want {
		var n int
		if err := db.QueryRowContext(context.Background(),
			"SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Errorf("table %s missing", table)
		}
	}
}

func TestPhase4Migrations_NodeUUIDBackfilled(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()
	if err := MigrateUp(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	rows, err := db.QueryContext(context.Background(),
		"SELECT name FROM pragma_table_info('memory') WHERE name='node_uuid'")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		t.Fatal("memory.node_uuid missing")
	}
}

func TestPhase4Migrations_Idempotent(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()
	if err := MigrateUp(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if err := MigrateUp(context.Background(), db); err != nil {
		t.Fatalf("second MigrateUp must be a no-op, got: %v", err)
	}
}

func TestPhase4Migrations_BudgetSeeds(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()
	if err := MigrateUp(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRowContext(context.Background(),
		"SELECT count(*) FROM budget_bucket").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 7 {
		t.Errorf("budget_bucket seed rows = %d, want 7", n)
	}
}

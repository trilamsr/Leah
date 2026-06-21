package sqlstore

import (
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestOpenWAL_PragmasActive(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.db")
	db, err := OpenWAL(p)
	if err != nil {
		t.Fatalf("OpenWAL: %v", err)
	}
	defer func() { _ = db.Close() }()

	var mode string
	if err := db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatalf("journal_mode: %v", err)
	}
	if mode != "wal" {
		t.Fatalf("journal_mode = %q, want wal", mode)
	}
	var busy int
	if err := db.QueryRow("PRAGMA busy_timeout").Scan(&busy); err != nil {
		t.Fatalf("busy_timeout: %v", err)
	}
	if busy != 5000 {
		t.Fatalf("busy_timeout = %d, want 5000", busy)
	}
	if got := db.Stats().MaxOpenConnections; got != 1 {
		t.Fatalf("MaxOpenConnections = %d, want 1 (single-writer)", got)
	}
}

func TestOpenWAL_ExtraPragma(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.db")
	db, err := OpenWAL(p, "synchronous(NORMAL)")
	if err != nil {
		t.Fatalf("OpenWAL: %v", err)
	}
	defer func() { _ = db.Close() }()
	var sync int
	if err := db.QueryRow("PRAGMA synchronous").Scan(&sync); err != nil {
		t.Fatalf("synchronous: %v", err)
	}
	if sync != 1 { // NORMAL == 1
		t.Fatalf("synchronous = %d, want 1 (NORMAL)", sync)
	}
}

func TestEnsureSchemaVersion_RoundTrips(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.db")
	db, err := OpenWAL(p)
	if err != nil {
		t.Fatalf("OpenWAL: %v", err)
	}
	defer func() { _ = db.Close() }()

	// First call against a fresh db: no version row yet, no guard trip.
	if err := EnsureSchemaVersion(db, "x.db", 3); err != nil {
		t.Fatalf("first EnsureSchemaVersion: %v", err)
	}
	// Caller stamps after applying DDL.
	if _, err := db.Exec(`INSERT OR REPLACE INTO schema_meta(key, value) VALUES('version', '3')`); err != nil {
		t.Fatalf("stamp: %v", err)
	}
	// Re-open path: on-disk == embedded is fine.
	if err := EnsureSchemaVersion(db, "x.db", 3); err != nil {
		t.Fatalf("re-open EnsureSchemaVersion: %v", err)
	}
	// On-disk newer than binary must refuse.
	if err := EnsureSchemaVersion(db, "x.db", 2); err == nil {
		t.Fatal("EnsureSchemaVersion accepted on-disk newer than binary; want refusal")
	}
}

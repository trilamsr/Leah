package router

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

const consentSchema = `CREATE TABLE vision_consent (
    id          INTEGER PRIMARY KEY,
    mode        TEXT NOT NULL,
    granted_at  INTEGER NOT NULL,
    expires_at  INTEGER,
    scope       TEXT NOT NULL CHECK(scope IN ('this_session','until_quit','persistent'))
);`

func openConsentDB(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "consent.db")
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(consentSchema); err != nil {
		t.Fatalf("schema: %v", err)
	}
	return db
}

func TestDBConsent_SessionGrantNotPersisted(t *testing.T) {
	db := openConsentDB(t)
	s, err := NewDBConsent(db)
	if err != nil {
		t.Fatalf("NewDBConsent: %v", err)
	}
	s.Grant("screenshot", ScopeThisSession)
	if !s.Granted("screenshot") {
		t.Fatal("session grant must show as granted in current process")
	}
	// Fresh store reading the same DB must NOT see the session grant.
	s2, err := NewDBConsent(db)
	if err != nil {
		t.Fatalf("NewDBConsent: %v", err)
	}
	if s2.Granted("screenshot") {
		t.Fatal("session grant must not survive a process restart")
	}
}

func TestDBConsent_PersistentGrantSurvivesReopen(t *testing.T) {
	db := openConsentDB(t)
	s, err := NewDBConsent(db)
	if err != nil {
		t.Fatalf("NewDBConsent: %v", err)
	}
	s.Grant("live_screen", ScopePersistent)
	s2, err := NewDBConsent(db)
	if err != nil {
		t.Fatalf("NewDBConsent reopen: %v", err)
	}
	if !s2.Granted("live_screen") {
		t.Fatal("persistent grant must reload on reopen")
	}
}

func TestDBConsent_RevokeClearsDB(t *testing.T) {
	db := openConsentDB(t)
	s, err := NewDBConsent(db)
	if err != nil {
		t.Fatalf("NewDBConsent: %v", err)
	}
	s.Grant("screenshot", ScopePersistent)
	s.Revoke("screenshot")
	s2, err := NewDBConsent(db)
	if err != nil {
		t.Fatalf("NewDBConsent reopen: %v", err)
	}
	if s2.Granted("screenshot") {
		t.Fatal("revoke must clear persistent row")
	}
}

func TestDBConsent_PersistentRegrantReplacesRow(t *testing.T) {
	// Regranting Persistent over an existing Persistent row must NOT pile up
	// duplicate rows — otherwise Revoke would have to delete N rows to clear.
	db := openConsentDB(t)
	s, err := NewDBConsent(db)
	if err != nil {
		t.Fatalf("NewDBConsent: %v", err)
	}
	s.Grant("screenshot", ScopePersistent)
	s.Grant("screenshot", ScopePersistent)
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM vision_consent WHERE mode='screenshot'`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Fatalf("regrant must coalesce; got %d rows", n)
	}
}

func TestDBConsent_GrantRevokeLogOnDBError(t *testing.T) {
	// Closing the DB before Grant/Revoke forces db.Exec to fail. The store
	// must not panic and the in-memory cache must still reflect the call —
	// the interface forbids returning an error, so the dbConsent logs and
	// proceeds. This pins the contract so a refactor doesn't silently revert
	// to a panic or skip the cache update.
	db := openConsentDB(t)
	s, err := NewDBConsent(db)
	if err != nil {
		t.Fatalf("NewDBConsent: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	s.Grant("screenshot", ScopePersistent)
	if !s.Granted("screenshot") {
		t.Fatal("cache must reflect Grant even when DB write fails")
	}
	s.Revoke("screenshot")
	if s.Granted("screenshot") {
		t.Fatal("cache must reflect Revoke even when DB write fails")
	}
}

func TestDBConsent_DowngradeRemovesPersistentRow(t *testing.T) {
	// Persistent → ThisSession downgrade must drop the DB row so a future
	// process doesn't pick up the stale persistent grant.
	db := openConsentDB(t)
	s, err := NewDBConsent(db)
	if err != nil {
		t.Fatalf("NewDBConsent: %v", err)
	}
	s.Grant("screenshot", ScopePersistent)
	s.Grant("screenshot", ScopeThisSession)
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM vision_consent WHERE mode='screenshot'`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Fatalf("downgrade must clear persistent row; got %d rows", n)
	}
	if !s.Granted("screenshot") {
		t.Fatal("session grant still active in current process")
	}
}

package notes

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

type fakeAttestor struct {
	err   error
	calls int
}

func (f *fakeAttestor) Attest(_ context.Context, _ string) error {
	f.calls++
	return f.err
}

// seedDB writes a minimal ZICCLOUDSYNCINGOBJECT fixture. Notes co-tables
// folders/accounts in the same row store; ZNOTEDATA non-null discriminates
// note rows (CoreData NSArchiver-encoded body, deferred per W28 brief).
func seedDB(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open seed db: %v", err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(`CREATE TABLE ZICCLOUDSYNCINGOBJECT (
		Z_PK INTEGER PRIMARY KEY,
		ZIDENTIFIER TEXT,
		ZTITLE1 TEXT,
		ZCREATIONDATE1 REAL,
		ZMODIFICATIONDATE1 REAL,
		ZNOTEDATA INTEGER
	)`); err != nil {
		t.Fatalf("create: %v", err)
	}
	t1 := time.Date(2026, 6, 9, 14, 0, 0, 0, time.UTC).Sub(macEpoch).Seconds()
	t2 := time.Date(2026, 6, 10, 8, 15, 0, 0, time.UTC).Sub(macEpoch).Seconds()
	if _, err := db.Exec(
		`INSERT INTO ZICCLOUDSYNCINGOBJECT VALUES
		 (1,?,?,?,?,?),
		 (2,?,?,?,?,?),
		 (3,?,?,?,?,?)`,
		"NOTE-1", "Grocery list", t1, t1, 100,
		"NOTE-2", "Trip itinerary", t2, t2, 200,
		// row 3 is a folder (ZNOTEDATA NULL) and must be filtered out.
		"FOLDER-1", "Travel", t1, t1, nil,
	); err != nil {
		t.Fatalf("insert: %v", err)
	}
}

func TestNotes_Name(t *testing.T) {
	n, err := New(Config{DBPath: "/dev/null", Attestor: &fakeAttestor{}})
	if err != nil {
		t.Fatal(err)
	}
	if got := n.Name(); got != "Notes" {
		t.Fatalf("Name=%q want Notes", got)
	}
}

func TestNotes_Available_FileMissing(t *testing.T) {
	n, err := New(Config{DBPath: filepath.Join(t.TempDir(), "missing"), Attestor: &fakeAttestor{}})
	if err != nil {
		t.Fatal(err)
	}
	if n.Available(context.Background()) {
		t.Fatal("Available=true for missing file")
	}
}

func TestNotes_Available_FilePresent(t *testing.T) {
	p := filepath.Join(t.TempDir(), "notes.sqlite")
	seedDB(t, p)
	n, err := New(Config{DBPath: p, Attestor: &fakeAttestor{}})
	if err != nil {
		t.Fatal(err)
	}
	if !n.Available(context.Background()) {
		t.Fatal("Available=false for seeded file")
	}
}

// Denial must short-circuit BEFORE the DB is opened — a leaked credential
// scenario is unsafe if the file handle ever attaches.
func TestNotes_Query_AttestationDenied_NoDBOpen(t *testing.T) {
	var opens int
	n, err := New(Config{
		DBPath:   filepath.Join(t.TempDir(), "notes.sqlite"),
		Attestor: &fakeAttestor{err: errors.New("no")},
		Open: func(driver, dsn string) (*sql.DB, error) {
			opens++
			return sql.Open(driver, dsn)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = n.Query(context.Background(), Query{})
	if !errors.Is(err, ErrAttestationDenied) {
		t.Fatalf("err=%v want ErrAttestationDenied", err)
	}
	if opens != 0 {
		t.Fatalf("DB opened %d times after denial", opens)
	}
}

func TestNotes_Query_HappyPath(t *testing.T) {
	p := filepath.Join(t.TempDir(), "notes.sqlite")
	seedDB(t, p)
	n, err := New(Config{DBPath: p, Attestor: &fakeAttestor{}})
	if err != nil {
		t.Fatal(err)
	}
	items, err := n.Query(context.Background(), Query{
		Since: time.Date(2026, 6, 9, 0, 0, 0, 0, time.UTC),
		Until: time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len=%d want 2 (folder row must be excluded); items=%+v", len(items), items)
	}
	if items[0].ID != "notes:NOTE-1" || items[0].Title != "Grocery list" || items[0].Source != "notes" {
		t.Fatalf("row0=%+v", items[0])
	}
	if !items[0].Timestamp.Equal(time.Date(2026, 6, 9, 14, 0, 0, 0, time.UTC)) {
		t.Fatalf("ts=%v want 2026-06-09T14:00:00Z", items[0].Timestamp)
	}
	if items[0].Body != "" {
		t.Fatalf("Body=%q want empty (NSArchiver decode deferred)", items[0].Body)
	}
}

// Spec §4: every Apple SQLite opens mode=ro&immutable=1.
func TestNotes_Query_ReadOnly(t *testing.T) {
	p := filepath.Join(t.TempDir(), "notes.sqlite")
	seedDB(t, p)
	var seenDSN string
	n, err := New(Config{
		DBPath:   p,
		Attestor: &fakeAttestor{},
		Open: func(driver, dsn string) (*sql.DB, error) {
			seenDSN = dsn
			return sql.Open(driver, dsn)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := n.Query(context.Background(), Query{}); err != nil {
		t.Fatalf("Query: %v", err)
	}
	for _, frag := range []string{"mode=ro", "immutable=1"} {
		if !strings.Contains(seenDSN, frag) {
			t.Fatalf("DSN %q missing %q", seenDSN, frag)
		}
	}
}

func TestNotes_Query_SourceUnavailable(t *testing.T) {
	n, err := New(Config{
		DBPath:   filepath.Join(t.TempDir(), "absent"),
		Attestor: &fakeAttestor{},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = n.Query(context.Background(), Query{})
	if !errors.Is(err, ErrSourceUnavailable) {
		t.Fatalf("err=%v want ErrSourceUnavailable", err)
	}
}

func TestNotes_Sync_NoOp(t *testing.T) {
	n, err := New(Config{DBPath: "/dev/null", Attestor: &fakeAttestor{}})
	if err != nil {
		t.Fatal(err)
	}
	if err := n.Sync(context.Background()); err != nil {
		t.Fatalf("Sync: %v", err)
	}
}

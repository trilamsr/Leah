package calendar

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

// seedDB writes a minimal ZCALENDARITEM fixture to path.
func seedDB(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open seed db: %v", err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(`CREATE TABLE ZCALENDARITEM (
		ZUNIQUEIDENTIFIER TEXT,
		ZSUMMARY TEXT,
		ZNOTES TEXT,
		ZSTARTDATE REAL,
		ZENDDATE REAL
	)`); err != nil {
		t.Fatalf("create: %v", err)
	}
	// 2026-06-10 12:00:00 UTC -> seconds since 2001-01-01.
	t1 := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC).Sub(macEpoch).Seconds()
	t2 := time.Date(2026, 6, 11, 9, 0, 0, 0, time.UTC).Sub(macEpoch).Seconds()
	if _, err := db.Exec(
		`INSERT INTO ZCALENDARITEM VALUES (?,?,?,?,?),(?,?,?,?,?)`,
		"EVT-1", "Standup", "daily sync", t1, t1+1800,
		"EVT-2", "1:1 with Sam", "career", t2, t2+1800,
	); err != nil {
		t.Fatalf("insert: %v", err)
	}
}

func TestCalendar_Name(t *testing.T) {
	c, err := New(Config{DBPath: "/dev/null", Attestor: &fakeAttestor{}})
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Name(); got != "Calendar" {
		t.Fatalf("Name=%q want Calendar", got)
	}
}

func TestCalendar_Available_FileMissing(t *testing.T) {
	c, err := New(Config{DBPath: filepath.Join(t.TempDir(), "missing"), Attestor: &fakeAttestor{}})
	if err != nil {
		t.Fatal(err)
	}
	if c.Available(context.Background()) {
		t.Fatal("Available=true for missing file")
	}
}

func TestCalendar_Available_FilePresent(t *testing.T) {
	p := filepath.Join(t.TempDir(), "cache")
	seedDB(t, p)
	c, err := New(Config{DBPath: p, Attestor: &fakeAttestor{}})
	if err != nil {
		t.Fatal(err)
	}
	if !c.Available(context.Background()) {
		t.Fatal("Available=false for seeded file")
	}
}

// Denial must short-circuit BEFORE the DB is opened — a leaked credential
// scenario is unsafe if the file handle ever attaches.
func TestCalendar_Query_AttestationDenied_NoDBOpen(t *testing.T) {
	var opens int
	c, err := New(Config{
		DBPath:   filepath.Join(t.TempDir(), "cache"),
		Attestor: &fakeAttestor{err: errors.New("no")},
		Open: func(driver, dsn string) (*sql.DB, error) {
			opens++
			return sql.Open(driver, dsn)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Query(context.Background(), Query{})
	if !errors.Is(err, ErrAttestationDenied) {
		t.Fatalf("err=%v want ErrAttestationDenied", err)
	}
	if opens != 0 {
		t.Fatalf("DB opened %d times after denial", opens)
	}
}

func TestCalendar_Query_HappyPath(t *testing.T) {
	p := filepath.Join(t.TempDir(), "cache")
	seedDB(t, p)
	c, err := New(Config{DBPath: p, Attestor: &fakeAttestor{}})
	if err != nil {
		t.Fatal(err)
	}
	items, err := c.Query(context.Background(), Query{
		Since: time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC),
		Until: time.Date(2026, 6, 12, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len=%d want 2; items=%+v", len(items), items)
	}
	if items[0].ID != "calendar:EVT-1" || items[0].Title != "Standup" || items[0].Source != "calendar" {
		t.Fatalf("row0=%+v", items[0])
	}
	if !items[0].Timestamp.Equal(time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("ts=%v want 2026-06-10T12:00:00Z", items[0].Timestamp)
	}
}

// Spec §4: every Apple SQLite opens mode=ro&immutable=1.
func TestCalendar_Query_ReadOnly(t *testing.T) {
	p := filepath.Join(t.TempDir(), "cache")
	seedDB(t, p)
	var seenDSN string
	c, err := New(Config{
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
	if _, err := c.Query(context.Background(), Query{}); err != nil {
		t.Fatalf("Query: %v", err)
	}
	for _, frag := range []string{"mode=ro", "immutable=1"} {
		if !strings.Contains(seenDSN, frag) {
			t.Fatalf("DSN %q missing %q", seenDSN, frag)
		}
	}
}

func TestCalendar_Query_SourceUnavailable(t *testing.T) {
	c, err := New(Config{
		DBPath:   filepath.Join(t.TempDir(), "absent"),
		Attestor: &fakeAttestor{},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Query(context.Background(), Query{})
	if !errors.Is(err, ErrSourceUnavailable) {
		t.Fatalf("err=%v want ErrSourceUnavailable", err)
	}
}

func TestCalendar_Sync_NoOp(t *testing.T) {
	c, err := New(Config{DBPath: "/dev/null", Attestor: &fakeAttestor{}})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Sync(context.Background()); err != nil {
		t.Fatalf("Sync: %v", err)
	}
}

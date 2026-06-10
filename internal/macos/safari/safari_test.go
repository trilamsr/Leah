package safari

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

// seedDB mirrors Safari History.db: history_items holds url+visit_count;
// history_visits joins by history_item, with visit_time stored as Cocoa
// absolute time (REAL seconds since 2001-01-01 UTC).
func seedDB(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open seed db: %v", err)
	}
	defer func() { _ = db.Close() }()
	stmts := []string{
		`CREATE TABLE history_items (
			id INTEGER PRIMARY KEY,
			url TEXT NOT NULL,
			visit_count INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE history_visits (
			id INTEGER PRIMARY KEY,
			history_item INTEGER NOT NULL,
			visit_time REAL NOT NULL,
			title TEXT
		)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("create: %v", err)
		}
	}
	// Cocoa epoch = 2001-01-01 UTC. 2026-06-09T14:00:00Z = 803916000s.
	cocoa := func(tm time.Time) float64 {
		return tm.Sub(time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)).Seconds()
	}
	t1 := cocoa(time.Date(2026, 6, 9, 14, 0, 0, 0, time.UTC))
	t2 := cocoa(time.Date(2026, 6, 10, 8, 15, 0, 0, time.UTC))
	exec := func(q string, args ...any) {
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatalf("insert %q: %v", q, err)
		}
	}
	exec(`INSERT INTO history_items VALUES (1,'https://example.com/a',3),(2,'https://example.com/b',1)`)
	exec(`INSERT INTO history_visits VALUES (10,1,?,'Example A'),(11,2,?,'Example B')`, t1, t2)
}

func TestSafari_Name(t *testing.T) {
	s, err := New(Config{DBPath: "/dev/null", Attestor: &fakeAttestor{}})
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Name(); got != "Safari" {
		t.Fatalf("Name=%q want Safari", got)
	}
}

func TestSafari_Available_FileMissing(t *testing.T) {
	s, err := New(Config{DBPath: filepath.Join(t.TempDir(), "missing"), Attestor: &fakeAttestor{}})
	if err != nil {
		t.Fatal(err)
	}
	if s.Available(context.Background()) {
		t.Fatal("Available=true for missing file")
	}
}

func TestSafari_Available_FilePresent(t *testing.T) {
	p := filepath.Join(t.TempDir(), "History.db")
	seedDB(t, p)
	s, err := New(Config{DBPath: p, Attestor: &fakeAttestor{}})
	if err != nil {
		t.Fatal(err)
	}
	if !s.Available(context.Background()) {
		t.Fatal("Available=false for seeded file")
	}
}

// Denial must short-circuit BEFORE the DB is opened — Safari's store may
// hold private-browsing crumbs, so a denied operator never gets a handle.
func TestSafari_Query_AttestationDenied_NoDBOpen(t *testing.T) {
	var opens int
	s, err := New(Config{
		DBPath:   filepath.Join(t.TempDir(), "History.db"),
		Attestor: &fakeAttestor{err: errors.New("no")},
		Open: func(driver, dsn string) (*sql.DB, error) {
			opens++
			return sql.Open(driver, dsn)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Query(context.Background(), Query{})
	if !errors.Is(err, ErrAttestationDenied) {
		t.Fatalf("err=%v want ErrAttestationDenied", err)
	}
	if opens != 0 {
		t.Fatalf("DB opened %d times after denial", opens)
	}
}

func TestSafari_Query_HappyPath(t *testing.T) {
	p := filepath.Join(t.TempDir(), "History.db")
	seedDB(t, p)
	s, err := New(Config{DBPath: p, Attestor: &fakeAttestor{}})
	if err != nil {
		t.Fatal(err)
	}
	items, err := s.Query(context.Background(), Query{
		Since: time.Date(2026, 6, 9, 0, 0, 0, 0, time.UTC),
		Until: time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len=%d want 2; items=%+v", len(items), items)
	}
	got := items[0]
	if got.URL != "https://example.com/a" || got.Title != "Example A" || got.VisitCount != 3 {
		t.Fatalf("row0=%+v", got)
	}
	if !got.VisitTime.Equal(time.Date(2026, 6, 9, 14, 0, 0, 0, time.UTC)) {
		t.Fatalf("visit_time=%v want 2026-06-09T14:00:00Z", got.VisitTime)
	}
	if items[1].URL != "https://example.com/b" || items[1].VisitCount != 1 {
		t.Fatalf("row1=%+v", items[1])
	}
}

func TestSafari_Query_TextFilter(t *testing.T) {
	p := filepath.Join(t.TempDir(), "History.db")
	seedDB(t, p)
	s, err := New(Config{DBPath: p, Attestor: &fakeAttestor{}})
	if err != nil {
		t.Fatal(err)
	}
	items, err := s.Query(context.Background(), Query{Text: "Example A"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(items) != 1 || items[0].Title != "Example A" {
		t.Fatalf("text-filter got=%+v want 1 row Example A", items)
	}
}

// Spec §4: every Apple SQLite opens mode=ro&immutable=1.
func TestSafari_Query_ReadOnly(t *testing.T) {
	p := filepath.Join(t.TempDir(), "History.db")
	seedDB(t, p)
	var seenDSN string
	s, err := New(Config{
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
	if _, err := s.Query(context.Background(), Query{}); err != nil {
		t.Fatalf("Query: %v", err)
	}
	for _, frag := range []string{"mode=ro", "immutable=1"} {
		if !strings.Contains(seenDSN, frag) {
			t.Fatalf("DSN %q missing %q", seenDSN, frag)
		}
	}
}

func TestSafari_Query_SourceUnavailable(t *testing.T) {
	s, err := New(Config{
		DBPath:   filepath.Join(t.TempDir(), "absent"),
		Attestor: &fakeAttestor{},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Query(context.Background(), Query{})
	if !errors.Is(err, ErrSourceUnavailable) {
		t.Fatalf("err=%v want ErrSourceUnavailable", err)
	}
}

func TestSafari_Sync_NoOp(t *testing.T) {
	s, err := New(Config{DBPath: "/dev/null", Attestor: &fakeAttestor{}})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Sync(context.Background()); err != nil {
		t.Fatalf("Sync: %v", err)
	}
}

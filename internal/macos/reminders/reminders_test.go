package reminders

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

// seedDB writes a minimal ZREMCDREMINDER fixture to path.
func seedDB(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open seed db: %v", err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(`CREATE TABLE ZREMCDREMINDER (
		ZUNIQUEIDENTIFIER TEXT,
		ZTITLE TEXT,
		ZNOTES TEXT,
		ZDUEDATE REAL,
		ZCOMPLETED INTEGER
	)`); err != nil {
		t.Fatalf("create: %v", err)
	}
	t1 := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC).Sub(macEpoch).Seconds()
	t2 := time.Date(2026, 6, 11, 17, 30, 0, 0, time.UTC).Sub(macEpoch).Seconds()
	if _, err := db.Exec(
		`INSERT INTO ZREMCDREMINDER VALUES (?,?,?,?,?),(?,?,?,?,?)`,
		"REM-1", "Buy milk", "2%", t1, 0,
		"REM-2", "File taxes", "before deadline", t2, 0,
	); err != nil {
		t.Fatalf("insert: %v", err)
	}
}

func TestReminders_Name(t *testing.T) {
	r, err := New(Config{DBPath: "/dev/null", Attestor: &fakeAttestor{}})
	if err != nil {
		t.Fatal(err)
	}
	if got := r.Name(); got != "Reminders" {
		t.Fatalf("Name=%q want Reminders", got)
	}
}

func TestReminders_Available_FileMissing(t *testing.T) {
	r, err := New(Config{DBPath: filepath.Join(t.TempDir(), "missing"), Attestor: &fakeAttestor{}})
	if err != nil {
		t.Fatal(err)
	}
	if r.Available(context.Background()) {
		t.Fatal("Available=true for missing file")
	}
}

func TestReminders_Available_FilePresent(t *testing.T) {
	p := filepath.Join(t.TempDir(), "rem.sqlite")
	seedDB(t, p)
	r, err := New(Config{DBPath: p, Attestor: &fakeAttestor{}})
	if err != nil {
		t.Fatal(err)
	}
	if !r.Available(context.Background()) {
		t.Fatal("Available=false for seeded file")
	}
}

// Denial must short-circuit BEFORE the DB is opened — a leaked credential
// scenario is unsafe if the file handle ever attaches.
func TestReminders_Query_AttestationDenied_NoDBOpen(t *testing.T) {
	var opens int
	r, err := New(Config{
		DBPath:   filepath.Join(t.TempDir(), "rem.sqlite"),
		Attestor: &fakeAttestor{err: errors.New("no")},
		Open: func(driver, dsn string) (*sql.DB, error) {
			opens++
			return sql.Open(driver, dsn)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.Query(context.Background(), Query{})
	if !errors.Is(err, ErrAttestationDenied) {
		t.Fatalf("err=%v want ErrAttestationDenied", err)
	}
	if opens != 0 {
		t.Fatalf("DB opened %d times after denial", opens)
	}
}

func TestReminders_Query_HappyPath(t *testing.T) {
	p := filepath.Join(t.TempDir(), "rem.sqlite")
	seedDB(t, p)
	r, err := New(Config{DBPath: p, Attestor: &fakeAttestor{}})
	if err != nil {
		t.Fatal(err)
	}
	items, err := r.Query(context.Background(), Query{
		Since: time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC),
		Until: time.Date(2026, 6, 12, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len=%d want 2; items=%+v", len(items), items)
	}
	if items[0].ID != "reminders:REM-1" || items[0].Title != "Buy milk" || items[0].Source != "reminders" {
		t.Fatalf("row0=%+v", items[0])
	}
	if !items[0].Timestamp.Equal(time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)) {
		t.Fatalf("ts=%v want 2026-06-10T09:00:00Z", items[0].Timestamp)
	}
}

// Spec §4: every Apple SQLite opens mode=ro&immutable=1.
func TestReminders_Query_ReadOnly(t *testing.T) {
	p := filepath.Join(t.TempDir(), "rem.sqlite")
	seedDB(t, p)
	var seenDSN string
	r, err := New(Config{
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
	if _, err := r.Query(context.Background(), Query{}); err != nil {
		t.Fatalf("Query: %v", err)
	}
	for _, frag := range []string{"mode=ro", "immutable=1"} {
		if !strings.Contains(seenDSN, frag) {
			t.Fatalf("DSN %q missing %q", seenDSN, frag)
		}
	}
}

func TestReminders_Query_SourceUnavailable(t *testing.T) {
	r, err := New(Config{
		DBPath:   filepath.Join(t.TempDir(), "absent"),
		Attestor: &fakeAttestor{},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.Query(context.Background(), Query{})
	if !errors.Is(err, ErrSourceUnavailable) {
		t.Fatalf("err=%v want ErrSourceUnavailable", err)
	}
}

func TestReminders_Sync_NoOp(t *testing.T) {
	r, err := New(Config{DBPath: "/dev/null", Attestor: &fakeAttestor{}})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Sync(context.Background()); err != nil {
		t.Fatalf("Sync: %v", err)
	}
}

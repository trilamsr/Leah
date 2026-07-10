package mail

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

// seedDB mirrors Envelope Index V10: messages references subjects/addresses/
// summaries by ROWID; recipients joins messages to addresses many-to-one.
func seedDB(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open seed db: %v", err)
	}
	defer func() { _ = db.Close() }()
	stmts := []string{
		`CREATE TABLE subjects (ROWID INTEGER PRIMARY KEY, subject TEXT)`,
		`CREATE TABLE addresses (ROWID INTEGER PRIMARY KEY, address TEXT, comment TEXT)`,
		`CREATE TABLE summaries (ROWID INTEGER PRIMARY KEY, summary TEXT)`,
		`CREATE TABLE messages (
			ROWID INTEGER PRIMARY KEY,
			message_id INTEGER,
			subject INTEGER,
			sender INTEGER,
			summary INTEGER,
			date_received INTEGER,
			date_sent INTEGER
		)`,
		`CREATE TABLE recipients (
			ROWID INTEGER PRIMARY KEY,
			message_id INTEGER,
			address_id INTEGER,
			type INTEGER
		)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("create: %v", err)
		}
	}
	t1 := time.Date(2026, 6, 9, 14, 0, 0, 0, time.UTC).Unix()
	t2 := time.Date(2026, 6, 10, 8, 15, 0, 0, time.UTC).Unix()
	body := strings.Repeat("x", 250) // verify 200-char snippet truncation
	exec := func(q string, args ...any) {
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatalf("insert %q: %v", q, err)
		}
	}
	exec(`INSERT INTO subjects VALUES (10,'Project kickoff'), (11,'Lunch?')`)
	exec(`INSERT INTO addresses VALUES (20,'alice@example.com','Alice'),(21,'bob@example.com','Bob'),(22,'carol@example.com','Carol')`)
	exec(`INSERT INTO summaries VALUES (30,?), (31,'Short body')`, body)
	exec(`INSERT INTO messages VALUES (1,1001,10,20,30,?,?), (2,1002,11,21,31,?,?)`, t1, t1, t2, t2)
	// Message 1 has two recipients (To=bob, carol); message 2 has none.
	exec(`INSERT INTO recipients VALUES (1,1,21,1),(2,1,22,1)`)
}

func TestMail_Name(t *testing.T) {
	m, err := New(Config{DBPath: "/dev/null", Attestor: &fakeAttestor{}})
	if err != nil {
		t.Fatal(err)
	}
	if got := m.Name(); got != "Mail" {
		t.Fatalf("Name=%q want Mail", got)
	}
}

func TestMail_Available_FileMissing(t *testing.T) {
	m, err := New(Config{DBPath: filepath.Join(t.TempDir(), "missing"), Attestor: &fakeAttestor{}})
	if err != nil {
		t.Fatal(err)
	}
	if m.Available(context.Background()) {
		t.Fatal("Available=true for missing file")
	}
}

func TestMail_Available_FilePresent(t *testing.T) {
	p := filepath.Join(t.TempDir(), "Envelope Index")
	seedDB(t, p)
	m, err := New(Config{DBPath: p, Attestor: &fakeAttestor{}})
	if err != nil {
		t.Fatal(err)
	}
	if !m.Available(context.Background()) {
		t.Fatal("Available=false for seeded file")
	}
}

// Denial must short-circuit BEFORE the DB is opened — a leaked credential
// scenario is unsafe if the file handle ever attaches.
func TestMail_Query_AttestationDenied_NoDBOpen(t *testing.T) {
	var opens int
	m, err := New(Config{
		DBPath:   filepath.Join(t.TempDir(), "Envelope Index"),
		Attestor: &fakeAttestor{err: errors.New("no")},
		Open: func(driver, dsn string) (*sql.DB, error) {
			opens++
			return sql.Open(driver, dsn)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = m.Query(context.Background(), Query{})
	if !errors.Is(err, ErrAttestationDenied) {
		t.Fatalf("err=%v want ErrAttestationDenied", err)
	}
	if opens != 0 {
		t.Fatalf("DB opened %d times after denial", opens)
	}
}

func TestMail_Query_HappyPath(t *testing.T) {
	p := filepath.Join(t.TempDir(), "Envelope Index")
	seedDB(t, p)
	m, err := New(Config{DBPath: p, Attestor: &fakeAttestor{}})
	if err != nil {
		t.Fatal(err)
	}
	items, err := m.Query(context.Background(), Query{
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
	if got.ID != "mail:1" || got.Subject != "Project kickoff" || got.From != "alice@example.com" {
		t.Fatalf("row0=%+v", got)
	}
	if !got.Date.Equal(time.Date(2026, 6, 9, 14, 0, 0, 0, time.UTC)) {
		t.Fatalf("date=%v want 2026-06-09T14:00:00Z", got.Date)
	}
	if len(got.To) != 2 || got.To[0] != "bob@example.com" || got.To[1] != "carol@example.com" {
		t.Fatalf("to=%v want [bob carol]", got.To)
	}
	if len(got.Snippet) != 200 {
		t.Fatalf("snippet len=%d want 200 (truncation)", len(got.Snippet))
	}
	if items[1].Snippet != "Short body" {
		t.Fatalf("row1 snippet=%q want 'Short body'", items[1].Snippet)
	}
	if len(items[1].To) != 0 {
		t.Fatalf("row1 to=%v want empty", items[1].To)
	}
}

// Spec §4: every Apple SQLite opens mode=ro&immutable=1.
func TestMail_Query_ReadOnly(t *testing.T) {
	p := filepath.Join(t.TempDir(), "Envelope Index")
	seedDB(t, p)
	var seenDSN string
	m, err := New(Config{
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
	if _, err := m.Query(context.Background(), Query{}); err != nil {
		t.Fatalf("Query: %v", err)
	}
	for _, frag := range []string{"mode=ro", "immutable=1"} {
		if !strings.Contains(seenDSN, frag) {
			t.Fatalf("DSN %q missing %q", seenDSN, frag)
		}
	}
}

func TestMail_Query_SourceUnavailable(t *testing.T) {
	m, err := New(Config{
		DBPath:   filepath.Join(t.TempDir(), "absent"),
		Attestor: &fakeAttestor{},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = m.Query(context.Background(), Query{})
	if !errors.Is(err, ErrSourceUnavailable) {
		t.Fatalf("err=%v want ErrSourceUnavailable", err)
	}
}

func TestMail_Sync_NoOp(t *testing.T) {
	m, err := New(Config{DBPath: "/dev/null", Attestor: &fakeAttestor{}})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Sync(context.Background()); err != nil {
		t.Fatalf("Sync: %v", err)
	}
}

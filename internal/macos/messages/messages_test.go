package messages

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/trilam/leah/internal/audit"
)

type fakeAttestor struct {
	err   error
	calls int
}

func (f *fakeAttestor) Attest(_ context.Context, _ string) error {
	f.calls++
	return f.err
}

type captureAudit struct{ rows []audit.Entry }

func (c *captureAudit) Append(e audit.Entry) error { c.rows = append(c.rows, e); return nil }

// seedDB writes a minimal Messages chat.db schema (message + handle) to
// path. We use the ns-since-2001 convention to exercise macTime's modern
// branch.
func seedDB(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open seed db: %v", err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(`CREATE TABLE handle (
		ROWID INTEGER PRIMARY KEY,
		id TEXT
	)`); err != nil {
		t.Fatalf("create handle: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE message (
		ROWID INTEGER PRIMARY KEY,
		text TEXT,
		date INTEGER,
		handle_id INTEGER
	)`); err != nil {
		t.Fatalf("create message: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO handle VALUES (?,?),(?,?)`,
		1, "+15551234567",
		2, "friend@example.com",
	); err != nil {
		t.Fatalf("insert handle: %v", err)
	}
	t1 := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC).Sub(macEpoch).Nanoseconds()
	t2 := time.Date(2026, 6, 11, 17, 30, 0, 0, time.UTC).Sub(macEpoch).Nanoseconds()
	if _, err := db.Exec(
		`INSERT INTO message VALUES (?,?,?,?),(?,?,?,?)`,
		101, "see you at 9", t1, 1,
		102, "thanks for lunch", t2, 2,
	); err != nil {
		t.Fatalf("insert message: %v", err)
	}
}

func TestMessages_Name(t *testing.T) {
	m, err := New(Config{DBPath: "/dev/null", Attestor: &fakeAttestor{}})
	if err != nil {
		t.Fatal(err)
	}
	if got := m.Name(); got != "Messages" {
		t.Fatalf("Name=%q want Messages", got)
	}
}

func TestMessages_Available_FileMissing(t *testing.T) {
	m, err := New(Config{DBPath: filepath.Join(t.TempDir(), "missing"), Attestor: &fakeAttestor{}})
	if err != nil {
		t.Fatal(err)
	}
	if m.Available(context.Background()) {
		t.Fatal("Available=true for missing chat.db")
	}
}

// Denial must short-circuit BEFORE the DB is opened — high-sensitivity
// content makes the leaked-credential window unacceptable.
func TestMessages_Query_AttestationDenied_NoDBOpen(t *testing.T) {
	var opens int
	m, err := New(Config{
		DBPath:   filepath.Join(t.TempDir(), "chat.db"),
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

func TestMessages_Query_HappyPath(t *testing.T) {
	p := filepath.Join(t.TempDir(), "chat.db")
	seedDB(t, p)
	m, err := New(Config{DBPath: p, Attestor: &fakeAttestor{}})
	if err != nil {
		t.Fatal(err)
	}
	items, err := m.Query(context.Background(), Query{
		Since: time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC),
		Until: time.Date(2026, 6, 12, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len=%d want 2; items=%+v", len(items), items)
	}
	if items[0].ID != "messages:101" ||
		items[0].Body != "see you at 9" ||
		items[0].Source != "messages" {
		t.Fatalf("row0=%+v", items[0])
	}
	if !items[0].Timestamp.Equal(time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)) {
		t.Fatalf("ts=%v want 2026-06-10T09:00:00Z", items[0].Timestamp)
	}
	if len(items[0].ContactRefs) != 1 || items[0].ContactRefs[0] != "+15551234567" {
		t.Fatalf("contacts=%v", items[0].ContactRefs)
	}
}

// Spec §4: every Apple SQLite opens mode=ro&immutable=1.
func TestMessages_Query_ReadOnly(t *testing.T) {
	p := filepath.Join(t.TempDir(), "chat.db")
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

// Plaintext bodies must never reach audit.jsonl. The Detail field stores
// only the sha256[:8] hex fingerprint — a hard guarantee against the
// "reasoner exfiltration via audit replay" attack in spec §7.
func TestMessages_Audit_HashesBody(t *testing.T) {
	p := filepath.Join(t.TempDir(), "chat.db")
	seedDB(t, p)
	cap := &captureAudit{}
	m, err := New(Config{DBPath: p, Attestor: &fakeAttestor{}, Audit: cap})
	if err != nil {
		t.Fatal(err)
	}
	items, err := m.Query(context.Background(), Query{})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(items) != len(cap.rows) {
		t.Fatalf("audit rows=%d want %d", len(cap.rows), len(items))
	}
	for i, row := range cap.rows {
		want := hex.EncodeToString(func() []byte {
			s := sha256.Sum256([]byte(items[i].Body))
			return s[:]
		}())[:8]
		if !strings.Contains(row.Detail, "bodyhash="+want) {
			t.Fatalf("row %d detail=%q missing bodyhash=%s", i, row.Detail, want)
		}
		if strings.Contains(row.Detail, items[i].Body) {
			t.Fatalf("row %d detail=%q LEAKED plaintext body %q",
				i, row.Detail, items[i].Body)
		}
		if row.Kind != "macos_messages_query" {
			t.Fatalf("row %d kind=%q", i, row.Kind)
		}
	}
}

func TestMessages_Query_SourceUnavailable(t *testing.T) {
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

func TestMessages_Sync_NoOp(t *testing.T) {
	m, err := New(Config{DBPath: "/dev/null", Attestor: &fakeAttestor{}})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Sync(context.Background()); err != nil {
		t.Fatalf("Sync: %v", err)
	}
}

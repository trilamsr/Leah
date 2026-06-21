package contacts

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/trilam/leah/internal/macos/sqliteopen"
)

// fakeAttestor matches the facetime/imessage adapter pattern: scope-grain
// consent gate that the adapter MUST pass through before any DB open.
type fakeAttestor struct {
	called int
	scopes []string
	err    error
}

func (f *fakeAttestor) Attest(_ context.Context, scope string) error {
	f.called++
	f.scopes = append(f.scopes, scope)
	return f.err
}

// seedFixture builds an AddressBook-shaped SQLite at the given path with two
// records. Schema is the minimum the reader uses; real abcddb has many more
// columns but the adapter only reads the fields exercised here.
func seedFixture(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("seed open: %v", err)
	}
	defer func() { _ = db.Close() }()
	stmts := []string{
		`CREATE TABLE ZABCDRECORD (
			Z_PK INTEGER PRIMARY KEY,
			ZFIRSTNAME TEXT,
			ZLASTNAME TEXT,
			ZORGANIZATION TEXT
		)`,
		`CREATE TABLE ZABCDEMAILADDRESS (
			Z_PK INTEGER PRIMARY KEY,
			ZOWNER INTEGER,
			ZADDRESS TEXT
		)`,
		`CREATE TABLE ZABCDPHONENUMBER (
			Z_PK INTEGER PRIMARY KEY,
			ZOWNER INTEGER,
			ZFULLNUMBER TEXT
		)`,
		`INSERT INTO ZABCDRECORD VALUES (1, 'Alice', 'Adams', NULL)`,
		`INSERT INTO ZABCDRECORD VALUES (2, NULL, NULL, 'Acme Co')`,
		`INSERT INTO ZABCDEMAILADDRESS VALUES (10, 1, 'alice@example.com')`,
		`INSERT INTO ZABCDPHONENUMBER VALUES (20, 1, '+15551234567')`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("seed exec %q: %v", s, err)
		}
	}
}

func newTestAdapter(t *testing.T, att Attestor, dbPath string) *Adapter {
	t.Helper()
	a, err := New(Config{Attestor: att, DBPath: dbPath})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return a
}

func TestContacts_Name(t *testing.T) {
	t.Parallel()
	a := newTestAdapter(t, &fakeAttestor{}, "/does/not/matter")
	if got := a.Name(); got != "Contacts" {
		t.Fatalf("Name()=%q, want %q", got, "Contacts")
	}
}

func TestContacts_Available_FileMissing(t *testing.T) {
	t.Parallel()
	a := newTestAdapter(t, &fakeAttestor{}, filepath.Join(t.TempDir(), "absent.abcddb"))
	if a.Available(context.Background()) {
		t.Fatal("Available()=true for missing file, want false")
	}
}

func TestContacts_Query_AttestationDenied_NoDBOpen(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "AddressBook-v22.abcddb")
	seedFixture(t, path)

	att := &fakeAttestor{err: errors.New("denied")}
	a := newTestAdapter(t, att, path)

	got, err := a.Query(context.Background(), Query{Limit: 10})
	if !errors.Is(err, ErrAttestationDenied) {
		t.Fatalf("err=%v, want ErrAttestationDenied", err)
	}
	if got != nil {
		t.Fatalf("items=%v, want nil on denial", got)
	}
	if att.called != 1 || att.scopes[0] != ScopeQuery {
		t.Fatalf("scopes=%v called=%d, want one %q invocation", att.scopes, att.called, ScopeQuery)
	}
}

func TestContacts_Query_HappyPath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "AddressBook-v22.abcddb")
	seedFixture(t, path)

	a := newTestAdapter(t, &fakeAttestor{}, path)
	items, err := a.Query(context.Background(), Query{Limit: 10})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("items=%d, want 2", len(items))
	}

	byID := map[string]Item{}
	for _, it := range items {
		byID[it.ID] = it
	}
	alice, ok := byID["contacts:1"]
	if !ok {
		t.Fatalf("missing contacts:1; got %v", byID)
	}
	if alice.Title != "Alice Adams" {
		t.Fatalf("alice.Title=%q, want %q", alice.Title, "Alice Adams")
	}
	if alice.Source != "contacts" {
		t.Fatalf("alice.Source=%q, want contacts", alice.Source)
	}
	wantRefs := map[string]bool{"alice@example.com": false, "+15551234567": false}
	for _, r := range alice.ContactRefs {
		if _, want := wantRefs[r]; !want {
			t.Fatalf("unexpected ref %q", r)
		}
		wantRefs[r] = true
	}
	for r, seen := range wantRefs {
		if !seen {
			t.Fatalf("missing ref %q", r)
		}
	}

	acme, ok := byID["contacts:2"]
	if !ok {
		t.Fatalf("missing contacts:2; got %v", byID)
	}
	if acme.Title != "Acme Co" {
		t.Fatalf("acme.Title=%q, want %q", acme.Title, "Acme Co")
	}
}

func TestContacts_Query_ReadOnly(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "AddressBook-v22.abcddb")
	seedFixture(t, path)

	a := newTestAdapter(t, &fakeAttestor{}, path)
	if _, err := a.Query(context.Background(), Query{Limit: 10}); err != nil {
		t.Fatalf("Query: %v", err)
	}

	// Direct write against the adapter's own handle would defeat the test
	// (the adapter could open RW and tests would pass). Instead, attempt the
	// write through a separate read-only handle opened with the exact DSN the
	// adapter uses; if that handle can mutate, the adapter's DSN is wrong.
	db, err := sql.Open("sqlite", sqliteopen.RODSN(path))
	if err != nil {
		t.Fatalf("ro open: %v", err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(`INSERT INTO ZABCDRECORD VALUES (3, 'Mallory', 'M', NULL)`); err == nil {
		t.Fatal("write succeeded against mode=ro DSN; adapter is not opening read-only")
	}
}

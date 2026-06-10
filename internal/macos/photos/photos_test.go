package photos

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

// seedDB writes a minimal ZASSET fixture. Photos.sqlite is a CoreData
// store; ZASSET holds one row per master image with ZUUID (asset GUID),
// ZDATECREATED (Apple-epoch REAL), and a join to ZADDITIONALASSETATTRIBUTES
// for filename. Location lives on ZASSET as ZLATITUDE / ZLONGITUDE.
func seedDB(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open seed db: %v", err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(`CREATE TABLE ZASSET (
		Z_PK INTEGER PRIMARY KEY,
		ZUUID TEXT,
		ZDATECREATED REAL,
		ZLATITUDE REAL,
		ZLONGITUDE REAL,
		ZADDITIONALATTRIBUTES INTEGER
	)`); err != nil {
		t.Fatalf("create ZASSET: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE ZADDITIONALASSETATTRIBUTES (
		Z_PK INTEGER PRIMARY KEY,
		ZORIGINALFILENAME TEXT
	)`); err != nil {
		t.Fatalf("create ZADDITIONAL: %v", err)
	}
	t1 := time.Date(2026, 6, 9, 14, 0, 0, 0, time.UTC).Sub(macEpoch).Seconds()
	t2 := time.Date(2026, 6, 10, 8, 15, 0, 0, time.UTC).Sub(macEpoch).Seconds()
	if _, err := db.Exec(
		`INSERT INTO ZADDITIONALASSETATTRIBUTES VALUES (10, ?), (20, ?)`,
		"IMG_0001.HEIC", "IMG_0002.JPG",
	); err != nil {
		t.Fatalf("insert ZADDITIONAL: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO ZASSET VALUES (1,?,?,?,?,?), (2,?,?,?,?,?)`,
		"UUID-AAA", t1, 37.7749, -122.4194, 10,
		"UUID-BBB", t2, nil, nil, 20,
	); err != nil {
		t.Fatalf("insert ZASSET: %v", err)
	}
}

func TestPhotos_Name(t *testing.T) {
	p, err := New(Config{DBPath: "/dev/null", Attestor: &fakeAttestor{}})
	if err != nil {
		t.Fatal(err)
	}
	if got := p.Name(); got != "Photos" {
		t.Fatalf("Name=%q want Photos", got)
	}
}

func TestPhotos_Available_FileMissing(t *testing.T) {
	p, err := New(Config{DBPath: filepath.Join(t.TempDir(), "missing"), Attestor: &fakeAttestor{}})
	if err != nil {
		t.Fatal(err)
	}
	if p.Available(context.Background()) {
		t.Fatal("Available=true for missing file")
	}
}

// Denial must short-circuit BEFORE the DB is opened — Photos.sqlite is
// HIGHEST sensitivity per spec §7; a denied operator may not attach the
// store at all.
func TestPhotos_Query_AttestationDenied_NoDBOpen(t *testing.T) {
	var opens int
	p, err := New(Config{
		DBPath:   filepath.Join(t.TempDir(), "photos.sqlite"),
		Attestor: &fakeAttestor{err: errors.New("no")},
		Open: func(driver, dsn string) (*sql.DB, error) {
			opens++
			return sql.Open(driver, dsn)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Query(context.Background(), Query{})
	if !errors.Is(err, ErrAttestationDenied) {
		t.Fatalf("err=%v want ErrAttestationDenied", err)
	}
	if opens != 0 {
		t.Fatalf("DB opened %d times after denial", opens)
	}
}

func TestPhotos_Query_HappyPath(t *testing.T) {
	p := filepath.Join(t.TempDir(), "photos.sqlite")
	seedDB(t, p)
	ph, err := New(Config{DBPath: p, Attestor: &fakeAttestor{}})
	if err != nil {
		t.Fatal(err)
	}
	items, err := ph.Query(context.Background(), Query{
		Since: time.Date(2026, 6, 9, 0, 0, 0, 0, time.UTC),
		Until: time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len=%d want 2; items=%+v", len(items), items)
	}
	got0 := items[0]
	if got0.UUID != "UUID-AAA" {
		t.Fatalf("row0 uuid=%q want UUID-AAA", got0.UUID)
	}
	if got0.ID != "photos:UUID-AAA" {
		t.Fatalf("row0 id=%q want photos:UUID-AAA", got0.ID)
	}
	if got0.Filename != "IMG_0001.HEIC" {
		t.Fatalf("row0 filename=%q", got0.Filename)
	}
	if !got0.Timestamp.Equal(time.Date(2026, 6, 9, 14, 0, 0, 0, time.UTC)) {
		t.Fatalf("row0 ts=%v want 2026-06-09T14:00:00Z", got0.Timestamp)
	}
	if got0.Latitude == nil || got0.Longitude == nil {
		t.Fatalf("row0 lat/lng nil: %+v", got0)
	}
	if *got0.Latitude != 37.7749 || *got0.Longitude != -122.4194 {
		t.Fatalf("row0 lat/lng=%v/%v want 37.7749/-122.4194", *got0.Latitude, *got0.Longitude)
	}
	// Row 2: NULL lat/lng → "no GPS fix" → nil pointers.
	got1 := items[1]
	if got1.Latitude != nil || got1.Longitude != nil {
		t.Fatalf("row1 should have nil lat/lng (NULL=no fix), got %v/%v",
			ptrOr(got1.Latitude), ptrOr(got1.Longitude))
	}
}

func ptrOr(p *float64) float64 {
	if p == nil {
		return -999
	}
	return *p
}

// Spec §4: every Apple SQLite opens mode=ro&immutable=1.
func TestPhotos_Query_ReadOnly(t *testing.T) {
	p := filepath.Join(t.TempDir(), "photos.sqlite")
	seedDB(t, p)
	var seenDSN string
	ph, err := New(Config{
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
	if _, err := ph.Query(context.Background(), Query{}); err != nil {
		t.Fatalf("Query: %v", err)
	}
	for _, frag := range []string{"mode=ro", "immutable=1"} {
		if !strings.Contains(seenDSN, frag) {
			t.Fatalf("DSN %q missing %q", seenDSN, frag)
		}
	}
}

// Spec §7: Photos audit row MUST hash UUID and MUST NOT contain lat/lng
// plaintext. A reversible UUID or a plaintext coordinate in audit.jsonl
// reopens the "reasoner exfiltration via audit replay" attack.
func TestPhotos_Audit_HashesUUID_NoLatLng(t *testing.T) {
	p := filepath.Join(t.TempDir(), "photos.sqlite")
	seedDB(t, p)
	cap := &captureAudit{}
	ph, err := New(Config{DBPath: p, Attestor: &fakeAttestor{}, Audit: cap})
	if err != nil {
		t.Fatal(err)
	}
	items, err := ph.Query(context.Background(), Query{})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(items) != len(cap.rows) {
		t.Fatalf("audit rows=%d want %d", len(cap.rows), len(items))
	}
	for i, row := range cap.rows {
		sum := sha256.Sum256([]byte(items[i].UUID))
		want := hex.EncodeToString(sum[:])[:8]
		if !strings.Contains(row.Detail, "uuidhash="+want) {
			t.Fatalf("row %d detail=%q missing uuidhash=%s", i, row.Detail, want)
		}
		if strings.Contains(row.Detail, items[i].UUID) {
			t.Fatalf("row %d detail=%q LEAKED plaintext UUID %q",
				i, row.Detail, items[i].UUID)
		}
		// Lat/lng plaintext must never appear in audit.
		if items[i].Latitude != nil {
			coord := "37.7749"
			if strings.Contains(row.Detail, coord) {
				t.Fatalf("row %d detail=%q LEAKED latitude", i, row.Detail)
			}
		}
		if items[i].Longitude != nil {
			coord := "-122.4194"
			if strings.Contains(row.Detail, coord) {
				t.Fatalf("row %d detail=%q LEAKED longitude", i, row.Detail)
			}
		}
		if row.Kind != "macos_photos_query" {
			t.Fatalf("row %d kind=%q", i, row.Kind)
		}
	}
}

func TestPhotos_Query_SourceUnavailable(t *testing.T) {
	ph, err := New(Config{
		DBPath:   filepath.Join(t.TempDir(), "absent"),
		Attestor: &fakeAttestor{},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = ph.Query(context.Background(), Query{})
	if !errors.Is(err, ErrSourceUnavailable) {
		t.Fatalf("err=%v want ErrSourceUnavailable", err)
	}
}

func TestPhotos_Sync_NoOp(t *testing.T) {
	ph, err := New(Config{DBPath: "/dev/null", Attestor: &fakeAttestor{}})
	if err != nil {
		t.Fatal(err)
	}
	if err := ph.Sync(context.Background()); err != nil {
		t.Fatalf("Sync: %v", err)
	}
}

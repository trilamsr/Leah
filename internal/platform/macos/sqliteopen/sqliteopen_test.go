package sqliteopen

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestRODSN_Fragments(t *testing.T) {
	got := RODSN("/Library/Calendars/Calendar Cache")
	for _, frag := range []string{"mode=ro", "immutable=1", "_journal_mode=OFF", "_query_only=1", "Calendar%20Cache"} {
		if !strings.Contains(got, frag) {
			t.Fatalf("DSN %q missing %q", got, frag)
		}
	}
}

// A handle opened via RODSN must reject writes — the DSN is the only thing
// standing between us and a clobbered Apple store.
func TestRODSN_RejectsWrite(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.db")
	seed, err := sql.Open("sqlite", "file:"+p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := seed.Exec("CREATE TABLE t(a INTEGER)"); err != nil {
		t.Fatal(err)
	}
	_ = seed.Close()

	db, err := sql.Open("sqlite", RODSN(p))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec("INSERT INTO t VALUES (1)"); err == nil {
		t.Fatal("write succeeded against RODSN; not read-only")
	}
	_ = os.Remove(p)
}

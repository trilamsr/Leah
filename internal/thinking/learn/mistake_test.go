package learn

import (
	"path/filepath"
	"testing"
	"time"
)

// TestMistakeAddRoundtrip asserts that Add stores a row that List retrieves
// with all fields intact (spec §3.1 + §3.2).
func TestMistakeAddRoundtrip(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "memory.db")
	store, err := OpenMistakeStore(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = store.Close() }()

	created := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	store.Now = func() time.Time { return created }

	in := Mistake{
		AuditTS:    "2026-06-03T10:00:00Z",
		AuditKind:  "ship",
		AuditHash:  "abc",
		RootCause:  "wrong-pr",
		Prevention: "double-check the repo arg before dispatching",
	}
	id, err := store.Add(in)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if id == "" {
		t.Fatal("Add: empty id")
	}

	rows, err := store.List(ListOptions{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}
	got := rows[0]
	if got.ID != id {
		t.Errorf("id: want %q, got %q", id, got.ID)
	}
	if !got.CreatedAt.Equal(created) {
		t.Errorf("createdAt: want %s, got %s", created, got.CreatedAt)
	}
	if got.AuditTS != in.AuditTS || got.AuditKind != in.AuditKind || got.AuditHash != in.AuditHash {
		t.Errorf("audit-ref roundtrip mismatch: %+v vs %+v", got, in)
	}
	if got.RootCause != in.RootCause || got.Prevention != in.Prevention {
		t.Errorf("payload roundtrip mismatch: %+v vs %+v", got, in)
	}
}

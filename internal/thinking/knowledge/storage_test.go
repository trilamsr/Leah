package knowledge

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func statFile(p string) (fs.FileInfo, error) { return os.Stat(p) }

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	return string(b)
}

func TestStorage_OpenIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "knowledge.db")
	s1, err := openStorage(path)
	if err != nil {
		t.Fatalf("open1: %v", err)
	}
	_ = s1.Close()
	s2, err := openStorage(path)
	if err != nil {
		t.Fatalf("open2: %v", err)
	}
	t.Cleanup(func() { _ = s2.Close() })
}

func TestStorage_SchemaVersionGuard(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "knowledge.db")
	s, err := openStorage(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := s.db.Exec(`UPDATE schema_meta SET value='999' WHERE key='version'`); err != nil {
		t.Fatalf("bump: %v", err)
	}
	_ = s.Close()
	if _, err := openStorage(path); err == nil {
		t.Fatal("expected open to fail with newer-than-binary schema")
	}
}

func TestStorage_GetEntity_Unknown(t *testing.T) {
	dir := t.TempDir()
	s, err := openStorage(filepath.Join(dir, "knowledge.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if _, err := s.getEntity(KindPerson, "missing"); !errors.Is(err, ErrUnknownEntity) {
		t.Fatalf("want ErrUnknownEntity, got %v", err)
	}
}

func TestStorage_ParentDirCreated(t *testing.T) {
	dir := t.TempDir()
	// Two levels deep so MkdirAll's effect is observable.
	path := filepath.Join(dir, "nested", "deeper", "knowledge.db")
	s, err := openStorage(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	// Parent should be the SQLite file itself (not the dir mode check —
	// tmpdir umask may interfere — but its existence and file 0600 are
	// the contract).
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat db: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("want 0600, got %#o", info.Mode().Perm())
	}
}

func TestStorage_DeleteEntity_ReturnsFalseWhenMissing(t *testing.T) {
	s, err := openStorage(filepath.Join(t.TempDir(), "knowledge.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	ok, err := s.deleteEntity(KindPerson, "missing")
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if ok {
		t.Fatal("want false for missing entity")
	}
}

func TestStorage_UpsertEntity_UpdatesLastTouched(t *testing.T) {
	s, err := openStorage(filepath.Join(t.TempDir(), "knowledge.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	t0 := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Hour)
	e := Entity{Kind: KindPerson, Key: "k", Display: "k"}
	if err := s.upsertEntity(e, t0); err != nil {
		t.Fatalf("upsert1: %v", err)
	}
	if err := s.upsertEntity(e, t1); err != nil {
		t.Fatalf("upsert2: %v", err)
	}
	got, err := s.getEntity(KindPerson, "k")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !got.LastTouched.Equal(t1) {
		t.Fatalf("last_touched: want %v, got %v", t1, got.LastTouched)
	}
}

func TestSearchRelevantTopK(t *testing.T) {
	s, err := openStorage(filepath.Join(t.TempDir(), "knowledge.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	ctx := context.Background()
	chunks, err := s.SearchRelevant(ctx, "test query", 5)
	if err != nil {
		t.Fatalf("SearchRelevant: %v", err)
	}
	if len(chunks) != 0 {
		t.Fatalf("want 0 chunks on empty store, got %d", len(chunks))
	}
}

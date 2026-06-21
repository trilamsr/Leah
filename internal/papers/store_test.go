package papers

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestSave_PersistsAndDedupes(t *testing.T) {
	dir := t.TempDir()
	s := &Store{Path: filepath.Join(dir, "papers.jsonl")}

	p := Paper{
		ID:       "2406.12345",
		Title:    "A Paper",
		Abstract: "abstract body",
		Authors:  []string{"Alice", "Bob"},
		Link:     "https://arxiv.org/abs/2406.12345",
		SavedTS:  time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC),
		Status:   StatusUnread,
	}
	if err := s.Save(p); err != nil {
		t.Fatalf("Save: %v", err)
	}
	// Dedup on second Save with same ID — original SavedTS preserved.
	p2 := p
	p2.SavedTS = p.SavedTS.Add(time.Hour)
	if err := s.Save(p2); err != nil {
		t.Fatalf("Save dup: %v", err)
	}

	got, err := s.List("")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 paper after dedup, got %d", len(got))
	}
	if !got[0].SavedTS.Equal(p.SavedTS) {
		t.Fatalf("dedup should preserve original SavedTS; got %v want %v", got[0].SavedTS, p.SavedTS)
	}
}

func TestList_FiltersByStatus(t *testing.T) {
	dir := t.TempDir()
	s := &Store{Path: filepath.Join(dir, "papers.jsonl")}

	now := time.Now().UTC()
	_ = s.Save(Paper{ID: "1", Title: "u1", Status: StatusUnread, SavedTS: now})
	_ = s.Save(Paper{ID: "2", Title: "r1", Status: StatusRead, SavedTS: now})
	_ = s.Save(Paper{ID: "3", Title: "u2", Status: StatusUnread, SavedTS: now})

	all, _ := s.List("")
	if len(all) != 3 {
		t.Fatalf("List(all) want 3 got %d", len(all))
	}
	unread, _ := s.List(StatusUnread)
	if len(unread) != 2 {
		t.Fatalf("List(unread) want 2 got %d", len(unread))
	}
	ids := []string{unread[0].ID, unread[1].ID}
	sort.Strings(ids)
	if ids[0] != "1" || ids[1] != "3" {
		t.Fatalf("unread filter wrong: %v", ids)
	}
}

func TestList_MissingFileEmpty(t *testing.T) {
	s := &Store{Path: filepath.Join(t.TempDir(), "missing.jsonl")}
	got, err := s.List("")
	if err != nil {
		t.Fatalf("List on missing: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("want empty, got %d", len(got))
	}
}

func TestSetStatus(t *testing.T) {
	dir := t.TempDir()
	s := &Store{Path: filepath.Join(dir, "papers.jsonl")}
	now := time.Now().UTC()
	_ = s.Save(Paper{ID: "2406.12345", Title: "p", Status: StatusUnread, SavedTS: now})

	if err := s.SetStatus("2406.12345", StatusRead); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	got, _ := s.List("")
	if got[0].Status != StatusRead {
		t.Fatalf("status not updated: %v", got[0].Status)
	}
}

func TestSetStatus_UnknownIDError(t *testing.T) {
	s := &Store{Path: filepath.Join(t.TempDir(), "papers.jsonl")}
	if err := s.SetStatus("nope", StatusRead); err == nil {
		t.Fatalf("expected error on unknown ID")
	}
}

func TestSetStatus_RejectsInvalidEnum(t *testing.T) {
	dir := t.TempDir()
	s := &Store{Path: filepath.Join(dir, "papers.jsonl")}
	_ = s.Save(Paper{ID: "1", Title: "p", Status: StatusUnread, SavedTS: time.Now().UTC()})
	if err := s.SetStatus("1", "garbage"); err == nil {
		t.Fatalf("expected error on invalid status enum")
	}
}

// On-disk format is one JSON object per line — pin the contract so a future
// consumer (brief, dashboard) can stream-parse without loading the array.
func TestSave_JSONLOnDiskFormat(t *testing.T) {
	dir := t.TempDir()
	s := &Store{Path: filepath.Join(dir, "papers.jsonl")}
	_ = s.Save(Paper{ID: "1", Title: "a", Status: StatusUnread, SavedTS: time.Now().UTC()})
	_ = s.Save(Paper{ID: "2", Title: "b", Status: StatusUnread, SavedTS: time.Now().UTC()})

	b, err := os.ReadFile(s.Path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %d: %q", len(lines), string(b))
	}
	for i, ln := range lines {
		if !strings.HasPrefix(ln, "{") || !strings.HasSuffix(ln, "}") {
			t.Fatalf("line %d not a JSON object: %q", i, ln)
		}
	}
}

// Atomic rename leaves no .tmp turds in the state dir — a half-written
// papers.jsonl read by the morning brief would crash the synth.
func TestSave_AtomicNoTempLeftover(t *testing.T) {
	dir := t.TempDir()
	s := &Store{Path: filepath.Join(dir, "papers.jsonl")}
	_ = s.Save(Paper{ID: "1", Title: "a", Status: StatusUnread, SavedTS: time.Now().UTC()})

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if e.Name() != "papers.jsonl" {
			t.Fatalf("unexpected leftover %q", e.Name())
		}
	}
}

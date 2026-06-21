package watchlist

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func TestAddNormalizesAndDedupes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "watchlist.json")

	s := &Store{Path: path}
	if err := s.Add("aapl"); err != nil {
		t.Fatalf("Add aapl: %v", err)
	}
	if err := s.Add("AAPL"); err != nil {
		t.Fatalf("Add AAPL (dup): %v", err)
	}
	if err := s.Add("msft"); err != nil {
		t.Fatalf("Add msft: %v", err)
	}

	got, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	sort.Strings(got)
	want := []string{"AAPL", "MSFT"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}

	// Verify on-disk shape matches the morning-brief consumer's contract.
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	var doc struct {
		Symbols []string `json:"symbols"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	sort.Strings(doc.Symbols)
	if !reflect.DeepEqual(doc.Symbols, want) {
		t.Fatalf("on-disk symbols: got %v want %v", doc.Symbols, want)
	}
}

func TestAddRejectsInvalid(t *testing.T) {
	s := &Store{Path: filepath.Join(t.TempDir(), "w.json")}
	cases := []string{"", "TOOLONG", "AB1", "ab-c", "  "}
	for _, c := range cases {
		if err := s.Add(c); err == nil {
			t.Fatalf("Add(%q) expected error, got nil", c)
		}
	}
}

func TestRemove(t *testing.T) {
	s := &Store{Path: filepath.Join(t.TempDir(), "w.json")}
	_ = s.Add("AAPL")
	_ = s.Add("MSFT")
	if err := s.Remove("aapl"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	got, _ := s.List()
	if !reflect.DeepEqual(got, []string{"MSFT"}) {
		t.Fatalf("after remove got %v", got)
	}
	// Removing a non-member is a no-op (silent).
	if err := s.Remove("GOOG"); err != nil {
		t.Fatalf("Remove non-member: %v", err)
	}
}

func TestListEmptyMissingFile(t *testing.T) {
	s := &Store{Path: filepath.Join(t.TempDir(), "missing.json")}
	got, err := s.List()
	if err != nil {
		t.Fatalf("List missing: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty, got %v", got)
	}
}

func TestAtomicWriteNoTempLeftover(t *testing.T) {
	dir := t.TempDir()
	s := &Store{Path: filepath.Join(dir, "w.json")}
	if err := s.Add("AAPL"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if e.Name() != "w.json" {
			t.Fatalf("unexpected leftover %q", e.Name())
		}
	}
}

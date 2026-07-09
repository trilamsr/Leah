package monthly

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Charge persists across reopen; boot recovers totals via the same path.
func TestCostMonth_BootRecoversTotals(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cost-month.json")
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)

	s, err := OpenAt(path, 50.0, now)
	if err != nil {
		t.Fatalf("OpenAt: %v", err)
	}
	if err := s.Charge("reasoner", 1.50); err != nil {
		t.Fatalf("Charge: %v", err)
	}
	if err := s.Charge("judge", 0.25); err != nil {
		t.Fatalf("Charge: %v", err)
	}

	s2, err := OpenAt(path, 50.0, now)
	if err != nil {
		t.Fatalf("OpenAt reopen: %v", err)
	}
	if got := s2.Spent(); got != 1.75 {
		t.Errorf("Spent reopen = %v, want 1.75", got)
	}
}

// YM mismatch on Open archives the prior month and resets totals.
func TestCostMonth_RolloverAtBoundary(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cost-month.json")

	june := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	s, err := OpenAt(path, 50.0, june)
	if err != nil {
		t.Fatalf("OpenAt June: %v", err)
	}
	if err := s.Charge("reasoner", 4.20); err != nil {
		t.Fatalf("Charge: %v", err)
	}

	july := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	s2, err := OpenAt(path, 50.0, july)
	if err != nil {
		t.Fatalf("OpenAt July: %v", err)
	}
	if got := s2.Spent(); got != 0 {
		t.Errorf("Spent post-rollover = %v, want 0", got)
	}

	archiveDir := filepath.Join(dir, "cost-month-archive")
	archive := filepath.Join(archiveDir, "2026-06.json")
	if _, err := os.Stat(archive); err != nil {
		t.Fatalf("archive missing: %v", err)
	}
	var arch state
	b, err := os.ReadFile(archive)
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}
	if err := json.Unmarshal(b, &arch); err != nil {
		t.Fatalf("unmarshal archive: %v", err)
	}
	if arch.TotalsByKind["reasoner"] != 4.20 {
		t.Errorf("archive reasoner total = %v, want 4.20", arch.TotalsByKind["reasoner"])
	}
}

// Concurrent charges produce a final file whose totals match the sum of
// successful charges — tmp+rename never leaves a half-written file.
func TestCostMonth_AtomicWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cost-month.json")
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)

	s, err := OpenAt(path, 100.0, now)
	if err != nil {
		t.Fatalf("OpenAt: %v", err)
	}

	const N = 200
	done := make(chan struct{}, N)
	for i := 0; i < N; i++ {
		go func() {
			_ = s.Charge("reasoner", 0.01)
			done <- struct{}{}
		}()
	}
	for i := 0; i < N; i++ {
		<-done
	}

	// File must parse cleanly (no torn write) and totals must match.
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var st state
	if err := json.Unmarshal(b, &st); err != nil {
		t.Fatalf("Unmarshal: %v (file=%q)", err, string(b))
	}
	want := 0.01 * float64(N)
	got := st.TotalsByKind["reasoner"]
	if diff := got - want; diff > 1e-6 || diff < -1e-6 {
		t.Errorf("file total = %v, want %v", got, want)
	}
}

// Corrupt JSON on disk is archived and the store starts fresh — no panic,
// no operator data loss (the corrupt file is preserved for forensics).
func TestCostMonth_BootArchivesCorrupt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cost-month.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	s, err := OpenAt(path, 50.0, now)
	if err != nil {
		t.Fatalf("OpenAt: %v", err)
	}
	if got := s.Spent(); got != 0 {
		t.Errorf("Spent fresh = %v, want 0", got)
	}
	// Corrupt file moved aside; primary path now holds a valid fresh state.
	matches, _ := filepath.Glob(filepath.Join(dir, "cost-month-corrupt-*.json"))
	if len(matches) == 0 {
		t.Error("expected cost-month-corrupt-*.json sidecar after corrupt boot")
	}
}

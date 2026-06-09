package operatormodel

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/memory"
)

// writeAudit appends RFC3339-stamped entries to a fresh audit.jsonl.
func writeAudit(t *testing.T, dir string, entries []audit.Entry) string {
	t.Helper()
	path := filepath.Join(dir, "audit.jsonl")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create audit: %v", err)
	}
	defer func() { _ = f.Close() }()
	enc := json.NewEncoder(f)
	for _, e := range entries {
		if err := enc.Encode(e); err != nil {
			t.Fatalf("encode: %v", err)
		}
	}
	return path
}

// TestProfileUpdateColdStartBelowRowFloor asserts profile is NOT Ready
// when row count under 50, regardless of day span.
func TestProfileUpdateColdStartBelowRowFloor(t *testing.T) {
	dir := t.TempDir()
	store, err := memory.NewStore(filepath.Join(dir, "memory.db"))
	if err != nil {
		t.Fatalf("memory: %v", err)
	}
	defer func() { _ = store.Close() }()

	var entries []audit.Entry
	// 30 rows spread over 10 days — passes days gate, fails rows gate.
	base := time.Now().UTC().Add(-10 * 24 * time.Hour)
	for i := 0; i < 30; i++ {
		entries = append(entries, audit.Entry{
			Timestamp: base.Add(time.Duration(i*8) * time.Hour).Format(time.RFC3339),
			Kind:      "leah.ship",
		})
	}
	path := writeAudit(t, dir, entries)

	p := &Profile{Now: func() time.Time { return time.Now().UTC() }}
	if err := p.Update(context.Background(), store, path, base.Add(-time.Hour)); err != nil {
		t.Fatalf("update: %v", err)
	}
	if p.Ready {
		t.Errorf("want Ready=false at 30 rows; got true (rows=%d days=%d)", p.RowsObserved, p.DaysObserved)
	}
}

// TestProfileUpdateColdStartBelowDayFloor asserts profile is NOT Ready
// when day span under 7, regardless of row count.
func TestProfileUpdateColdStartBelowDayFloor(t *testing.T) {
	dir := t.TempDir()
	store, err := memory.NewStore(filepath.Join(dir, "memory.db"))
	if err != nil {
		t.Fatalf("memory: %v", err)
	}
	defer func() { _ = store.Close() }()

	var entries []audit.Entry
	// 60 rows in 2 days — passes rows gate, fails days gate.
	base := time.Now().UTC().Add(-2 * 24 * time.Hour)
	for i := 0; i < 60; i++ {
		entries = append(entries, audit.Entry{
			Timestamp: base.Add(time.Duration(i*30) * time.Minute).Format(time.RFC3339),
			Kind:      "leah.ship",
		})
	}
	path := writeAudit(t, dir, entries)

	p := &Profile{Now: func() time.Time { return time.Now().UTC() }}
	if err := p.Update(context.Background(), store, path, base.Add(-time.Hour)); err != nil {
		t.Fatalf("update: %v", err)
	}
	if p.Ready {
		t.Errorf("want Ready=false at 2-day span; got true (rows=%d days=%d)", p.RowsObserved, p.DaysObserved)
	}
}

// TestProfileUpdateReadyAboveBothFloors asserts profile is Ready when
// both 50-row + 7-day gates pass, and rows landed in operator_profile.
func TestProfileUpdateReadyAboveBothFloors(t *testing.T) {
	dir := t.TempDir()
	store, err := memory.NewStore(filepath.Join(dir, "memory.db"))
	if err != nil {
		t.Fatalf("memory: %v", err)
	}
	defer func() { _ = store.Close() }()

	var entries []audit.Entry
	base := time.Now().UTC().Add(-8 * 24 * time.Hour)
	for i := 0; i < 60; i++ {
		entries = append(entries, audit.Entry{
			Timestamp: base.Add(time.Duration(i*3) * time.Hour).Format(time.RFC3339),
			Kind:      "leah.ship",
		})
	}
	path := writeAudit(t, dir, entries)

	p := &Profile{Now: func() time.Time { return time.Now().UTC() }}
	if err := p.Update(context.Background(), store, path, base.Add(-time.Hour)); err != nil {
		t.Fatalf("update: %v", err)
	}
	if !p.Ready {
		t.Errorf("want Ready=true above both floors; got false (rows=%d days=%d)", p.RowsObserved, p.DaysObserved)
	}
	if len(p.Rows) == 0 {
		t.Errorf("expected at least one ProfileRow after Update; got 0")
	}

	// operator_profile table should hold rows.
	var n int
	if err := store.DB().QueryRow(`SELECT COUNT(*) FROM operator_profile`).Scan(&n); err != nil {
		t.Fatalf("count operator_profile: %v", err)
	}
	if n == 0 {
		t.Errorf("expected operator_profile rows; got 0")
	}
}

// TestProfileUpdateRebuildsFromScratch asserts a second Update wipes
// stale rows from prior runs (rebuild-from-scratch contract §3.2).
func TestProfileUpdateRebuildsFromScratch(t *testing.T) {
	dir := t.TempDir()
	store, err := memory.NewStore(filepath.Join(dir, "memory.db"))
	if err != nil {
		t.Fatalf("memory: %v", err)
	}
	defer func() { _ = store.Close() }()

	// Run 1: ship rows.
	var ent1 []audit.Entry
	base := time.Now().UTC().Add(-8 * 24 * time.Hour)
	for i := 0; i < 60; i++ {
		ent1 = append(ent1, audit.Entry{
			Timestamp: base.Add(time.Duration(i*3) * time.Hour).Format(time.RFC3339),
			Kind:      "leah.ship",
		})
	}
	path := writeAudit(t, dir, ent1)
	p := &Profile{Now: func() time.Time { return time.Now().UTC() }}
	if err := p.Update(context.Background(), store, path, base.Add(-time.Hour)); err != nil {
		t.Fatalf("update1: %v", err)
	}

	// Run 2: only ask rows (ship rows should NOT survive).
	var ent2 []audit.Entry
	for i := 0; i < 60; i++ {
		ent2 = append(ent2, audit.Entry{
			Timestamp: base.Add(time.Duration(i*3) * time.Hour).Format(time.RFC3339),
			Kind:      "leah.ask",
		})
	}
	path2 := writeAudit(t, dir, ent2)
	if err := p.Update(context.Background(), store, path2, base.Add(-time.Hour)); err != nil {
		t.Fatalf("update2: %v", err)
	}
	var n int
	if err := store.DB().QueryRow(
		`SELECT COUNT(*) FROM operator_profile WHERE key='leah.ship'`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Errorf("rebuild should wipe stale leah.ship rows; got %d", n)
	}
}

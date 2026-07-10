package operatormodel

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/trilam/leah/internal/platform/audit"
	"github.com/trilam/leah/internal/platform/activectx"
	"github.com/trilam/leah/internal/memory/store"
)

// fakeSwitchSource feeds canned switches to Profile.Update without a real ctxmgr DB.
type fakeSwitchSource struct {
	switches []activectx.Switch
	calls    int
}

func (f *fakeSwitchSource) Since(_ time.Time) ([]activectx.Switch, error) {
	f.calls++
	return f.switches, nil
}

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

// TestLoad_ReadsPersistedProfile asserts Load reads back the profile
// (meta counters + rows) that Update persisted, across a Close+reopen
// of the underlying store — mirrors the suggest.go cold-read path.
func TestLoad_ReadsPersistedProfile(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "memory.db")
	store, err := memory.NewStore(dbPath)
	if err != nil {
		t.Fatalf("memory: %v", err)
	}

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
	wantRows := p.RowsObserved
	wantDays := p.DaysObserved
	wantProfileRows := len(p.Rows)
	if err := store.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	store2, err := memory.NewStore(dbPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer func() { _ = store2.Close() }()

	loaded, err := Load(context.Background(), store2.DB())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.RowsObserved != wantRows {
		t.Errorf("RowsObserved: got %d want %d", loaded.RowsObserved, wantRows)
	}
	if loaded.DaysObserved != wantDays {
		t.Errorf("DaysObserved: got %d want %d", loaded.DaysObserved, wantDays)
	}
	if !loaded.Ready {
		t.Errorf("Ready: got false; want true (rows=%d days=%d)", loaded.RowsObserved, loaded.DaysObserved)
	}
	if len(loaded.Rows) != wantProfileRows {
		t.Errorf("len(Rows): got %d want %d", len(loaded.Rows), wantProfileRows)
	}
	for i, r := range loaded.Rows {
		if r.Class == "" || r.Key == "" {
			t.Errorf("row %d: empty Class/Key (%+v)", i, r)
		}
		if r.WindowStart.IsZero() || r.WindowEnd.IsZero() {
			t.Errorf("row %d: zero window timestamps (%+v)", i, r)
		}
	}
}

// TestLoad_MissingTablesReturnsEmptyProfile asserts Load is forgiving
// on a fresh store where operator_profile* tables have never been
// populated — returns Ready=false, no error.
func TestLoad_MissingTablesReturnsEmptyProfile(t *testing.T) {
	dir := t.TempDir()
	store, err := memory.NewStore(filepath.Join(dir, "memory.db"))
	if err != nil {
		t.Fatalf("memory: %v", err)
	}
	defer func() { _ = store.Close() }()

	loaded, err := Load(context.Background(), store.DB())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.Ready {
		t.Errorf("want Ready=false on fresh store; got true")
	}
	if len(loaded.Rows) != 0 {
		t.Errorf("want 0 rows on fresh store; got %d", len(loaded.Rows))
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

// TestProfileUpdate_WithCtxSwitches_ObservesTransitions asserts wiring a
// non-empty SwitchSource yields context_transition observations (closes #10).
func TestProfileUpdate_WithCtxSwitches_ObservesTransitions(t *testing.T) {
	dir := t.TempDir()
	store, err := memory.NewStore(filepath.Join(dir, "memory.db"))
	if err != nil {
		t.Fatalf("memory: %v", err)
	}
	defer func() { _ = store.Close() }()

	base := time.Now().UTC().Add(-8 * 24 * time.Hour)
	var entries []audit.Entry
	for i := 0; i < 60; i++ {
		entries = append(entries, audit.Entry{
			Timestamp: base.Add(time.Duration(i*3) * time.Hour).Format(time.RFC3339),
			Kind:      "leah.ship",
		})
	}
	path := writeAudit(t, dir, entries)

	src := &fakeSwitchSource{switches: []activectx.Switch{
		{From: "", To: "leah", SwitchedAt: base.Add(-1 * time.Hour)},
		{From: "leah", To: "regatta", SwitchedAt: base.Add(50 * time.Hour)},
		{From: "regatta", To: "leah", SwitchedAt: base.Add(120 * time.Hour)},
	}}
	p := &Profile{Now: func() time.Time { return time.Now().UTC() }, SwitchSource: src}
	if err := p.Update(context.Background(), store, path, base.Add(-time.Hour)); err != nil {
		t.Fatalf("update: %v", err)
	}
	if src.calls == 0 {
		t.Errorf("SwitchSource.Since never called")
	}
	var n int
	if err := store.DB().QueryRow(
		`SELECT COUNT(*) FROM operator_profile WHERE class='context_transition'`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n == 0 {
		t.Errorf("want >0 context_transition rows with switches wired; got 0 (silent-2/3 bug)")
	}
}

// TestProfileUpdate_NilSwitchSource_NoObservations pins the documented
// degraded-mode behavior when no ctxmgr is wired (back-compat baseline).
func TestProfileUpdate_NilSwitchSource_NoObservations(t *testing.T) {
	dir := t.TempDir()
	store, err := memory.NewStore(filepath.Join(dir, "memory.db"))
	if err != nil {
		t.Fatalf("memory: %v", err)
	}
	defer func() { _ = store.Close() }()

	base := time.Now().UTC().Add(-8 * 24 * time.Hour)
	var entries []audit.Entry
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
	var n int
	if err := store.DB().QueryRow(
		`SELECT COUNT(*) FROM operator_profile WHERE class='context_transition'`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Errorf("want 0 context_transition rows when SwitchSource nil; got %d", n)
	}
	// time_of_day + cadence still populated — degraded mode, not broken.
	var other int
	if err := store.DB().QueryRow(
		`SELECT COUNT(*) FROM operator_profile WHERE class IN ('time_of_day','cadence')`).Scan(&other); err != nil {
		t.Fatalf("count other: %v", err)
	}
	if other == 0 {
		t.Errorf("want >0 time_of_day+cadence rows; got 0")
	}
}

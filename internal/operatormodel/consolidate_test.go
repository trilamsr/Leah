package operatormodel

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/memory"
)

// fixedT is the synthetic "now" anchor used across the consolidate tests.
// Picked deep in 2026 so 30d-back stays in 2026 and tests are not seasonal.
var fixedT = time.Date(2026, 6, 10, 3, 0, 0, 0, time.UTC)

func newConsolidateStore(t *testing.T) *memory.Store {
	t.Helper()
	dir := t.TempDir()
	s, err := memory.NewStore(filepath.Join(dir, "memory.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// writeAuditRows writes one Entry per timestamp at audit jsonl path.
func writeAuditRows(t *testing.T, path string, times []time.Time, kind string) {
	t.Helper()
	for _, ts := range times {
		e := audit.Entry{Timestamp: ts.UTC().Format(time.RFC3339), Kind: kind, Outcome: "success"}
		buf, _ := json.Marshal(e)
		buf = append(buf, '\n')
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			t.Fatalf("open audit: %v", err)
		}
		if _, err := f.Write(buf); err != nil {
			t.Fatalf("write audit: %v", err)
		}
		_ = f.Close()
	}
}

// TestConsolidate_WritesUpsertOnStableCell — 20d-old stable cell yields a
// consolidated row and a snapshot row.
func TestConsolidate_WritesUpsertOnStableCell(t *testing.T) {
	store := newConsolidateStore(t)
	dir := t.TempDir()
	auditPath := filepath.Join(dir, "audit.jsonl")
	archivePath := filepath.Join(dir, "consolidated.jsonl")

	// 8 rows over the 16-25d window — all "ask" at hour 09 UTC.
	var times []time.Time
	for d := 16; d <= 23; d++ {
		// Each row at hour=09 UTC on its respective day. fixedT is 03:00 UTC,
		// so jumping back d days then to hour 9 means d*24h - 18h offset.
		ts := time.Date(fixedT.Year(), fixedT.Month(), fixedT.Day()-d, 9, 0, 0, 0, time.UTC)
		times = append(times, ts)
	}
	writeAuditRows(t, auditPath, times, "ask")

	cp := &ConsolidatePass{
		Now:         func() time.Time { return fixedT },
		Store:       store,
		AuditPath:   auditPath,
		ArchivePath: archivePath,
		TZ:          time.UTC,
	}
	if err := cp.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	var n int
	if err := store.DB().QueryRow(
		`SELECT COUNT(*) FROM operator_profile_consolidated WHERE class='time_of_day' AND key='ask' AND slot='09'`,
	).Scan(&n); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if n != 1 {
		t.Fatalf("want 1 consolidated row, got %d", n)
	}

	// Re-run idempotent — UPSERT path; same (class, key, slot) row count stays 1.
	if err := cp.Run(context.Background()); err != nil {
		t.Fatalf("Run #2: %v", err)
	}
	if err := store.DB().QueryRow(
		`SELECT COUNT(*) FROM operator_profile_consolidated WHERE class='time_of_day' AND key='ask' AND slot='09'`,
	).Scan(&n); err != nil {
		t.Fatalf("scan #2: %v", err)
	}
	if n != 1 {
		t.Fatalf("after second pass: time_of_day/ask/09 row count = %d, want 1 (upsert)", n)
	}
}

// TestConsolidate_SnapshotsCurrentWeights — every pass writes a fresh
// snapshot row per cell so the next pass has a 14d-old anchor (§3.3).
func TestConsolidate_SnapshotsCurrentWeights(t *testing.T) {
	store := newConsolidateStore(t)
	dir := t.TempDir()
	auditPath := filepath.Join(dir, "audit.jsonl")
	archivePath := filepath.Join(dir, "consolidated.jsonl")

	var times []time.Time
	for d := 16; d <= 22; d++ {
		// Each row at hour=09 UTC on its respective day. fixedT is 03:00 UTC,
		// so jumping back d days then to hour 9 means d*24h - 18h offset.
		ts := time.Date(fixedT.Year(), fixedT.Month(), fixedT.Day()-d, 9, 0, 0, 0, time.UTC)
		times = append(times, ts)
	}
	writeAuditRows(t, auditPath, times, "ask")

	cp := &ConsolidatePass{
		Now:         func() time.Time { return fixedT },
		Store:       store,
		AuditPath:   auditPath,
		ArchivePath: archivePath,
		TZ:          time.UTC,
	}
	if err := cp.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	var n int
	var ts string
	if err := store.DB().QueryRow(
		`SELECT COUNT(*), MAX(snapshot_ts) FROM operator_profile_snapshot`,
	).Scan(&n, &ts); err != nil {
		t.Fatalf("scan snapshot: %v", err)
	}
	if n < 1 {
		t.Fatalf("want >=1 snapshot row, got %d", n)
	}
	if ts == "" {
		t.Fatal("snapshot_ts empty")
	}
}

// TestConsolidate_ArchivesRawOlderThan14d — rows ≥14d old land in the
// archive jsonl; rows <14d stay in audit.jsonl.
func TestConsolidate_ArchivesRawOlderThan14d(t *testing.T) {
	store := newConsolidateStore(t)
	dir := t.TempDir()
	auditPath := filepath.Join(dir, "audit.jsonl")
	archivePath := filepath.Join(dir, "consolidated.jsonl")

	var times []time.Time
	// 6 rows ≥14d old (consolidatable)
	for d := 16; d <= 21; d++ {
		// Each row at hour=09 UTC on its respective day. fixedT is 03:00 UTC,
		// so jumping back d days then to hour 9 means d*24h - 18h offset.
		ts := time.Date(fixedT.Year(), fixedT.Month(), fixedT.Day()-d, 9, 0, 0, 0, time.UTC)
		times = append(times, ts)
	}
	// 3 rows <14d (must remain)
	for d := 3; d <= 5; d++ {
		// Each row at hour=09 UTC on its respective day. fixedT is 03:00 UTC,
		// so jumping back d days then to hour 9 means d*24h - 18h offset.
		ts := time.Date(fixedT.Year(), fixedT.Month(), fixedT.Day()-d, 9, 0, 0, 0, time.UTC)
		times = append(times, ts)
	}
	writeAuditRows(t, auditPath, times, "ask")

	cp := &ConsolidatePass{
		Now:         func() time.Time { return fixedT },
		Store:       store,
		AuditPath:   auditPath,
		ArchivePath: archivePath,
		TZ:          time.UTC,
	}
	if err := cp.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	archived, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}
	gotArchived := strings.Count(strings.TrimRight(string(archived), "\n"), "\n") + 1
	if len(archived) == 0 {
		gotArchived = 0
	}
	if gotArchived != 6 {
		t.Fatalf("archive line count = %d, want 6", gotArchived)
	}

	live, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("read audit: %v", err)
	}
	gotLive := strings.Count(strings.TrimRight(string(live), "\n"), "\n") + 1
	if len(live) == 0 {
		gotLive = 0
	}
	if gotLive != 3 {
		t.Fatalf("post-pass audit line count = %d, want 3", gotLive)
	}
}

// TestConsolidate_KillSwitch_EnvDisables — LEAH_CONSOLIDATION=0 short-
// circuits the pass; zero side effects.
func TestConsolidate_KillSwitch_EnvDisables(t *testing.T) {
	t.Setenv("LEAH_CONSOLIDATION", "0")
	store := newConsolidateStore(t)
	dir := t.TempDir()
	auditPath := filepath.Join(dir, "audit.jsonl")
	archivePath := filepath.Join(dir, "consolidated.jsonl")

	var times []time.Time
	for d := 16; d <= 25; d++ {
		// Each row at hour=09 UTC on its respective day. fixedT is 03:00 UTC,
		// so jumping back d days then to hour 9 means d*24h - 18h offset.
		ts := time.Date(fixedT.Year(), fixedT.Month(), fixedT.Day()-d, 9, 0, 0, 0, time.UTC)
		times = append(times, ts)
	}
	writeAuditRows(t, auditPath, times, "ask")
	before, _ := os.ReadFile(auditPath)

	cp := &ConsolidatePass{
		Now:         func() time.Time { return fixedT },
		Store:       store,
		AuditPath:   auditPath,
		ArchivePath: archivePath,
		TZ:          time.UTC,
	}
	if err := cp.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	after, _ := os.ReadFile(auditPath)
	if string(before) != string(after) {
		t.Fatal("audit.jsonl mutated under LEAH_CONSOLIDATION=0")
	}
	if _, err := os.Stat(archivePath); !os.IsNotExist(err) {
		t.Fatal("archive created under LEAH_CONSOLIDATION=0")
	}
	var n int
	_ = store.DB().QueryRow(`SELECT COUNT(*) FROM operator_profile_consolidated`).Scan(&n)
	if n != 0 {
		t.Fatalf("consolidated rows = %d, want 0", n)
	}
}

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
// consolidated row + snapshot row, but only on the SECOND pass; the first
// pass writes a snapshot only because the §2-rule-2 stability gate fails
// open on missing anchor (delta=1.0 ⇒ FAIL ⇒ snapshot-only).
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
	// Pass 1 — first-time cell, snapshot-only.
	if err := cp.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	var n int
	if err := store.DB().QueryRow(
		`SELECT COUNT(*) FROM operator_profile_consolidated WHERE class='time_of_day' AND key='ask' AND slot='09'`,
	).Scan(&n); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if n != 0 {
		t.Fatalf("first-pass consolidated row count = %d, want 0 (snapshot-only on cold cell)", n)
	}

	// Pass 2 — anchor present, stability gate clears, consolidation row writes.
	if err := cp.Run(context.Background()); err != nil {
		t.Fatalf("Run #2: %v", err)
	}
	if err := store.DB().QueryRow(
		`SELECT COUNT(*) FROM operator_profile_consolidated WHERE class='time_of_day' AND key='ask' AND slot='09'`,
	).Scan(&n); err != nil {
		t.Fatalf("scan #2: %v", err)
	}
	if n != 1 {
		t.Fatalf("after second pass: time_of_day/ask/09 row count = %d, want 1", n)
	}

	// Pass 3 — UPSERT idempotent; count stays 1.
	if err := cp.Run(context.Background()); err != nil {
		t.Fatalf("Run #3: %v", err)
	}
	if err := store.DB().QueryRow(
		`SELECT COUNT(*) FROM operator_profile_consolidated WHERE class='time_of_day' AND key='ask' AND slot='09'`,
	).Scan(&n); err != nil {
		t.Fatalf("scan #3: %v", err)
	}
	if n != 1 {
		t.Fatalf("after third pass: time_of_day/ask/09 row count = %d, want 1 (upsert)", n)
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
	// Two passes — first warms the snapshot anchor (cold-cell snapshot-only),
	// second clears the §2-rule-2 stability gate and writes the archive.
	if err := cp.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if err := cp.Run(context.Background()); err != nil {
		t.Fatalf("Run #2: %v", err)
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

// TestConsolidate_FirstTimeCellSnapshotOnly — a cell with no prior snapshot
// (anchor=0 ⇒ delta=1.0 ⇒ FAIL) must write a snapshot only and skip the
// consolidation/archive write. The §2-rule-2 stability gate is symmetric on
// hasAnchor=false (failure to prove 14d stability ≠ permission to consolidate).
func TestConsolidate_FirstTimeCellSnapshotOnly(t *testing.T) {
	store := newConsolidateStore(t)
	dir := t.TempDir()
	auditPath := filepath.Join(dir, "audit.jsonl")
	archivePath := filepath.Join(dir, "consolidated.jsonl")

	var times []time.Time
	for d := 16; d <= 21; d++ {
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

	var nCons int
	_ = store.DB().QueryRow(`SELECT COUNT(*) FROM operator_profile_consolidated`).Scan(&nCons)
	if nCons != 0 {
		t.Fatalf("cold-cell consolidated rows = %d, want 0", nCons)
	}
	var nSnap int
	_ = store.DB().QueryRow(`SELECT COUNT(*) FROM operator_profile_snapshot`).Scan(&nSnap)
	if nSnap < 1 {
		t.Fatalf("snapshot rows = %d, want >=1", nSnap)
	}
	if _, err := os.Stat(archivePath); !os.IsNotExist(err) {
		t.Fatal("archive written under cold-cell snapshot-only pass")
	}
}

// TestConsolidate_ConcurrentAppend_NoRowsLost asserts the audit.jsonl
// rewrite is bracketed by QuiesceForConsolidation across BOTH readAudit and
// the tmp+rename — Appends racing the pass land either fully before
// readAudit (in rows) or fully after release (in the rewritten file). No
// row is silently dropped on the orphaned pre-rename inode.
func TestConsolidate_ConcurrentAppend_NoRowsLost(t *testing.T) {
	store := newConsolidateStore(t)
	dir := t.TempDir()
	auditPath := filepath.Join(dir, "audit.jsonl")
	archivePath := filepath.Join(dir, "consolidated.jsonl")

	// Seed: 6 old "ask" rows + an existing snapshot so the first pass
	// archives (not snapshot-only) and exercises the rewrite path.
	var seed []time.Time
	for d := 16; d <= 21; d++ {
		ts := time.Date(fixedT.Year(), fixedT.Month(), fixedT.Day()-d, 9, 0, 0, 0, time.UTC)
		seed = append(seed, ts)
	}
	writeAuditRows(t, auditPath, seed, "ask")
	// Seed snapshots with the same wNow the pass will compute on this seed
	// so the stability gate clears (delta ≈ 0 < 0.05) and the rows archive
	// on the first pass — exercising the rewriteAudit path which is what
	// the quiesce contract protects.
	wTOD := decayedWeight(seed, fixedT, halflifeDays())
	if err := upsertSnapshot(context.Background(), store.DB(), "time_of_day", "ask", "09", wTOD, fixedT.Add(-14*24*time.Hour)); err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}
	for _, ts := range seed {
		wkday := ts.Weekday().String()[:3]
		w := decayedWeight([]time.Time{ts}, fixedT, halflifeDays())
		if err := upsertSnapshot(context.Background(), store.DB(), "cadence", "ask", wkday, w, fixedT.Add(-14*24*time.Hour)); err != nil {
			t.Fatalf("seed cadence snapshot %s: %v", wkday, err)
		}
	}

	// Pin Append's timestamp to a (hour, weekday) disjoint from every seeded
	// cell — a racing Append whose wall-clock slot collides with a seed cell
	// drags a fresh-weighted time into that cell, blows the stability gate,
	// and leaves the seed row unarchived. The orphan-inode contract this
	// test guards is independent of clock alignment, and a wall-clock-pinned
	// Append clock is what makes the assertion deterministic across the day.
	// Tue 03:00 UTC: hour 3 ≠ seed hour 09; Tue ≠ any seed weekday (seeds span
	// Wed→Mon, May 20→25, 2026).
	appendNow := time.Date(2026, 6, 16, 3, 0, 0, 0, time.UTC)
	a := &audit.Logger{Path: auditPath, Now: func() time.Time { return appendNow }}
	cp := &ConsolidatePass{
		Now:         func() time.Time { return fixedT },
		Store:       store,
		AuditPath:   auditPath,
		ArchivePath: archivePath,
		Audit:       a,
		TZ:          time.UTC,
	}

	// Fire 100 concurrent Appends; each blocks on quiesceMu until the pass
	// releases the exclusive lock — they MUST all land on the post-rewrite
	// inode (not the orphaned pre-rename one).
	const n = 100
	done := make(chan struct{}, n)
	for i := 0; i < n; i++ {
		go func() {
			_ = a.Append(audit.Entry{Kind: "ask", Outcome: "success"})
			done <- struct{}{}
		}()
	}

	if err := cp.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	for i := 0; i < n; i++ {
		<-done
	}

	// Count rows now in audit.jsonl — must equal (n racing Appends) + any
	// young (<14d) seed rows still present. Seed = 6 rows ≥14d → all
	// archived → 0 young seed rows; total expected = n.
	data, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("read audit: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		lines = nil
	}
	if len(lines) != n {
		t.Fatalf("post-pass audit rows = %d, want %d (n racing Appends, 6 seed archived); rows lost on orphan inode",
			len(lines), n)
	}
}

// TestConsolidate_ArchiveRollbackOnTxFail asserts that appendArchive +
// os.Truncate(path, archiveSize(path)-baseline) round-trips — when the tx
// commit fails after a successful archive append, truncating back to the
// pre-append offset removes the just-written rows so a retry doesn't
// duplicate them.
func TestConsolidate_ArchiveRollbackOnTxFail(t *testing.T) {
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "consolidated.jsonl")

	// Pre-seed with sentinel content; preSize is the rollback anchor.
	sentinel := []byte("{\"ts\":\"sentinel\"}\n")
	if err := os.WriteFile(archivePath, sentinel, 0o600); err != nil {
		t.Fatalf("seed archive: %v", err)
	}
	preSize, err := archiveSize(archivePath)
	if err != nil {
		t.Fatalf("archiveSize: %v", err)
	}
	if preSize != int64(len(sentinel)) {
		t.Fatalf("archiveSize = %d, want %d", preSize, len(sentinel))
	}

	// Simulate appendArchive writing rows then a tx commit failure.
	ts := time.Date(fixedT.Year(), fixedT.Month(), fixedT.Day()-16, 9, 0, 0, 0, time.UTC)
	consolidated := []consolidatedCell{{
		cell:     cellAggregate{class: "time_of_day", key: "ask", slot: "09"},
		oldTimes: []time.Time{ts},
	}}
	rows := []audit.Entry{{Timestamp: ts.Format(time.RFC3339), Kind: "ask", Outcome: "success"}}
	if err := appendArchive(archivePath, consolidated, rows, time.UTC); err != nil {
		t.Fatalf("appendArchive: %v", err)
	}
	fi, _ := os.Stat(archivePath)
	if fi.Size() <= preSize {
		t.Fatalf("post-append size = %d, want >%d", fi.Size(), preSize)
	}

	// Tx commit "fails" — roll back archive to preSize.
	if err := os.Truncate(archivePath, preSize); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	fi2, _ := os.Stat(archivePath)
	if fi2.Size() != preSize {
		t.Fatalf("rollback size = %d, want %d (sentinel intact)", fi2.Size(), preSize)
	}
	got, _ := os.ReadFile(archivePath)
	if string(got) != string(sentinel) {
		t.Fatalf("post-rollback content = %q, want %q (no duplicate rows on retry)", got, sentinel)
	}

	// Retry: appendArchive again — total rows = 1 (no duplicate from prior attempt).
	if err := appendArchive(archivePath, consolidated, rows, time.UTC); err != nil {
		t.Fatalf("appendArchive #2: %v", err)
	}
	data, _ := os.ReadFile(archivePath)
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("post-retry line count = %d, want 2 (sentinel + 1 row, no duplicate)", len(lines))
	}
}

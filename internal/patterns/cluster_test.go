package patterns

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeAudit(t *testing.T, lines []string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "audit.jsonl")
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("write audit: %v", err)
	}
	return p
}

// TestDetectGroupsByKindAndPrefix asserts (kind, args_hash[:8]) bucketing.
func TestDetectGroupsByKindAndPrefix(t *testing.T) {
	// 5 events sharing (ask, deadbeefcafebabe-prefix). Same kind, args differ
	// only past char 8 — must collapse to one cluster of count=5.
	lines := []string{}
	for i := 0; i < 5; i++ {
		lines = append(lines, `{"ts":"2026-06-09T10:00:00Z","kind":"ask","args_hash":"deadbeefcafebabe`+
			string(rune('a'+i))+`","blast_radius":0,"outcome":"success","detail":"q`+
			string(rune('0'+i))+`"}`)
	}
	// Plus one ship with different hash — shouldn't merge.
	lines = append(lines, `{"ts":"2026-06-09T11:00:00Z","kind":"ship","args_hash":"feedface","blast_radius":3,"outcome":"success"}`)

	got, err := Detect(writeAudit(t, lines), time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 cluster (ship below threshold), got %d: %+v", len(got), got)
	}
	c := got[0]
	if c.Kind != "ask" || c.HashPrefix != "deadbeef" || c.Count != 5 {
		t.Errorf("cluster mismatch: %+v", c)
	}
	if len(c.Samples) != MaxSamples {
		t.Errorf("want %d samples, got %d", MaxSamples, len(c.Samples))
	}
}

// TestDetectFiltersBelowThreshold asserts clusters with count<min are dropped.
func TestDetectFiltersBelowThreshold(t *testing.T) {
	// 4 events of one kind — below default 5.
	lines := []string{}
	for i := 0; i < 4; i++ {
		lines = append(lines, `{"ts":"2026-06-09T10:00:00Z","kind":"review","args_hash":"abcdef01","blast_radius":0,"outcome":"success"}`)
	}
	got, err := Detect(writeAudit(t, lines), time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("want 0 clusters (below threshold), got %d", len(got))
	}

	// Same data with min_count=3 must surface.
	got2, err := DetectWithThreshold(writeAudit(t, lines), time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), 3)
	if err != nil {
		t.Fatalf("DetectWithThreshold: %v", err)
	}
	if len(got2) != 1 || got2[0].Count != 4 {
		t.Errorf("want 1 cluster count=4, got %+v", got2)
	}
}

// TestDetect_MissingAuditFileNoError asserts a non-existent audit file
// returns (nil, nil) rather than erroring — fresh installs have no audit.
func TestDetect_MissingAuditFileNoError(t *testing.T) {
	got, err := Detect(filepath.Join(t.TempDir(), "no-such-audit.jsonl"),
		time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Errorf("missing audit should not error, got %v", err)
	}
	if len(got) != 0 {
		t.Errorf("want empty clusters, got %d", len(got))
	}
}

// TestDetect_EmptyAuditFile asserts an empty audit log returns empty
// clusters (no panic on zero-row read).
func TestDetect_EmptyAuditFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "audit.jsonl")
	if err := os.WriteFile(p, []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Detect(p, time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("want 0 clusters on empty audit, got %d", len(got))
	}
}

// TestDetect_AllRowsBeforeSinceCutoff asserts every row predating since is
// dropped — the time window must be honored even when rows match by key.
func TestDetect_AllRowsBeforeSinceCutoff(t *testing.T) {
	lines := []string{}
	for i := 0; i < 10; i++ {
		lines = append(lines, `{"ts":"2026-01-01T10:00:00Z","kind":"ask","args_hash":"deadbeef","blast_radius":0,"outcome":"success"}`)
	}
	got, err := Detect(writeAudit(t, lines), time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("want 0 clusters (all rows pre-cutoff), got %d", len(got))
	}
}

// TestDetect_HashShorterThanPrefixLen asserts a row whose args_hash is
// shorter than HashPrefixLen is bucketed by the full short hash (not panic).
func TestDetect_HashShorterThanPrefixLen(t *testing.T) {
	lines := []string{}
	for i := 0; i < 5; i++ {
		// 4-char hash; HashPrefixLen=8; full short value used as the key suffix.
		lines = append(lines, `{"ts":"2026-06-09T10:00:00Z","kind":"ask","args_hash":"abcd","blast_radius":0,"outcome":"success","detail":"q"}`)
	}
	got, err := Detect(writeAudit(t, lines), time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(got) != 1 || got[0].HashPrefix != "abcd" {
		t.Errorf("want 1 cluster with prefix=abcd, got %+v", got)
	}
}

// TestDetect_SortsByCountDescThenKindAsc asserts the secondary sort key
// (Kind asc) breaks Count ties deterministically.
func TestDetect_SortsByCountDescThenKindAsc(t *testing.T) {
	lines := []string{}
	// 5 rows for kind="ask"
	for i := 0; i < 5; i++ {
		lines = append(lines, `{"ts":"2026-06-09T10:00:00Z","kind":"ask","args_hash":"aaaaaaaa","blast_radius":0,"outcome":"success"}`)
	}
	// 5 rows for kind="ship" — Count tie with ask; Kind asc → ask before ship
	for i := 0; i < 5; i++ {
		lines = append(lines, `{"ts":"2026-06-09T10:00:00Z","kind":"ship","args_hash":"bbbbbbbb","blast_radius":3,"outcome":"success"}`)
	}
	// 7 rows for kind="review" — higher Count → first
	for i := 0; i < 7; i++ {
		lines = append(lines, `{"ts":"2026-06-09T10:00:00Z","kind":"review","args_hash":"cccccccc","blast_radius":1,"outcome":"success"}`)
	}
	got, err := Detect(writeAudit(t, lines), time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 clusters, got %d", len(got))
	}
	if got[0].Kind != "review" {
		t.Errorf("want review first (highest count), got %q", got[0].Kind)
	}
	if got[1].Kind != "ask" || got[2].Kind != "ship" {
		t.Errorf("Count-tied clusters must sort Kind asc; got %q,%q", got[1].Kind, got[2].Kind)
	}
}

// TestDetect_FirstLastTimestampsTrackExtremes asserts the First/Last fields
// of a cluster correctly span the earliest + latest seen timestamps.
func TestDetect_FirstLastTimestampsTrackExtremes(t *testing.T) {
	lines := []string{
		`{"ts":"2026-06-09T12:00:00Z","kind":"ask","args_hash":"deadbeef","blast_radius":0,"outcome":"success"}`,
		`{"ts":"2026-06-09T08:00:00Z","kind":"ask","args_hash":"deadbeef","blast_radius":0,"outcome":"success"}`,
		`{"ts":"2026-06-09T16:00:00Z","kind":"ask","args_hash":"deadbeef","blast_radius":0,"outcome":"success"}`,
		`{"ts":"2026-06-09T10:00:00Z","kind":"ask","args_hash":"deadbeef","blast_radius":0,"outcome":"success"}`,
		`{"ts":"2026-06-09T14:00:00Z","kind":"ask","args_hash":"deadbeef","blast_radius":0,"outcome":"success"}`,
	}
	got, err := Detect(writeAudit(t, lines), time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 cluster, got %d", len(got))
	}
	wantFirst, _ := time.Parse(time.RFC3339, "2026-06-09T08:00:00Z")
	wantLast, _ := time.Parse(time.RFC3339, "2026-06-09T16:00:00Z")
	if !got[0].First.Equal(wantFirst) || !got[0].Last.Equal(wantLast) {
		t.Errorf("First/Last off: got %v / %v, want %v / %v",
			got[0].First, got[0].Last, wantFirst, wantLast)
	}
}

// TestDetectSkipsCorruptLines asserts JSON-corrupt lines don't break the run.
func TestDetectSkipsCorruptLines(t *testing.T) {
	lines := []string{
		`{"ts":"2026-06-09T10:00:00Z","kind":"ask","args_hash":"aaaaaaaa","blast_radius":0,"outcome":"success"}`,
		`{not valid json`,
		`{"ts":"2026-06-09T10:01:00Z","kind":"ask","args_hash":"aaaaaaaa","blast_radius":0,"outcome":"success"}`,
		`{"ts":"BAD_TIMESTAMP","kind":"ask","args_hash":"aaaaaaaa","blast_radius":0,"outcome":"success"}`,
		`{"ts":"2026-06-09T10:02:00Z","kind":"ask","args_hash":"aaaaaaaa","blast_radius":0,"outcome":"success"}`,
		`{"ts":"2026-06-09T10:03:00Z","kind":"ask","args_hash":"aaaaaaaa","blast_radius":0,"outcome":"success"}`,
		`{"ts":"2026-06-09T10:04:00Z","kind":"ask","args_hash":"aaaaaaaa","blast_radius":0,"outcome":"success"}`,
	}
	got, err := Detect(writeAudit(t, lines), time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(got) != 1 || got[0].Count != 5 {
		t.Fatalf("want 1 cluster count=5 (skipping 2 bad lines), got %+v", got)
	}
}

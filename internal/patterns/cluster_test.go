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

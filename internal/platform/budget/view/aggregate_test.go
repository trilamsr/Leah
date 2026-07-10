package view

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeJSONL writes one entry per line to path. Entries are interface{}
// because tests deliberately include rows missing fields the production
// audit.Entry struct does not yet declare (Model).
func writeJSONL(t *testing.T, path string, rows []map[string]any) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer func() { _ = f.Close() }()
	for _, r := range rows {
		b, err := json.Marshal(r)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if _, err := f.Write(append(b, '\n')); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
}

// TestAggregateEmpty asserts that a missing audit file returns a zero
// Summary without error (operator may run `leah cost` before any spend).
func TestAggregateEmpty(t *testing.T) {
	dir := t.TempDir()
	s, err := Aggregate(filepath.Join(dir, "missing.jsonl"), time.Unix(0, 0))
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	if s.TotalUSD != 0 || s.Count != 0 {
		t.Fatalf("want zero, got %+v", s)
	}
}

// TestAggregateSumsAndCounts asserts TotalUSD + Count across all rows
// after the since cutoff.
func TestAggregateSumsAndCounts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	writeJSONL(t, path, []map[string]any{
		{"ts": "2026-06-01T10:00:00Z", "kind": "ask", "cost_dollars": 0.10},
		{"ts": "2026-06-02T10:00:00Z", "kind": "ship", "cost_dollars": 0.25},
		{"ts": "2026-06-03T10:00:00Z", "kind": "ask", "cost_dollars": 0.15},
	})
	s, err := Aggregate(path, mustTime(t, "2026-05-31T00:00:00Z"))
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	if got, want := s.TotalUSD, 0.50; !floatEq(got, want) {
		t.Errorf("TotalUSD: got %v, want %v", got, want)
	}
	if s.Count != 3 {
		t.Errorf("Count: got %d, want 3", s.Count)
	}
}

// TestAggregateSinceFilter asserts rows before `since` are excluded.
func TestAggregateSinceFilter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	writeJSONL(t, path, []map[string]any{
		{"ts": "2026-05-01T10:00:00Z", "kind": "ask", "cost_dollars": 1.00},
		{"ts": "2026-06-05T10:00:00Z", "kind": "ask", "cost_dollars": 0.10},
	})
	s, err := Aggregate(path, mustTime(t, "2026-06-01T00:00:00Z"))
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	if !floatEq(s.TotalUSD, 0.10) {
		t.Errorf("filtered TotalUSD: got %v, want 0.10", s.TotalUSD)
	}
	if s.Count != 1 {
		t.Errorf("filtered Count: got %d, want 1", s.Count)
	}
}

// TestAggregateByKind asserts per-kind buckets.
func TestAggregateByKind(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	writeJSONL(t, path, []map[string]any{
		{"ts": "2026-06-01T10:00:00Z", "kind": "ask", "cost_dollars": 0.10},
		{"ts": "2026-06-02T10:00:00Z", "kind": "ship", "cost_dollars": 0.30},
		{"ts": "2026-06-03T10:00:00Z", "kind": "ask", "cost_dollars": 0.05},
	})
	s, _ := Aggregate(path, mustTime(t, "2026-05-01T00:00:00Z"))
	if !floatEq(s.ByKind["ask"], 0.15) {
		t.Errorf("ByKind[ask]: got %v, want 0.15", s.ByKind["ask"])
	}
	if !floatEq(s.ByKind["ship"], 0.30) {
		t.Errorf("ByKind[ship]: got %v, want 0.30", s.ByKind["ship"])
	}
}

// TestAggregateByDay asserts UTC-date string keys ("YYYY-MM-DD").
func TestAggregateByDay(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	writeJSONL(t, path, []map[string]any{
		{"ts": "2026-06-01T23:59:00Z", "kind": "ask", "cost_dollars": 0.10},
		{"ts": "2026-06-02T00:01:00Z", "kind": "ask", "cost_dollars": 0.20},
		{"ts": "2026-06-02T15:00:00Z", "kind": "ship", "cost_dollars": 0.30},
	})
	s, _ := Aggregate(path, mustTime(t, "2026-05-01T00:00:00Z"))
	if !floatEq(s.ByDay["2026-06-01"], 0.10) {
		t.Errorf("ByDay[2026-06-01]: got %v, want 0.10", s.ByDay["2026-06-01"])
	}
	if !floatEq(s.ByDay["2026-06-02"], 0.50) {
		t.Errorf("ByDay[2026-06-02]: got %v, want 0.50", s.ByDay["2026-06-02"])
	}
}

// TestAggregateMalformedLineSkipped asserts a corrupt JSONL line does not
// abort the whole scan — audit log integrity is best-effort.
func TestAggregateMalformedLineSkipped(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	f, _ := os.Create(path)
	_, _ = f.WriteString(`{"ts":"2026-06-01T10:00:00Z","kind":"ask","cost_dollars":0.10}` + "\n")
	_, _ = f.WriteString("not-json\n")
	_, _ = f.WriteString(`{"ts":"2026-06-02T10:00:00Z","kind":"ship","cost_dollars":0.30}` + "\n")
	_ = f.Close()

	s, err := Aggregate(path, mustTime(t, "2026-05-01T00:00:00Z"))
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	if s.Count != 2 {
		t.Errorf("Count after skip: got %d, want 2", s.Count)
	}
	if !floatEq(s.TotalUSD, 0.40) {
		t.Errorf("TotalUSD after skip: got %v, want 0.40", s.TotalUSD)
	}
}

// TestAggregateModelFieldOptional asserts rows lacking `model` bucket into
// "unknown" — the audit.Entry struct does not declare Model today, so
// every existing row will end up in this bucket; future Entry extension
// will auto-populate the per-model breakdown without touching this code.
func TestAggregateModelFieldOptional(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	writeJSONL(t, path, []map[string]any{
		{"ts": "2026-06-01T10:00:00Z", "kind": "ask", "cost_dollars": 0.10},
		{"ts": "2026-06-02T10:00:00Z", "kind": "ask", "cost_dollars": 0.20, "model": "claude-sonnet-4-6"},
	})
	s, _ := Aggregate(path, mustTime(t, "2026-05-01T00:00:00Z"))
	if !floatEq(s.ByModel["unknown"], 0.10) {
		t.Errorf("ByModel[unknown]: got %v, want 0.10", s.ByModel["unknown"])
	}
	if !floatEq(s.ByModel["claude-sonnet-4-6"], 0.20) {
		t.Errorf("ByModel[claude-sonnet-4-6]: got %v, want 0.20", s.ByModel["claude-sonnet-4-6"])
	}
}

// TestAggregateZeroCostRowsCounted asserts rows with cost_dollars==0 still
// increment Count (operator wants to see invocations even when free / cached).
func TestAggregateZeroCostRowsCounted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	writeJSONL(t, path, []map[string]any{
		{"ts": "2026-06-01T10:00:00Z", "kind": "status"},
		{"ts": "2026-06-02T10:00:00Z", "kind": "ask", "cost_dollars": 0.10},
	})
	s, _ := Aggregate(path, mustTime(t, "2026-05-01T00:00:00Z"))
	if s.Count != 2 {
		t.Errorf("Count: got %d, want 2", s.Count)
	}
}

// TestTopKindsReturnsSortedDesc asserts the helper used by the dashboard
// to surface the top-3 cost contributors.
func TestTopKindsReturnsSortedDesc(t *testing.T) {
	s := Summary{ByKind: map[string]float64{"ask": 0.10, "ship": 0.50, "review": 0.30, "retro": 0.05}}
	top := s.TopKinds(3)
	if len(top) != 3 {
		t.Fatalf("TopKinds len: got %d, want 3", len(top))
	}
	if top[0].Name != "ship" || !floatEq(top[0].USD, 0.50) {
		t.Errorf("top[0]: got %+v, want {ship, 0.50}", top[0])
	}
	if top[1].Name != "review" || top[2].Name != "ask" {
		t.Errorf("sort order: got %+v", top)
	}
}

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	tt, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return tt
}

func floatEq(a, b float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < 1e-9
}

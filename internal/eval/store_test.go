package eval

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestStore_RecordRun_PersistsRunAndResults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "eval.db")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = s.Close() }()

	now := time.Date(2026, 6, 22, 18, 0, 0, 0, time.UTC)
	runID, err := s.RecordRun(context.Background(), RunRecord{
		StartedAt: now,
		Source:    "scheduler",
		Results: []ResultRecord{
			{FixtureID: "smoke-1", Pass: true, Score: 1.0, Actual: "ok"},
			{FixtureID: "smoke-2", Pass: false, Score: 0.0, Actual: "nope", Reason: "must_contain missing"},
		},
	})
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if runID <= 0 {
		t.Fatalf("runID should be positive, got %d", runID)
	}

	rows, err := s.ListResults(context.Background(), runID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("want 2 results, got %d", len(rows))
	}
	if rows[0].FixtureID != "smoke-1" || !rows[0].Pass {
		t.Errorf("row0 mismatch: %+v", rows[0])
	}
	if rows[1].FixtureID != "smoke-2" || rows[1].Pass {
		t.Errorf("row1 mismatch: %+v", rows[1])
	}
}

func TestStore_OpenIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "eval.db")
	s1, err := OpenStore(path)
	if err != nil {
		t.Fatalf("open1: %v", err)
	}
	_ = s1.Close()
	s2, err := OpenStore(path)
	if err != nil {
		t.Fatalf("open2: %v", err)
	}
	_ = s2.Close()
}

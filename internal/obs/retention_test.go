package obs

import (
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func dayFile(t *testing.T, dir string, daysAgo int, now time.Time, gz bool) string {
	t.Helper()
	date := now.AddDate(0, 0, -daysAgo).UTC().Format("2006-01-02")
	name := "leah-" + date + ".jsonl"
	if gz {
		name += ".gz"
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("{\"ts\":\"x\"}\n"), 0o600); err != nil {
		t.Fatalf("seed %s: %v", p, err)
	}
	return p
}

func TestPruneLogs(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)

	today := dayFile(t, dir, 0, now, false)
	recent := dayFile(t, dir, 2, now, false)
	old := dayFile(t, dir, 8, now, false)
	ancient := dayFile(t, dir, 40, now, false)
	ancientGz := dayFile(t, dir, 41, now, true)

	if err := PruneLogs(dir, now, 30); err != nil {
		t.Fatalf("PruneLogs: %v", err)
	}

	if _, err := os.Stat(today); err != nil {
		t.Errorf("active current-day file must survive: %v", err)
	}
	if _, err := os.Stat(recent); err != nil {
		t.Errorf("recent (<7d) file must stay uncompressed: %v", err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Errorf("8d file should have been gzipped (original removed), got %v", err)
	}
	if _, err := os.Stat(old + ".gz"); err != nil {
		t.Errorf("8d file should exist as .gz: %v", err)
	}
	if _, err := os.Stat(ancient); !os.IsNotExist(err) {
		t.Errorf("40d file should be deleted: %v", err)
	}
	if _, err := os.Stat(ancientGz); !os.IsNotExist(err) {
		t.Errorf("41d .gz should be deleted: %v", err)
	}

	raw, err := os.Open(old + ".gz")
	if err != nil {
		t.Fatalf("open gz: %v", err)
	}
	defer func() { _ = raw.Close() }()
	zr, err := gzip.NewReader(raw)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	got, _ := io.ReadAll(zr)
	if string(got) != "{\"ts\":\"x\"}\n" {
		t.Errorf("gz content mismatch: %q", got)
	}
}

func TestPruneLogsIdempotent(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	dayFile(t, dir, 8, now, false)

	for i := 0; i < 3; i++ {
		if err := PruneLogs(dir, now, 30); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 || filepath.Ext(entries[0].Name()) != ".gz" {
		t.Errorf("idempotent re-run should leave exactly one .gz, got %v", names(entries))
	}
}

func TestRetentionDaysEnv(t *testing.T) {
	if got := RetentionDays(); got != 30 {
		t.Errorf("default = %d, want 30", got)
	}
	t.Setenv("LEAH_AUDIT_RETENTION_DAYS", "7")
	if got := RetentionDays(); got != 7 {
		t.Errorf("override = %d, want 7", got)
	}
	t.Setenv("LEAH_AUDIT_RETENTION_DAYS", "garbage")
	if got := RetentionDays(); got != 30 {
		t.Errorf("bad value must fall back to 30, got %d", got)
	}
	t.Setenv("LEAH_AUDIT_RETENTION_DAYS", "0")
	if got := RetentionDays(); got != 30 {
		t.Errorf("non-positive must fall back to 30, got %d", got)
	}
}

func names(e []os.DirEntry) []string {
	out := make([]string, len(e))
	for i, x := range e {
		out[i] = x.Name()
	}
	return out
}

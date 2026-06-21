package obs

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestDailyRotator_WriteRacesRotate exercises the close-while-write window:
// a write that has already obtained the handle but not yet written must not
// land in (or fail because of) a file the rotator has just closed.
func TestDailyRotator_WriteRacesRotate(t *testing.T) {
	logsDir := filepath.Join(t.TempDir(), "logs")
	rot := newDailyRotator(logsDir)
	mw := &multiWriter{rotator: rot}

	// Derive both days from real wall-clock UTC. multiWriter.Write stamps
	// time.Now().UTC() per call — day1 MUST be today so the writer's date
	// matches the file the assertion reads.
	now := time.Now().UTC()
	day1 := now
	day2 := now.Add(24 * time.Hour)

	if _, err := rot.writerFor(day1); err != nil {
		t.Fatalf("prime day1: %v", err)
	}

	// Per-writer bounded counts: deterministic upper bound on attempted writes
	// eliminates the prior busy-loop that under contended CI scheduling could
	// fail to drain before close(stop), losing accounting in atomic.Int64.
	const (
		writers      = 8
		writesPerGo  = 2000
		closeCycles  = 500
	)
	record := []byte("record\n")

	var wg sync.WaitGroup
	var writeErrs atomic.Int64
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < writesPerGo; j++ {
				if _, err := mw.Write(record); err != nil {
					writeErrs.Add(1)
				}
			}
		}()
	}

	// Closer races the writers by alternately closing curFile and re-priming
	// for day2 then day1. Bounded iterations so the goroutine terminates
	// without depending on writer drain timing.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < closeCycles; i++ {
			rot.close()
			_, _ = rot.writerFor(day2)
			rot.close()
			_, _ = rot.writerFor(day1)
		}
	}()

	wg.Wait()
	rot.close()

	if got := writeErrs.Load(); got != 0 {
		t.Fatalf("mw.Write returned err on %d calls; want 0", got)
	}

	// Every record must land in day1 or day2 — those are the only dates the
	// rotator ever opened. Lost or truncated writes indicate the close-during-
	// write race.
	f1, _ := os.ReadFile(filepath.Join(logsDir, "leah-"+day1.Format("2006-01-02")+".jsonl"))
	f2, _ := os.ReadFile(filepath.Join(logsDir, "leah-"+day2.Format("2006-01-02")+".jsonl"))
	got := strings.Count(string(f1), "record\n") + strings.Count(string(f2), "record\n")
	want := writers * writesPerGo
	if got != want {
		t.Fatalf("lost writes: file lines=%d, want %d (day1=%d bytes, day2=%d bytes)",
			got, want, len(f1), len(f2))
	}
}

// TestDailyRotator_ConcurrentWritesIntact asserts bytes from concurrent
// writes are not interleaved mid-record.
func TestDailyRotator_ConcurrentWritesIntact(t *testing.T) {
	logsDir := filepath.Join(t.TempDir(), "logs")
	rot := newDailyRotator(logsDir)
	mw := &multiWriter{rotator: rot}

	const (
		workers      = 16
		perWorker    = 64
		recordPrefix = "rec-"
	)
	record := []byte(recordPrefix + strings.Repeat("a", 200) + "\n")

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < perWorker; j++ {
				if _, err := mw.Write(record); err != nil {
					t.Errorf("write: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()
	rot.close()

	dateFile := filepath.Join(logsDir, "leah-"+time.Now().UTC().Format("2006-01-02")+".jsonl")
	raw, err := os.ReadFile(dateFile)
	if err != nil {
		t.Fatalf("read log %s: %v", dateFile, err)
	}
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	if got, want := len(lines), workers*perWorker; got != want {
		t.Fatalf("line count = %d, want %d", got, want)
	}
	for i, line := range lines {
		if !strings.HasPrefix(line, recordPrefix) {
			t.Fatalf("line %d corrupted: %q", i, line)
		}
		if len(line) != len(record)-1 {
			t.Fatalf("line %d length = %d, want %d", i, len(line), len(record)-1)
		}
	}
}

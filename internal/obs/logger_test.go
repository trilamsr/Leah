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

	day1 := time.Date(2026, 6, 9, 23, 59, 59, 0, time.UTC)
	day2 := time.Date(2026, 6, 10, 0, 0, 1, 0, time.UTC)

	// Prime day1.
	if _, err := rot.writerFor(day1); err != nil {
		t.Fatalf("prime day1: %v", err)
	}

	var (
		wg          sync.WaitGroup
		writes      atomic.Int64
		shortWrites atomic.Int64
		writeErrs   atomic.Int64
	)

	// Writers stamp records continuously. Each record MUST land somewhere.
	const writers = 8
	stop := make(chan struct{})
	record := []byte("record\n")
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				n, err := mw.Write(record)
				writes.Add(1)
				if err != nil {
					writeErrs.Add(1)
				}
				if n != len(record) {
					shortWrites.Add(1)
				}
			}
		}()
	}

	// Closer thread forces rotation by alternately closing and re-priming
	// for day2 — the window between writerFor's unlock and *os.File.Write
	// is exactly the bug.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			rot.close()
			_, _ = rot.writerFor(day2)
			rot.close()
			_, _ = rot.writerFor(day1)
		}
	}()

	time.Sleep(50 * time.Millisecond) // allow-sleep: let writers race the rotator long enough to expose the window
	close(stop)
	wg.Wait()
	rot.close()

	// Every recorded line must be present across both day files. Lost or
	// truncated writes indicate the close-during-write race.
	f1, _ := os.ReadFile(filepath.Join(logsDir, "leah-2026-06-09.jsonl"))
	f2, _ := os.ReadFile(filepath.Join(logsDir, "leah-2026-06-10.jsonl"))
	combined := string(f1) + string(f2)
	got := strings.Count(combined, "record\n")
	want := int(writes.Load())
	if got != want {
		t.Fatalf("lost writes: file lines=%d, attempted writes=%d (errs=%d, short=%d)",
			got, want, writeErrs.Load(), shortWrites.Load())
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

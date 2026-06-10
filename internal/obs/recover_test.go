package obs

import (
	"runtime"
	"strings"
	"sync"
	"testing"
)

// Asserts pool reuse: pooled captureStack stays well under the 64KB fresh-buffer regime.
func TestCaptureStack_PoolReusesBuffer(t *testing.T) {
	captureStack() // warm pool
	const iters = 50
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	for i := 0; i < iters; i++ {
		_ = captureStack()
	}
	runtime.ReadMemStats(&after)
	bytesPerCall := (after.TotalAlloc - before.TotalAlloc) / iters
	if bytesPerCall > 32*1024 {
		t.Fatalf("captureStack alloc = %d B/op, want <32KB (pool reuse expected; unpooled is ~66KB)", bytesPerCall)
	}
}

func TestCaptureStack_ProducesNonEmptyStack(t *testing.T) {
	s := captureStack()
	if s == "" {
		t.Fatal("captureStack returned empty string")
	}
	if !strings.Contains(s, "goroutine") {
		t.Fatalf("captureStack output missing 'goroutine' marker: %q", s[:min(len(s), 200)])
	}
}

// Race-detector coverage for the shared pool path.
func TestCaptureStack_ConcurrentSafe(t *testing.T) {
	const workers = 16
	const iters = 32
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iters; j++ {
				if captureStack() == "" {
					t.Error("empty stack under contention")
					return
				}
			}
		}()
	}
	wg.Wait()
}

func BenchmarkCaptureStack(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = captureStack()
	}
}

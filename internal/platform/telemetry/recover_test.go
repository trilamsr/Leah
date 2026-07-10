package telemetry

import (
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// Counts stackBufPool.New invocations directly — prior TotalAlloc-delta variant flaked because runtime.Stack(all=true) bleeds per-goroutine accounting into MemStats.
func TestCaptureStack_PoolReusesBuffer(t *testing.T) {
	origNew := stackBufPool.New
	t.Cleanup(func() { stackBufPool.New = origNew })
	// Drain any pre-existing entries before swapping New so the counter only
	// reflects calls made during this test.
	stackBufPool.New = nil
	for stackBufPool.Get() != nil {
	}
	var news int64
	stackBufPool.New = func() any {
		atomic.AddInt64(&news, 1)
		b := make([]byte, 64*1024)
		return &b
	}
	captureStack() // first call seeds the pool — exactly one New
	for i := 0; i < 50; i++ {
		_ = captureStack()
	}
	// GC may evict pool entries between calls (worse under -race); bug guarded against is "pool never reuses" = news == 51.
	if n := atomic.LoadInt64(&news); n > 30 {
		t.Fatalf("stackBufPool.New called %d times across 51 captureStack calls; expected pool reuse", n)
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

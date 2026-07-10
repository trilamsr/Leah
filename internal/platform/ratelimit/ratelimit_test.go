package ratelimit_test

import (
	"sync"
	"testing"
	"time"

	"github.com/trilam/leah/internal/platform/ratelimit"
)

func TestWindowAllowsUpToBudgetThenRejects(t *testing.T) {
	now := time.Unix(0, 0)
	w := ratelimit.NewWindow(time.Minute, 3, func() time.Time { return now })
	for i := 0; i < 3; i++ {
		if !w.Allow("k") {
			t.Fatalf("call %d: want allow", i)
		}
	}
	if w.Allow("k") {
		t.Fatal("4th call: want reject (budget 3 exhausted)")
	}
}

func TestWindowRefillsAfterWindow(t *testing.T) {
	now := time.Unix(0, 0)
	w := ratelimit.NewWindow(time.Minute, 1, func() time.Time { return now })
	if !w.Allow("k") {
		t.Fatal("first call must allow")
	}
	if w.Allow("k") {
		t.Fatal("second call within window must reject")
	}
	now = now.Add(time.Minute + time.Nanosecond)
	if !w.Allow("k") {
		t.Fatal("call after window must allow")
	}
}

func TestWindowPerKeyIsolation(t *testing.T) {
	now := time.Unix(0, 0)
	w := ratelimit.NewWindow(time.Minute, 1, func() time.Time { return now })
	if !w.Allow("a") {
		t.Fatal("key a first call must allow")
	}
	if !w.Allow("b") {
		t.Fatal("key b must have its own budget")
	}
	if w.Allow("a") {
		t.Fatal("key a second call must reject")
	}
}

func TestStatsCountsSendsAndDenialsAcrossKeys(t *testing.T) {
	now := time.Unix(0, 0)
	w := ratelimit.NewWindow(time.Minute, 1, func() time.Time { return now })
	w.Allow("a") // allowed
	w.Allow("a") // denied (budget 1)
	w.Allow("b") // allowed
	s := w.Stats()
	if s.Sends != 2 {
		t.Errorf("Sends: got %d, want 2 (one retained per key)", s.Sends)
	}
	if s.Denied != 1 {
		t.Errorf("Denied: got %d, want 1", s.Denied)
	}
}

func TestStatsSendsDecayWithWindow(t *testing.T) {
	now := time.Unix(0, 0)
	w := ratelimit.NewWindow(time.Minute, 5, func() time.Time { return now })
	w.Allow("k")
	now = now.Add(time.Minute + time.Nanosecond)
	if s := w.Stats(); s.Sends != 0 {
		t.Errorf("Sends after window: got %d, want 0 (decayed)", s.Sends)
	}
}

func TestStatsDeniedIsCumulative(t *testing.T) {
	now := time.Unix(0, 0)
	w := ratelimit.NewWindow(time.Minute, 1, func() time.Time { return now })
	w.Allow("k")
	w.Allow("k") // denied
	now = now.Add(2 * time.Minute)
	w.Allow("k")
	w.Allow("k") // denied
	if s := w.Stats(); s.Denied != 2 {
		t.Errorf("Denied: got %d, want 2 (cumulative across windows)", s.Denied)
	}
}

func TestWindowConcurrent(t *testing.T) {
	now := time.Unix(0, 0)
	w := ratelimit.NewWindow(time.Minute, 100, func() time.Time { return now })
	var wg sync.WaitGroup
	var mu sync.Mutex
	allowed := 0
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if w.Allow("k") {
				mu.Lock()
				allowed++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if allowed != 100 {
		t.Fatalf("budget 100 must allow exactly 100 under concurrency, got %d", allowed)
	}
}

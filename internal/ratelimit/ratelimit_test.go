package ratelimit_test

import (
	"sync"
	"testing"
	"time"

	"github.com/trilam/leah/internal/ratelimit"
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

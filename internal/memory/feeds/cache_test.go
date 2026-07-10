package feeds_test

import (
	"sync"
	"testing"
	"time"

	"github.com/trilam/leah/internal/memory/feeds"
	"github.com/trilam/leah/internal/platform/testutil"
)

// TestCache_GetSet_TTLEnforced — entry expires after its TTL window.
func TestCache_GetSet_TTLEnforced(t *testing.T) {
	c := feeds.NewMemCache()
	c.Set("k", 42, 100*time.Millisecond)

	v, ok := c.Get("k")
	if !ok || v.(int) != 42 {
		t.Fatalf("immediate Get got (%v, %v); want (42, true)", v, ok)
	}

	testutil.Eventually(t, 500*time.Millisecond, 10*time.Millisecond, func() bool {
		_, ok := c.Get("k")
		return !ok
	})
}

// TestCache_GetSet_MissReturnsFalse — unknown key never reports a value.
func TestCache_GetSet_MissReturnsFalse(t *testing.T) {
	c := feeds.NewMemCache()
	if _, ok := c.Get("nope"); ok {
		t.Fatalf("Get on empty cache returned ok=true")
	}
}

// TestCache_ConcurrentSafe — go test -race clean under parallel Get/Set.
func TestCache_ConcurrentSafe(t *testing.T) {
	c := feeds.NewMemCache()
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			c.Set("k", i, time.Second)
		}(i)
		go func() {
			defer wg.Done()
			_, _ = c.Get("k")
		}()
	}
	wg.Wait()
}

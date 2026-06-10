package obs

import (
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"
)

// TestFlatten_MemoizeReturnsSameString asserts repeat label-sets hit cache.
func TestFlatten_MemoizeReturnsSameString(t *testing.T) {
	c := &Counter{name: "x", values: map[string]int64{}}
	labels := map[string]string{"a": "1", "b": "2"}

	k1 := c.flattenKey(labels)
	k2 := c.flattenKey(labels)

	if k1 != k2 {
		t.Fatalf("flattenKey not stable: %q vs %q", k1, k2)
	}
	// Backing string MUST be the cached entry — equal string data pointer.
	if unsafe.StringData(k1) != unsafe.StringData(k2) {
		t.Errorf("flattenKey returned distinct backing strings; memoize miss")
	}
}

// TestFlatten_DistinctLabelsBypass asserts different label-sets yield different keys.
func TestFlatten_DistinctLabelsBypass(t *testing.T) {
	c := &Counter{name: "x", values: map[string]int64{}}
	k1 := c.flattenKey(map[string]string{"a": "1"})
	k2 := c.flattenKey(map[string]string{"a": "2"})
	if k1 == k2 {
		t.Errorf("distinct labels produced same key: %q", k1)
	}
	k3 := c.flattenKey(map[string]string{"a": "1", "b": "2"})
	if k3 == k1 {
		t.Errorf("superset labels collided with subset: %q", k3)
	}
}

// TestFlatten_LabelOrderInvariant asserts insertion order does not change the key.
func TestFlatten_LabelOrderInvariant(t *testing.T) {
	c := &Counter{name: "x", values: map[string]int64{}}
	m1 := map[string]string{}
	m1["b"] = "2"
	m1["a"] = "1"
	m2 := map[string]string{}
	m2["a"] = "1"
	m2["b"] = "2"

	if got1, got2 := c.flattenKey(m1), c.flattenKey(m2); got1 != got2 {
		t.Errorf("label-order invariance broken: %q vs %q", got1, got2)
	}
}

// TestFlatten_CacheCollisionFallsBackToCorrectKey ensures two label-sets with the
// same fingerprint still resolve to their canonical sorted key.
func TestFlatten_CacheCollisionResolves(t *testing.T) {
	c := &Counter{name: "x", values: map[string]int64{}}
	// Drive enough distinct label-sets to make collision-prone branches run.
	seen := map[string]string{}
	for i := 0; i < 100; i++ {
		labels := map[string]string{"i": stringFromInt(i), "k": "v"}
		k := c.flattenKey(labels)
		expected := flatten(c.name, labels)
		if k != expected {
			t.Errorf("flattenKey diverged from flatten for %v: got %q want %q", labels, k, expected)
		}
		if prev, ok := seen[k]; ok && prev != k {
			t.Errorf("non-deterministic cache result")
		}
		seen[k] = k
	}
}

// TestRegistry_Snapshot_DoesNotBlockObserve runs concurrent observers + a
// Snapshot loop; copy-and-release retains ≥50% of baseline registration
// throughput vs. the nested-lock impl which collapses to <10%.
func TestRegistry_Snapshot_DoesNotBlockObserve(t *testing.T) {
	if testing.Short() {
		t.Skip("contention timing test; skipped in -short")
	}
	build := func() *Registry {
		r := NewRegistry()
		// Large enough that a single Snapshot's marshal phase is hundreds of µs.
		const series = 2000
		const pointsPerSeries = 20
		for i := 0; i < series; i++ {
			c := r.Counter("c_" + stringFromInt(i))
			for j := 0; j < pointsPerSeries; j++ {
				c.Add(map[string]string{"k": stringFromInt(j)}, 1)
			}
		}
		return r
	}

	const window = 200 * time.Millisecond
	// Baseline: registrants alone, no snapshot.
	registerFor := func(r *Registry, snap func()) int64 {
		stop := make(chan struct{})
		var wg sync.WaitGroup
		if snap != nil {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for {
					select {
					case <-stop:
						return
					default:
					}
					snap()
				}
			}()
		}
		var total atomic.Int64
		for w := 0; w < 4; w++ {
			w := w
			wg.Add(1)
			go func() {
				defer wg.Done()
				i := 0
				for {
					select {
					case <-stop:
						return
					default:
					}
					_ = r.Counter("dyn_" + stringFromInt(w) + "_" + stringFromInt(i))
					i++
					total.Add(1)
				}
			}()
		}
		time.Sleep(window)
		close(stop)
		wg.Wait()
		return total.Load()
	}

	baseline := registerFor(build(), nil)
	r := build()
	dir := t.TempDir()
	path := filepath.Join(dir, "snap.json")
	contended := registerFor(r, func() { _ = r.Snapshot(path) })

	// Nested-lock Snapshot serialized Registry.Counter behind the full marshal,
	// collapsing throughput. Copy-and-release keeps r.mu held only for the
	// brief handle-copy window, so registrants retain at least half throughput.
	if contended*2 < baseline {
		t.Fatalf("Counter throughput collapsed: baseline=%d contended=%d (%.1f%%) — Snapshot blocks r.mu across marshal",
			baseline, contended, 100*float64(contended)/float64(baseline))
	}
	t.Logf("Counter throughput: baseline=%d contended=%d (%.1f%%)",
		baseline, contended, 100*float64(contended)/float64(baseline))
}

// BenchmarkRegistry_Snapshot measures wall-clock per Snapshot on a populated registry.
func BenchmarkRegistry_Snapshot(b *testing.B) {
	r := NewRegistry()
	const series = 50
	const pointsPerSeries = 20
	for i := 0; i < series; i++ {
		c := r.Counter("c_" + stringFromInt(i))
		for j := 0; j < pointsPerSeries; j++ {
			c.Add(map[string]string{"k": stringFromInt(j)}, 1)
		}
	}
	dir := b.TempDir()
	path := filepath.Join(dir, "snap.json")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := r.Snapshot(path); err != nil {
			b.Fatalf("Snapshot: %v", err)
		}
	}
}

func stringFromInt(i int) string {
	const digits = "0123456789"
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = digits[i%10]
		i /= 10
	}
	return string(buf[pos:])
}

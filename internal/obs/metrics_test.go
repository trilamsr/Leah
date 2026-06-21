package obs

import (
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
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

// TestRegistry_Snapshot_DoesNotBlockObserve proves Snapshot releases r.mu
// before marshalling: Counter() registrations interleave freely with a fixed
// run of Snapshots. Nested-lock throttles Counter to ~1 per Snapshot;
// copy-and-release lets many complete per Snapshot.
func TestRegistry_Snapshot_DoesNotBlockObserve(t *testing.T) {
	r := NewRegistry()
	// Large enough that one Snapshot's marshal dwarfs one Counter() — so under
	// nested-lock a registrant could complete at most ~1 per Snapshot.
	const series = 2000
	const pointsPerSeries = 20
	for i := 0; i < series; i++ {
		c := r.Counter("c_" + stringFromInt(i))
		for j := 0; j < pointsPerSeries; j++ {
			c.Add(map[string]string{"k": stringFromInt(j)}, 1)
		}
	}
	path := filepath.Join(t.TempDir(), "snap.json")

	const snapshots = 20
	stop := make(chan struct{})
	var registered atomic.Int64
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Re-lookup one fixed name: Counter still takes r.mu (the contended
		// lock) but never grows the registry, so snapshot cost stays flat.
		for {
			select {
			case <-stop:
				return
			default:
			}
			_ = r.Counter("dyn")
			registered.Add(1)
		}
	}()
	for i := 0; i < snapshots; i++ {
		if err := r.Snapshot(path); err != nil {
			t.Fatalf("Snapshot: %v", err)
		}
	}
	close(stop)
	wg.Wait()

	// Nested-lock would serialize the registrant to ~1 Counter() per Snapshot
	// (~snapshots total). Copy-and-release lets it interleave; 10x is a floor
	// the real ratio (hundreds) clears even under GOMAXPROCS=1.
	if got := registered.Load(); got < 10*snapshots {
		t.Fatalf("Counter starved during Snapshot: %d registrations across %d snapshots — Snapshot holds r.mu across marshal",
			got, snapshots)
	}
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

// TestHistogramDeclare_NoSampleRecorded asserts Declare creates the series
// at count=0/sum=0 — Observe(0) would skew p50 with a real sample.
func TestHistogramDeclare_NoSampleRecorded(t *testing.T) {
	r := NewRegistry()
	h := r.Histogram("h", []float64{0.1, 1})
	h.Declare(map[string]string{"k": "v"})

	path := filepath.Join(t.TempDir(), "m.json")
	if err := r.Snapshot(path); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	// Re-Observe must not mistake a Declare for a prior sample.
	h.Observe(map[string]string{"k": "v"}, 0.5)
	h.mu.Lock()
	defer h.mu.Unlock()
	s := h.values["h|k=v"]
	if s == nil {
		t.Fatalf("expected series after Declare+Observe")
	}
	if s.count != 1 || s.sum != 0.5 {
		t.Fatalf("Declare leaked a sample: count=%d sum=%v want count=1 sum=0.5", s.count, s.sum)
	}
}

// TestCounterDeclare_ZeroNoOverwrite asserts Declare leaves an existing value untouched.
func TestCounterDeclare_ZeroNoOverwrite(t *testing.T) {
	r := NewRegistry()
	c := r.Counter("c")
	c.Inc(map[string]string{"k": "v"})
	c.Declare(map[string]string{"k": "v"})
	c.mu.Lock()
	defer c.mu.Unlock()
	if got := c.values["c|k=v"]; got != 1 {
		t.Fatalf("Declare overwrote existing counter: got %d want 1", got)
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

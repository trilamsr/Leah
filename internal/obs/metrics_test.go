package obs

import (
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

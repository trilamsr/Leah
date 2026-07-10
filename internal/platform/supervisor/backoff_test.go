package supervisor

import (
	"testing"
	"time"
)

func TestBackoff_ExponentialCapped(t *testing.T) {
	b := newBackoff(200*time.Millisecond, 30*time.Second)
	for i := 0; i < 12; i++ {
		b.next()
	}
	if b.cur > 30*time.Second {
		t.Fatalf("backoff exceeded cap: %v", b.cur)
	}
	if b.cur != 30*time.Second {
		t.Fatalf("backoff should be pinned at cap after many steps, got %v", b.cur)
	}
}

func TestBackoff_DoublesFromMin(t *testing.T) {
	b := newBackoff(200*time.Millisecond, 30*time.Second)
	want := []time.Duration{
		200 * time.Millisecond,
		400 * time.Millisecond,
		800 * time.Millisecond,
		1600 * time.Millisecond,
	}
	for i, w := range want {
		got := b.next()
		if got != w {
			t.Fatalf("step %d: got %v want %v", i, got, w)
		}
	}
}

func TestBackoff_ResetReturnsToMin(t *testing.T) {
	b := newBackoff(200*time.Millisecond, 30*time.Second)
	for i := 0; i < 5; i++ {
		b.next()
	}
	b.reset()
	got := b.next()
	if got != 200*time.Millisecond {
		t.Fatalf("after reset want 200ms, got %v", got)
	}
}

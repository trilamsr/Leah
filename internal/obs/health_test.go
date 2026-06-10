package obs

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type fakeChecker struct {
	err   error
	delay time.Duration
	calls atomic.Int32
}

func (f *fakeChecker) SelfCheck(ctx context.Context) error {
	f.calls.Add(1)
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return f.err
}

// TestHealthRegistry_ProbeAllOK: every checker nil → status ok.
func TestHealthRegistry_ProbeAllOK(t *testing.T) {
	r := NewHealthRegistry()
	r.Register("a", &fakeChecker{})
	r.Register("b", &fakeChecker{})
	r.Register("c", &fakeChecker{})
	rep := r.Probe(context.Background(), time.Second)
	if rep.Status != HealthOK {
		t.Fatalf("status: got %q, want ok", rep.Status)
	}
	for _, want := range []string{"a", "b", "c"} {
		if got := rep.PackageHealth[want]; got != string(HealthOK) {
			t.Errorf("package %s: got %q, want ok", want, got)
		}
	}
}

// TestHealthRegistry_OneDegraded: one failing checker → degraded.
func TestHealthRegistry_OneDegraded(t *testing.T) {
	r := NewHealthRegistry()
	r.Register("ok1", &fakeChecker{})
	r.Register("bad", &fakeChecker{err: errors.New("disk full")})
	r.Register("ok2", &fakeChecker{})
	rep := r.Probe(context.Background(), time.Second)
	if rep.Status != HealthDegraded {
		t.Fatalf("status: got %q, want degraded", rep.Status)
	}
	if got := rep.PackageHealth["bad"]; got == string(HealthOK) || got == "" {
		t.Errorf("bad pkg: got %q, want degraded:*", got)
	}
}

// TestHealthRegistry_ThreeDegraded_Unhealthy: 3+ failing → unhealthy.
func TestHealthRegistry_ThreeDegraded_Unhealthy(t *testing.T) {
	r := NewHealthRegistry()
	r.Register("a", &fakeChecker{err: errors.New("e1")})
	r.Register("b", &fakeChecker{err: errors.New("e2")})
	r.Register("c", &fakeChecker{err: errors.New("e3")})
	rep := r.Probe(context.Background(), time.Second)
	if rep.Status != HealthUnhealthy {
		t.Fatalf("status: got %q, want unhealthy", rep.Status)
	}
}

// TestHealthRegistry_TimeoutEnforced: checker exceeds budget → reported degraded.
func TestHealthRegistry_TimeoutEnforced(t *testing.T) {
	r := NewHealthRegistry()
	r.Register("slow", &fakeChecker{delay: 200 * time.Millisecond})
	rep := r.Probe(context.Background(), 20*time.Millisecond)
	if rep.Status != HealthDegraded {
		t.Fatalf("status: got %q, want degraded", rep.Status)
	}
	if got := rep.PackageHealth["slow"]; got == string(HealthOK) {
		t.Errorf("slow pkg: got %q, want degraded:*", got)
	}
}

// TestHealthRegistry_UptimeAdvances: uptime is non-zero after Probe.
func TestHealthRegistry_UptimeAdvances(t *testing.T) {
	r := NewHealthRegistry()
	time.Sleep(2 * time.Millisecond)
	rep := r.Probe(context.Background(), time.Second)
	if rep.UptimeSeconds <= 0 {
		t.Fatalf("uptime: got %v, want >0", rep.UptimeSeconds)
	}
}

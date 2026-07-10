package supervisor

import (
	"testing"
	"time"
)

func TestCircuit_OpensAtMaxInWindow(t *testing.T) {
	c := newCircuit(CircuitPolicy{MaxCrashes: 5, Window: time.Minute})
	base := time.Unix(0, 0)
	for i := 0; i < 4; i++ {
		c.recordCrash(base.Add(time.Duration(i) * 10 * time.Second))
		if c.isOpen(base.Add(time.Duration(i) * 10 * time.Second)) {
			t.Fatalf("opened too early at crash %d", i)
		}
	}
	c.recordCrash(base.Add(40 * time.Second))
	if !c.isOpen(base.Add(40 * time.Second)) {
		t.Fatal("circuit must open at 5th crash within window")
	}
}

func TestCircuit_OldCrashesAgeOut(t *testing.T) {
	c := newCircuit(CircuitPolicy{MaxCrashes: 3, Window: time.Minute})
	base := time.Unix(0, 0)
	c.recordCrash(base)
	c.recordCrash(base.Add(10 * time.Second))
	c.recordCrash(base.Add(120 * time.Second)) // first two aged out
	if c.isOpen(base.Add(120 * time.Second)) {
		t.Fatal("circuit must stay closed when crashes aged out")
	}
}

func TestCircuit_ResetClears(t *testing.T) {
	c := newCircuit(CircuitPolicy{MaxCrashes: 2, Window: time.Minute})
	base := time.Unix(0, 0)
	c.recordCrash(base)
	c.recordCrash(base.Add(time.Second))
	if !c.isOpen(base.Add(time.Second)) {
		t.Fatal("should be open")
	}
	c.reset()
	if c.isOpen(base.Add(time.Second)) {
		t.Fatal("reset must clear")
	}
}

func TestCircuit_DefaultPolicyIsAlwaysClosed(t *testing.T) {
	c := newCircuit(CircuitPolicy{})
	c.recordCrash(time.Now())
	if c.isOpen(time.Now()) {
		t.Fatal("zero policy must be no-op")
	}
}

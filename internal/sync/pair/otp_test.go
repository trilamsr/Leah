package pair

import (
	"testing"
	"time"
)

func TestOTP_FormatIsThreeDashThree(t *testing.T) {
	o := GenerateOTP()
	s := o.String()
	if len(s) != 7 {
		t.Fatalf("OTP length: want 7, got %d (%q)", len(s), s)
	}
	if s[3] != '-' {
		t.Fatalf("OTP separator: want '-' at index 3, got %q (%q)", s[3], s)
	}
	for i, b := range []byte(s) {
		if i == 3 {
			continue
		}
		if b < '0' || b > '9' {
			t.Fatalf("OTP digit %d: want 0-9, got %q (%q)", i, b, s)
		}
	}
}

func TestOTP_EntropyAcrossSamples(t *testing.T) {
	// 6 digits = 1_000_000 possible values. Drawing 1024 samples should
	// produce at least 1000 distinct OTPs in expectation; the birthday
	// collision probability for getting fewer than 1000 unique is < 1e-9.
	seen := make(map[OTP]struct{}, 1024)
	for i := 0; i < 1024; i++ {
		seen[GenerateOTP()] = struct{}{}
	}
	if len(seen) < 1000 {
		t.Fatalf("OTP entropy: 1024 samples yielded %d unique values (expected ≥1000)", len(seen))
	}
}

func TestOTP_EqualConstantTime(t *testing.T) {
	a := OTP{'1', '2', '3', '4', '5', '6'}
	b := OTP{'1', '2', '3', '4', '5', '6'}
	c := OTP{'1', '2', '3', '4', '5', '7'}
	if !a.Equal(b) {
		t.Fatal("Equal: identical OTPs not equal")
	}
	if a.Equal(c) {
		t.Fatal("Equal: differing OTPs reported equal")
	}
}

func TestAttemptCounter_LocksAfterThreshold(t *testing.T) {
	c := NewAttemptCounter()
	for i := 0; i < PairAttemptLimit; i++ {
		if err := c.Allow(); err != nil {
			t.Fatalf("Allow #%d before threshold: %v", i+1, err)
		}
		c.Record(false)
	}
	if err := c.Allow(); err != ErrLocked {
		t.Fatalf("Allow after %d failures: got %v, want ErrLocked", PairAttemptLimit, err)
	}
}

func TestAttemptCounter_UnlocksAfterDuration(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	c := &AttemptCounter{now: func() time.Time { return now }}
	for i := 0; i < PairAttemptLimit; i++ {
		_ = c.Allow()
		c.Record(false)
	}
	if err := c.Allow(); err != ErrLocked {
		t.Fatalf("expected lock, got %v", err)
	}
	now = now.Add(PairLockDuration + time.Second)
	if err := c.Allow(); err != nil {
		t.Fatalf("expected unlock after PairLockDuration, got %v", err)
	}
}

func TestAttemptCounter_HitResetsFailures(t *testing.T) {
	c := NewAttemptCounter()
	for i := 0; i < PairAttemptLimit-1; i++ {
		_ = c.Allow()
		c.Record(false)
	}
	c.Record(true)
	if err := c.Allow(); err != nil {
		t.Fatalf("Allow after hit: %v", err)
	}
}

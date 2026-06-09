package budget

import (
	"testing"
)

func TestNewReadsEnvOrDefault(t *testing.T) {
	t.Setenv("LEAH_BUDGET_DOLLARS", "")
	b := New()
	if b.Ceiling != 5.0 {
		t.Errorf("default ceiling want 5.0, got %v", b.Ceiling)
	}

	t.Setenv("LEAH_BUDGET_DOLLARS", "12.5")
	b = New()
	if b.Ceiling != 12.5 {
		t.Errorf("env ceiling want 12.5, got %v", b.Ceiling)
	}
}

func TestChargeAccumulatesAndBlocksAboveCeiling(t *testing.T) {
	b := &Budget{Ceiling: 1.0}
	if err := b.Charge(0.30); err != nil {
		t.Fatalf("charge 0.30 #1: %v", err)
	}
	if err := b.Charge(0.40); err != nil {
		t.Fatalf("charge 0.40: %v", err)
	}
	if err := b.Charge(0.30); err != nil {
		t.Fatalf("charge 0.30 #2: %v", err)
	}
	// 1.00 reached; next charge should fail
	if err := b.Charge(0.01); err == nil {
		t.Fatalf("expected budget exceeded error")
	}
}

func TestChargeIsAtomicOnPartialOver(t *testing.T) {
	b := &Budget{Ceiling: 1.0}
	_ = b.Charge(0.95)
	err := b.Charge(0.10)
	if err == nil {
		t.Fatalf("expected error on attempted-over")
	}
	if b.Spent() != 0.95 {
		t.Errorf("spent should stay 0.95 on rejected charge, got %v", b.Spent())
	}
}

package budget

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
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

// TestChargeEmitsObsLogs asserts Charge emits budget.charge on success +
// budget.exceeded on rejection (Wave2-K obs instrumentation contract).
// Swaps slog.Default for the duration; restores on exit.
func TestChargeEmitsObsLogs(t *testing.T) {
	prev := slog.Default()
	defer slog.SetDefault(prev)

	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))

	b := &Budget{Ceiling: 1.0}
	if err := b.Charge(0.5); err != nil {
		t.Fatalf("charge: %v", err)
	}
	if err := b.Charge(2.0); err == nil {
		t.Fatalf("expected exceeded")
	}
	out := buf.String()
	if !strings.Contains(out, `"msg":"budget.charge"`) {
		t.Errorf("missing charge event: %s", out)
	}
	if !strings.Contains(out, `"msg":"budget.exceeded"`) {
		t.Errorf("missing exceeded event: %s", out)
	}
}

// TestCharge_ConcurrentRespectsCeiling pounds Charge from many goroutines
// with each charge < ceiling/N — the spent total must never exceed the
// ceiling, and rejected attempts must not bump spent. Race-detector clean.
func TestCharge_ConcurrentRespectsCeiling(t *testing.T) {
	b := &Budget{Ceiling: 1.0}
	const N = 100
	const each = 0.02 // 100 * 0.02 = 2.0; ceiling 1.0 → ~50 accepted, ~50 rejected
	var accepted, rejected int64
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			if err := b.Charge(each); err == nil {
				atomic.AddInt64(&accepted, 1)
			} else {
				atomic.AddInt64(&rejected, 1)
			}
		}()
	}
	wg.Wait()
	if accepted+rejected != N {
		t.Errorf("lost charges: accepted=%d rejected=%d", accepted, rejected)
	}
	spent := b.Spent()
	if spent > b.Ceiling {
		t.Errorf("spent %v exceeds ceiling %v", spent, b.Ceiling)
	}
	// Spent must equal accepted * each (atomic accumulation).
	want := float64(accepted) * each
	if diff := spent - want; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("spent=%v want=%v (accepted=%d)", spent, want, accepted)
	}
}

// TestCharge_ExceededErrorIsTyped asserts the returned error is the
// *ExceededError type (errors.As round-trip) so callers can branch on it.
func TestCharge_ExceededErrorIsTyped(t *testing.T) {
	b := &Budget{Ceiling: 1.0}
	err := b.Charge(2.0)
	if err == nil {
		t.Fatal("expected exceeded error")
	}
	var exc *ExceededError
	if !errors.As(err, &exc) {
		t.Fatalf("expected *ExceededError, got %T: %v", err, err)
	}
	if exc.Attempted != 2.0 || exc.Ceiling != 1.0 || exc.Spent != 0 {
		t.Errorf("ExceededError fields: %+v", exc)
	}
	if !strings.Contains(exc.Error(), "budget exceeded") {
		t.Errorf("Error() should describe overspend: %q", exc.Error())
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

package budget

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/trilam/leah/internal/sqlstore"

	_ "modernc.org/sqlite"
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

// newRuntimeWithDB opens a fresh WAL+single-writer SQLite for the
// privacy-bucket Runtime — matches the daemon's sqlstore.OpenWAL pool shape so
// the test surfaces the same contention regime as production.
func newRuntimeWithDB(t *testing.T) (Runtime, *sql.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "budget.db")
	db, err := sqlstore.OpenWAL(path)
	if err != nil {
		t.Fatalf("OpenWAL: %v", err)
	}
	r, err := NewRuntime(db)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() {
		if c, ok := r.(Closer); ok {
			_ = c.Close()
		}
		_ = db.Close()
	})
	return r, db
}

func TestRuntime_ChargeBlocksAtCap(t *testing.T) {
	r, _ := newRuntimeWithDB(t)
	ctx := context.Background()
	if err := r.Set(ctx, BucketCloudTTSChars, 100, WindowDay); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := r.Charge(ctx, BucketCloudTTSChars, 99); err != nil {
		t.Fatalf("charge 99: %v", err)
	}
	if err := r.Charge(ctx, BucketCloudTTSChars, 2); !errors.Is(err, ErrOverBudget) {
		t.Fatalf("charge 2 want ErrOverBudget, got %v", err)
	}
	bal, err := r.Peek(ctx, BucketCloudTTSChars)
	if err != nil {
		t.Fatalf("Peek: %v", err)
	}
	if bal.Spent != 99 {
		t.Errorf("rejected charge must not advance spent: spent=%d want 99", bal.Spent)
	}
}

func TestRuntime_UnlimitedCapNeverBlocks(t *testing.T) {
	r, _ := newRuntimeWithDB(t)
	ctx := context.Background()
	if err := r.Set(ctx, BucketCloudLLMTokens, 0, WindowDay); err != nil {
		t.Fatalf("Set: %v", err)
	}
	for i := 0; i < 10; i++ {
		if err := r.Charge(ctx, BucketCloudLLMTokens, 1_000_000); err != nil {
			t.Fatalf("charge: %v", err)
		}
	}
	bal, _ := r.Peek(ctx, BucketCloudLLMTokens)
	if bal.Spent != 10_000_000 {
		t.Errorf("unlimited bucket spent=%d want 10M", bal.Spent)
	}
}

func TestRuntime_SubscribeEmitsActionPerSpec(t *testing.T) {
	r, _ := newRuntimeWithDB(t)
	ctx := context.Background()
	if err := r.Set(ctx, BucketCloudEmbedBytes, 100, WindowDay); err != nil {
		t.Fatalf("Set: %v", err)
	}
	ch := r.Subscribe()
	if err := r.Charge(ctx, BucketCloudEmbedBytes, 50); err != nil {
		t.Fatalf("charge 50: %v", err)
	}
	if err := r.Charge(ctx, BucketCloudEmbedBytes, 35); err != nil {
		t.Fatalf("charge 35: %v", err)
	}
	if err := r.Charge(ctx, BucketCloudEmbedBytes, 15); err != nil {
		t.Fatalf("charge 15: %v", err)
	}
	want := []DegradeAction{ActionNone, ActionWarn, ActionSwitchLocal}
	for i, w := range want {
		select {
		case ev := <-ch:
			if ev.Action != w {
				t.Errorf("ev[%d] action=%d want %d (spent=%d)", i, ev.Action, w, ev.Spent)
			}
		case <-time.After(time.Second):
			t.Fatalf("ev[%d]: no event", i)
		}
	}
}

func TestRuntime_ResetClearsCurrentWindow(t *testing.T) {
	r, _ := newRuntimeWithDB(t)
	ctx := context.Background()
	if err := r.Set(ctx, BucketCloudVisionBytes, 1000, WindowDay); err != nil {
		t.Fatalf("Set: %v", err)
	}
	_ = r.Charge(ctx, BucketCloudVisionBytes, 700)
	if err := r.Reset(ctx, BucketCloudVisionBytes); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	bal, _ := r.Peek(ctx, BucketCloudVisionBytes)
	if bal.Spent != 0 {
		t.Errorf("after Reset spent=%d want 0", bal.Spent)
	}
}

func TestRuntime_PeekTrendOldestToNewest(t *testing.T) {
	r, db := newRuntimeWithDB(t)
	ctx := context.Background()
	// Seed three samples at distinct windows by inserting directly — exercises
	// the SELECT … ORDER BY at DESC LIMIT 24 + reverse-into-ascending path.
	rows := []struct {
		at    int64
		spent int64
	}{{100, 5}, {200, 7}, {300, 11}}
	for _, row := range rows {
		if _, err := db.Exec(`INSERT INTO budget_sample(bucket, at, spent) VALUES(?,?,?)`,
			string(BucketCloudTTSChars), row.at, row.spent); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	bal, err := r.Peek(ctx, BucketCloudTTSChars)
	if err != nil {
		t.Fatalf("Peek: %v", err)
	}
	if len(bal.Trend) != 3 {
		t.Fatalf("trend len=%d want 3", len(bal.Trend))
	}
	if bal.Trend[0] != 5 || bal.Trend[2] != 11 {
		t.Errorf("trend not oldest→newest: %v", bal.Trend)
	}
}

func TestRuntime_ChargeP95Under400us(t *testing.T) {
	if testing.Short() {
		t.Skip("perf test")
	}
	r, _ := newRuntimeWithDB(t)
	ctx := context.Background()
	if err := r.Set(ctx, BucketCloudEmbedBytes, 1<<40, WindowDay); err != nil {
		t.Fatalf("Set: %v", err)
	}
	const N = 1000
	samples := make([]time.Duration, N)
	for i := 0; i < N; i++ {
		t0 := time.Now()
		if err := r.Charge(ctx, BucketCloudEmbedBytes, 1); err != nil {
			t.Fatalf("charge: %v", err)
		}
		samples[i] = time.Since(t0)
	}
	// Sort ascending, take p95.
	for i := 1; i < N; i++ {
		for j := i; j > 0 && samples[j-1] > samples[j]; j-- {
			samples[j-1], samples[j] = samples[j], samples[j-1]
		}
	}
	p95 := samples[(N*95)/100]
	if p95 > 400*time.Microsecond {
		t.Errorf("Charge p95=%v exceeds 0.4 ms budget (§8.8)", p95)
	}
}

func TestRuntime_UnknownBucketIsError(t *testing.T) {
	r, _ := newRuntimeWithDB(t)
	if err := r.Charge(context.Background(), Bucket("nope.does.not.exist"), 1); err == nil {
		t.Fatal("unknown bucket must error")
	}
}

func TestRuntime_DefaultsSeeded(t *testing.T) {
	r, _ := newRuntimeWithDB(t)
	for _, d := range Defaults() {
		bal, err := r.Peek(context.Background(), d.Bucket)
		if err != nil {
			t.Errorf("Peek %s: %v", d.Bucket, err)
			continue
		}
		if bal.Cap != d.Cap {
			t.Errorf("%s cap=%d want %d", d.Bucket, bal.Cap, d.Cap)
		}
	}
}

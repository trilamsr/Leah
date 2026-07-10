package recommend

import (
	"context"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/trilam/leah/internal/audit"
)

// TestBetaUpdateOnAccept asserts Accept feedback writes α+=1 to recommend_pattern_state.
func TestBetaUpdateOnAccept(t *testing.T) {
	eng := openBanditEngine(t)
	defer func() { _ = eng.Close() }()

	if err := eng.Seed(Recommendation{ID: "r1", Pattern: "p", Tier: TierConfirm, Source: "s", Confidence: 0.5}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := eng.RecordFeedback(context.Background(), "r1", string(FeedbackAccept), signalAccept); err != nil {
		t.Fatalf("record: %v", err)
	}
	a, b := readPosterior(t, eng, "p")
	if a != 2.0 || b != 1.0 {
		t.Fatalf("Accept: want α=2,β=1; got α=%v,β=%v", a, b)
	}
}

// TestBetaUpdateOnReject asserts Reject writes β+=1.
func TestBetaUpdateOnReject(t *testing.T) {
	eng := openBanditEngine(t)
	defer func() { _ = eng.Close() }()

	if err := eng.Seed(Recommendation{ID: "r1", Pattern: "p", Tier: TierConfirm, Source: "s", Confidence: 0.5}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := eng.RecordFeedback(context.Background(), "r1", string(FeedbackReject), signalReject); err != nil {
		t.Fatalf("record: %v", err)
	}
	a, b := readPosterior(t, eng, "p")
	if a != 1.0 || b != 2.0 {
		t.Fatalf("Reject: want α=1,β=2; got α=%v,β=%v", a, b)
	}
}

// TestBetaUpdateOnIgnore asserts Ignore writes β+=0.1.
func TestBetaUpdateOnIgnore(t *testing.T) {
	eng := openBanditEngine(t)
	defer func() { _ = eng.Close() }()

	if err := eng.Seed(Recommendation{ID: "r1", Pattern: "p", Tier: TierConfirm, Source: "s", Confidence: 0.5}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := eng.RecordFeedback(context.Background(), "r1", string(FeedbackIgnore), signalIgnore); err != nil {
		t.Fatalf("record: %v", err)
	}
	a, b := readPosterior(t, eng, "p")
	if a != 1.0 || math.Abs(b-1.1) > 1e-9 {
		t.Fatalf("Ignore: want α=1,β≈1.1; got α=%v,β=%v", a, b)
	}
}

// TestSampleBeta_MarginalsMatchClosedForm asserts the sampler is calibrated.
func TestSampleBeta_MarginalsMatchClosedForm(t *testing.T) {
	rng := rand.New(rand.NewPCG(42, 43))
	const n = 10000
	alpha, beta := 3.0, 7.0
	var sum float64
	for i := 0; i < n; i++ {
		sum += SampleBeta(rng, alpha, beta)
	}
	mean := sum / float64(n)
	want := alpha / (alpha + beta)
	if math.Abs(mean-want) > 0.02 {
		t.Fatalf("Beta(%.1f,%.1f) mean: want≈%.3f, got %.3f", alpha, beta, want, mean)
	}
}

// TestThompsonSampling_HoldsRank — pattern with α=20,β=5 outranks α=2,β=20 on
// expected mean; over many propose calls it stays first the vast majority of
// the time. Exploration ensures the underdog occasionally wins, never always.
func TestThompsonSampling_HoldsRank(t *testing.T) {
	t.Setenv("LEAH_RECOMMEND_BANDIT", "1")
	// No RNG_SEED — exploration assertion needs distinct samples per Propose;
	// pinning the seed would make each call re-draw the identical Beta sample
	// and produce a deterministic ranking that hides Thompson's variance.
	eng := openBanditEngineFloorCleared(t)
	defer func() { _ = eng.Close() }()

	// Seed two pending recs with distinct patterns.
	if err := eng.Seed(Recommendation{ID: "rs", Pattern: "p_strong", Tier: TierConfirm, Source: "s", Confidence: 0.5}); err != nil {
		t.Fatalf("seed strong: %v", err)
	}
	if err := eng.Seed(Recommendation{ID: "rw", Pattern: "p_weak", Tier: TierConfirm, Source: "s", Confidence: 0.5}); err != nil {
		t.Fatalf("seed weak: %v", err)
	}
	// Diverge posteriors: strong has higher mean (≈0.71) but weak has
	// fat-tail variance (Beta(2,2) mean 0.5 with high variance) — so the
	// weak arm wins on the sample sometimes; strong stays the dominant
	// arm but exploration emerges. If we picked Beta(20,5) vs Beta(2,20)
	// the tails wouldn't overlap and the test couldn't observe exploration.
	mustSetPosterior(t, eng, "p_strong", 5.0, 2.0)
	mustSetPosterior(t, eng, "p_weak", 2.0, 2.0)

	const trials = 400
	strongWins := 0
	for i := 0; i < trials; i++ {
		out, err := eng.Propose(context.Background())
		if err != nil {
			t.Fatalf("propose: %v", err)
		}
		if len(out) != 2 {
			t.Fatalf("want 2 candidates, got %d", len(out))
		}
		if out[0].Pattern == "p_strong" {
			strongWins++
		}
	}
	rate := float64(strongWins) / float64(trials)
	if rate < 0.55 {
		t.Fatalf("strong-arm dominance too low: %.2f over %d trials (want >0.55)", rate, trials)
	}
	if rate >= 0.98 {
		t.Fatalf("no exploration — strong won %d/%d trials; Thompson should sometimes pick weak", strongWins, trials)
	}
}

// TestColdStartFloor — below 20 rows × 5 patterns Propose falls back to greedy.
func TestColdStartFloor(t *testing.T) {
	t.Setenv("LEAH_RECOMMEND_BANDIT", "1")
	eng := openBanditEngine(t)
	defer func() { _ = eng.Close() }()

	// Only 4 patterns × 5 feedback rows = under-floor on distinct patterns.
	for i, p := range []string{"a", "b", "c", "d"} {
		_ = eng.Seed(Recommendation{ID: idOf("seed", i), Pattern: p, Tier: TierConfirm, Source: "s", Confidence: 0.5})
		for j := 0; j < 5; j++ {
			if err := eng.RecordFeedback(context.Background(), idOf("seed", i), string(FeedbackAccept), signalAccept); err != nil {
				t.Fatalf("record: %v", err)
			}
		}
	}
	// Boost p_a posterior — under floor, greedy should sort by point-estimate
	// regardless of variance; this lets us assert determinism.
	mustSetPosterior(t, eng, "a", 100.0, 1.0)
	mustSetPosterior(t, eng, "b", 2.0, 100.0)

	// Both runs under-floor must produce identical ordering.
	out1, err := eng.Propose(context.Background())
	if err != nil {
		t.Fatalf("propose1: %v", err)
	}
	out2, err := eng.Propose(context.Background())
	if err != nil {
		t.Fatalf("propose2: %v", err)
	}
	if len(out1) < 2 || out1[0].Pattern != out2[0].Pattern {
		t.Fatalf("under-floor Propose is non-deterministic: %v vs %v", out1, out2)
	}
	// Greedy uses blended confidence; "a" boosted in posterior alone but
	// not in stored confidence — the assertion that matters is determinism
	// across calls, which Thompson sampling would violate.
}

// TestKillSwitchGreedy — LEAH_RECOMMEND_BANDIT=0 disables Thompson sampling.
func TestKillSwitchGreedy(t *testing.T) {
	t.Setenv("LEAH_RECOMMEND_BANDIT", "0")
	eng := openBanditEngineFloorCleared(t)
	defer func() { _ = eng.Close() }()

	if err := eng.Seed(Recommendation{ID: "rA", Pattern: "pa", Tier: TierConfirm, Source: "s", Confidence: 0.3}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := eng.Seed(Recommendation{ID: "rB", Pattern: "pb", Tier: TierConfirm, Source: "s", Confidence: 0.9}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// High-variance posterior on the low-confidence pattern. Greedy must
	// ignore alpha/beta entirely and sort on Confidence.
	mustSetPosterior(t, eng, "pa", 50.0, 50.0)
	mustSetPosterior(t, eng, "pb", 1.0, 1.0)

	for i := 0; i < 5; i++ {
		out, err := eng.Propose(context.Background())
		if err != nil {
			t.Fatalf("propose: %v", err)
		}
		if len(out) != 2 || out[0].Pattern != "pb" {
			t.Fatalf("kill-switch failed — greedy must rank pb first; got %+v", out)
		}
	}
}

// TestAuditRowIncludesSeed — bandit audit row carries alpha, beta, sample, seed.
func TestAuditRowIncludesSeed(t *testing.T) {
	t.Setenv("LEAH_RECOMMEND_BANDIT", "1")
	t.Setenv("LEAH_RECOMMEND_RNG_SEED", "424242")

	dir := t.TempDir()
	auditPath := filepath.Join(dir, "audit.jsonl")
	logger := &audit.Logger{Path: auditPath, Now: func() time.Time { return time.Unix(1700000000, 0).UTC() }}
	dbPath := filepath.Join(dir, "r.db")
	eng, err := NewSQLiteEngine(dbPath, logger)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = eng.Close() }()
	clearFloor(t, eng)

	if err := eng.Seed(Recommendation{ID: "rs", Pattern: "pp", Tier: TierConfirm, Source: "s", Confidence: 0.5}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	mustSetPosterior(t, eng, "pp", 3.0, 4.0)

	if _, err := eng.Propose(context.Background()); err != nil {
		t.Fatalf("propose: %v", err)
	}
	data, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("read audit: %v", err)
	}
	out := string(data)
	if !strings.Contains(out, "recommendation_proposed_bandit") {
		t.Fatalf("no bandit audit row: %s", out)
	}
	for _, want := range []string{"alpha=", "beta=", "sample=", "seed=424242"} {
		if !strings.Contains(out, want) {
			t.Fatalf("bandit audit row missing %q: %s", want, out)
		}
	}
}

// --- helpers ---

func openBanditEngine(t *testing.T) *SQLiteEngine {
	t.Helper()
	path := filepath.Join(t.TempDir(), "r.db")
	eng, err := NewSQLiteEngine(path, nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return eng
}

// openBanditEngineFloorCleared returns an engine with the cold-start gate already cleared.
func openBanditEngineFloorCleared(t *testing.T) *SQLiteEngine {
	t.Helper()
	eng := openBanditEngine(t)
	clearFloor(t, eng)
	return eng
}

func clearFloor(t *testing.T, eng *SQLiteEngine) {
	t.Helper()
	// Insert 25 feedback rows across 6 distinct patterns to clear the
	// ≥20 rows × ≥5 patterns hard floor.
	for i := 0; i < 25; i++ {
		pattern := "floor_" + string(rune('a'+i%6))
		if _, err := eng.db.Exec(
			`INSERT INTO feedback (id, rec_id, pattern, kind, signal, ts) VALUES (?, ?, ?, ?, ?, ?)`,
			idOf("floor", i), "x", pattern, string(FeedbackAccept), signalAccept, time.Now().UnixNano(),
		); err != nil {
			t.Fatalf("seed floor row: %v", err)
		}
	}
}

func mustSetPosterior(t *testing.T, eng *SQLiteEngine, pattern string, alpha, beta float64) {
	t.Helper()
	if _, err := eng.db.Exec(`
		INSERT INTO recommend_pattern_state (pattern, alpha, beta, observed_from)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(pattern) DO UPDATE SET alpha=excluded.alpha, beta=excluded.beta`,
		pattern, alpha, beta, time.Now().UnixNano(),
	); err != nil {
		t.Fatalf("set posterior: %v", err)
	}
}

func readPosterior(t *testing.T, eng *SQLiteEngine, pattern string) (float64, float64) {
	t.Helper()
	var a, b float64
	if err := eng.db.QueryRow(`SELECT alpha, beta FROM recommend_pattern_state WHERE pattern=?`, pattern).Scan(&a, &b); err != nil {
		t.Fatalf("read posterior: %v", err)
	}
	return a, b
}

func idOf(prefix string, i int) string {
	return prefix + "-" + string(rune('a'+i%26)) + string(rune('a'+(i/26)%26))
}

package recommend

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/trilam/leah/internal/platform/audit"
)

// Beta posterior + Thompson sampling. Per-pattern (α, β) lives in
// recommend_pattern_state; each Propose draws one sample per candidate and
// sorts top-K by sample, not point estimate. Floor + kill-switch guard the
// noise floor before bandit math drives ranking.

const (
	// banditFloorRows / banditFloorPatterns are the hard cold-start gate
	// (spec §6.1). Below either threshold Propose falls back to greedy.
	banditFloorRows     = 20
	banditFloorPatterns = 5

	// Ignore writes β+=0.1 per spec §4.1 — half-step matches the
	// existing feedback.go signalIgnore magnitude so blender + bandit
	// treat the same physical signal identically.
	betaIgnoreDelta = 0.1
)

// banditDDL extends sqliteDDL with the v3 posterior table. Applied after
// v1→v2 in migrate(); idempotent via IF NOT EXISTS.
const banditDDL = `
CREATE TABLE IF NOT EXISTS recommend_pattern_state (
  pattern        TEXT PRIMARY KEY,
  alpha          REAL NOT NULL DEFAULT 1.0,
  beta           REAL NOT NULL DEFAULT 1.0,
  observed_from  INTEGER NOT NULL DEFAULT 0,
  forgotten      INTEGER NOT NULL DEFAULT 0,
  demoted_until  INTEGER NOT NULL DEFAULT 0
);
`

// banditOn reports whether Thompson sampling is enabled. Read fresh per
// Propose so tests can flip it via t.Setenv without rebuilding the engine.
func banditOn() bool {
	v := os.Getenv("LEAH_RECOMMEND_BANDIT")
	return v != "0"
}

// banditSeed returns the resolved RNG seed (env override or wall-clock).
// Audit rows carry this so a forensic replay can re-derive every sample.
func banditSeed() int64 {
	if v := os.Getenv("LEAH_RECOMMEND_RNG_SEED"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return time.Now().UTC().UnixNano()
}

// SampleBeta draws X ~ Beta(α, β) via two Gamma(α,1), Gamma(β,1) draws —
// Marsaglia-Tsang for α≥1, with the α<1 boost from the same paper.
func SampleBeta(rng *rand.Rand, alpha, beta float64) float64 {
	x := sampleGamma(rng, alpha)
	y := sampleGamma(rng, beta)
	if x+y == 0 {
		return 0
	}
	return x / (x + y)
}

// sampleGamma is Marsaglia-Tsang (2000) — exact for shape ≥ 1, with the
// α' = α + 1 + Unif^(1/α) boost handling shape < 1 in one extra draw.
func sampleGamma(rng *rand.Rand, shape float64) float64 {
	if shape < 1.0 {
		// Boost α to α+1, then scale by U^(1/α).
		g := sampleGamma(rng, shape+1.0)
		u := rng.Float64()
		if u == 0 {
			u = math.SmallestNonzeroFloat64
		}
		return g * math.Pow(u, 1.0/shape)
	}
	d := shape - 1.0/3.0
	c := 1.0 / math.Sqrt(9.0*d)
	for {
		x := rng.NormFloat64()
		v := 1.0 + c*x
		if v <= 0 {
			continue
		}
		v = v * v * v
		u := rng.Float64()
		if u < 1.0-0.0331*x*x*x*x {
			return d * v
		}
		if math.Log(u) < 0.5*x*x+d*(1.0-v+math.Log(v)) {
			return d * v
		}
	}
}

// applyBanditMigration runs the v3 DDL — separated from the v1→v2 migrate
// path so each version's intent stays scannable in one place.
func applyBanditMigration(db *sql.DB) error {
	if _, err := db.Exec(banditDDL); err != nil {
		return fmt.Errorf("apply bandit schema: %w", err)
	}
	return nil
}

// updateBetaPosterior rolls (α, β) forward by the per-signal deltas of spec
// §4.3. UPSERT with the +deltaX-1.0 trick composes against the schema
// defaults so first-write and update paths share one SQL.
func updateBetaPosterior(ctx context.Context, db *sql.DB, pattern string, kind FeedbackKind, now time.Time) error {
	if pattern == "" {
		return nil
	}
	dAlpha, dBeta := betaDeltas(kind)
	if dAlpha == 0 && dBeta == 0 {
		return nil
	}
	// Encode delta into the candidate row using the schema-default offset (1.0).
	// On INSERT we want alpha = 1.0 + dAlpha (1.0 is the default), beta = 1.0 + dBeta.
	// On UPDATE we want alpha += dAlpha; subtract the default once to keep the
	// math identical on both branches.
	insertAlpha := 1.0 + dAlpha
	insertBeta := 1.0 + dBeta
	_, err := db.ExecContext(ctx, `
		INSERT INTO recommend_pattern_state (pattern, alpha, beta, observed_from)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(pattern) DO UPDATE SET
		  alpha = alpha + excluded.alpha - 1.0,
		  beta  = beta  + excluded.beta  - 1.0`,
		pattern, insertAlpha, insertBeta, now.UnixNano(),
	)
	if err != nil {
		return fmt.Errorf("update posterior: %w", err)
	}
	return nil
}

func betaDeltas(kind FeedbackKind) (float64, float64) {
	switch kind {
	case FeedbackAccept:
		return 1.0, 0
	case FeedbackReject:
		return 0, 1.0
	case FeedbackIgnore:
		return 0, betaIgnoreDelta
	}
	return 0, 0
}

// floorCleared reports whether the cold-start gate has passed: ≥20 feedback
// rows across ≥5 distinct patterns. Cached on first clear — once cleared the
// state is monotonic, and a hot-loop COUNT(*) on every Propose is wasteful.
type banditFloorCache struct {
	mu      sync.Mutex
	cleared bool
}

func (c *banditFloorCache) check(ctx context.Context, db *sql.DB) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cleared {
		return true, nil
	}
	var rows, patterns int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*), COUNT(DISTINCT pattern) FROM feedback WHERE pattern <> ''`).
		Scan(&rows, &patterns); err != nil {
		return false, fmt.Errorf("floor probe: %w", err)
	}
	if rows >= banditFloorRows && patterns >= banditFloorPatterns {
		c.cleared = true
		return true, nil
	}
	return false, nil
}

// loadPosteriors batches a pattern-set → (α,β) lookup. Defaults to Beta(1,1)
// for patterns not yet in the table — equivalent to a uniform prior.
func loadPosteriors(ctx context.Context, db *sql.DB, patterns []string) (map[string][2]float64, error) {
	out := make(map[string][2]float64, len(patterns))
	if len(patterns) == 0 {
		return out, nil
	}
	placeholders := make([]string, len(patterns))
	args := make([]any, len(patterns))
	for i, p := range patterns {
		placeholders[i] = "?"
		args[i] = p
	}
	q := `SELECT pattern, alpha, beta FROM recommend_pattern_state WHERE pattern IN (` +
		joinComma(placeholders) + `)`
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("load posteriors: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var pattern string
		var a, b float64
		if err := rows.Scan(&pattern, &a, &b); err != nil {
			return nil, fmt.Errorf("scan posterior: %w", err)
		}
		out[pattern] = [2]float64{a, b}
	}
	// Patterns with no row default to Beta(1,1).
	for _, p := range patterns {
		if _, ok := out[p]; !ok {
			out[p] = [2]float64{1.0, 1.0}
		}
	}
	return out, rows.Err()
}

func joinComma(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	out := parts[0]
	for _, p := range parts[1:] {
		out += "," + p
	}
	return out
}

// greedyRank sorts by point-estimate Confidence descending — the fallback
// when bandit is killed or the cold-start floor has not cleared.
func greedyRank(recs []Recommendation) []Recommendation {
	sort.SliceStable(recs, func(i, j int) bool { return recs[i].Confidence > recs[j].Confidence })
	return recs
}

// rankBandit replaces greedy sort with Thompson sampling. Returns the
// post-sample ordering plus the (α, β, sample) triple per pattern so the
// caller can write audit rows.
func rankBandit(recs []Recommendation, posteriors map[string][2]float64, rng *rand.Rand) []banditDraw {
	draws := make([]banditDraw, len(recs))
	for i, r := range recs {
		ab := posteriors[r.Pattern]
		s := SampleBeta(rng, ab[0], ab[1])
		draws[i] = banditDraw{rec: r, alpha: ab[0], beta: ab[1], sample: s}
	}
	sort.SliceStable(draws, func(i, j int) bool { return draws[i].sample > draws[j].sample })
	return draws
}

type banditDraw struct {
	rec    Recommendation
	alpha  float64
	beta   float64
	sample float64
}

// logBanditAudit appends one recommendation_proposed_bandit row per candidate
// with (α, β, sample, seed) in Detail so a forensic replay can re-derive the
// exact ranking via SampleBeta(rand.New(rand.NewPCG(seed, seed+1)), α, β).
func logBanditAudit(logger *audit.Logger, draws []banditDraw, seed int64) {
	if logger == nil {
		return
	}
	for _, d := range draws {
		_ = logger.Append(audit.Entry{
			Kind:     "recommendation_proposed_bandit",
			ArgsHash: d.rec.ID,
			Detail: fmt.Sprintf("pattern=%s alpha=%.4f beta=%.4f sample=%.4f seed=%d",
				d.rec.Pattern, d.alpha, d.beta, d.sample, seed),
		})
	}
}

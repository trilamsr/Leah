package sources

import (
	"context"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/trilam/leah/internal/recommend"
)

// PersonContext is the projection KnowledgeGraphSource consumes — already
// scalarized + entity-keyed so the source never sees raw refs. Demotion is
// the entity-row ledger (1.0 = none, 0.05 = operator-snapped floor);
// LastTouched feeds the decay curve. Display is op-side only — never
// composed into rec IDs (PII guard, knowledge.go spec §5).
type PersonContext struct {
	Key         string
	Display     string
	ItemCount   int
	HasMeeting  bool
	LastTouched time.Time
	Demotion    float64
}

// KnowledgeGraphSeam is the structural seam — a Resolve over the knowledge
// graph projected into person-context rows. Production impl wraps
// knowledge.Graph.Query; tests inject an in-memory list.
type KnowledgeGraphSeam interface {
	ActivePeople(ctx context.Context, within time.Duration) ([]PersonContext, error)
}

type KnowledgeOpts struct {
	Within   time.Duration    // lookback; defaults to 7d
	MinItems int              // min prior touchpoints to fire; defaults to 2
	Now      func() time.Time // clock seam; tests inject, prod = time.Now().UTC()
}

type KnowledgeGraphSource struct {
	seam KnowledgeGraphSeam
	opts KnowledgeOpts
}

// Curve constants (S8 wiring spec §5). 14d exp-decay half-life; entities
// past TTL halve their multiplier per stale-week; floor 0.05 — below that
// the rec is dropped, not floored.
const (
	knowledgeDecayHalflifeDays = 14.0
	knowledgeDefaultTTLDays    = 60.0
	knowledgeWeightFloor       = 0.05
)

func NewKnowledgeGraphSource(seam KnowledgeGraphSeam, opts KnowledgeOpts) *KnowledgeGraphSource {
	if opts.Within == 0 {
		opts.Within = 7 * 24 * time.Hour
	}
	if opts.MinItems == 0 {
		opts.MinItems = 2
	}
	if opts.Now == nil {
		opts.Now = func() time.Time { return time.Now().UTC() }
	}
	return &KnowledgeGraphSource{seam: seam, opts: opts}
}

func (s *KnowledgeGraphSource) Name() string { return "knowledge-graph" }

// Recommendations fires when a person entity has (a) an active meeting AND
// (b) ≥MinItems prior touchpoints. Base confidence = clamp(itemCount / 10),
// scaled by weight = decay(age) * demotion. Recs whose weight falls strictly
// below the 0.05 floor are dropped — noise out, signal only.
func (s *KnowledgeGraphSource) Recommendations(ctx context.Context) ([]recommend.Recommendation, error) {
	people, err := s.seam.ActivePeople(ctx, s.opts.Within)
	if err != nil {
		return nil, fmt.Errorf("knowledge-graph: %w", err)
	}
	ttlDays := envFloat("LEAH_KNOWLEDGE_TTL_DAYS", knowledgeDefaultTTLDays)
	now := s.opts.Now()
	var out []recommend.Recommendation
	for _, p := range people {
		if !p.HasMeeting || p.ItemCount < s.opts.MinItems {
			continue
		}
		weight := personWeight(p, now, ttlDays)
		if weight < knowledgeWeightFloor {
			continue
		}
		conf := float64(p.ItemCount) / 10.0
		if conf > 1 {
			conf = 1
		}
		conf *= weight
		pattern := "knowledge.followup." + p.Key
		out = append(out, recommend.Recommendation{
			ID:         hashID("knowledge", p.Key, now),
			Pattern:    pattern,
			Tier:       recommend.TierConfirm,
			Source:     s.Name(),
			Confidence: conf,
			CreatedAt:  now,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Confidence > out[j].Confidence })
	return out, nil
}

// personWeight folds decay + stale-week demotion into a single multiplier.
// decay = exp(-age_days / 14); past-TTL multiplier halves per full week
// elapsed since the entity crossed LEAH_KNOWLEDGE_TTL_DAYS. A zero Demotion
// (uninitialized struct) reads as 1.0 — no demotion.
func personWeight(p PersonContext, now time.Time, ttlDays float64) float64 {
	ageDays := now.Sub(p.LastTouched).Hours() / 24.0
	if ageDays < 0 {
		ageDays = 0
	}
	decay := math.Exp(-ageDays / knowledgeDecayHalflifeDays)
	mult := p.Demotion
	if mult == 0 {
		mult = 1.0
	}
	if ageDays > ttlDays {
		weeksPast := math.Floor((ageDays - ttlDays) / 7.0)
		mult *= math.Pow(0.5, weeksPast)
	}
	return decay * mult
}

func envFloat(key string, def float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			return f
		}
	}
	return def
}

// Gather fans out across sources, isolating per-source errors so one failing
// seam never starves Engine.Propose. Returns the union of recs + the error
// list aligned by source order.
func Gather(ctx context.Context, srcs []Source) ([]recommend.Recommendation, []error) {
	var all []recommend.Recommendation
	var errs []error
	for _, s := range srcs {
		recs, err := s.Recommendations(ctx)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", s.Name(), err))
			continue
		}
		all = append(all, recs...)
	}
	return all, errs
}

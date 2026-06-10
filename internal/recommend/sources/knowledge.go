package sources

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/trilam/leah/internal/recommend"
)

// PersonContext is the projection KnowledgeGraphSource consumes — already
// scalarized + entity-keyed so the source never sees raw refs. Display is
// carried for op-side rendering but never composed into rec IDs (PII guard,
// knowledge.go spec §5).
type PersonContext struct {
	Key         string
	Display     string
	ItemCount   int
	HasMeeting  bool
	LastTouched time.Time
}

// KnowledgeGraphSeam is the structural seam — a Resolve over the knowledge
// graph projected into person-context rows. Production impl wraps
// knowledge.Graph.Query; tests inject an in-memory list.
type KnowledgeGraphSeam interface {
	ActivePeople(ctx context.Context, within time.Duration) ([]PersonContext, error)
}

type KnowledgeOpts struct {
	Within   time.Duration // lookback; defaults to 7d
	MinItems int           // min prior touchpoints to fire; defaults to 2
}

type KnowledgeGraphSource struct {
	seam KnowledgeGraphSeam
	opts KnowledgeOpts
}

func NewKnowledgeGraphSource(seam KnowledgeGraphSeam, opts KnowledgeOpts) *KnowledgeGraphSource {
	if opts.Within == 0 {
		opts.Within = 7 * 24 * time.Hour
	}
	if opts.MinItems == 0 {
		opts.MinItems = 2
	}
	return &KnowledgeGraphSource{seam: seam, opts: opts}
}

func (s *KnowledgeGraphSource) Name() string { return "knowledge-graph" }

// Recommendations fires when a person entity has (a) an active meeting AND
// (b) ≥MinItems prior touchpoints. Confidence = clamp(itemCount / 10).
func (s *KnowledgeGraphSource) Recommendations(ctx context.Context) ([]recommend.Recommendation, error) {
	people, err := s.seam.ActivePeople(ctx, s.opts.Within)
	if err != nil {
		return nil, fmt.Errorf("knowledge-graph: %w", err)
	}
	var out []recommend.Recommendation
	now := time.Now().UTC()
	for _, p := range people {
		if !p.HasMeeting || p.ItemCount < s.opts.MinItems {
			continue
		}
		conf := float64(p.ItemCount) / 10.0
		if conf > 1 {
			conf = 1
		}
		// Pattern composed from the entity key (PII-free per knowledge.go),
		// never the Display name — keeps the audit/propose row safe to log.
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

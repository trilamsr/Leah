package sources

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/trilam/leah/internal/recommend"
)

// WorkItem is the minimal operator-scoped projection every work tool maps to.
type WorkItem struct {
	Key     string
	Title   string
	Updated time.Time
}

// WorkItemSeam lists the operator's items touched within a window.
type WorkItemSeam interface {
	StaleItems(ctx context.Context, within time.Duration) ([]WorkItem, error)
}

type WorkToolOpts struct {
	Within    time.Duration
	StaleSpan time.Duration
	Now       func() time.Time
}

type workToolSource struct {
	name string
	seam WorkItemSeam
	opts WorkToolOpts
}

func newWorkToolSource(name string, seam WorkItemSeam, opts WorkToolOpts) *workToolSource {
	if opts.Within == 0 {
		opts.Within = 30 * 24 * time.Hour
	}
	if opts.StaleSpan == 0 {
		opts.StaleSpan = 3 * 24 * time.Hour
	}
	if opts.Now == nil {
		opts.Now = func() time.Time { return time.Now().UTC() }
	}
	return &workToolSource{name: name, seam: seam, opts: opts}
}

// NewJiraSource wires jira.ListMyIssues; Linear/Confluence wire their analogues.
func NewJiraSource(seam WorkItemSeam, opts WorkToolOpts) Source {
	return newWorkToolSource("jira", seam, opts)
}

func NewLinearSource(seam WorkItemSeam, opts WorkToolOpts) Source {
	return newWorkToolSource("linear", seam, opts)
}

func NewConfluenceSource(seam WorkItemSeam, opts WorkToolOpts) Source {
	return newWorkToolSource("confluence", seam, opts)
}

func (s *workToolSource) Name() string { return s.name }

// Recommendations fires one confirm-tier rec per item untouched ≥StaleSpan.
func (s *workToolSource) Recommendations(ctx context.Context) ([]recommend.Recommendation, error) {
	now := s.opts.Now()
	items, err := s.seam.StaleItems(ctx, s.opts.Within)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", s.name, err)
	}
	var out []recommend.Recommendation
	for _, it := range items {
		age := now.Sub(it.Updated)
		if age < s.opts.StaleSpan {
			continue
		}
		conf := age.Hours() / s.opts.Within.Hours()
		if conf > 1 {
			conf = 1
		}
		pattern := s.name + ".followup." + it.Key
		out = append(out, recommend.Recommendation{
			ID:         hashID(s.name, it.Key, now),
			Pattern:    pattern,
			Tier:       recommend.TierConfirm,
			Source:     s.name,
			Confidence: conf,
			CreatedAt:  now,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Confidence > out[j].Confidence })
	return out, nil
}

package recommend

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/trilam/leah/internal/platform/audit"
	"github.com/trilam/leah/internal/platform/contracts"
)

// autoApplyMaxPerHour bounds runaway auto-tier execution per spec §6.
const autoApplyMaxPerHour = 10

var (
	ErrAutoLimitExceeded = errors.New("recommend: auto-apply rate limit exceeded")
	ErrPatternUnknown    = errors.New("recommend: pattern not registered")
)

type AutoTierPattern struct {
	Pattern string
	Action  Action
}

// AutoApplier holds the execution surface; MemoryEngine holds tier-rubric semantics.
type AutoApplier struct {
	audit *audit.Logger
	now   func() time.Time

	mu       sync.Mutex
	patterns map[string]Action
	history  map[string][]time.Time
}

func NewAutoApplier(logger *audit.Logger, now func() time.Time) *AutoApplier {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &AutoApplier{
		audit:    logger,
		now:      now,
		patterns: make(map[string]Action),
		history:  make(map[string][]time.Time),
	}
}

func (a *AutoApplier) Register(p AutoTierPattern) error {
	if p.Pattern == "" || p.Action == nil {
		return errors.New("recommend: pattern and action required")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.patterns[p.Pattern] = p.Action
	return nil
}

func (a *AutoApplier) Apply(ctx context.Context, rec Recommendation) error {
	a.mu.Lock()
	action, ok := a.patterns[rec.Pattern]
	if !ok {
		a.mu.Unlock()
		return ErrPatternUnknown
	}
	now := a.now()
	a.history[rec.Pattern] = pruneOlderThan(a.history[rec.Pattern], now.Add(-time.Hour))
	if len(a.history[rec.Pattern]) >= autoApplyMaxPerHour {
		a.mu.Unlock()
		return ErrAutoLimitExceeded
	}
	a.history[rec.Pattern] = append(a.history[rec.Pattern], now)
	a.mu.Unlock()

	if err := action(ctx); err != nil {
		_ = a.logApply(rec, "failed")
		return err
	}
	return a.logApply(rec, "success")
}

func (a *AutoApplier) logApply(rec Recommendation, outcome string) error {
	if a.audit == nil {
		return nil
	}
	return a.audit.Append(audit.Entry{
		Kind:     "recommendation_apply",
		ArgsHash: rec.ID,
		Outcome:  outcome,
		Detail:   rec.Pattern,
	})
}

func pruneOlderThan(ts []time.Time, cutoff time.Time) []time.Time {
	out := ts[:0]
	for _, t := range ts {
		if !t.Before(cutoff) {
			out = append(out, t)
		}
	}
	return out
}

// FormatGoOnSave is auto-tier: reversible (gofmt is idempotent) and local-only.
func FormatGoOnSave(exec contracts.OSExec, file string) AutoTierPattern {
	return AutoTierPattern{
		Pattern: "format_go_on_save",
		Action: func(ctx context.Context) error {
			_, _, err := exec.Run(ctx, "gofmt", "-w", file)
			return err
		},
	}
}

// CommitAtFocusEnd is auto-tier: reversible via `git reset HEAD~1`, local-only.
func CommitAtFocusEnd(exec contracts.OSExec, msg string) AutoTierPattern {
	return AutoTierPattern{
		Pattern: "commit_at_focus_end",
		Action: func(ctx context.Context) error {
			_, _, err := exec.Run(ctx, "git", "commit", "-m", msg)
			return err
		},
	}
}

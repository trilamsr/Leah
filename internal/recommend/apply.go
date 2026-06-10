package recommend

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/contracts"
)

// autoApplyMaxPerHour bounds runaway auto-tier execution per spec §6.
const autoApplyMaxPerHour = 10

var (
	// ErrAutoLimitExceeded fires when an auto-tier pattern has applied
	// autoApplyMaxPerHour times in the trailing hour.
	ErrAutoLimitExceeded = errors.New("recommend: auto-apply rate limit exceeded")

	// ErrPatternUnknown fires when Apply receives a Pattern that has not
	// been Register()-ed with the AutoApplier.
	ErrPatternUnknown = errors.New("recommend: pattern not registered")
)

// AutoTierPattern pairs a pattern key with the Action that fires for it.
// Built by FormatGoOnSave / CommitAtFocusEnd / future adapters.
type AutoTierPattern struct {
	Pattern string
	Action  Action
}

// AutoApplier wires 1-2 vetted auto-tier patterns to a rate-limited Apply
// path. Distinct from MemoryEngine: the engine holds tier-rubric semantics
// (auto/confirm/silent/blocked); the applier owns the auto-tier executable
// surface and the per-pattern rate-limit failsafe.
type AutoApplier struct {
	exec  contracts.OSExec
	audit *audit.Logger
	now   func() time.Time

	mu       sync.Mutex
	patterns map[string]Action
	// history keys on pattern; values are recent apply timestamps inside
	// the trailing hour, GC'd on every Apply call.
	history map[string][]time.Time
}

// NewAutoApplier returns an AutoApplier with no patterns registered.
// now defaults to time.Now when nil — kept injectable for hermetic tests.
func NewAutoApplier(exec contracts.OSExec, logger *audit.Logger, now func() time.Time) *AutoApplier {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &AutoApplier{
		exec:     exec,
		audit:    logger,
		now:      now,
		patterns: make(map[string]Action),
		history:  make(map[string][]time.Time),
	}
}

// Register adds an AutoTierPattern. Re-registering the same key overwrites.
func (a *AutoApplier) Register(p AutoTierPattern) error {
	if p.Pattern == "" || p.Action == nil {
		return errors.New("recommend: pattern and action required")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.patterns[p.Pattern] = p.Action
	return nil
}

// Apply runs the registered Action for rec.Pattern subject to the per-pattern
// hourly cap. Emits exactly one recommendation_apply audit row whose Outcome
// is success or failed. Returns ErrPatternUnknown when the pattern has not
// been Register()-ed and ErrAutoLimitExceeded once the cap is hit.
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

// pruneOlderThan returns ts with entries strictly before cutoff dropped.
// Stays O(n) per call; n is bounded by the hourly cap so this never grows.
func pruneOlderThan(ts []time.Time, cutoff time.Time) []time.Time {
	out := ts[:0]
	for _, t := range ts {
		if !t.Before(cutoff) {
			out = append(out, t)
		}
	}
	return out
}

// FormatGoOnSave runs `gofmt -w <file>` against an operator-touched file.
// Reversible (gofmt is idempotent), local-only — meets the auto-tier rubric.
func FormatGoOnSave(exec contracts.OSExec, file string) AutoTierPattern {
	return AutoTierPattern{
		Pattern: "format_go_on_save",
		Action: func(ctx context.Context) error {
			_, _, err := exec.Run(ctx, "gofmt", "-w", file)
			return err
		},
	}
}

// CommitAtFocusEnd runs `git commit -m <msg>` at the end of a focus block.
// Reversible via `git reset HEAD~1`, local-only — auto-tier per rubric §6.
func CommitAtFocusEnd(exec contracts.OSExec, msg string) AutoTierPattern {
	return AutoTierPattern{
		Pattern: "commit_at_focus_end",
		Action: func(ctx context.Context) error {
			_, _, err := exec.Run(ctx, "git", "commit", "-m", msg)
			return err
		},
	}
}

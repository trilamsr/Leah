package dispatcher

import (
	"sync"
	"time"

	"github.com/trilam/leah/internal/obs"
)

// cliProgressBuckets — A7 target is felt-latency ≤500ms from dispatch to the
// first user-visible "still working" signal; bands cluster sub-second so
// regressions show up as a bucket shift, not as a long-tail dilution.
var cliProgressBuckets = []float64{0.05, 0.1, 0.25, 0.5, 1, 2}

// ProgressTimer measures dispatch → first progress emission (spinner tick,
// status banner — anything that signals "still working" before terminal output
// begins). Nil-safe at every level: a nil receiver, a nil registry, or a
// command that never emits all collapse to no-op without per-call guards.
type ProgressTimer struct {
	hist    *obs.Histogram
	command string
	start   time.Time
	once    sync.Once
}

func NewProgressTimer(reg *obs.Registry, command string) *ProgressTimer {
	t := &ProgressTimer{command: command, start: time.Now()}
	if reg != nil {
		t.hist = reg.Histogram("leah_cli_dispatch_to_first_progress_seconds", cliProgressBuckets)
	}
	return t
}

// Emit records the dispatch→first-progress latency on the first call; later
// calls are dropped so spinner ticks don't flood the histogram.
func (t *ProgressTimer) Emit() {
	if t == nil || t.hist == nil {
		return
	}
	t.once.Do(func() {
		t.hist.Observe(
			map[string]string{"command": t.command},
			time.Since(t.start).Seconds(),
		)
	})
}

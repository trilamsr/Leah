package dispatcher

import (
	"io"
	"sync"
	"time"

	"github.com/trilam/leah/internal/obs"
)

// cliFirstByteBuckets — A6 target is p95 ≤200ms exclusive of LLM round-trip,
// so bands cluster sub-second; the 5s tail catches stalls without diluting
// the bucket near the SLA.
var cliFirstByteBuckets = []float64{0.025, 0.05, 0.1, 0.2, 0.5, 1, 2, 5}

// FirstByteTimer measures dispatch → first stdout byte for a CLI invocation,
// excluding any LLM round-trip (callers wrap only the non-LLM stdout writer).
// nil-safe: a nil registry produces a timer whose Wrap returns the underlying
// writer untouched.
type FirstByteTimer struct {
	hist    *telemetry.Histogram
	command string
	start   time.Time
}

func NewFirstByteTimer(reg *telemetry.Registry, command string) *FirstByteTimer {
	t := &FirstByteTimer{command: command, start: time.Now()}
	if reg != nil {
		t.hist = reg.Histogram("leah_cli_dispatch_to_first_byte_seconds", cliFirstByteBuckets)
	}
	return t
}

// Wrap returns w when the timer is disabled; otherwise a sniffer that fires
// the histogram once on the first non-empty Write.
func (t *FirstByteTimer) Wrap(w io.Writer) io.Writer {
	if t == nil || t.hist == nil {
		return w
	}
	return &firstByteSniffer{inner: w, timer: t}
}

type firstByteSniffer struct {
	inner io.Writer
	timer *FirstByteTimer
	once  sync.Once
}

func (s *firstByteSniffer) Write(p []byte) (int, error) {
	if len(p) > 0 {
		s.once.Do(func() {
			s.timer.hist.Observe(
				map[string]string{"command": s.timer.command},
				time.Since(s.timer.start).Seconds(),
			)
		})
	}
	return s.inner.Write(p)
}

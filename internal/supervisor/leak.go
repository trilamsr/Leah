package supervisor

import "time"

type leakVerdict int

const (
	leakHealthy leakVerdict = iota
	leakWarn
	leakEvict
)

type leakConfig struct {
	WarnSlopeMBPerMin  float64
	WarnSustain        time.Duration
	EvictSlopeMBPerMin float64
	EvictSustain       time.Duration
	SampleWindow       time.Duration
}

type rssSample struct {
	at time.Time
	mb int
}

type leakDetector struct {
	cfg    leakConfig
	series map[string][]rssSample
}

func newLeakDetector(cfg leakConfig) *leakDetector {
	return &leakDetector{cfg: cfg, series: map[string][]rssSample{}}
}

func (l *leakDetector) observe(name string, rssMB int, at time.Time) {
	s := append(l.series[name], rssSample{at: at, mb: rssMB})
	cutoff := at.Add(-l.cfg.SampleWindow)
	i := 0
	for ; i < len(s); i++ {
		if !s[i].at.Before(cutoff) {
			break
		}
	}
	l.series[name] = append(s[:0:0], s[i:]...)
}

func (l *leakDetector) classify(name string, now time.Time) leakVerdict {
	s := l.series[name]
	if len(s) < 3 {
		return leakHealthy
	}
	if v := l.slopeOver(s, now, l.cfg.EvictSustain); v >= l.cfg.EvictSlopeMBPerMin {
		return leakEvict
	}
	if v := l.slopeOver(s, now, l.cfg.WarnSustain); v >= l.cfg.WarnSlopeMBPerMin {
		return leakWarn
	}
	return leakHealthy
}

// slopeOver returns MB/min over `window`; 0 when span < window-30s.
func (l *leakDetector) slopeOver(s []rssSample, now time.Time, window time.Duration) float64 {
	if len(s) < 2 {
		return 0
	}
	cutoff := now.Add(-window)
	first := -1
	for i := range s {
		if !s[i].at.Before(cutoff) {
			first = i
			break
		}
	}
	if first < 0 || first >= len(s)-1 {
		return 0
	}
	a, b := s[first], s[len(s)-1]
	span := b.at.Sub(a.at)
	if span < window-30*time.Second {
		return 0
	}
	if span <= 0 {
		return 0
	}
	return float64(b.mb-a.mb) / span.Minutes()
}

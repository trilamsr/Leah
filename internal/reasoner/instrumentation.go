package reasoner

import "github.com/trilam/leah/internal/obs"

type CacheOutcome string

const (
	OutcomeHit      CacheOutcome = "hit"
	OutcomeMiss     CacheOutcome = "miss"
	OutcomeDisabled CacheOutcome = "disabled"
)

var cacheSavingsBuckets = []float64{128, 512, 1024, 2048, 4096, 8192, 16384}

func BindInstrumentation(r *telemetry.Registry) {
	if r == nil {
		return
	}
	_ = r.Counter("leah_reasoner_cache_hit_total")
	_ = r.Histogram("leah_reasoner_cache_savings_tokens", cacheSavingsBuckets)
}

func RecordCacheOutcome(r *telemetry.Registry, outcome CacheOutcome, savedTokens int64) {
	if r == nil {
		return
	}
	r.Counter("leah_reasoner_cache_hit_total").Inc(map[string]string{"outcome": string(outcome)})
	if outcome == OutcomeHit && savedTokens > 0 {
		r.Histogram("leah_reasoner_cache_savings_tokens", cacheSavingsBuckets).Observe(nil, float64(savedTokens))
	}
}

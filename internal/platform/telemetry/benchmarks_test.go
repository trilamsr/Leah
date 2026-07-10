package telemetry

import "testing"

// BenchmarkCounterAdd_NoLabels measures the bare-label per-observation cost.
func BenchmarkCounterAdd_NoLabels(b *testing.B) {
	r := NewRegistry()
	c := r.Counter("leah_action_total")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Inc(nil)
	}
}

// BenchmarkCounterAdd_TwoLabels exercises the hot path with a fixed label-set.
func BenchmarkCounterAdd_TwoLabels(b *testing.B) {
	r := NewRegistry()
	c := r.Counter("leah_action_total")
	labels := map[string]string{"kind": "ship", "outcome": "success"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Add(labels, 1)
	}
}

// BenchmarkGaugeSet_TwoLabels exercises Gauge.Set with a fixed label-set.
func BenchmarkGaugeSet_TwoLabels(b *testing.B) {
	r := NewRegistry()
	g := r.Gauge("leah_budget_spent_usd")
	labels := map[string]string{"kind": "tokens", "model": "opus"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		g.Set(labels, float64(i))
	}
}

// BenchmarkHistogramObserve_TwoLabels exercises the histogram hot path.
func BenchmarkHistogramObserve_TwoLabels(b *testing.B) {
	r := NewRegistry()
	h := r.Histogram("leah_reasoner_latency_ms", []float64{50, 100, 250, 500, 1000})
	labels := map[string]string{"model": "opus", "outcome": "ok"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.Observe(labels, 42)
	}
}

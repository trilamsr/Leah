package obs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Registry holds the in-process metric series. No Prometheus dep — pure
// Go; snapshot to JSON for downstream selflearn / dashboards.
type Registry struct {
	mu         sync.Mutex
	counters   map[string]*Counter
	gauges     map[string]*Gauge
	histograms map[string]*Histogram
}

// NewRegistry returns an empty registry. Callers typically share one
// per-process via a package-global; the daemon owns its own.
func NewRegistry() *Registry {
	return &Registry{
		counters:   map[string]*Counter{},
		gauges:     map[string]*Gauge{},
		histograms: map[string]*Histogram{},
	}
}

// Counter returns (creating if needed) the counter for name.
func (r *Registry) Counter(name string) *Counter {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c, ok := r.counters[name]; ok {
		return c
	}
	c := &Counter{name: name, values: map[string]int64{}}
	r.counters[name] = c
	return c
}

// Gauge returns (creating if needed) the gauge for name.
func (r *Registry) Gauge(name string) *Gauge {
	r.mu.Lock()
	defer r.mu.Unlock()
	if g, ok := r.gauges[name]; ok {
		return g
	}
	g := &Gauge{name: name, values: map[string]float64{}}
	r.gauges[name] = g
	return g
}

// Histogram returns (creating if needed) the histogram for name with the
// given upper-bucket bounds (exclusive +Inf is always implicit).
func (r *Registry) Histogram(name string, buckets []float64) *Histogram {
	r.mu.Lock()
	defer r.mu.Unlock()
	if h, ok := r.histograms[name]; ok {
		return h
	}
	sorted := append([]float64(nil), buckets...)
	sort.Float64s(sorted)
	h := &Histogram{
		name:    name,
		buckets: sorted,
		values:  map[string]*histSeries{},
	}
	r.histograms[name] = h
	return h
}

// Counter is a monotonic per-label-set counter.
type Counter struct {
	name   string
	mu     sync.Mutex
	values map[string]int64
}

// Inc adds 1 to the counter at the given label set.
func (c *Counter) Inc(labels map[string]string) { c.Add(labels, 1) }

// Add adds n to the counter at the given label set.
func (c *Counter) Add(labels map[string]string, n int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.values[flatten(c.name, labels)] += n
}

// Gauge holds a settable float value per label set.
type Gauge struct {
	name   string
	mu     sync.Mutex
	values map[string]float64
}

// Set overwrites the gauge value at the given label set.
func (g *Gauge) Set(labels map[string]string, v float64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.values[flatten(g.name, labels)] = v
}

// Histogram records counted observations into fixed buckets.
type Histogram struct {
	name    string
	buckets []float64
	mu      sync.Mutex
	values  map[string]*histSeries
}

type histSeries struct {
	count   int64
	sum     float64
	buckets []int64 // parallel to Histogram.buckets; final slot = +Inf
}

// Observe records v in the histogram at the given label set.
func (h *Histogram) Observe(labels map[string]string, v float64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	key := flatten(h.name, labels)
	s, ok := h.values[key]
	if !ok {
		s = &histSeries{buckets: make([]int64, len(h.buckets)+1)}
		h.values[key] = s
	}
	s.count++
	s.sum += v
	placed := false
	for i, b := range h.buckets {
		if v <= b {
			s.buckets[i]++
			placed = true
			break
		}
	}
	if !placed {
		s.buckets[len(h.buckets)]++ // +Inf bucket
	}
}

// flatten produces the canonical "name|k1=v1,k2=v2" key. Label keys are
// sorted so the same map produces the same key regardless of insertion
// order — required for stable snapshot diffs.
func flatten(name string, labels map[string]string) string {
	if len(labels) == 0 {
		return name
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(name)
	b.WriteString("|")
	for i, k := range keys {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(labels[k])
	}
	return b.String()
}

// Snapshot atomically writes the current registry state to path as JSON
// matching spec §3.2. Parent dir is created with 0700.
func (r *Registry) Snapshot(path string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	type histOut struct {
		Count   int64            `json:"count"`
		Sum     float64          `json:"sum"`
		Buckets map[string]int64 `json:"buckets"`
	}

	counters := map[string]int64{}
	for _, c := range r.counters {
		c.mu.Lock()
		for k, v := range c.values {
			counters[k] = v
		}
		c.mu.Unlock()
	}
	gauges := map[string]float64{}
	for _, g := range r.gauges {
		g.mu.Lock()
		for k, v := range g.values {
			gauges[k] = v
		}
		g.mu.Unlock()
	}
	hists := map[string]histOut{}
	for _, h := range r.histograms {
		h.mu.Lock()
		for k, s := range h.values {
			bk := map[string]int64{}
			for i, b := range h.buckets {
				bk[strconv.FormatFloat(b, 'f', -1, 64)] = s.buckets[i]
			}
			bk["+Inf"] = s.buckets[len(h.buckets)]
			hists[k] = histOut{Count: s.count, Sum: s.sum, Buckets: bk}
		}
		h.mu.Unlock()
	}

	out := map[string]any{
		"ts":         time.Now().UTC().Format(time.RFC3339),
		"counters":   counters,
		"gauges":     gauges,
		"histograms": hists,
	}
	raw, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

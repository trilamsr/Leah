package obs

import (
	"encoding/json"
	"hash/fnv"
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
	name string
	mu   sync.Mutex
	// keyCache amortizes the sort+build cost of flatten over repeated
	// observations of the same label-set — the hot-path optimization for
	// #22. Per-series scope bounds growth to the metric's lifetime.
	keyCache map[uint64]string
	values   map[string]int64
}

// Inc adds 1 to the counter at the given label set.
func (c *Counter) Inc(labels map[string]string) { c.Add(labels, 1) }

// Add adds n to the counter at the given label set.
func (c *Counter) Add(labels map[string]string, n int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.values[c.flattenKey(labels)] += n
}

// Declare materializes the series at labels with zero count without
// recording any observation. Use for cold-seeding /metrics so the series
// surfaces pre-event.
func (c *Counter) Declare(labels map[string]string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := c.flattenKey(labels)
	if _, ok := c.values[key]; !ok {
		c.values[key] = 0
	}
}

func (c *Counter) flattenKey(labels map[string]string) string {
	return cachedFlatten(c.name, labels, &c.keyCache)
}

// Gauge holds a settable float value per label set.
type Gauge struct {
	name     string
	mu       sync.Mutex
	keyCache map[uint64]string
	values   map[string]float64
}

// Set overwrites the gauge value at the given label set.
func (g *Gauge) Set(labels map[string]string, v float64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.values[g.flattenKey(labels)] = v
}

// Declare materializes the series at labels with zero value without
// disturbing an existing value. Use for cold-seeding /metrics.
func (g *Gauge) Declare(labels map[string]string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	key := g.flattenKey(labels)
	if _, ok := g.values[key]; !ok {
		g.values[key] = 0
	}
}

func (g *Gauge) flattenKey(labels map[string]string) string {
	return cachedFlatten(g.name, labels, &g.keyCache)
}

// Histogram records counted observations into fixed buckets.
type Histogram struct {
	name     string
	buckets  []float64
	mu       sync.Mutex
	keyCache map[uint64]string
	values   map[string]*histSeries
}

func (h *Histogram) flattenKey(labels map[string]string) string {
	return cachedFlatten(h.name, labels, &h.keyCache)
}

type histSeries struct {
	count   int64
	sum     float64
	buckets []int64 // parallel to Histogram.buckets; final slot = +Inf
}

// Declare materializes the series at labels with count=0, sum=0 without
// recording an observation. Use for cold-seeding /metrics — Observe(0)
// would record a real sample and skew p50.
func (h *Histogram) Declare(labels map[string]string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	key := h.flattenKey(labels)
	if _, ok := h.values[key]; !ok {
		h.values[key] = &histSeries{buckets: make([]int64, len(h.buckets)+1)}
	}
}

// Observe records v in the histogram at the given label set.
func (h *Histogram) Observe(labels map[string]string, v float64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	key := h.flattenKey(labels)
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

// cachedFlatten returns the canonical flattened key, computing it on first
// sight and serving cached entries thereafter. Caller MUST hold the
// owning series' mutex — cache writes are not concurrency-safe alone.
func cachedFlatten(name string, labels map[string]string, cache *map[uint64]string) string {
	if len(labels) == 0 {
		return name
	}
	fp := labelFingerprint(labels)
	if *cache != nil {
		if s, ok := (*cache)[fp]; ok {
			return s
		}
	}
	s := flatten(name, labels)
	if *cache == nil {
		*cache = map[uint64]string{}
	}
	(*cache)[fp] = s
	return s
}

// labelFingerprint hashes the label-set order-independently so the cache
// key is computable without sorting on the hot path. Per-pair FNV-1a is
// XOR-combined; collision is astronomically unlikely for typical N≲10³
// distinct sets per series.
func labelFingerprint(labels map[string]string) uint64 {
	var fp uint64
	for k, v := range labels {
		h := fnv.New64a()
		_, _ = h.Write([]byte(k))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(v))
		fp ^= h.Sum64()
	}
	return fp
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
//
// Copy-and-release: r.mu is held only long enough to snapshot the series
// handles, and each per-series lock only long enough to copy its values.
// Marshal and file I/O run lock-free so concurrent observers never stall
// on snapshot duration.
func (r *Registry) Snapshot(path string) error {
	type histOut struct {
		Count   int64            `json:"count"`
		Sum     float64          `json:"sum"`
		Buckets map[string]int64 `json:"buckets"`
	}

	r.mu.Lock()
	counterHandles := make([]*Counter, 0, len(r.counters))
	for _, c := range r.counters {
		counterHandles = append(counterHandles, c)
	}
	gaugeHandles := make([]*Gauge, 0, len(r.gauges))
	for _, g := range r.gauges {
		gaugeHandles = append(gaugeHandles, g)
	}
	histHandles := make([]*Histogram, 0, len(r.histograms))
	for _, h := range r.histograms {
		histHandles = append(histHandles, h)
	}
	r.mu.Unlock()

	counters := map[string]int64{}
	for _, c := range counterHandles {
		c.mu.Lock()
		for k, v := range c.values {
			counters[k] = v
		}
		c.mu.Unlock()
	}
	gauges := map[string]float64{}
	for _, g := range gaugeHandles {
		g.mu.Lock()
		for k, v := range g.values {
			gauges[k] = v
		}
		g.mu.Unlock()
	}
	hists := map[string]histOut{}
	for _, h := range histHandles {
		h.mu.Lock()
		// Buckets list is immutable post-construction; snapshot the per-key
		// counters + sum into a local copy so marshal runs unlocked.
		type localSeries struct {
			count   int64
			sum     float64
			buckets []int64
		}
		local := make(map[string]localSeries, len(h.values))
		for k, s := range h.values {
			cp := make([]int64, len(s.buckets))
			copy(cp, s.buckets)
			local[k] = localSeries{count: s.count, sum: s.sum, buckets: cp}
		}
		buckets := h.buckets
		h.mu.Unlock()
		for k, s := range local {
			bk := map[string]int64{}
			for i, b := range buckets {
				bk[strconv.FormatFloat(b, 'f', -1, 64)] = s.buckets[i]
			}
			bk["+Inf"] = s.buckets[len(buckets)]
			hists[k] = histOut{Count: s.count, Sum: s.sum, Buckets: bk}
		}
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

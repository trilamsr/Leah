package telemetry

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

// WritePrometheus emits the registry in Prometheus text exposition format,
// sorted for deterministic output. Histogram bucket counts are cumulative
// (Prometheus semantics) — the registry stores per-bucket counts, so we
// accumulate on emission.
func (r *Registry) WritePrometheus(w io.Writer) error {
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

	sort.Slice(counterHandles, func(i, j int) bool { return counterHandles[i].name < counterHandles[j].name })
	sort.Slice(gaugeHandles, func(i, j int) bool { return gaugeHandles[i].name < gaugeHandles[j].name })
	sort.Slice(histHandles, func(i, j int) bool { return histHandles[i].name < histHandles[j].name })

	for _, c := range counterHandles {
		c.mu.Lock()
		series := make([]string, 0, len(c.values))
		for key := range c.values {
			series = append(series, key)
		}
		sort.Strings(series)
		vals := make(map[string]int64, len(c.values))
		for k, v := range c.values {
			vals[k] = v
		}
		name := c.name
		c.mu.Unlock()

		pn := promName(name)
		if _, err := fmt.Fprintf(w, "# TYPE %s counter\n", pn); err != nil {
			return err
		}
		for _, key := range series {
			labels := parseKey(name, key)
			if _, err := fmt.Fprintf(w, "%s%s %d\n", pn, promLabels(labels), vals[key]); err != nil {
				return err
			}
		}
	}

	for _, g := range gaugeHandles {
		g.mu.Lock()
		series := make([]string, 0, len(g.values))
		for key := range g.values {
			series = append(series, key)
		}
		sort.Strings(series)
		vals := make(map[string]float64, len(g.values))
		for k, v := range g.values {
			vals[k] = v
		}
		name := g.name
		g.mu.Unlock()

		pn := promName(name)
		if _, err := fmt.Fprintf(w, "# TYPE %s gauge\n", pn); err != nil {
			return err
		}
		for _, key := range series {
			labels := parseKey(name, key)
			if _, err := fmt.Fprintf(w, "%s%s %s\n", pn, promLabels(labels), formatFloat(vals[key])); err != nil {
				return err
			}
		}
	}

	for _, h := range histHandles {
		h.mu.Lock()
		series := make([]string, 0, len(h.values))
		for key := range h.values {
			series = append(series, key)
		}
		sort.Strings(series)
		type local struct {
			count   int64
			sum     float64
			buckets []int64
		}
		copies := make(map[string]local, len(h.values))
		for k, s := range h.values {
			cp := make([]int64, len(s.buckets))
			copy(cp, s.buckets)
			copies[k] = local{count: s.count, sum: s.sum, buckets: cp}
		}
		buckets := h.buckets
		name := h.name
		h.mu.Unlock()

		pn := promName(name)
		if _, err := fmt.Fprintf(w, "# TYPE %s histogram\n", pn); err != nil {
			return err
		}
		for _, key := range series {
			labels := parseKey(name, key)
			s := copies[key]
			// Cumulative bucket counts: Prometheus expects le="0.1" to include
			// observations <=0.1, le="1" to include <=1, etc.
			var cumulative int64
			for i, b := range buckets {
				cumulative += s.buckets[i]
				lbls := withLabel(labels, "le", strconv.FormatFloat(b, 'f', -1, 64))
				if _, err := fmt.Fprintf(w, "%s_bucket%s %d\n", pn, promLabels(lbls), cumulative); err != nil {
					return err
				}
			}
			cumulative += s.buckets[len(buckets)]
			lbls := withLabel(labels, "le", "+Inf")
			if _, err := fmt.Fprintf(w, "%s_bucket%s %d\n", pn, promLabels(lbls), cumulative); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(w, "%s_sum%s %s\n", pn, promLabels(labels), formatFloat(s.sum)); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(w, "%s_count%s %d\n", pn, promLabels(labels), s.count); err != nil {
				return err
			}
		}
	}
	return nil
}

// parseKey reverses flatten()'s "name|k1=v1,k2=v2" encoding. Returns nil for
// the no-labels case where the key equals the metric name.
func parseKey(name, key string) map[string]string {
	if key == name {
		return nil
	}
	rest := strings.TrimPrefix(key, name+"|")
	if rest == key {
		return nil
	}
	pairs := strings.Split(rest, ",")
	out := make(map[string]string, len(pairs))
	for _, p := range pairs {
		eq := strings.IndexByte(p, '=')
		if eq < 0 {
			continue
		}
		out[p[:eq]] = p[eq+1:]
	}
	return out
}

func withLabel(in map[string]string, k, v string) map[string]string {
	out := make(map[string]string, len(in)+1)
	for kk, vv := range in {
		out[kk] = vv
	}
	out[k] = v
	return out
}

func promName(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
			return r
		}
		return '_'
	}, s)
}

func promLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf(`%s="%s"`, promName(k), promEscape(labels[k])))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func promEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	return s
}

func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'g', -1, 64)
}

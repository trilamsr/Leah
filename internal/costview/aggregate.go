// Package costview aggregates leah's per-invocation cost across the
// append-only audit.jsonl log. Read-only, no SQLite, streaming parse.
package costview

import (
	"bufio"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"sort"
	"time"
)

// Summary is the aggregated view returned by Aggregate. All monetary
// values are USD. ByModel keys "unknown" when the audit row does not
// carry a model field (current schema; reserved for future Entry
// extension).
type Summary struct {
	TotalUSD float64            `json:"total_usd"`
	Count    int                `json:"count"`
	ByKind   map[string]float64 `json:"by_kind"`
	ByDay    map[string]float64 `json:"by_day"`
	ByModel  map[string]float64 `json:"by_model"`
	Since    time.Time          `json:"since"`
}

// Bucket is one row of a Top-N projection over a Summary's grouping map.
type Bucket struct {
	Name string  `json:"name"`
	USD  float64 `json:"usd"`
}

// auditRow mirrors the on-disk audit.Entry shape but only the fields
// costview reads. Decoupled from audit.Entry so a schema extension does
// not force a costview recompile (and to admit the future "model" field
// without a circular import in tests).
type auditRow struct {
	TS    string  `json:"ts"`
	Kind  string  `json:"kind"`
	Cost  float64 `json:"cost_dollars"`
	Model string  `json:"model"`
}

// Aggregate scans audit.jsonl at path and returns a Summary of rows whose
// ts >= since. Missing file is not an error — returns a zero Summary so
// `leah cost` works on a fresh install. Malformed lines are skipped.
func Aggregate(path string, since time.Time) (Summary, error) {
	out := Summary{
		ByKind:  map[string]float64{},
		ByDay:   map[string]float64{},
		ByModel: map[string]float64{},
		Since:   since,
	}
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return out, nil
		}
		return out, err
	}
	defer func() { _ = f.Close() }()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		var r auditRow
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			continue
		}
		ts, err := time.Parse(time.RFC3339, r.TS)
		if err != nil {
			continue
		}
		if ts.Before(since) {
			continue
		}
		out.Count++
		out.TotalUSD += r.Cost
		if r.Kind != "" {
			out.ByKind[r.Kind] += r.Cost
		}
		day := ts.UTC().Format("2006-01-02")
		out.ByDay[day] += r.Cost
		model := r.Model
		if model == "" {
			model = "unknown"
		}
		out.ByModel[model] += r.Cost
	}
	return out, sc.Err()
}

// TopKinds returns the top-n {kind, usd} pairs from ByKind, sorted
// descending by USD. Ties break alphabetically for deterministic output.
func (s Summary) TopKinds(n int) []Bucket {
	return topN(s.ByKind, n)
}

// TopModels returns the top-n {model, usd} pairs from ByModel, sorted
// descending by USD.
func (s Summary) TopModels(n int) []Bucket {
	return topN(s.ByModel, n)
}

func topN(m map[string]float64, n int) []Bucket {
	out := make([]Bucket, 0, len(m))
	for k, v := range m {
		out = append(out, Bucket{Name: k, USD: v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].USD != out[j].USD {
			return out[i].USD > out[j].USD
		}
		return out[i].Name < out[j].Name
	})
	if n > 0 && len(out) > n {
		out = out[:n]
	}
	return out
}

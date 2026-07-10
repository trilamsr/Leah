// Package patterns clusters audit-log entries to surface repeated operator
// actions as skill candidates. Reads audit.jsonl directly — no new storage.
package patterns

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/trilam/leah/internal/platform/audit"
)

// HashPrefixLen is the args_hash prefix length used for clustering.
// 8 hex chars = 32 bits, birthday-collision domain ~65k events — well above
// any single-operator 30-day audit volume.
const HashPrefixLen = 8

// DefaultMinCount is the floor for emitting a cluster. Configurable per-run.
const DefaultMinCount = 5

// MaxSamples is the per-cluster sample cap surfaced to the operator.
const MaxSamples = 3

// Cluster is one repeated-action group surfaced for operator review.
type Cluster struct {
	Kind       string
	HashPrefix string
	Count      int
	First      time.Time
	Last       time.Time
	Samples    []string
}

// Detect streams auditPath, buckets entries by (Kind, args_hash[:8]) within
// [since, now], and returns clusters with Count >= DefaultMinCount sorted by
// Count desc then Kind asc. Corrupt JSON lines are skipped (not fatal — audit
// corruption must not break the retro flow).
func Detect(auditPath string, since time.Time) ([]Cluster, error) {
	return DetectWithThreshold(auditPath, since, DefaultMinCount)
}

// DetectWithThreshold is Detect with an explicit minCount, used by the CLI to
// honor LEAH_PATTERN_MIN_COUNT.
func DetectWithThreshold(auditPath string, since time.Time, minCount int) ([]Cluster, error) {
	f, err := os.Open(auditPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open audit log: %w", err)
	}
	defer func() { _ = f.Close() }()

	type bucket struct {
		kind       string
		hashPrefix string
		count      int
		first      time.Time
		last       time.Time
		samples    []string
	}
	buckets := map[string]*bucket{}

	scanner := bufio.NewScanner(f)
	// Audit detail can be long; bump scanner buffer to 1MB per line.
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var e audit.Entry
		if err := json.Unmarshal(line, &e); err != nil {
			continue
		}
		ts, err := time.Parse(time.RFC3339, e.Timestamp)
		if err != nil {
			continue
		}
		if ts.Before(since) {
			continue
		}
		prefix := e.ArgsHash
		if len(prefix) > HashPrefixLen {
			prefix = prefix[:HashPrefixLen]
		}
		key := e.Kind + "|" + prefix
		b := buckets[key]
		if b == nil {
			b = &bucket{kind: e.Kind, hashPrefix: prefix, first: ts, last: ts}
			buckets[key] = b
		}
		b.count++
		if ts.Before(b.first) {
			b.first = ts
		}
		if ts.After(b.last) {
			b.last = ts
		}
		if e.Detail != "" && len(b.samples) < MaxSamples {
			b.samples = append(b.samples, e.Detail)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan audit log: %w", err)
	}

	out := make([]Cluster, 0, len(buckets))
	for _, b := range buckets {
		if b.count < minCount {
			continue
		}
		out = append(out, Cluster{
			Kind:       b.kind,
			HashPrefix: b.hashPrefix,
			Count:      b.count,
			First:      b.first,
			Last:       b.last,
			Samples:    b.samples,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Kind < out[j].Kind
	})
	return out, nil
}

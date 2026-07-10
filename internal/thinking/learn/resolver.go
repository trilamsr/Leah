package learn

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/trilam/leah/internal/platform/audit"
)

// RetroOutcome is the resolver verdict for a pending audit row.
type RetroOutcome string

const (
	RetroSuccess RetroOutcome = "success"
	RetroFailed  RetroOutcome = "failed"
	RetroUnknown RetroOutcome = "unknown"
	RetroPending RetroOutcome = "pending"
)

// Rule resolves a single pending audit Entry. Returns the verdict and a
// short probe-result string folded into the resolver.update Detail.
type Rule interface {
	Resolve(ctx context.Context, e audit.Entry) (RetroOutcome, string)
}

// Resolver walks audit.jsonl for pending rows within Since and dispatches
// each to a Kind-specific Rule. Idempotent: skips rows that already have
// a resolver.update entry covering them.
type Resolver struct {
	AuditPath string
	Logger    *audit.Logger
	Rules     map[string]Rule
	Since     time.Duration
	Now       func() time.Time
	Out       io.Writer
	// OnOutcome, if set, receives each resolved verdict so a downstream
	// observer (operatormodel) reflects recent ship results. Must not block —
	// the daemon wires it through RetroOutcomeSink (queue-with-drop).
	OnOutcome func(RetroOutcome)

	mu sync.Mutex
}

// Run walks the audit log + resolves pending rows. Safe to call from a
// daemon tick via `go r.Run(ctx)`; the mutex makes overlapping runs no-op.
func (r *Resolver) Run(ctx context.Context) error {
	if !r.mu.TryLock() {
		return nil // another Run in flight; skip
	}
	defer r.mu.Unlock()

	if r.Now == nil {
		r.Now = time.Now
	}
	if r.Since == 0 {
		r.Since = 7 * 24 * time.Hour
	}

	entries, err := readAudit(r.AuditPath)
	if err != nil {
		return err
	}

	cutoff := r.Now().Add(-r.Since)
	resolvedKeys := map[string]bool{}
	for _, e := range entries {
		if e.Kind == "resolver.update" {
			if key, ok := parseResolvedKey(e.Detail); ok {
				resolvedKeys[key] = true
			}
		}
	}

	for _, e := range entries {
		if e.Outcome != string(RetroPending) {
			continue
		}
		ts, err := time.Parse(time.RFC3339, e.Timestamp)
		if err != nil || ts.Before(cutoff) {
			continue
		}
		key := rowKey(e)
		if resolvedKeys[key] {
			continue
		}
		rule, ok := r.Rules[e.Kind]
		if !ok {
			continue
		}
		verdict, probeDetail := rule.Resolve(ctx, e)
		if verdict == RetroPending {
			continue // rule says "wait longer"
		}
		detail := fmt.Sprintf("resolved %s -> %s", key, probeDetail)
		if err := r.Logger.Append(audit.Entry{
			Kind:        "resolver.update",
			ArgsHash:    e.ArgsHash,
			BlastRadius: 0,
			Outcome:     string(verdict),
			Detail:      detail,
		}); err != nil {
			if r.Out != nil {
				_, _ = fmt.Fprintf(r.Out, "resolver: append failed for %s: %v\n", key, err)
			}
			continue
		}
		resolvedKeys[key] = true
		if r.OnOutcome != nil {
			r.OnOutcome(verdict)
		}
	}
	return nil
}

// rowKey is the (Kind, ArgsHash, Timestamp) composite per spec §2.
// Used as both the dedup key and the human-readable token in
// resolver.update Detail.
func rowKey(e audit.Entry) string {
	return fmt.Sprintf("%s,%s,%s", e.Kind, e.ArgsHash, e.Timestamp)
}

// parseResolvedKey extracts the rowKey from a resolver.update Detail
// of the form "resolved <kind>,<hash>,<ts> -> ...".
func parseResolvedKey(detail string) (string, bool) {
	const prefix = "resolved "
	if len(detail) < len(prefix) {
		return "", false
	}
	if detail[:len(prefix)] != prefix {
		return "", false
	}
	rest := detail[len(prefix):]
	for i := 0; i < len(rest)-1; i++ {
		if rest[i] == ' ' && rest[i+1] == '-' {
			return rest[:i], true
		}
	}
	return "", false
}

func readAudit(path string) ([]audit.Entry, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("open audit: %w", err)
	}
	defer func() { _ = f.Close() }()

	var entries []audit.Entry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		var e audit.Entry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			continue
		}
		entries = append(entries, e)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan audit: %w", err)
	}
	return entries, nil
}

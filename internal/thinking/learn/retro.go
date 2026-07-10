package learn

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/trilam/leah/internal/platform/audit"
)

// Retro renders a weekly markdown report from audit + mistake_log.
// Spec §4.
type Retro struct {
	AuditPath string
	Store     *MistakeStore
	Now       func() time.Time
	Budget    float64 // weekly $ ceiling; 0 = unset

	// AttestationScanner is an optional probe for the H3 attestation-gate
	// section. When set, Generate calls it with AuditPath and renders the
	// returned violations under "## Attestation gate violations".
	// Implemented by rules.AttestationGate.Scan; an interface here keeps
	// selflearn → rules dependency one-way (rules already imports
	// selflearn for Outcome).
	AttestationScanner func(ctx context.Context, auditPath string) ([]AttestationViolation, error)
}

// AttestationViolation mirrors rules.Violation in shape; redeclared in
// the selflearn package to avoid an import cycle. Retro callers adapt
// rules.Violation → AttestationViolation at the wiring site (cmd/leah).
type AttestationViolation struct {
	Repo     string
	PRNumber int
	URL      string
}

// Generate produces a markdown report for the given ISO week (YYYY-WW).
// Empty week defaults to the current ISO week.
func (r *Retro) Generate(week string) (string, error) {
	if r.Now == nil {
		r.Now = time.Now
	}
	if week == "" {
		y, w := r.Now().UTC().ISOWeek()
		week = fmt.Sprintf("%04d-%02d", y, w)
	}

	entries, err := readAudit(r.AuditPath)
	if err != nil {
		return "", err
	}

	type bucket struct {
		success, failed, pending, unknown int
		cost                              float64
		wins                              []audit.Entry
		kindCount                         map[string]int
	}
	b := bucket{kindCount: map[string]int{}}
	for _, e := range entries {
		ts, err := time.Parse(time.RFC3339, e.Timestamp)
		if err != nil {
			continue
		}
		y, w := ts.ISOWeek()
		if fmt.Sprintf("%04d-%02d", y, w) != week {
			continue
		}
		b.kindCount[e.Kind]++
		b.cost += e.CostDollars
		switch e.Outcome {
		case "success":
			b.success++
			b.wins = append(b.wins, e)
		case "failed":
			b.failed++
		case "pending":
			b.pending++
		case "unknown":
			b.unknown++
		}
	}

	mistakes, err := r.Store.List(ListOptions{Week: week})
	if err != nil {
		return "", err
	}

	// top kind
	topKind := ""
	topCount := 0
	for k, c := range b.kindCount {
		if c > topCount {
			topKind, topCount = k, c
		}
	}

	// top wins (limit 5, newest first)
	sort.SliceStable(b.wins, func(i, j int) bool {
		return b.wins[i].Timestamp > b.wins[j].Timestamp
	})
	if len(b.wins) > 5 {
		b.wins = b.wins[:5]
	}

	// mistakes: group by root_cause, count, take top 5
	rcCount := map[string]int{}
	rcExample := map[string]string{} // root_cause -> first prevention
	for _, m := range mistakes {
		rcCount[m.RootCause]++
		if _, ok := rcExample[m.RootCause]; !ok {
			rcExample[m.RootCause] = m.Prevention
		}
	}
	type rcRow struct {
		cause      string
		count      int
		prevention string
	}
	var rcs []rcRow
	for c, n := range rcCount {
		rcs = append(rcs, rcRow{c, n, rcExample[c]})
	}
	sort.Slice(rcs, func(i, j int) bool { return rcs[i].count > rcs[j].count })
	if len(rcs) > 5 {
		rcs = rcs[:5]
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "# Leah retro — week %s\n\n", week)
	fmt.Fprintf(&sb, "## Summary\n")
	fmt.Fprintf(&sb, "- actions taken: %d\n", b.success+b.failed+b.pending+b.unknown)
	fmt.Fprintf(&sb, "- success: %d / failed: %d / pending: %d / unknown: %d\n", b.success, b.failed, b.pending, b.unknown)
	if r.Budget > 0 {
		fmt.Fprintf(&sb, "- cost: $%.2f of $%.2f ceiling\n", b.cost, r.Budget)
	} else {
		fmt.Fprintf(&sb, "- cost: $%.2f\n", b.cost)
	}
	if topKind != "" {
		fmt.Fprintf(&sb, "- top action_kind: %s (%d calls)\n", topKind, topCount)
	}
	fmt.Fprintln(&sb)

	fmt.Fprintln(&sb, "## Wins (top 5 success)")
	if len(b.wins) == 0 {
		fmt.Fprintln(&sb, "- (none)")
	}
	for _, w := range b.wins {
		fmt.Fprintf(&sb, "- %s %s %s $%.2f\n", w.Timestamp, w.Kind, w.Detail, w.CostDollars)
	}
	fmt.Fprintln(&sb)

	fmt.Fprintln(&sb, "## Mistakes (top 5 by root_cause frequency)")
	if len(rcs) == 0 {
		fmt.Fprintln(&sb, "- (none logged)")
	}
	for _, r := range rcs {
		fmt.Fprintf(&sb, "- %s ×%d   prevention: %q\n", r.cause, r.count, r.prevention)
	}
	fmt.Fprintln(&sb)

	fmt.Fprintln(&sb, "## Attestation gate violations")
	if r.AttestationScanner == nil {
		fmt.Fprintln(&sb, "- (scanner not wired)")
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		vs, err := r.AttestationScanner(ctx, r.AuditPath)
		switch {
		case err != nil:
			fmt.Fprintf(&sb, "- scanner error: %v\n", err)
		case len(vs) == 0:
			fmt.Fprintln(&sb, "- (none)")
		default:
			for _, v := range vs {
				fmt.Fprintf(&sb, "- %s — merged without `Attestation:` comment\n", v.URL)
			}
		}
	}
	fmt.Fprintln(&sb)

	fmt.Fprintln(&sb, "## Drift from stated prefs")
	fmt.Fprintln(&sb, "n/a (Phase X — prefs table)")
	return sb.String(), nil
}

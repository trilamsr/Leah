// Package operatormodel observes operator behavior from the audit log
// and the context-switch log, then ranks likely next actions.
//
// Three observation classes:
//   - time_of_day: per action-kind, which 24h hour does this happen most
//   - context_transition: per (from_context), which action-kind tends to follow
//   - cadence: per action-kind, which weekday does this happen most
//
// All observers are pure functions over inputs — no DB / file IO — so they
// can be tested + composed by Profile.Update().
package operatormodel

import (
	"fmt"
	"time"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/ctxmgr"
)

// Observation is one (class, key, slot) cell produced by an observer.
// Count is the raw event count for the cell; Profile.Update later applies
// decay weighting to yield ProfileRow.Weight.
type Observation struct {
	Class string
	Key   string
	Slot  string
	Count int
	// Times retains every event timestamp behind this cell so the decay
	// pass can compute per-event ages without re-scanning the audit log.
	Times []time.Time
}

// ObserveTimeOfDay buckets audit entries by (Kind, HH) where HH is the
// 2-digit hour ("00".."23") in the given location. Entries with
// unparseable timestamps are silently dropped (audit corruption MUST
// not break the weekly tick — same posture as patterns.Detect).
func ObserveTimeOfDay(rows []audit.Entry, tz *time.Location) []Observation {
	if tz == nil {
		tz = time.UTC
	}
	type cellKey struct{ kind, slot string }
	cells := map[cellKey]*Observation{}
	for _, r := range rows {
		ts, err := time.Parse(time.RFC3339, r.Timestamp)
		if err != nil {
			continue
		}
		slot := fmt.Sprintf("%02d", ts.In(tz).Hour())
		k := cellKey{r.Kind, slot}
		c, ok := cells[k]
		if !ok {
			c = &Observation{Class: "time_of_day", Key: r.Kind, Slot: slot}
			cells[k] = c
		}
		c.Count++
		c.Times = append(c.Times, ts)
	}
	return flatten(cells)
}

// ObserveContextTransitions buckets audit entries by (from_context,
// action_kind) where from_context is the operator's active context at
// the audit entry's timestamp — i.e. the most recent ctxmgr.Switch
// whose SwitchedAt <= the entry's timestamp.
//
// switches MUST be sorted by SwitchedAt ascending. Entries that predate
// every switch (rare — only possible if audit started before ctxmgr)
// are skipped silently.
func ObserveContextTransitions(rows []audit.Entry, switches []ctxmgr.Switch) []Observation {
	if len(switches) == 0 {
		return nil
	}
	type cellKey struct{ ctx, kind string }
	cells := map[cellKey]*Observation{}
	for _, r := range rows {
		ts, err := time.Parse(time.RFC3339, r.Timestamp)
		if err != nil {
			continue
		}
		ctx := activeAt(switches, ts)
		if ctx == "" {
			continue
		}
		k := cellKey{ctx, r.Kind}
		c, ok := cells[k]
		if !ok {
			c = &Observation{Class: "context_transition", Key: ctx, Slot: r.Kind}
			cells[k] = c
		}
		c.Count++
		c.Times = append(c.Times, ts)
	}
	return flatten(cells)
}

// ObserveCadence buckets audit entries by (Kind, weekday-abbrev) where
// weekday-abbrev is the 3-letter prefix of time.Weekday().String() —
// "Mon", "Tue", ... "Sun".
func ObserveCadence(rows []audit.Entry, tz *time.Location) []Observation {
	if tz == nil {
		tz = time.UTC
	}
	type cellKey struct{ kind, slot string }
	cells := map[cellKey]*Observation{}
	for _, r := range rows {
		ts, err := time.Parse(time.RFC3339, r.Timestamp)
		if err != nil {
			continue
		}
		slot := ts.In(tz).Weekday().String()[:3]
		k := cellKey{r.Kind, slot}
		c, ok := cells[k]
		if !ok {
			c = &Observation{Class: "cadence", Key: r.Kind, Slot: slot}
			cells[k] = c
		}
		c.Count++
		c.Times = append(c.Times, ts)
	}
	return flatten(cells)
}

// activeAt returns the operator's active context at ts by walking
// switches (sorted ascending). Linear scan is fine — switches/week is O(10s)
// for a single operator.
func activeAt(switches []ctxmgr.Switch, ts time.Time) string {
	active := ""
	for _, s := range switches {
		if s.SwitchedAt.After(ts) {
			break
		}
		active = s.To
	}
	return active
}

func flatten[T comparable](m map[T]*Observation) []Observation {
	out := make([]Observation, 0, len(m))
	for _, v := range m {
		out = append(out, *v)
	}
	return out
}

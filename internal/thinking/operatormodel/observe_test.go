package operatormodel

import (
	"testing"
	"time"

	"github.com/trilam/leah/internal/platform/audit"
	"github.com/trilam/leah/internal/platform/activectx"
)

// TestObserveTimeOfDayBuckets asserts time-of-day buckets per kind by
// 24h slot. UTC tz isolates from operator-laptop drift.
func TestObserveTimeOfDayBuckets(t *testing.T) {
	rows := []audit.Entry{
		{Timestamp: "2026-06-09T10:15:00Z", Kind: "leah.ship"},
		{Timestamp: "2026-06-09T10:42:00Z", Kind: "leah.ship"},
		{Timestamp: "2026-06-09T14:00:00Z", Kind: "leah.ask"},
		{Timestamp: "bad-ts", Kind: "leah.ship"}, // skipped
	}
	obs := ObserveTimeOfDay(rows, time.UTC)
	count := map[[3]string]int{}
	for _, o := range obs {
		count[[3]string{o.Class, o.Key, o.Slot}] = o.Count
	}
	if got := count[[3]string{"time_of_day", "leah.ship", "10"}]; got != 2 {
		t.Errorf("want 2 ship@10; got %d", got)
	}
	if got := count[[3]string{"time_of_day", "leah.ask", "14"}]; got != 1 {
		t.Errorf("want 1 ask@14; got %d", got)
	}
}

// TestObserveContextTransitionsCounts asserts (from_context, action_kind)
// pairs are counted when an action follows a switch into from_context.
func TestObserveContextTransitionsCounts(t *testing.T) {
	mustTS := func(s string) time.Time {
		ts, err := time.Parse(time.RFC3339, s)
		if err != nil {
			t.Fatalf("parse %q: %v", s, err)
		}
		return ts
	}
	switches := []activectx.Switch{
		{From: "default", To: "leah", SwitchedAt: mustTS("2026-06-09T09:00:00Z")},
		{From: "leah", To: "regatta", SwitchedAt: mustTS("2026-06-09T11:00:00Z")},
	}
	rows := []audit.Entry{
		{Timestamp: "2026-06-09T09:30:00Z", Kind: "leah.ask"},  // after entering leah
		{Timestamp: "2026-06-09T10:00:00Z", Kind: "leah.ship"}, // still in leah
		{Timestamp: "2026-06-09T11:15:00Z", Kind: "regatta.status"},
	}
	obs := ObserveContextTransitions(rows, switches)
	count := map[[2]string]int{}
	for _, o := range obs {
		if o.Class != "context_transition" {
			continue
		}
		count[[2]string{o.Key, o.Slot}] += o.Count
	}
	if got := count[[2]string{"leah", "leah.ask"}]; got != 1 {
		t.Errorf("want 1 leah->leah.ask; got %d", got)
	}
	if got := count[[2]string{"leah", "leah.ship"}]; got != 1 {
		t.Errorf("want 1 leah->leah.ship; got %d", got)
	}
	if got := count[[2]string{"regatta", "regatta.status"}]; got != 1 {
		t.Errorf("want 1 regatta->regatta.status; got %d", got)
	}
}

// TestObserveCadenceWeekday asserts cadence buckets per kind by weekday
// abbrev.
func TestObserveCadenceWeekday(t *testing.T) {
	// 2026-06-09 = Tue, 2026-06-07 = Sun.
	rows := []audit.Entry{
		{Timestamp: "2026-06-07T18:00:00Z", Kind: "leah.retro"}, // Sun
		{Timestamp: "2026-06-07T19:00:00Z", Kind: "leah.retro"}, // Sun
		{Timestamp: "2026-06-09T10:00:00Z", Kind: "leah.ship"},  // Tue
	}
	obs := ObserveCadence(rows, time.UTC)
	count := map[[2]string]int{}
	for _, o := range obs {
		if o.Class != "cadence" {
			continue
		}
		count[[2]string{o.Key, o.Slot}] += o.Count
	}
	if got := count[[2]string{"leah.retro", "Sun"}]; got != 2 {
		t.Errorf("want 2 retro@Sun; got %d", got)
	}
	if got := count[[2]string{"leah.ship", "Tue"}]; got != 1 {
		t.Errorf("want 1 ship@Tue; got %d", got)
	}
}

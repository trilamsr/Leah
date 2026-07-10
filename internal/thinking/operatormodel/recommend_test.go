package operatormodel

import (
	"testing"
	"time"
)

// TestRecommendHonorsProfileTZ asserts time-of-day + cadence slot lookup
// uses the operator's tz (profile.TZ) instead of always UTC. Profile.Update
// already records slots in the operator's local tz; matching at lookup
// time in UTC silently misses every slot for any operator outside UTC.
// Audit L9.
func TestRecommendHonorsProfileTZ(t *testing.T) {
	// Operator is in Asia/Tokyo (UTC+9). Their 09:00 local is 00:00 UTC.
	tokyo, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Skipf("tz data unavailable: %v", err)
	}
	p := Profile{
		Ready: true,
		TZ:    tokyo,
		Rows: []ProfileRow{
			// Slot is the local-hour string the observer recorded ("09").
			{Class: "time_of_day", Key: "leah.ship", Slot: "09", Count: 12, Weight: 12.0},
			// Tuesday cadence — local weekday-abbrev.
			{Class: "cadence", Key: "leah.standup", Slot: "Tue", Count: 8, Weight: 8.0},
		},
	}
	// Monday 00:00 UTC = Monday 09:00 Tokyo. The profile rows above
	// recorded "Tue" cadence + "09" hour in Tokyo-local time. Pick a UTC
	// instant that lands on Tue Tokyo: Mon 15:00 UTC = Tue 00:00 Tokyo.
	// Actually for hour=09 + weekday=Tue together we need Mon 00:00 UTC
	// → Mon 09:00 Tokyo; so use a Mon-Tokyo row instead.
	utc := time.Date(2026, 6, 8, 0, 0, 0, 0, time.UTC) // Mon UTC
	tokyoTime := utc.In(tokyo)
	if tokyoTime.Hour() != 9 {
		t.Fatalf("test setup bug: tokyo hour = %d, want 9", tokyoTime.Hour())
	}
	wantDay := tokyoTime.Weekday().String()[:3] // "Mon"
	// Update the cadence row to match the actual Tokyo weekday.
	p.Rows[1].Slot = wantDay
	got, err := Recommend(p, "default", utc)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	var sawShip, sawStandup bool
	for _, r := range got {
		if r.Kind == "leah.ship" {
			sawShip = true
		}
		if r.Kind == "leah.standup" {
			sawStandup = true
		}
	}
	if !sawShip {
		t.Errorf("time_of_day rec missed — tz lookup likely in UTC; got %+v", got)
	}
	if !sawStandup {
		t.Errorf("cadence rec missed — tz lookup likely in UTC; got %+v", got)
	}
}

// TestRecommendBelowColdStartReturnsEmpty asserts Recommend returns []
// (not error) when Profile.Ready == false (#wave2-J §3.5).
func TestRecommendBelowColdStartReturnsEmpty(t *testing.T) {
	p := Profile{
		Ready:        false,
		RowsObserved: 12,
		DaysObserved: 2,
	}
	got, err := Recommend(p, "leah", time.Date(2026, 6, 9, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("cold start must return empty; got %d recs: %v", len(got), got)
	}
}

// TestRecommendByTimeOfDay asserts the time_of_day class promotes the
// action-kind with the highest weight at the current hour. TZ=UTC keeps
// the test deterministic across hosts (Recommend honors profile.TZ per
// L9 fix).
func TestRecommendByTimeOfDay(t *testing.T) {
	p := Profile{
		Ready: true,
		TZ:    time.UTC,
		Rows: []ProfileRow{
			{Class: "time_of_day", Key: "leah.ship", Slot: "10", Count: 12, Weight: 12.0},
			{Class: "time_of_day", Key: "leah.ask", Slot: "10", Count: 3, Weight: 3.0},
			{Class: "time_of_day", Key: "regatta.status", Slot: "14", Count: 9, Weight: 9.0},
		},
	}
	got, err := Recommend(p, "default", time.Date(2026, 6, 9, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(got) == 0 {
		t.Fatalf("expected at least 1 rec; got 0")
	}
	if got[0].Kind != "leah.ship" {
		t.Errorf("want top rec leah.ship at 10h; got %q", got[0].Kind)
	}
	// The 14h row must not appear when hour=10.
	for _, r := range got {
		if r.Kind == "regatta.status" && r.Class == "time_of_day" {
			t.Errorf("regatta.status@14h leaked into hour=10 recs: %+v", r)
		}
	}
}

// TestRecommendByContextTransition asserts the context_transition class
// promotes the action-kind most often observed AFTER entering the given
// context.
func TestRecommendByContextTransition(t *testing.T) {
	p := Profile{
		Ready: true,
		Rows: []ProfileRow{
			{Class: "context_transition", Key: "leah", Slot: "leah.ask", Count: 8, Weight: 8.0},
			{Class: "context_transition", Key: "leah", Slot: "leah.ship", Count: 2, Weight: 2.0},
			{Class: "context_transition", Key: "regatta", Slot: "regatta.status", Count: 9, Weight: 9.0},
		},
	}
	got, err := Recommend(p, "leah", time.Date(2026, 6, 9, 3, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(got) == 0 {
		t.Fatalf("expected at least 1 rec; got 0")
	}
	// First context_transition rec for ctx 'leah' must be leah.ask.
	var firstCtx Recommendation
	for _, r := range got {
		if r.Class == "context_transition" {
			firstCtx = r
			break
		}
	}
	if firstCtx.Kind != "leah.ask" {
		t.Errorf("want first context_transition rec leah.ask; got %q", firstCtx.Kind)
	}
	// regatta-context row must not leak into ctx='leah' recs.
	for _, r := range got {
		if r.Class == "context_transition" && r.Kind == "regatta.status" {
			t.Errorf("regatta-context rec leaked into ctx='leah': %+v", r)
		}
	}
}

// TestRecommendCapsAtThree asserts at most 3 recommendations returned (§4.1).
func TestRecommendCapsAtThree(t *testing.T) {
	p := Profile{Ready: true, TZ: time.UTC}
	for i := 0; i < 10; i++ {
		p.Rows = append(p.Rows, ProfileRow{
			Class:  "time_of_day",
			Key:    "kind." + string(rune('a'+i)),
			Slot:   "10",
			Count:  10 - i,
			Weight: float64(10 - i),
		})
	}
	got, err := Recommend(p, "default", time.Date(2026, 6, 9, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(got) > 3 {
		t.Errorf("recommend must cap at 3; got %d", len(got))
	}
}

// TestRecommendCadenceForToday asserts the cadence class includes recs
// whose slot matches today's weekday abbrev.
func TestRecommendCadenceForToday(t *testing.T) {
	// 2026-06-09 is a Tuesday — slot label "Tue".
	day := time.Date(2026, 6, 9, 18, 0, 0, 0, time.UTC)
	if day.Weekday().String()[:3] != "Tue" {
		t.Fatalf("test calendar drift: 2026-06-09 weekday=%s", day.Weekday())
	}
	p := Profile{
		Ready: true,
		TZ:    time.UTC,
		Rows: []ProfileRow{
			{Class: "cadence", Key: "leah.retro", Slot: "Sun", Count: 6, Weight: 6.0},
			{Class: "cadence", Key: "leah.standup", Slot: "Tue", Count: 4, Weight: 4.0},
		},
	}
	got, err := Recommend(p, "default", day)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	var sawStandup, sawRetro bool
	for _, r := range got {
		if r.Class == "cadence" && r.Kind == "leah.standup" {
			sawStandup = true
		}
		if r.Class == "cadence" && r.Kind == "leah.retro" {
			sawRetro = true
		}
	}
	if !sawStandup {
		t.Errorf("Tue cadence rec missing for leah.standup")
	}
	if sawRetro {
		t.Errorf("Sun-only cadence leaked into Tue: leah.retro")
	}
}

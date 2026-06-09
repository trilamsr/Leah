package main

import (
	"testing"
	"time"
)

// TestParseCostDurationAcceptsDays asserts the Nd suffix is converted to
// hours (stdlib time.ParseDuration does not accept "d").
func TestParseCostDurationAcceptsDays(t *testing.T) {
	cases := map[string]time.Duration{
		"7d":  7 * 24 * time.Hour,
		"30d": 30 * 24 * time.Hour,
		"1d":  24 * time.Hour,
		"24h": 24 * time.Hour,
		"15m": 15 * time.Minute,
	}
	for in, want := range cases {
		got, err := parseCostDuration(in)
		if err != nil {
			t.Errorf("%q: unexpected err %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("%q: got %v, want %v", in, got, want)
		}
	}
}

// TestParseCostDurationRejectsGarbage asserts garbage input fails closed.
func TestParseCostDurationRejectsGarbage(t *testing.T) {
	bad := []string{"", "xx", "-1d", "abc"}
	for _, in := range bad {
		if _, err := parseCostDuration(in); err == nil {
			t.Errorf("%q: expected err, got nil", in)
		}
	}
}

// TestHumanDurationCollapsesDays asserts 720h renders as "30d".
func TestHumanDurationCollapsesDays(t *testing.T) {
	cases := map[time.Duration]string{
		720 * time.Hour: "30d",
		24 * time.Hour:  "1d",
		12 * time.Hour:  "12h",
		36 * time.Hour:  "36h",
	}
	for in, want := range cases {
		if got := humanDuration(in); got != want {
			t.Errorf("%v: got %q, want %q", in, got, want)
		}
	}
}

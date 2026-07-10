package operatormodel

import (
	"sync"
	"time"
)

const (
	VerdictOK       = "ok"
	VerdictFailed   = "failed"
	VerdictDangling = "dangling"
)

type ShipOutcome struct {
	Verdict string
	At      time.Time
}

// FeedbackObserver accumulates ShipOutcomes between profile rebuilds.
type FeedbackObserver struct {
	mu  sync.Mutex
	obs []Observation
}

func (f *FeedbackObserver) Observe(o ShipOutcome) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.obs = append(f.obs, Observation{
		Class: "ship_outcome",
		Key:   "ship",
		Slot:  o.Verdict,
		Count: 1,
		Times: []time.Time{o.At},
	})
}

func (f *FeedbackObserver) Drain() []Observation {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := f.obs
	f.obs = nil
	return out
}

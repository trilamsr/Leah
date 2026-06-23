package supervisor

import "time"

// CircuitPolicy bounds restart attempts: more than MaxCrashes within Window opens the breaker.
type CircuitPolicy struct {
	MaxCrashes int
	Window     time.Duration
}

type circuit struct {
	policy CircuitPolicy
	hits   []time.Time
}

func newCircuit(p CircuitPolicy) *circuit { return &circuit{policy: p} }

func (c *circuit) recordCrash(at time.Time) {
	if c.policy.MaxCrashes <= 0 || c.policy.Window <= 0 {
		return
	}
	c.hits = append(c.hits, at)
	c.prune(at)
}

func (c *circuit) isOpen(now time.Time) bool {
	if c.policy.MaxCrashes <= 0 || c.policy.Window <= 0 {
		return false
	}
	c.prune(now)
	return len(c.hits) >= c.policy.MaxCrashes
}

func (c *circuit) reset() { c.hits = c.hits[:0] }

func (c *circuit) prune(now time.Time) {
	cutoff := now.Add(-c.policy.Window)
	i := 0
	for ; i < len(c.hits); i++ {
		if !c.hits[i].Before(cutoff) {
			break
		}
	}
	if i > 0 {
		c.hits = append(c.hits[:0], c.hits[i:]...)
	}
}

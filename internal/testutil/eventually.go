// Package testutil provides shared test helpers — predicate-based settle
// waiters that replace bare time.Sleep polling in test code.
package testutil

import (
	"testing"
	"time"
)

// Eventually polls cond until it returns true or deadline expires.
// Use instead of bare time.Sleep in tight retry loops — fails closed
// with iteration count + elapsed wall-clock for diagnostic, so a
// failing test record names how much budget the predicate burned
// and how many evaluations it survived (distinguishes "predicate
// never fired" from "predicate fired once but read wrong state").
func Eventually(t testing.TB, deadline, interval time.Duration, cond func() bool) {
	t.Helper()
	start := time.Now()
	end := start.Add(deadline)
	iter := 0
	for time.Now().Before(end) {
		iter++
		if cond() {
			return
		}
		time.Sleep(interval) // allow-sleep: poll interval inside Eventually helper, not assertion wait
	}
	elapsed := time.Since(start)
	t.Fatalf("condition not met after %d iteration(s) / %s elapsed (deadline %s, interval %s)",
		iter, elapsed.Round(time.Millisecond), deadline, interval)
}

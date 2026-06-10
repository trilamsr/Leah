// Package testutil provides shared test helpers — predicate-based settle
// waiters that replace bare time.Sleep polling in test code.
package testutil

import (
	"testing"
	"time"
)

// Eventually polls cond until it returns true or deadline expires.
// Use instead of bare time.Sleep in tight retry loops — fails closed
// with the test's deadline for diagnostic.
func Eventually(t testing.TB, deadline, interval time.Duration, cond func() bool) {
	t.Helper()
	end := time.Now().Add(deadline)
	for time.Now().Before(end) {
		if cond() {
			return
		}
		time.Sleep(interval) // allow-sleep: poll interval inside Eventually helper, not assertion wait
	}
	t.Fatalf("condition not met within %s", deadline)
}

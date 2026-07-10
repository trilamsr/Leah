package daemonloop

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/trilam/leah/internal/platform/audit"
	"github.com/trilam/leah/internal/actions/regattaclient"
)

// BUG-7 regression: pre-fix, "weekly skipped" and "daily deferred" printed on
// every poll (~30s cadence). Repeated tick calls with the same skip reason
// must emit the log line at most once until the reason changes.
func TestLoop_SkipLogThrottle_EmitsOncePerReason(t *testing.T) {
	rc := &fakeRegatta{resps: [][]regattaclient.Agent{{}, {}, {}}}
	a := &audit.Logger{Path: t.TempDir() + "/audit.jsonl"}
	dir := t.TempDir()
	tracker := dir + "/last-weekly.txt"
	recent := time.Now().Add(-24 * time.Hour).UTC().Format(time.RFC3339)
	if err := os.WriteFile(tracker, []byte(recent), 0o600); err != nil {
		t.Fatal(err)
	}
	buf := &bytes.Buffer{}
	l := New(rc, &fakeHb{}, &fakeNf{}, a, buf, 1*time.Millisecond)
	l.WeeklyTracker = tracker
	l.Weekly = []WeeklyTask{func(context.Context) {}}

	for i := 0; i < 5; i++ {
		l.tick(context.Background())
	}
	got := strings.Count(buf.String(), "weekly skipped")
	if got != 1 {
		t.Fatalf("expected 1 'weekly skipped' log across 5 ticks, got %d\nlog:\n%s", got, buf.String())
	}
}

func TestLoop_SkipLogThrottle_DailyDeferOnlyOnce(t *testing.T) {
	rc := &fakeRegatta{resps: [][]regattaclient.Agent{{}, {}, {}}}
	a := &audit.Logger{Path: t.TempDir() + "/audit.jsonl"}
	dir := t.TempDir()
	tracker := dir + "/last-daily.txt"
	buf := &bytes.Buffer{}
	l := New(rc, &fakeHb{}, &fakeNf{}, a, buf, 1*time.Millisecond)
	l.DailyTracker = tracker
	l.DailyHour = 23 // 11pm gate — deferred at almost any test-run time
	l.Daily = []WeeklyTask{func(context.Context) {}}

	for i := 0; i < 5; i++ {
		l.tick(context.Background())
	}
	got := strings.Count(buf.String(), "daily deferred")
	if got > 1 {
		t.Fatalf("expected daily-defer log at most once, got %d\nlog:\n%s", got, buf.String())
	}
}

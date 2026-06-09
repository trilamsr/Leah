package daemonloop

import (
	"bytes"
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/regattaclient"
)

type fakeRegatta struct {
	resps [][]regattaclient.Agent
	call  int
}

func (f *fakeRegatta) List(ctx context.Context) ([]regattaclient.Agent, error) {
	if f.call >= len(f.resps) {
		return f.resps[len(f.resps)-1], nil
	}
	r := f.resps[f.call]
	f.call++
	return r, nil
}

type fakeHb struct{ pings int }

func (f *fakeHb) Ping(ctx context.Context) error { f.pings++; return nil }

type fakeNf struct{ calls []string }

func (f *fakeNf) Notify(ctx context.Context, title, body string) error {
	f.calls = append(f.calls, title+":"+body)
	return nil
}

func TestLoopColdStartDoesNotNotify(t *testing.T) {
	rc := &fakeRegatta{resps: [][]regattaclient.Agent{
		{{ID: "a1", State: "merged", PR: 1}},
	}}
	hb := &fakeHb{}
	nf := &fakeNf{}
	a := &audit.Logger{Path: t.TempDir() + "/audit.jsonl"}

	l := New(rc, hb, nf, a, &bytes.Buffer{}, 1*time.Millisecond)
	l.tick(context.Background())
	if len(nf.calls) != 0 {
		t.Errorf("cold start should not notify; got %v", nf.calls)
	}
	if hb.pings != 1 {
		t.Errorf("heartbeat should fire on cold tick; got %d", hb.pings)
	}
}

func TestLoopNotifiesOnTerminalTransition(t *testing.T) {
	rc := &fakeRegatta{resps: [][]regattaclient.Agent{
		{{ID: "a1", State: "running", PR: 0}},
		{{ID: "a1", State: "merged", PR: 1234}},
	}}
	dir := t.TempDir()
	a := &audit.Logger{Path: dir + "/audit.jsonl"}
	nf := &fakeNf{}

	l := New(rc, &fakeHb{}, nf, a, &bytes.Buffer{}, 1*time.Millisecond)
	l.tick(context.Background())
	l.tick(context.Background())

	if len(nf.calls) != 1 {
		t.Fatalf("want 1 notify call, got %d: %v", len(nf.calls), nf.calls)
	}
	if !strings.Contains(nf.calls[0], "merged") || !strings.Contains(nf.calls[0], "PR #1234") {
		t.Errorf("notify body: %v", nf.calls[0])
	}

	data, _ := os.ReadFile(dir + "/audit.jsonl")
	if !strings.Contains(string(data), `"kind":"daemon.transition"`) {
		t.Errorf("audit missing daemon.transition: %s", data)
	}
}

func TestLoopIgnoresNonTerminalTransition(t *testing.T) {
	rc := &fakeRegatta{resps: [][]regattaclient.Agent{
		{{ID: "a1", State: "running", PR: 0}},
		{{ID: "a1", State: "spawning", PR: 0}},
	}}
	nf := &fakeNf{}
	a := &audit.Logger{Path: t.TempDir() + "/audit.jsonl"}

	l := New(rc, &fakeHb{}, nf, a, &bytes.Buffer{}, 1*time.Millisecond)
	l.tick(context.Background())
	l.tick(context.Background())

	if len(nf.calls) != 0 {
		t.Errorf("non-terminal change should not notify; got %v", nf.calls)
	}
}

func TestWeeklyTaskFiresAfter7Days(t *testing.T) {
	rc := &fakeRegatta{resps: [][]regattaclient.Agent{{}}}
	a := &audit.Logger{Path: t.TempDir() + "/audit.jsonl"}
	dir := t.TempDir()
	tracker := dir + "/last-weekly.txt"

	// Seed tracker to 8d ago so weekly should fire.
	old := time.Now().Add(-8 * 24 * time.Hour).UTC().Format(time.RFC3339)
	if err := os.WriteFile(tracker, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}

	var fired int
	var mu sync.Mutex
	done := make(chan struct{}, 1)
	weekly := func(ctx context.Context) {
		mu.Lock()
		fired++
		mu.Unlock()
		select {
		case done <- struct{}{}:
		default:
		}
	}

	l := New(rc, &fakeHb{}, &fakeNf{}, a, &bytes.Buffer{}, 1*time.Millisecond)
	l.WeeklyTracker = tracker
	l.Weekly = []WeeklyTask{weekly}
	l.tick(context.Background())

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("weekly task did not fire after 7d boundary")
	}
	mu.Lock()
	if fired != 1 {
		t.Errorf("want 1 weekly fire, got %d", fired)
	}
	mu.Unlock()

	// Tracker file should be updated to ~now.
	data, err := os.ReadFile(tracker)
	if err != nil {
		t.Fatalf("read tracker: %v", err)
	}
	ts, err := time.Parse(time.RFC3339, strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatalf("parse tracker: %v", err)
	}
	if time.Since(ts) > time.Minute {
		t.Errorf("tracker stale: %v", ts)
	}
}

func TestWeeklyTaskDoesNotFireWithin7Days(t *testing.T) {
	rc := &fakeRegatta{resps: [][]regattaclient.Agent{{}}}
	a := &audit.Logger{Path: t.TempDir() + "/audit.jsonl"}
	dir := t.TempDir()
	tracker := dir + "/last-weekly.txt"

	// Seed tracker to 1d ago — weekly should NOT fire.
	recent := time.Now().Add(-24 * time.Hour).UTC().Format(time.RFC3339)
	if err := os.WriteFile(tracker, []byte(recent), 0o600); err != nil {
		t.Fatal(err)
	}

	var fired int
	var mu sync.Mutex
	weekly := func(ctx context.Context) {
		mu.Lock()
		fired++
		mu.Unlock()
	}

	l := New(rc, &fakeHb{}, &fakeNf{}, a, &bytes.Buffer{}, 1*time.Millisecond)
	l.WeeklyTracker = tracker
	l.Weekly = []WeeklyTask{weekly}
	l.tick(context.Background())

	// Allow any goroutine that would have fired to run.
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	if fired != 0 {
		t.Errorf("weekly should not fire within 7d; got %d", fired)
	}
	mu.Unlock()

	// Tracker should be unchanged.
	data, _ := os.ReadFile(tracker)
	if strings.TrimSpace(string(data)) != recent {
		t.Errorf("tracker changed unexpectedly: %s", data)
	}
}

func TestWeeklyTaskFiresOnFirstRunWhenTrackerMissing(t *testing.T) {
	rc := &fakeRegatta{resps: [][]regattaclient.Agent{{}}}
	a := &audit.Logger{Path: t.TempDir() + "/audit.jsonl"}
	dir := t.TempDir()
	tracker := dir + "/last-weekly.txt" // does not exist

	done := make(chan struct{}, 1)
	weekly := func(ctx context.Context) {
		select {
		case done <- struct{}{}:
		default:
		}
	}

	l := New(rc, &fakeHb{}, &fakeNf{}, a, &bytes.Buffer{}, 1*time.Millisecond)
	l.WeeklyTracker = tracker
	l.Weekly = []WeeklyTask{weekly}
	l.tick(context.Background())

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("weekly task did not fire on first run (missing tracker)")
	}
	if _, err := os.Stat(tracker); err != nil {
		t.Errorf("tracker should be created: %v", err)
	}
}

func TestLoopRespectsContextCancellation(t *testing.T) {
	rc := &fakeRegatta{resps: [][]regattaclient.Agent{{}}}
	l := New(rc, &fakeHb{}, &fakeNf{}, &audit.Logger{Path: t.TempDir() + "/audit.jsonl"},
		&bytes.Buffer{}, 100*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- l.Run(ctx) }()
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run returned error: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Run did not exit after cancel")
	}
}

package daemonloop

import (
	"bytes"
	"context"
	"os"
	"strings"
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

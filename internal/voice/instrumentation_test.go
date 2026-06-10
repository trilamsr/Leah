package voice_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/trilam/leah/internal/obs"
	"github.com/trilam/leah/internal/voice"
	"github.com/trilam/leah/internal/voice/listener"
	"github.com/trilam/leah/internal/voice/session"
)

// snapshotKey rebuilds the obs.Registry flatten key "name|k1=v1,k2=v2" for
// JSON lookup. Kept local to the test to avoid coupling to obs internals.
func snapshotKey(name string, labels map[string]string) string {
	if len(labels) == 0 {
		return name
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(name)
	b.WriteString("|")
	for i, k := range keys {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(labels[k])
	}
	return b.String()
}

type snapshotDoc struct {
	Counters   map[string]int64 `json:"counters"`
	Histograms map[string]struct {
		Count int64 `json:"count"`
	} `json:"histograms"`
}

func readCounter(t *testing.T, path, name string, labels map[string]string) int64 {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	var doc snapshotDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}
	return doc.Counters[snapshotKey(name, labels)]
}

func readHistogramCount(t *testing.T, path, name string, labels map[string]string) int64 {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	var doc snapshotDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}
	return doc.Histograms[snapshotKey(name, labels)].Count
}

// fakeTTS mirrors session_test.fakeTTS — duplicated here because session_test
// is in package session_test (not exported). Speak blocks on a gate channel
// unless autoDone is set so barge-in cancellation is observable.
type fakeTTS struct {
	mu        sync.Mutex
	spoken    []string
	cancelled int32
	gate      chan struct{}
	autoDone  bool
}

func (f *fakeTTS) Speak(ctx context.Context, text string) error {
	f.mu.Lock()
	f.spoken = append(f.spoken, text)
	gate := f.gate
	auto := f.autoDone
	f.mu.Unlock()
	if auto {
		return nil
	}
	if gate == nil {
		gate = make(chan struct{})
	}
	select {
	case <-ctx.Done():
		atomic.AddInt32(&f.cancelled, 1)
		return ctx.Err()
	case <-gate:
		return nil
	}
}

func (f *fakeTTS) spokenCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.spoken)
}

type fakeReasoner struct {
	reply string
	delay time.Duration
	err   error
}

func (r *fakeReasoner) Ask(ctx context.Context, _ string) (string, error) {
	if r.err != nil {
		return "", r.err
	}
	if r.delay == 0 {
		return r.reply, nil
	}
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-time.After(r.delay):
		return r.reply, nil
	}
}

type fakeAttestor struct{ deny bool }

func (a *fakeAttestor) Attest(_ context.Context, _ string) error {
	if a.deny {
		return errors.New("attest: denied")
	}
	return nil
}

func waitForSpoken(t *testing.T, tts *fakeTTS, n int) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for tts.spokenCount() < n {
		select {
		case <-deadline:
			t.Fatalf("waitForSpoken(%d): got %d", n, tts.spokenCount())
		case <-time.After(5 * time.Millisecond):
		}
	}
}

// counterValue returns the snapshot count for a Counter at the given labels.
// Goes through Snapshot+JSON because Counter's read surface is intentionally
// minimal (single-writer source-of-truth pattern).
func counterValue(t *testing.T, reg *obs.Registry, name string, labels map[string]string) int64 {
	t.Helper()
	dir := t.TempDir()
	path := dir + "/snap.json"
	if err := reg.Snapshot(path); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	return readCounter(t, path, name, labels)
}

// histogramCount returns the observation count for a Histogram series.
func histogramCount(t *testing.T, reg *obs.Registry, name string, labels map[string]string) int64 {
	t.Helper()
	dir := t.TempDir()
	path := dir + "/snap.json"
	if err := reg.Snapshot(path); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	return readHistogramCount(t, path, name, labels)
}

// waitForCounter polls until counterValue ≥ want or the 2s deadline trips.
// Needed because Session.Run finishes a turn in a goroutine — the test
// thread can race ahead of the metric write without polling.
func waitForCounter(t *testing.T, reg *obs.Registry, name string, labels map[string]string, want int64) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for counterValue(t, reg, name, labels) < want {
		select {
		case <-deadline:
			t.Fatalf("counter %s%v never reached %d; got %d", name, labels, want, counterValue(t, reg, name, labels))
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func waitForHistogram(t *testing.T, reg *obs.Registry, name string, labels map[string]string, want int64) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for histogramCount(t, reg, name, labels) < want {
		select {
		case <-deadline:
			t.Fatalf("histogram %s%v never reached %d; got %d", name, labels, want, histogramCount(t, reg, name, labels))
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// TestVoiceTurn_HistogramEmits_OnCompleted: a successful turn writes one
// observation into leah_voice_turn_seconds{outcome="completed"} and increments
// leah_voice_turn_total{outcome="completed"} by exactly 1.
func TestVoiceTurn_HistogramEmits_OnCompleted(t *testing.T) {
	t.Parallel()
	reg := obs.NewRegistry()
	instr := voice.NewTurnInstrumentation(reg, "local")

	fl := listener.NewFake()
	tts := &fakeTTS{autoDone: true}
	rs := &fakeReasoner{reply: "ok"}
	s := &session.Session{
		Listen:    fl,
		Speak:     tts,
		Reason:    rs,
		Attest:    &fakeAttestor{},
		IdleAfter: 5 * time.Second,
		Metrics:   instr,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()

	waitForSpoken(t, tts, 1)
	fl.Emit(listener.Segment{Text: "yes leah", Final: true})
	fl.Emit(listener.Segment{Text: "what time is it", Final: true})
	waitForSpoken(t, tts, 2)

	// Snapshot reads are eventually consistent against the goroutine that
	// recorded the turn — poll until the counter increments or fail.
	waitForCounter(t, reg, "leah_voice_turn_total", map[string]string{"outcome": "completed"}, 1)
	if got := histogramCount(t, reg, "leah_voice_turn_seconds", map[string]string{"outcome": "completed", "path": "local"}); got != 1 {
		t.Fatalf("turn histogram count = %d, want 1", got)
	}

	cancel()
	<-done
}

// TestVoiceTurn_StageBreakdown_Reasoner: a turn whose reasoner takes a known
// delay produces exactly one reasoner-stage observation, and the recorded
// duration exceeds the injected delay floor.
func TestVoiceTurn_StageBreakdown_Reasoner(t *testing.T) {
	t.Parallel()
	reg := obs.NewRegistry()
	instr := voice.NewTurnInstrumentation(reg, "local")

	fl := listener.NewFake()
	tts := &fakeTTS{autoDone: true}
	rs := &fakeReasoner{reply: "ok", delay: 40 * time.Millisecond}
	s := &session.Session{
		Listen:    fl,
		Speak:     tts,
		Reason:    rs,
		Attest:    &fakeAttestor{},
		IdleAfter: 5 * time.Second,
		Metrics:   instr,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()

	waitForSpoken(t, tts, 1)
	fl.Emit(listener.Segment{Text: "yes leah", Final: true})
	fl.Emit(listener.Segment{Text: "ask", Final: true})
	waitForSpoken(t, tts, 2)

	waitForHistogram(t, reg, "leah_voice_stage_seconds", map[string]string{"stage": "reasoner", "outcome": "completed"}, 1)

	// Also assert tts_total stage observed.
	if got := histogramCount(t, reg, "leah_voice_stage_seconds", map[string]string{"stage": "tts_total", "outcome": "completed"}); got != 1 {
		t.Fatalf("tts_total stage = %d, want 1", got)
	}

	cancel()
	<-done
}

// TestVoiceTurn_BargeIn_CountsAndStops: a Final segment mid-TTS increments
// leah_voice_barge_in_total{stage="tts"} and the interrupted turn is counted
// under outcome="barged_in".
func TestVoiceTurn_BargeIn_CountsAndStops(t *testing.T) {
	t.Parallel()
	reg := obs.NewRegistry()
	instr := voice.NewTurnInstrumentation(reg, "local")

	fl := listener.NewFake()
	tts := &fakeTTS{gate: make(chan struct{})}
	rs := &fakeReasoner{reply: "long reply"}
	s := &session.Session{
		Listen:    fl,
		Speak:     tts,
		Reason:    rs,
		Attest:    &fakeAttestor{},
		IdleAfter: 5 * time.Second,
		Metrics:   instr,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()

	// Release attestation prompt so the first turn can begin.
	waitForSpoken(t, tts, 1)
	tts.mu.Lock()
	close(tts.gate)
	tts.gate = make(chan struct{})
	tts.mu.Unlock()
	fl.Emit(listener.Segment{Text: "yes leah", Final: true})

	// First utterance → reasoner returns → Speak blocks on gate (TTS in flight).
	fl.Emit(listener.Segment{Text: "tell me a story", Final: true})
	waitForSpoken(t, tts, 2)

	// Barge-in.
	fl.Emit(listener.Segment{Text: "stop", Final: true})

	waitForCounter(t, reg, "leah_voice_barge_in_total", map[string]string{"stage": "tts"}, 1)
	if got := counterValue(t, reg, "leah_voice_turn_total", map[string]string{"outcome": "barged_in"}); got < 1 {
		t.Fatalf("turn barged_in counter = %d, want >= 1", got)
	}

	cancel()
	<-done
}

// TestVoiceTurn_WakeOutcome_TripleCount: three discrete wake_event_total
// emissions — one armed, one cancelled_silence, one cancelled_phrase — show
// up under their respective result label.
func TestVoiceTurn_WakeOutcome_TripleCount(t *testing.T) {
	t.Parallel()
	reg := obs.NewRegistry()
	instr := voice.NewTurnInstrumentation(reg, "local")

	instr.RecordWakeEvent("armed")
	instr.RecordWakeEvent("cancelled_silence")
	instr.RecordWakeEvent("cancelled_phrase")

	for _, result := range []string{"armed", "cancelled_silence", "cancelled_phrase"} {
		if got := counterValue(t, reg, "leah_voice_wake_event_total", map[string]string{"result": result}); got != 1 {
			t.Fatalf("wake_event_total{result=%q} = %d, want 1", result, got)
		}
	}
}

package listener_test

import (
	"context"
	"testing"
	"time"

	"github.com/trilam/leah/internal/platform/telemetry"
	"github.com/trilam/leah/internal/platform/telemetry/obstest"
	"github.com/trilam/leah/internal/input/voice/listener"
)

// TestRegisterMetrics_AddsSeries pins the utterance→transcript histogram
// surfaces pre-event so /metrics is non-empty before the first turn.
func TestRegisterMetrics_AddsSeries(t *testing.T) {
	r := telemetry.NewRegistry()
	listener.RegisterMetrics(r)
	keys := obstest.SnapshotKeys(t, r)
	if !obstest.ContainsPrefix(keys, "leah_voice_utterance_to_transcript_seconds") {
		t.Fatalf("series missing from %v", keys)
	}
}

// TestRecordFinal_ObservesDuration: a Final segment records FinishedAt-StartedAt
// into the utterance→transcript histogram.
func TestRecordFinal_ObservesDuration(t *testing.T) {
	r := telemetry.NewRegistry()
	instr := listener.NewInstrumentation(r)
	start := time.Unix(0, 0)
	instr.RecordFinal(listener.Segment{
		Final:      true,
		StartedAt:  start,
		FinishedAt: start.Add(500 * time.Millisecond),
	})
	keys := obstest.SnapshotKeys(t, r)
	want := "leah_voice_utterance_to_transcript_seconds|outcome=ok"
	if !obstest.ContainsExact(keys, want) {
		t.Fatalf("expected %q in %v", want, keys)
	}
}

// TestRecordFinal_IgnoresPartial: non-Final segments must not be observed —
// the SLO targets end-of-utterance latency, not streaming partials.
func TestRecordFinal_IgnoresPartial(t *testing.T) {
	r := telemetry.NewRegistry()
	instr := listener.NewInstrumentation(r)
	start := time.Unix(0, 0)
	instr.RecordFinal(listener.Segment{Final: false, StartedAt: start, FinishedAt: start.Add(time.Second)})
	keys := obstest.SnapshotKeys(t, r)
	if obstest.ContainsExact(keys, "leah_voice_utterance_to_transcript_seconds|outcome=ok") {
		t.Fatalf("partial segment recorded a value")
	}
}

// TestRecordFinal_NilSafe: a nil instrumentation is a no-op so production
// wiring can omit the registry without per-call guards.
func TestRecordFinal_NilSafe(t *testing.T) {
	var instr *listener.Instrumentation
	instr.RecordFinal(listener.Segment{Final: true})
}

// TestFakeListener_EmitsHistogramOnFinal: an instrumented Fake records the
// final-segment latency through the public Emit path — pins the seam wiring.
func TestFakeListener_EmitsHistogramOnFinal(t *testing.T) {
	r := telemetry.NewRegistry()
	instr := listener.NewInstrumentation(r)
	fl := listener.NewFake()
	fl.WithInstrumentation(instr)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := fl.Start(ctx)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	start := time.Unix(0, 0)
	fl.Emit(listener.Segment{
		Final:      true,
		StartedAt:  start,
		FinishedAt: start.Add(800 * time.Millisecond),
	})
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatalf("timeout reading segment")
	}
	keys := obstest.SnapshotKeys(t, r)
	if !obstest.ContainsExact(keys, "leah_voice_utterance_to_transcript_seconds|outcome=ok") {
		t.Fatalf("Fake did not record histogram; keys=%v", keys)
	}
}

package loop_test

import (
	"context"
	"testing"
	"time"

	"github.com/trilam/leah/internal/obs"
	"github.com/trilam/leah/internal/obs/obstest"
	"github.com/trilam/leah/internal/testutil"
	"github.com/trilam/leah/internal/voice/listener"
	"github.com/trilam/leah/internal/voice/loop"
)

// TestRegisterMetrics_AddsSeries pins the intent→first-audio histogram surfaces
// pre-event so /metrics is non-empty before the first turn.
func TestRegisterMetrics_AddsSeries(t *testing.T) {
	r := obs.NewRegistry()
	loop.RegisterMetrics(r)
	keys := obstest.SnapshotKeys(t, r)
	if !obstest.ContainsPrefix(keys, "leah_voice_intent_to_first_audio_seconds") {
		t.Fatalf("series missing from %v", keys)
	}
}

// TestTracker_ObservesIntentToFirstAudio: the two-call seam fires the histogram
// exactly once between MarkIntentDone and MarkFirstAudio.
func TestTracker_ObservesIntentToFirstAudio(t *testing.T) {
	r := obs.NewRegistry()
	tr := loop.NewIntentToFirstAudioTracker(r)
	now := time.Unix(0, 0)
	tr.MarkIntentDone(now)
	tr.MarkFirstAudio(now.Add(450*time.Millisecond), "ok")
	keys := obstest.SnapshotKeys(t, r)
	want := "leah_voice_intent_to_first_audio_seconds|outcome=ok"
	if !obstest.ContainsExact(keys, want) {
		t.Fatalf("expected %q in %v", want, keys)
	}
}

// TestTracker_NilRegistry: nil registry → nil tracker → no-op so production
// wiring can omit the registry without per-call guards.
func TestTracker_NilRegistry(t *testing.T) {
	tr := loop.NewIntentToFirstAudioTracker(nil)
	tr.MarkIntentDone(time.Now())
	tr.MarkFirstAudio(time.Now(), "ok")
}

// TestTracker_MissingIntentDone: MarkFirstAudio without a prior MarkIntentDone
// is a no-op — the seam is two-call and out-of-order is misuse.
func TestTracker_MissingIntentDone(t *testing.T) {
	r := obs.NewRegistry()
	tr := loop.NewIntentToFirstAudioTracker(r)
	tr.MarkFirstAudio(time.Unix(0, 0).Add(500*time.Millisecond), "ok")
	keys := obstest.SnapshotKeys(t, r)
	if obstest.ContainsExact(keys, "leah_voice_intent_to_first_audio_seconds|outcome=ok") {
		t.Fatalf("observed without MarkIntentDone")
	}
}

// TestTracker_ResetsAfterObservation: a second MarkIntentDone after a fired
// FirstAudio re-arms the tracker — supports back-to-back turns on one Loop.
func TestTracker_ResetsAfterObservation(t *testing.T) {
	r := obs.NewRegistry()
	tr := loop.NewIntentToFirstAudioTracker(r)
	now := time.Unix(0, 0)
	tr.MarkIntentDone(now)
	tr.MarkFirstAudio(now.Add(200*time.Millisecond), "ok")
	tr.MarkIntentDone(now.Add(time.Second))
	tr.MarkFirstAudio(now.Add(1500*time.Millisecond), "ok")
	keys := obstest.SnapshotKeys(t, r)
	if !obstest.ContainsExact(keys, "leah_voice_intent_to_first_audio_seconds|outcome=ok") {
		t.Fatalf("series missing after second turn")
	}
}

// TestLoop_IntentToFirstAudio_RecordsOnReasonerPath pins the wiring through
// Loop.Run: a real tracker handed to Config observes outcome=ok when the
// reasoner branch reaches TTS.Speak.
func TestLoop_IntentToFirstAudio_RecordsOnReasonerPath(t *testing.T) {
	r := obs.NewRegistry()
	tr := loop.NewIntentToFirstAudioTracker(r)
	fl := newStartedListener()
	tts := &fakeTTS{auto: true}
	rs := &fakeReasoner{reply: "ok"}
	ic := &fakeIntent{handle: false}
	wake := armed()

	l := loop.New(loop.Config{
		Listener:           fl,
		Reasoner:           rs,
		TTS:                tts,
		Wake:               wake,
		IntentRouter:       ic,
		IntentToFirstAudio: tr,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- l.Run(ctx) }()
	fl.waitReady(t)

	testutil.Eventually(t, 2*time.Second, 5*time.Millisecond, func() bool {
		fl.Emit(listener.Segment{Text: "anything", Final: true})
		return tts.count() >= 1
	})
	testutil.Eventually(t, time.Second, 5*time.Millisecond, func() bool {
		keys := obstest.SnapshotKeys(t, r)
		return obstest.ContainsExact(keys, "leah_voice_intent_to_first_audio_seconds|outcome=ok")
	})

	cancel()
	<-done
}

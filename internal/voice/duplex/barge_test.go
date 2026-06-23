package duplex

import (
	"testing"
	"time"
)

func TestBargeArbiter_HaltsTTSWithin80ms(t *testing.T) {
	arb := newBargeArbiter()
	halted := make(chan time.Duration, 1)
	arb.ttsStarted()
	start := time.Now()
	arb.onHalt = func() { halted <- time.Since(start) }
	arb.micVoiceDetected()
	select {
	case d := <-halted:
		if d > 80*time.Millisecond {
			t.Fatalf("barge-in took %v, budget is 80ms", d)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("barge-in never fired")
	}
}

func TestBargeArbiter_IgnoresVoiceBeforeTTS(t *testing.T) {
	arb := newBargeArbiter()
	fired := make(chan struct{}, 1)
	arb.onHalt = func() { fired <- struct{}{} }
	arb.micVoiceDetected()
	select {
	case <-fired:
		t.Fatal("barge fired without active TTS")
	case <-time.After(20 * time.Millisecond):
	}
}

func TestBargeArbiter_OnlyFiresOncePerTTSWindow(t *testing.T) {
	arb := newBargeArbiter()
	calls := make(chan struct{}, 4)
	arb.onHalt = func() { calls <- struct{}{} }
	arb.ttsStarted()
	arb.micVoiceDetected()
	arb.micVoiceDetected()
	arb.micVoiceDetected()
	time.Sleep(5 * time.Millisecond)
	close(calls)
	n := 0
	for range calls {
		n++
	}
	if n != 1 {
		t.Fatalf("expected 1 halt per TTS window, got %d", n)
	}
}

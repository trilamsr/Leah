package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/trilam/leah/internal/platform/ipc"
	"github.com/trilam/leah/internal/input/voice/duplex"
)

// fakeDuplex is a controllable DuplexSession for IPC handler tests. The events
// channel is exposed so tests can push DuplexEvents one at a time and assert
// the wire frames the handler emits.
type fakeDuplex struct {
	events    chan duplex.DuplexEvent
	startErr  error
	interrupt int
	end       int
}

func (f *fakeDuplex) Start(_ context.Context, _ duplex.DuplexOpts) (<-chan duplex.DuplexEvent, error) {
	if f.startErr != nil {
		return nil, f.startErr
	}
	if f.events == nil {
		f.events = make(chan duplex.DuplexEvent, 4)
	}
	return f.events, nil
}

func (f *fakeDuplex) Interrupt() { f.interrupt++ }
func (f *fakeDuplex) End()       { f.end++ }

// BUG-3 regression: when the HUD drops mid-stream the handler must exit its
// fan-out goroutine. Pre-fix the goroutine blocks forever on `out <- frame`.
// Reproduction: produce more events than the out buffer can hold while no
// reader drains. The handler must observe ctx done and exit.
func TestHandleVoiceStart_GoroutineExitsOnContextCancel(t *testing.T) {
	sess := &fakeDuplex{events: make(chan duplex.DuplexEvent, 32)}
	ctx, cancel := context.WithCancel(context.Background())
	out, err := handleVoiceStart(ctx, ipc.Frame{Kind: ipc.KindVoiceStart, TurnID: "v1"}, sess)
	if err != nil {
		t.Fatalf("handleVoiceStart: %v", err)
	}
	// Saturate the producer queue beyond the handler's 8-frame out buffer so
	// the inner send blocks (frame 9 onward parks on `out <- ...`). Do NOT
	// close events — a real producer keeps sending until it observes ctx done.
	for i := 0; i < 32; i++ {
		sess.events <- duplex.DuplexEvent{Kind: duplex.PartialIn, Text: "x"}
	}
	cancel()
	// Now the only escape is for the handler's send to honour ctx.Done. The
	// handler must close `out` within 500ms; pre-fix it parks forever because
	// the goroutine has no ctx selector around the send.
	deadline := time.After(500 * time.Millisecond)
	for {
		select {
		case _, ok := <-out:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("voice handler goroutine leaked: out never closed after ctx cancel with live producer")
		}
	}
}

// BUG-4 regression: nil session must surface as an error frame, not a fake ok.
func TestHandleVoiceBarge_NilSessionEmitsErrorFrame(t *testing.T) {
	out, err := handleVoiceBarge(context.Background(), ipc.Frame{Kind: ipc.KindVoiceBarge, TurnID: "v2"}, nil)
	if err != nil {
		t.Fatalf("handleVoiceBarge: %v", err)
	}
	f := <-out
	if f.Kind != ipc.KindError {
		t.Fatalf("nil session must error, got kind=%s payload=%s", f.Kind, string(f.Payload))
	}
}

func TestHandleVoiceEnd_NilSessionEmitsErrorFrame(t *testing.T) {
	out, err := handleVoiceEnd(context.Background(), ipc.Frame{Kind: ipc.KindVoiceEnd, TurnID: "v3"}, nil)
	if err != nil {
		t.Fatalf("handleVoiceEnd: %v", err)
	}
	f := <-out
	if f.Kind != ipc.KindError {
		t.Fatalf("nil session must error, got kind=%s payload=%s", f.Kind, string(f.Payload))
	}
}

func TestHandleVoiceBarge_LiveSessionAcks(t *testing.T) {
	sess := &fakeDuplex{}
	out, _ := handleVoiceBarge(context.Background(), ipc.Frame{Kind: ipc.KindVoiceBarge, TurnID: "v4"}, sess)
	f := <-out
	if f.Kind != ipc.KindVoiceBarge {
		t.Fatalf("live session must ack with VoiceBarge, got %s", f.Kind)
	}
	if sess.interrupt != 1 {
		t.Fatalf("Interrupt() not called; count=%d", sess.interrupt)
	}
}

func TestHandleVoiceEnd_LiveSessionAcks(t *testing.T) {
	sess := &fakeDuplex{}
	out, _ := handleVoiceEnd(context.Background(), ipc.Frame{Kind: ipc.KindVoiceEnd, TurnID: "v5"}, sess)
	f := <-out
	if f.Kind != ipc.KindVoiceEnd {
		t.Fatalf("live session must ack with VoiceEnd, got %s", f.Kind)
	}
	if sess.end != 1 {
		t.Fatalf("End() not called; count=%d", sess.end)
	}
}

// BUG-6 regression: unknown duplex event kinds must be observable on the wire
// rather than silently masquerading as an empty VoicePartial. The HUD partial
// counter would otherwise tick on a future event addition (e.g. TTSStart-like
// markers) and corrupt state.
func TestVoiceFrameOf_UnknownKindIsObservable(t *testing.T) {
	ev := duplex.DuplexEvent{Kind: duplex.DuplexEventKind(0xFFFF), Text: "should-not-be-partial"}
	kind, payload := voiceFrameOf(ev)
	if kind == ipc.KindVoicePartial {
		// Empty-text partial is the silent-drop trap. Either kind diverges OR
		// payload carries an "unknown_kind" marker. Today: neither — failure.
		if !bytes.Contains(payload, []byte("unknown_kind")) {
			t.Fatalf("unknown duplex event masquerades as VoicePartial without marker; payload=%s", string(payload))
		}
	}
}

// Smoke for the existing happy paths so the bug fixes don't silently regress
// the wire mapping table.
func TestVoiceFrameOf_KnownKindsMapDeterministically(t *testing.T) {
	cases := []struct {
		ev   duplex.DuplexEvent
		kind string
	}{
		{duplex.DuplexEvent{Kind: duplex.PartialIn, Text: "p"}, ipc.KindVoicePartial},
		{duplex.DuplexEvent{Kind: duplex.FinalIn, Text: "f"}, ipc.KindVoicePartial},
		{duplex.DuplexEvent{Kind: duplex.TTSChunk, Text: "c"}, ipc.KindVoiceTTSChunk},
		{duplex.DuplexEvent{Kind: duplex.BargeIn}, ipc.KindVoiceBarge},
		{duplex.DuplexEvent{Kind: duplex.TTSEnd, Text: "done"}, ipc.KindVoiceEnd},
		{duplex.DuplexEvent{Kind: duplex.ErrorEvent, Err: errors.New("x")}, ipc.KindError},
	}
	for i, c := range cases {
		got, payload := voiceFrameOf(c.ev)
		if got != c.kind {
			t.Errorf("case %d: kind=%s want=%s payload=%s", i, got, c.kind, string(payload))
		}
		var m map[string]any
		if err := json.Unmarshal(payload, &m); err != nil {
			t.Errorf("case %d: payload not json: %v", i, err)
		}
	}
}

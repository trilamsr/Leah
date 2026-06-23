package apple_test

import (
	"context"
	"encoding/json"
	"io"
	"testing"

	"github.com/trilam/leah/internal/ipc"
	"github.com/trilam/leah/internal/tts"
	"github.com/trilam/leah/internal/tts/apple"
)

// Name is the §2.7 voice-canon provider id; pinned so reasoner routing matches.
func TestSynth_NameIsAppleAva(t *testing.T) {
	s := apple.New(func(ipc.Frame) {})
	if got := s.Name(); got != "apple-ava" {
		t.Fatalf("name: %q", got)
	}
}

// Speak emits a tts.local frame carrying text + voice + turn_id; no audio
// bytes traverse IPC because the HUD owns AVSpeechSynthesizer.
func TestSynth_SpeakEmitsLocalFrame(t *testing.T) {
	var got ipc.Frame
	emit := func(f ipc.Frame) { got = f }
	s := apple.New(emit)
	ctx := apple.WithTurnID(context.Background(), "turn-42")
	stream, err := s.Speak(ctx, "your balance is $100", tts.DefaultVoice)
	if err != nil {
		t.Fatalf("speak: %v", err)
	}
	defer func() { _ = stream.Close() }()
	if got.Kind != "tts.local" {
		t.Fatalf("kind: %q", got.Kind)
	}
	if got.TurnID != "turn-42" {
		t.Fatalf("turn_id: %q", got.TurnID)
	}
	var p struct {
		Text  string `json:"text"`
		Voice string `json:"voice"`
	}
	if err := json.Unmarshal(got.Payload, &p); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if p.Text != "your balance is $100" {
		t.Fatalf("text: %q", p.Text)
	}
	if p.Voice != tts.DefaultVoice {
		t.Fatalf("voice: %q", p.Voice)
	}
	out, _ := io.ReadAll(stream)
	if len(out) != 0 {
		t.Fatalf("audio bytes leaked daemon-side: %d", len(out))
	}
}

// Cancel emits tts.cancel-local with matching turn_id so the HUD stops the
// in-flight AVSpeechSynthesizer utterance.
func TestSynth_CancelEmitsCancelFrame(t *testing.T) {
	var got ipc.Frame
	emit := func(f ipc.Frame) { got = f }
	s := apple.New(emit)
	if err := s.Cancel(context.Background(), "turn-42"); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if got.Kind != "tts.cancel-local" {
		t.Fatalf("kind: %q", got.Kind)
	}
	if got.TurnID != "turn-42" {
		t.Fatalf("turn_id: %q", got.TurnID)
	}
}

// Nil emit is a constructor-time error rather than a silent drop — wiring bug
// surfaces at daemon boot, not at first utterance.
func TestSynth_NilEmitRejected(t *testing.T) {
	s := apple.New(nil)
	if _, err := s.Speak(context.Background(), "hi", tts.DefaultVoice); err == nil {
		t.Fatalf("expected error for nil emit")
	}
	if err := s.Cancel(context.Background(), "turn-1"); err == nil {
		t.Fatalf("expected cancel error for nil emit")
	}
}

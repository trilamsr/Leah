// Package apple is the §17.17 offline / privacy-flagged TTS provider. The
// daemon side is a router only: Speak emits a tts.local IPC frame and the
// HUD calls AVSpeechSynthesizer locally, so no audio bytes traverse IPC.
package apple

import (
	"context"
	"encoding/json"
	"errors"
	"io"

	"github.com/trilam/leah/internal/platform/ipc"
	"github.com/trilam/leah/internal/actions/tts"
)

type ctxKey int

const turnIDKey ctxKey = 0

// WithTurnID attaches the reasoner turn id so Speak can stamp tts.local
// frames; the HUD pairs Speak with Cancel by this id.
func WithTurnID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, turnIDKey, id)
}

func turnIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(turnIDKey).(string); ok {
		return v
	}
	return ""
}

// Synth is the daemon-side router for §17.17 Apple Ava. The actual
// AVSpeechSynthesizer call lives in the HUD (Task 7); we just emit frames.
type Synth struct {
	emit func(ipc.Frame)
}

// New binds the router to the daemon's IPC emit fn. Nil emit is captured
// here so Speak/Cancel surface the wiring bug instead of silently dropping.
func New(emit func(ipc.Frame)) *Synth {
	return &Synth{emit: emit}
}

func (s *Synth) Name() string { return "apple-ava" }

// PreWarm asks the HUD to warm-load the Ava voice model so first-utterance
// TTFB pays no cold-load cost (decision-log #81).
func (s *Synth) PreWarm(ctx context.Context) error {
	if s.emit == nil {
		return errors.New("apple: emit nil")
	}
	s.emit(ipc.Frame{Kind: "tts.local.prewarm", TurnID: turnIDFromContext(ctx)})
	return nil
}

// Speak emits tts.local{text,voice,turn_id}; HUD synthesizes locally. The
// returned stream is empty by contract — audio bytes never cross IPC.
func (s *Synth) Speak(ctx context.Context, text, voice string) (tts.AudioStream, error) {
	if s.emit == nil {
		return nil, errors.New("apple: emit nil")
	}
	payload, err := json.Marshal(map[string]string{"text": text, "voice": voice})
	if err != nil {
		return nil, err
	}
	s.emit(ipc.Frame{Kind: "tts.local", TurnID: turnIDFromContext(ctx), Payload: payload})
	return &nopStream{}, nil
}

// Cancel emits tts.cancel-local so the HUD stops the in-flight utterance
// for turnID; used when the user barge-ins or aborts a turn.
func (s *Synth) Cancel(_ context.Context, turnID string) error {
	if s.emit == nil {
		return errors.New("apple: emit nil")
	}
	s.emit(ipc.Frame{Kind: "tts.cancel-local", TurnID: turnID})
	return nil
}

type nopStream struct{}

func (n *nopStream) Read(_ []byte) (int, error) { return 0, io.EOF }
func (n *nopStream) Close() error               { return nil }
func (n *nopStream) MIME() string               { return "" }

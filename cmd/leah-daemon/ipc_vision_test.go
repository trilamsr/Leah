package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"

	"github.com/trilam/leah/internal/ipc"
	"github.com/trilam/leah/internal/vision"
	"github.com/trilam/leah/internal/vision/router"
)

type stubRouter struct {
	askErr    error
	events    []router.ReasonerEvent
	gotMode   router.VisionMode
	gotPrompt string
	gotImage  vision.Image
}

func (s *stubRouter) Ask(_ context.Context, img vision.Image, prompt string, mode router.VisionMode) (<-chan router.ReasonerEvent, error) {
	s.gotImage, s.gotPrompt, s.gotMode = img, prompt, mode
	if s.askErr != nil {
		return nil, s.askErr
	}
	ch := make(chan router.ReasonerEvent, len(s.events))
	for _, ev := range s.events {
		ch <- ev
	}
	close(ch)
	return ch, nil
}

func (s *stubRouter) OCR(_ context.Context, _ vision.Image) ([]vision.TextBlock, error) {
	return nil, nil
}

func snapFrame(t *testing.T, p snapPayload) ipc.Frame {
	t.Helper()
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	return ipc.Frame{Kind: ipc.KindVisionSnap, TurnID: "t1", Seq: 1, Payload: b}
}

func drainFrames(ch <-chan ipc.Frame) []ipc.Frame {
	var out []ipc.Frame
	for f := range ch {
		out = append(out, f)
	}
	return out
}

func TestHandleVisionSnap_StreamsProseAndTurnEnd(t *testing.T) {
	sr := &stubRouter{events: []router.ReasonerEvent{
		{Text: "a"}, {Text: "b"}, {IsFinal: true},
	}}
	frame := snapFrame(t, snapPayload{
		Prompt:   "what",
		Mode:     int(router.VisionSonnet),
		ImageB64: base64.StdEncoding.EncodeToString([]byte{1, 2, 3, 4}),
		Width:    1, Height: 1, MIME: "image/rgba",
	})
	ch, err := handleVisionSnap(context.Background(), frame, sr)
	if err != nil {
		t.Fatalf("handleVisionSnap: %v", err)
	}
	got := drainFrames(ch)
	if len(got) != 3 {
		t.Fatalf("expected 3 frames, got %d: %+v", len(got), got)
	}
	if got[0].Kind != ipc.KindProseDelta || got[1].Kind != ipc.KindProseDelta || got[2].Kind != ipc.KindTurnEnd {
		t.Fatalf("frame kinds mismatch: %+v", got)
	}
	if sr.gotMode != router.VisionSonnet || sr.gotPrompt != "what" {
		t.Fatalf("router got wrong inputs: mode=%d prompt=%q", sr.gotMode, sr.gotPrompt)
	}
	if len(sr.gotImage.Pixels) != 4 {
		t.Fatalf("image pixels not decoded: %d", len(sr.gotImage.Pixels))
	}
}

func TestHandleVisionSnap_ConsentDeniedEmitsConsentRequired(t *testing.T) {
	sr := &stubRouter{askErr: router.ErrConsentDenied}
	frame := snapFrame(t, snapPayload{Mode: int(router.VisionSonnet)})
	ch, err := handleVisionSnap(context.Background(), frame, sr)
	if err != nil {
		t.Fatalf("handleVisionSnap: %v", err)
	}
	got := drainFrames(ch)
	if len(got) != 1 || got[0].Kind != ipc.KindVisionConsentRequired {
		t.Fatalf("expected single consent.required frame; got %+v", got)
	}
}

func TestHandleVisionSnap_BadBase64IsErrFrame(t *testing.T) {
	frame := snapFrame(t, snapPayload{ImageB64: "!!!not base64!!!"})
	ch, err := handleVisionSnap(context.Background(), frame, &stubRouter{})
	if err != nil {
		t.Fatalf("handleVisionSnap: %v", err)
	}
	got := drainFrames(ch)
	if len(got) != 1 || got[0].Kind != ipc.KindError {
		t.Fatalf("expected single error frame; got %+v", got)
	}
}

func TestHandleVisionSnap_TerminalErrorPropagates(t *testing.T) {
	sr := &stubRouter{events: []router.ReasonerEvent{
		{Err: errors.New("upstream boom"), IsFinal: true},
	}}
	frame := snapFrame(t, snapPayload{Mode: int(router.VisionSonnet)})
	ch, err := handleVisionSnap(context.Background(), frame, sr)
	if err != nil {
		t.Fatalf("handleVisionSnap: %v", err)
	}
	got := drainFrames(ch)
	if len(got) != 1 || got[0].Kind != ipc.KindError {
		t.Fatalf("expected single error frame; got %+v", got)
	}
}

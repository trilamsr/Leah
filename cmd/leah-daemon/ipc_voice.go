package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/trilam/leah/internal/platform/ipc"
	"github.com/trilam/leah/internal/input/voice/duplex"
)

// handleVoiceStart begins a duplex session; the HUD subscribes to the same
// turn_id for partial / tts.chunk / end frames. Session is nil-safe — when the
// daemon boots without a voice stack the HUD gets a single error frame.
func handleVoiceStart(ctx context.Context, req ipc.Frame, sess duplex.DuplexSession) (<-chan ipc.Frame, error) {
	if sess == nil {
		return errFrame(req, "voice session unavailable"), nil
	}
	var p struct {
		VoiceOnly bool   `json:"voiceOnly"`
		Source    string `json:"source"`
	}
	if len(req.Payload) > 0 && string(req.Payload) != "null" {
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errFrame(req, fmt.Sprintf("bad payload: %v", err)), nil
		}
	}
	events, err := sess.Start(ctx, duplex.DuplexOpts{VoiceOnly: p.VoiceOnly})
	if err != nil {
		return errFrame(req, err.Error()), nil
	}
	out := make(chan ipc.Frame, 8)
	go func() {
		defer close(out)
		var seq int64
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-events:
				if !ok {
					return
				}
				seq++
				kind, payload := voiceFrameOf(ev)
				select {
				case <-ctx.Done():
					return
				case out <- ipc.Frame{Kind: kind, TurnID: req.TurnID, Seq: seq, Payload: payload}:
				}
			}
		}
	}()
	return out, nil
}

func handleVoiceBarge(_ context.Context, req ipc.Frame, sess duplex.DuplexSession) (<-chan ipc.Frame, error) {
	if sess == nil {
		return errFrame(req, "voice session unavailable"), nil
	}
	sess.Interrupt()
	payload, _ := json.Marshal(map[string]bool{"ok": true})
	out := make(chan ipc.Frame, 1)
	out <- ipc.Frame{Kind: ipc.KindVoiceBarge, TurnID: req.TurnID, Seq: 1, Payload: payload}
	close(out)
	return out, nil
}

func handleVoiceEnd(_ context.Context, req ipc.Frame, sess duplex.DuplexSession) (<-chan ipc.Frame, error) {
	if sess == nil {
		return errFrame(req, "voice session unavailable"), nil
	}
	sess.End()
	payload, _ := json.Marshal(map[string]bool{"ok": true})
	out := make(chan ipc.Frame, 1)
	out <- ipc.Frame{Kind: ipc.KindVoiceEnd, TurnID: req.TurnID, Seq: 1, Payload: payload}
	close(out)
	return out, nil
}

// voiceFrameOf maps a duplex.DuplexEvent to wire form. Unknown kinds surface
// as VoicePartial with an "unknown_kind" marker so the HUD can distinguish
// a real partial from an unrecognized event.
func voiceFrameOf(ev duplex.DuplexEvent) (string, json.RawMessage) {
	switch ev.Kind {
	case duplex.PartialIn, duplex.FinalIn, duplex.WakeDetected:
		payload, _ := json.Marshal(map[string]any{"text": ev.Text, "final": ev.Kind == duplex.FinalIn, "latency_ms": ev.LatencyMS})
		return ipc.KindVoicePartial, payload
	case duplex.TTSChunk, duplex.TTSStart:
		payload, _ := json.Marshal(map[string]any{"text": ev.Text})
		return ipc.KindVoiceTTSChunk, payload
	case duplex.BargeIn:
		payload, _ := json.Marshal(map[string]bool{"ok": true})
		return ipc.KindVoiceBarge, payload
	case duplex.TTSEnd:
		payload, _ := json.Marshal(map[string]string{"reason": ev.Text})
		return ipc.KindVoiceEnd, payload
	case duplex.ErrorEvent:
		msg := ""
		if ev.Err != nil {
			msg = ev.Err.Error()
		}
		payload, _ := json.Marshal(map[string]string{"error": msg})
		return ipc.KindError, payload
	default:
		payload, _ := json.Marshal(map[string]any{
			"text":         ev.Text,
			"unknown_kind": int(ev.Kind),
		})
		return ipc.KindVoicePartial, payload
	}
}

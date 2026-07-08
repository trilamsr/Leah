package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/trilam/leah/internal/ipc"
	"github.com/trilam/leah/internal/vision"
	"github.com/trilam/leah/internal/vision/router"
)

// snapPayload is the KindVisionSnap wire format. ImageB64 is the rgba pixels
// captured by SnapTool.swift base64-encoded so JSON survives the AF_UNIX
// frame size cap; Width/Height/MIME echo vision.Image fields verbatim.
type snapPayload struct {
	Prompt   string `json:"prompt"`
	Mode     int    `json:"mode"`
	ImageB64 string `json:"image_b64"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	MIME     string `json:"mime"`
}

// handleVisionSnap routes a one-shot screenshot Ask through router.Router.
// Returns either streamed prose.delta frames (Sonnet path), a single
// vision.consent.required nudge (no consent + no auto-prompt), or a terminal
// error frame (budget over cap → falls back to OCR in T04.b once HUD wiring
// can render the local result inline).
//
// Wired by main.go in T04.b — kept out of newIPCHandlerWithClassifyEnrich's
// switch this PR to avoid colliding with T02's voice kinds in the same file.
func handleVisionSnap(ctx context.Context, req ipc.Frame, r router.Router) (<-chan ipc.Frame, error) {
	var p snapPayload
	if len(req.Payload) > 0 && string(req.Payload) != "null" {
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return visionErr(req, fmt.Sprintf("bad payload: %v", err)), nil
		}
	}
	pixels, err := base64.StdEncoding.DecodeString(p.ImageB64)
	if err != nil {
		return visionErr(req, fmt.Sprintf("image_b64 decode: %v", err)), nil
	}
	img := vision.Image{Pixels: pixels, Width: p.Width, Height: p.Height, MIME: p.MIME}

	events, err := r.Ask(ctx, img, p.Prompt, router.VisionMode(p.Mode))
	if err != nil {
		if errors.Is(err, router.ErrConsentDenied) {
			out := make(chan ipc.Frame, 1)
			out <- ipc.Frame{
				Kind:    ipc.KindVisionConsentRequired,
				TurnID:  req.TurnID,
				Seq:     1,
				Payload: json.RawMessage(`{"mode":"screenshot"}`),
			}
			close(out)
			return out, nil
		}
		// Frame any other Ask error so the HUD sees a terminal error rather
		// than the AF_UNIX conn dropping — server.go closes on handler-error
		// return per spec §10.7, which would orphan the turn on the HUD side.
		return visionErr(req, fmt.Sprintf("vision router: %v", err)), nil
	}

	out := make(chan ipc.Frame, 8)
	go func() {
		defer close(out)
		var seq int64
		send := func(f ipc.Frame) bool {
			select {
			case <-ctx.Done():
				return false
			case out <- f:
				return true
			}
		}
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-events:
				if !ok {
					return
				}
				seq++
				if ev.Err != nil {
					payload, _ := json.Marshal(map[string]string{"error": ev.Err.Error()})
					send(ipc.Frame{Kind: ipc.KindError, TurnID: req.TurnID, Seq: seq, Payload: payload})
					return
				}
				if ev.IsFinal {
					send(ipc.Frame{Kind: ipc.KindTurnEnd, TurnID: req.TurnID, Seq: seq, Payload: json.RawMessage(`{}`)})
					return
				}
				payload, _ := json.Marshal(map[string]string{"text": ev.Text})
				if !send(ipc.Frame{Kind: ipc.KindProseDelta, TurnID: req.TurnID, Seq: seq, Payload: payload}) {
					return
				}
			}
		}
	}()
	return out, nil
}

func visionErr(req ipc.Frame, msg string) <-chan ipc.Frame {
	out := make(chan ipc.Frame, 1)
	payload, _ := json.Marshal(map[string]string{"error": msg})
	out <- ipc.Frame{Kind: ipc.KindError, TurnID: req.TurnID, Seq: 1, Payload: payload}
	close(out)
	return out
}

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/trilam/leah/internal/platform/ipc"
	"github.com/trilam/leah/internal/actions/tts"
)

// ttsRegistry holds per-turn ctx.Cancel funcs so KindTTSCancel propagates
// into the in-flight Provider.Speak call (not merely deletes the entry).
type ttsRegistry struct {
	mu sync.Mutex
	m  map[string]context.CancelFunc
}

func newTTSRegistry() *ttsRegistry { return &ttsRegistry{m: map[string]context.CancelFunc{}} }

func (r *ttsRegistry) put(turnID string, cancel context.CancelFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if prev, ok := r.m[turnID]; ok {
		prev()
	}
	r.m[turnID] = cancel
}

func (r *ttsRegistry) take(turnID string) (context.CancelFunc, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.m[turnID]
	if ok {
		delete(r.m, turnID)
	}
	return c, ok
}

func (r *ttsRegistry) has(turnID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.m[turnID]
	return ok
}

// handleTTSSpeak routes via classifier; cloud streams chunks, Apple emits
// one local-play trigger (no audio bytes traverse IPC).
func handleTTSSpeak(
	ctx context.Context,
	req ipc.Frame,
	cloud, local tts.Provider,
	cls tts.Classifier,
	reg *ttsRegistry,
) (<-chan ipc.Frame, error) {
	var p struct {
		Text  string `json:"text"`
		Voice string `json:"voice"`
	}
	// nil/null/missing payload all coerce to empty p — uniform error.
	if len(req.Payload) > 0 && string(req.Payload) != "null" {
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return ttsErrChan(req, fmt.Sprintf("bad payload: %v", err)), nil
		}
	}
	if strings.TrimSpace(p.Text) == "" {
		return ttsErrChan(req, "text required"), nil
	}
	if p.Voice == "" {
		p.Voice = tts.DefaultVoice
	}

	// Route: classifier RouteLocal → Apple. Cloud path is preferred fallback,
	// but if cloud is nil (no API key) → degrade to Apple universally so the
	// operator still gets TTS instead of "not configured" for benign text.
	prov := cloud
	if cls != nil && cls.Route(p.Text) == tts.RouteLocal {
		prov = local
	}
	if prov == nil {
		prov = local // graceful degradation when cloud absent
	}
	if prov == nil {
		return ttsErrChan(req, "tts provider not configured"), nil
	}

	speakCtx, cancel := context.WithCancel(ctx)
	reg.put(req.TurnID, cancel)

	out := make(chan ipc.Frame, 32)
	go func() {
		defer close(out)
		defer cancel()
		defer func() { _, _ = reg.take(req.TurnID) }()

		if prov.Name() == "apple-ava" {
			payload, _ := json.Marshal(map[string]string{"text": p.Text, "voice": p.Voice})
			if stream, err := prov.Speak(speakCtx, p.Text, p.Voice); err == nil {
				_ = stream.Close()
			}
			select {
			case <-speakCtx.Done():
				return
			case out <- ipc.Frame{Kind: ipc.KindTTSAppleSpeak, TurnID: req.TurnID, Seq: 1, Payload: payload}:
			}
			return
		}

		stream, err := prov.Speak(speakCtx, p.Text, p.Voice)
		if err != nil {
			emitTTSErr(speakCtx, out, req.TurnID, 1, err)
			return
		}
		defer func() { _ = stream.Close() }()

		buf := make([]byte, 4096)
		var seq int64 = 1
		for {
			n, rerr := stream.Read(buf)
			if n > 0 {
				chunk := append([]byte(nil), buf[:n]...)
				payload, _ := json.Marshal(map[string]any{
					"bytes": chunk,
					"mime":  stream.MIME(),
				})
				select {
				case <-speakCtx.Done():
					return
				case out <- ipc.Frame{Kind: ipc.KindTTSCloudFrame, TurnID: req.TurnID, Seq: seq, Payload: payload}:
				}
				seq++
			}
			if errors.Is(rerr, io.EOF) {
				select {
				case <-speakCtx.Done():
					return
				case out <- ipc.Frame{Kind: ipc.KindTTSSpeakDone, TurnID: req.TurnID, Seq: seq, Payload: json.RawMessage(`{}`)}:
				}
				return
			}
			if rerr != nil {
				if errors.Is(rerr, context.Canceled) || errors.Is(speakCtx.Err(), context.Canceled) {
					return
				}
				emitTTSErr(speakCtx, out, req.TurnID, seq, rerr)
				return
			}
		}
	}()
	return out, nil
}

// handleTTSCancel propagates ctx.Cancel into the in-flight Speak.
func handleTTSCancel(_ context.Context, req ipc.Frame, reg *ttsRegistry) (<-chan ipc.Frame, error) {
	if cancel, ok := reg.take(req.TurnID); ok {
		cancel()
	}
	out := make(chan ipc.Frame, 1)
	out <- ipc.Frame{Kind: ipc.KindTTSCancelOK, TurnID: req.TurnID, Seq: 1, Payload: json.RawMessage(`{}`)}
	close(out)
	return out, nil
}

func ttsErrChan(req ipc.Frame, msg string) <-chan ipc.Frame {
	out := make(chan ipc.Frame, 1)
	payload, _ := json.Marshal(map[string]string{"error": msg})
	out <- ipc.Frame{Kind: ipc.KindTTSSpeakErr, TurnID: req.TurnID, Seq: 1, Payload: payload}
	close(out)
	return out
}

func emitTTSErr(ctx context.Context, out chan<- ipc.Frame, turnID string, seq int64, err error) {
	payload, _ := json.Marshal(map[string]string{"error": err.Error()})
	select {
	case <-ctx.Done():
	case out <- ipc.Frame{Kind: ipc.KindTTSSpeakErr, TurnID: turnID, Seq: seq, Payload: payload}:
	}
}

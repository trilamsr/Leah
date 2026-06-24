// Package router decides local-OCR vs Sonnet vision per spec §4.6 and gates
// the cloud path on a first-time-per-session ConsentStore prompt. Budget
// degrade (BucketCloudVisionBytes over cap) silently downgrades to local
// instead of failing the call — operators see the spec §8.4 ActionSwitchLocal
// rung surface in Settings, not a HUD error.
package router

import (
	"context"
	"errors"
	"sync"

	"github.com/trilam/leah/internal/vision"
)

// VisionMode is the operator preference for a single Ask. VisionAuto lets the
// router escalate to Sonnet when prompt intent + frame size suggest local OCR
// would not answer the prompt — heuristic lives in escalate.go.
type VisionMode int

const (
	VisionLocal VisionMode = iota
	VisionSonnet
	VisionAuto
)

// ReasonerEvent is a single chunk on the Ask channel. IsFinal=true marks the
// last frame (either turn end OR terminal error). Err non-nil implies IsFinal.
type ReasonerEvent struct {
	Text    string
	IsFinal bool
	Err     error
}

// Router fans Ask requests onto local OCR or Sonnet vision. OCR remains direct
// passthrough — the consent gate guards only cloud uploads.
type Router interface {
	Ask(ctx context.Context, frame vision.Image, prompt string, mode VisionMode) (<-chan ReasonerEvent, error)
	OCR(ctx context.Context, frame vision.Image) ([]vision.TextBlock, error)
}

// VisionChunk is a single SonnetClient stream frame. Text-only on the happy
// path; Err non-nil signals a mid-stream failure (network drop, rate-limit,
// invalid-image, auth) — implementations send one Err chunk and close. The
// router surfaces Err as a terminal ReasonerEvent so the HUD sees the failure
// rather than a silent truncated answer.
type VisionChunk struct {
	Text string
	Err  error
}

// SonnetClient is the cloud vision leg. Implementations base64-encode frame
// and call Anthropic Messages with a Base64ImageSource; the router itself
// stays SDK-free so tests can inject a chunk slice.
type SonnetClient interface {
	StreamVision(ctx context.Context, image vision.Image, prompt string) (<-chan VisionChunk, error)
}

// VisionMeter is the slice of budget.Runtime the router needs. Passing the
// interface (not *budget.Runtime) keeps the router test-injectable without
// spinning a SQLite DB per test.
type VisionMeter interface {
	Charge(ctx context.Context, bucket string, n int64) error
}

// BucketCloudVisionBytes mirrors budget.BucketCloudVisionBytes verbatim — kept
// as a router-side constant so callers don't import the budget package solely
// to charge through us.
const BucketCloudVisionBytes = "cloud.vision.bytes"

// ErrConsentDenied is returned by Ask when VisionSonnet (or VisionAuto that
// escalated) lacks a Grant and the prompt callback returned false. Callers
// surface this to the HUD as a one-shot toast, not a retry.
var ErrConsentDenied = errors.New("vision: consent denied")

type router struct {
	ocr     vision.OCREngine
	sonnet  SonnetClient
	consent ConsentStore
	meter   VisionMeter
	prompt  func(mode string) bool
	// gateMu serializes the consent check-prompt-grant sequence so two
	// concurrent VisionSonnet Asks can't both observe !Granted and each fire
	// the operator prompt — §4.6 mandates first-time-per-session, singular.
	gateMu sync.Mutex
}

// New wires a Router. prompt may be nil — Ask then treats absent consent as
// a hard deny rather than panicking, which matches the daemon's startup
// invariant: a nil prompt means "HUD not connected yet, refuse cloud".
func New(ocr vision.OCREngine, sonnet SonnetClient, consent ConsentStore, meter VisionMeter, prompt func(mode string) bool) Router {
	return &router{ocr: ocr, sonnet: sonnet, consent: consent, meter: meter, prompt: prompt}
}

func (r *router) Ask(ctx context.Context, frame vision.Image, prompt string, mode VisionMode) (<-chan ReasonerEvent, error) {
	useSonnet := mode == VisionSonnet || (mode == VisionAuto && shouldEscalate(frame, prompt))
	if !useSonnet {
		out := make(chan ReasonerEvent, 1)
		close(out)
		return out, nil
	}
	r.gateMu.Lock()
	if !r.consent.Granted("screenshot") {
		if r.prompt == nil || !r.prompt("screenshot") {
			r.gateMu.Unlock()
			return nil, ErrConsentDenied
		}
		r.consent.Grant("screenshot", ScopeThisSession)
	}
	r.gateMu.Unlock()
	if r.meter != nil {
		if err := r.meter.Charge(ctx, BucketCloudVisionBytes, int64(len(frame.Pixels))); err != nil {
			out := make(chan ReasonerEvent, 1)
			out <- ReasonerEvent{Err: err, IsFinal: true}
			close(out)
			return out, nil
		}
	}
	out := make(chan ReasonerEvent, 8)
	go func() {
		defer close(out)
		stream, err := r.sonnet.StreamVision(ctx, frame, prompt)
		if err != nil {
			out <- ReasonerEvent{Err: err, IsFinal: true}
			return
		}
		for chunk := range stream {
			if chunk.Err != nil {
				out <- ReasonerEvent{Err: chunk.Err, IsFinal: true}
				return
			}
			select {
			case <-ctx.Done():
				return
			case out <- ReasonerEvent{Text: chunk.Text}:
			}
		}
		out <- ReasonerEvent{IsFinal: true}
	}()
	return out, nil
}

func (r *router) OCR(ctx context.Context, frame vision.Image) ([]vision.TextBlock, error) {
	return r.ocr.Recognize(ctx, frame)
}

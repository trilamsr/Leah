package listener

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/trilam/leah/internal/contracts"
)

const (
	ScopeListenStart   = "voice:listen:start"
	DefaultRealtimeURL = "wss://api.openai.com/v1/realtime?model=gpt-4o-realtime-preview"

	// Mirrors Fake — partials cluster (~10/s), reader is single-select-loop fast.
	segmentChanDepth = 16
)

var ErrAttestationDenied = errors.New("voice/listener: attestation denied")

// Production wires gorilla/websocket; tests inject a fake yielding canned frames.
type WebSocketConn interface {
	ReadMessage() ([]byte, error)
	WriteMessage(p []byte) error
	Close() error
}

// Token stays on the dialer side of the seam so the secret never leaks back through the adapter.
type WebSocketDialer interface {
	Dial(ctx context.Context, url, token string) (WebSocketConn, error)
}

type Config struct {
	Attestor    contracts.Attestor
	TokenSource contracts.TokenSource
	Dialer      WebSocketDialer
	URL         string
}

type OpenAIRealtime struct {
	cfg     Config
	mu      sync.Mutex
	started bool
}

func NewOpenAIRealtime(cfg Config) *OpenAIRealtime {
	return &OpenAIRealtime{cfg: cfg}
}

func (r *OpenAIRealtime) Start(ctx context.Context) (<-chan Segment, error) {
	r.mu.Lock()
	if r.started {
		r.mu.Unlock()
		return nil, errors.New("voice/listener: OpenAIRealtime already started")
	}
	r.started = true
	r.mu.Unlock()

	if r.cfg.Dialer == nil {
		return nil, errors.New("voice/listener: OpenAIRealtime requires Config.Dialer")
	}
	if r.cfg.Attestor == nil {
		return nil, errors.New("voice/listener: OpenAIRealtime requires Config.Attestor")
	}
	if r.cfg.TokenSource == nil {
		return nil, errors.New("voice/listener: OpenAIRealtime requires Config.TokenSource")
	}

	// Attest BEFORE dial — denied operator never reaches the network.
	if err := r.cfg.Attestor.Attest(ctx, ScopeListenStart); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAttestationDenied, err)
	}
	tok, err := r.cfg.TokenSource.Token(ctx)
	if err != nil {
		return nil, fmt.Errorf("voice/listener: token load: %w", err)
	}

	url := r.cfg.URL
	if url == "" {
		url = DefaultRealtimeURL
	}
	conn, err := r.cfg.Dialer.Dial(ctx, url, tok)
	if err != nil {
		return nil, fmt.Errorf("voice/listener: realtime dial: %w", err)
	}

	out := make(chan Segment, segmentChanDepth)
	go r.readLoop(ctx, conn, out)
	return out, nil
}

func (r *OpenAIRealtime) readLoop(ctx context.Context, conn WebSocketConn, out chan<- Segment) {
	defer close(out)
	defer func() { _ = conn.Close() }()

	// ctx-cancel watcher unblocks ReadMessage — otherwise a stalled remote pins the goroutine.
	closed := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-closed:
		}
	}()
	defer close(closed)

	for {
		raw, err := conn.ReadMessage()
		if err != nil {
			return
		}
		seg, ok := parseRealtimeFrame(raw)
		if !ok {
			continue
		}
		select {
		case out <- seg:
		case <-ctx.Done():
			return
		}
	}
}

// Non-transcription frames (session.created, errors, audio acks) drop silently — surfacing them needs a richer seam than W14 ships.
func parseRealtimeFrame(raw []byte) (Segment, bool) {
	var env struct {
		Type       string `json:"type"`
		Delta      string `json:"delta"`
		Transcript string `json:"transcript"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return Segment{}, false
	}
	switch env.Type {
	case "conversation.item.input_audio_transcription.delta":
		return Segment{Text: env.Delta, Final: false, FinishedAt: time.Now()}, true
	case "conversation.item.input_audio_transcription.completed":
		now := time.Now()
		return Segment{Text: env.Transcript, Final: true, StartedAt: now, FinishedAt: now}, true
	}
	return Segment{}, false
}

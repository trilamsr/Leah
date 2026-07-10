// OpenAIRealtimeDecoder is a frame-decoder seam for the OpenAI Realtime STT
// websocket. It DOES NOT send audio — only the read path is wired because
// Whisper-local is the chosen backend and the OpenAI Realtime path sits
// behind a future LEAH_VOICE_ALLOW_OPENAI_STT opt-in. Production session code
// MUST NOT wire this decoder until the send path lands; SendAudio panics so
// premature wiring fails loud instead of blocking silently.
//
// Uplink checklist: session.update on connect, input_audio_buffer.append with
// base64 PCM frames, WSS reconnect/backoff, leah connect openai onboarding.

package listener

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sync"
	"sync/atomic"
	"time"

	"github.com/trilam/leah/internal/platform/contracts"
)

const (
	ScopeListenStart   = "voice:listen:start"
	DefaultRealtimeURL = "wss://api.openai.com/v1/realtime?model=gpt-4o-realtime-preview"

	// Mirrors Fake — partials cluster (~10/s), reader is single-select-loop fast.
	segmentChanDepth = 16

	// Spec §11 — secret-shape redaction. Anything decoded matching this
	// shape never reaches the segment channel; the audit row uses [REDACTED].
	secretShapePattern = `(?:sk-|gh[ps]_|xox[bp]-)\w{20,}`
)

var (
	ErrAttestationDenied = errors.New("voice/listener: attestation denied")
	ErrInsecureScheme    = errors.New("voice/listener: realtime URL must use wss:// (bearer-token cleartext risk)")
	ErrUplinkNotShipped  = errors.New("voice/listener: audio uplink not implemented — W14b owns mic→input_audio_buffer.append")

	secretShapeRE = regexp.MustCompile(secretShapePattern)
)

// Production wires gorilla/websocket; tests inject a fake yielding canned frames.
// Close is safe to call concurrently with ReadMessage — the wrapper must guard
// duplicate Close calls itself; this seam makes no concurrent-Close guarantee.
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

// DecoderMetrics surfaces frame-parse health for the session arbitrator and
// tests. Counters are read with sync/atomic; readers see at-least-eventual
// values, which is sufficient for the session's periodic health sample.
type DecoderMetrics struct {
	unknownFrames int64 // unmarshal failure OR type not in switch
	failedEvents  int64 // …transcription.failed observed
}

func (m *DecoderMetrics) UnknownFrames() int64 { return atomic.LoadInt64(&m.unknownFrames) }
func (m *DecoderMetrics) FailedEvents() int64  { return atomic.LoadInt64(&m.failedEvents) }

type OpenAIRealtimeDecoder struct {
	cfg     Config
	metrics DecoderMetrics
	mu      sync.Mutex
	started bool
}

func NewOpenAIRealtimeDecoder(config Config) *OpenAIRealtimeDecoder {
	return &OpenAIRealtimeDecoder{cfg: config}
}

// Metrics exposes parse-health counters. Safe for concurrent read.
func (r *OpenAIRealtimeDecoder) Metrics() *DecoderMetrics { return &r.metrics }

// SendAudio is the planned W14b uplink entry point. It panics so any caller
// that wires this decoder into production before W14b ships fails loud.
func (r *OpenAIRealtimeDecoder) SendAudio(_ []byte) error {
	panic(ErrUplinkNotShipped)
}

func (r *OpenAIRealtimeDecoder) Start(ctx context.Context) (<-chan Segment, error) {
	if err := r.validateConfig(); err != nil {
		return nil, err
	}

	r.mu.Lock()
	if r.started {
		r.mu.Unlock()
		return nil, errors.New("voice/listener: OpenAIRealtimeDecoder already started")
	}
	r.started = true
	r.mu.Unlock()

	// Attest BEFORE dial — denied operator never reaches the network.
	if err := r.cfg.Attestor.Attest(ctx, ScopeListenStart); err != nil {
		r.resetStarted()
		return nil, fmt.Errorf("%w: %v", ErrAttestationDenied, err)
	}
	tok, err := r.cfg.TokenSource.Token(ctx)
	if err != nil {
		r.resetStarted()
		return nil, fmt.Errorf("voice/listener: token load: %w", err)
	}

	conn, err := r.cfg.Dialer.Dial(ctx, r.dialURL(), tok)
	if err != nil {
		r.resetStarted()
		return nil, fmt.Errorf("voice/listener: realtime dial: %w", err)
	}

	out := make(chan Segment, segmentChanDepth)
	go r.readLoop(ctx, conn, out)
	return out, nil
}

func (r *OpenAIRealtimeDecoder) validateConfig() error {
	if r.cfg.Dialer == nil {
		return errors.New("voice/listener: OpenAIRealtimeDecoder requires Config.Dialer")
	}
	if r.cfg.Attestor == nil {
		return errors.New("voice/listener: OpenAIRealtimeDecoder requires Config.Attestor")
	}
	if r.cfg.TokenSource == nil {
		return errors.New("voice/listener: OpenAIRealtimeDecoder requires Config.TokenSource")
	}
	if r.cfg.URL != "" {
		u, err := url.Parse(r.cfg.URL)
		if err != nil {
			return fmt.Errorf("voice/listener: parse URL: %w", err)
		}
		if u.Scheme != "wss" {
			return fmt.Errorf("%w: scheme=%q", ErrInsecureScheme, u.Scheme)
		}
	}
	return nil
}

func (r *OpenAIRealtimeDecoder) dialURL() string {
	if r.cfg.URL == "" {
		return DefaultRealtimeURL
	}
	return r.cfg.URL
}

func (r *OpenAIRealtimeDecoder) resetStarted() {
	r.mu.Lock()
	r.started = false
	r.mu.Unlock()
}

func (r *OpenAIRealtimeDecoder) readLoop(ctx context.Context, conn WebSocketConn, out chan<- Segment) {
	defer close(out)
	defer func() { _ = conn.Close() }()
	defer r.resetStarted()

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

	// Track utterance start so Final segments carry real duration semantics
	// (spec §3.1 expects FinishedAt-StartedAt to be the utterance length).
	var utteranceStart time.Time

	for {
		raw, err := conn.ReadMessage()
		if err != nil {
			// Synthesize a Final so the session arbitrator's select unblocks
			// — without this, a broken WSS pins the session in "speaking" forever.
			r.emitSyntheticFinal(out, utteranceStart)
			return
		}
		seg, ok, kind := r.decodeFrame(raw, &utteranceStart)
		if !ok {
			if kind == frameKindFailed {
				r.emitSyntheticFinal(out, utteranceStart)
				utteranceStart = time.Time{}
			}
			continue
		}
		if seg.Final {
			utteranceStart = time.Time{}
		}
		select {
		case out <- seg:
		case <-ctx.Done():
			return
		}
	}
}

func (r *OpenAIRealtimeDecoder) emitSyntheticFinal(out chan<- Segment, start time.Time) {
	now := time.Now()
	if start.IsZero() {
		start = now
	}
	// Non-blocking send — channel has segmentChanDepth headroom; if the reader
	// has already drained and bailed, drop rather than block readLoop teardown.
	select {
	case out <- Segment{Final: true, StartedAt: start, FinishedAt: now}:
	default:
	}
}

type frameKind int

const (
	frameKindOther frameKind = iota
	frameKindFailed
)

func (r *OpenAIRealtimeDecoder) decodeFrame(raw []byte, utteranceStart *time.Time) (Segment, bool, frameKind) {
	var env struct {
		Type       string `json:"type"`
		Delta      string `json:"delta"`
		Transcript string `json:"transcript"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		atomic.AddInt64(&r.metrics.unknownFrames, 1)
		return Segment{}, false, frameKindOther
	}
	now := time.Now()
	switch env.Type {
	case "conversation.item.input_audio_transcription.delta":
		if utteranceStart.IsZero() {
			*utteranceStart = now
		}
		if secretShapeRE.MatchString(env.Delta) {
			// Discard the segment entirely — spec §11 OAuth-secret-via-voice.
			return Segment{}, false, frameKindOther
		}
		return Segment{Text: env.Delta, Final: false, StartedAt: *utteranceStart, FinishedAt: now}, true, frameKindOther
	case "conversation.item.input_audio_transcription.completed":
		start := *utteranceStart
		if start.IsZero() {
			start = now
		}
		if secretShapeRE.MatchString(env.Transcript) {
			// Replace text with [REDACTED] but still emit Final so session arbitration completes.
			return Segment{Text: "[REDACTED]", Final: true, StartedAt: start, FinishedAt: now}, true, frameKindOther
		}
		return Segment{Text: env.Transcript, Final: true, StartedAt: start, FinishedAt: now}, true, frameKindOther
	case "conversation.item.input_audio_transcription.failed":
		atomic.AddInt64(&r.metrics.failedEvents, 1)
		return Segment{}, false, frameKindFailed
	}
	atomic.AddInt64(&r.metrics.unknownFrames, 1)
	return Segment{}, false, frameKindOther
}

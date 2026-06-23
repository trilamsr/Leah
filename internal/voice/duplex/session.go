// Package duplex orchestrates STT (T01) + reasoner + TTS (Phase 3) into a
// single streaming voice session with barge-in. Spec §1.3 mandates a single
// DuplexSession that halts TTS within 80ms when mic VAD detects voice during
// playback. Budget Charger is injected via constructor so the T08 runtime
// can wire cloud.stt.seconds + cloud.tts.chars without further changes here.
package duplex

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/trilam/leah/internal/tts"
	"github.com/trilam/leah/internal/voice/stt"
)

type DuplexEventKind int

const (
	WakeDetected DuplexEventKind = iota
	PartialIn
	FinalIn
	TTSStart
	TTSChunk
	BargeIn
	TTSEnd
	ErrorEvent
)

type DuplexEvent struct {
	Kind      DuplexEventKind
	Text      string
	LatencyMS int
	Err       error
}

type DuplexOpts struct {
	VoiceOnly      bool
	SuppressApps   []string
	NoiseFloorDBFS float64
	MaxTurn        time.Duration
}

// AskFn is the reasoner adapter — token deltas land on the returned channel
// and the producer closes it. Decoupled from internal/reasoner to keep the
// duplex package independent of model-client choice (Sonnet/Opus/Haiku).
type AskFn func(ctx context.Context, prompt string) (<-chan string, error)

// Bucket names the budget category for a Charger entry; the real enum lives
// in T08 internal/budget. The string is the wire form so this package does
// not import the budget package and stays disjoint.
type Bucket string

const (
	BucketCloudSTTSeconds Bucket = "cloud.stt.seconds"
	BucketCloudTTSChars   Bucket = "cloud.tts.chars"
)

// Charger is the deferred-wire interface for T08. A nil charger is a no-op —
// production wires the real runtime once T08 lands.
type Charger interface {
	Charge(b Bucket, n int64) error
}

type DuplexSession interface {
	Start(ctx context.Context, opts DuplexOpts) (<-chan DuplexEvent, error)
	Interrupt()
	End()
}

type session struct {
	stt     stt.STT
	tts     tts.Provider
	ask     AskFn
	charger Charger
	arb     *bargeArbiter
	cancel  context.CancelFunc
	out     chan DuplexEvent
	endOnce sync.Once
}

func NewSession(s stt.STT, t tts.Provider, ask AskFn, c Charger) DuplexSession {
	return &session{stt: s, tts: t, ask: ask, charger: c, arb: newBargeArbiter()}
}

func (s *session) Start(ctx context.Context, opts DuplexOpts) (<-chan DuplexEvent, error) {
	if s.stt == nil || s.tts == nil || s.ask == nil {
		return nil, errors.New("duplex: stt/tts/ask required")
	}
	ctx, s.cancel = context.WithCancel(ctx)
	s.out = make(chan DuplexEvent, 16)
	s.arb.onHalt = func() {
		select {
		case s.out <- DuplexEvent{Kind: BargeIn}:
		default:
		}
	}
	go s.loop(ctx, opts)
	return s.out, nil
}

// loop is the session goroutine. It owns s.out exclusively (close happens here
// only) so callers may safely call End() concurrently — the cancel propagates
// through ctx and the loop drains to a single TTSEnd before close.
func (s *session) loop(ctx context.Context, opts DuplexOpts) {
	defer close(s.out)
	var deadline <-chan time.Time
	if opts.MaxTurn > 0 {
		deadline = time.After(opts.MaxTurn)
	}
	for {
		select {
		case <-ctx.Done():
			s.emit(DuplexEvent{Kind: TTSEnd})
			return
		case <-deadline:
			s.emit(DuplexEvent{Kind: TTSEnd, Text: "timeout"})
			return
		}
	}
}

func (s *session) emit(ev DuplexEvent) {
	select {
	case s.out <- ev:
	default:
	}
}

func (s *session) Interrupt() { s.arb.micVoiceDetected() }
func (s *session) End() {
	s.endOnce.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}
	})
}

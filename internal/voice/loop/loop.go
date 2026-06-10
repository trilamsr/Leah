package loop

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/trilam/leah/internal/voice/listener"
	"github.com/trilam/leah/internal/voice/session"
)

// Listener is the segment-stream surface. listener.Listener satisfies it.
type Listener = listener.Listener

// ReasonerSeam is the LLM seam — reasoner.Reasoner.Ask satisfies it
// structurally, so loop never imports the reasoner package.
type ReasonerSeam interface {
	Ask(ctx context.Context, prompt string) (string, error)
}

// TTSSeam is the speak-with-ctx surface. voice.ChainTTS satisfies it; barge-in
// is realized by cancelling the per-turn context passed to Speak.
type TTSSeam interface {
	Speak(ctx context.Context, text string) error
}

// WakeSeam emits a value whenever the loop should leave the "requires wake"
// state and arm STT. nil disables wake gating — every Final segment is treated
// as armed (matches W11 wiring before W14 wake integration).
type WakeSeam interface {
	Events() <-chan struct{}
}

// IntentRouterSeam is the intent pre-filter. When Recognize returns ok=true
// the transcript is handed to the handler instead of the reasoner.
type IntentRouterSeam interface {
	Recognize(ctx context.Context, transcript string) (handled bool, err error)
}

// Config is the loop's seam bundle. All fields except Listener, Reasoner, and
// TTS are optional; Session is held for future Transition wiring and is read
// only for its Audit field today.
type Config struct {
	Listener     Listener
	Session      *session.Session
	Reasoner     ReasonerSeam
	TTS          TTSSeam
	Wake         WakeSeam
	IntentRouter IntentRouterSeam
}

// Loop is the §4-arbitration goroutine: one select over wake events, listener
// finals, and ctx cancellation. Concurrent Run is undefined.
type Loop struct {
	cfg Config
}

// New returns a Loop ready for Run.
func New(cfg Config) *Loop { return &Loop{cfg: cfg} }

// Run blocks until ctx is cancelled or the listener channel closes. Returns
// the first non-nil error from the listener Start.
func (l *Loop) Run(ctx context.Context) error {
	if l.cfg.Listener == nil {
		return errors.New("voice/loop: Listener required")
	}
	if l.cfg.Reasoner == nil {
		return errors.New("voice/loop: Reasoner required")
	}
	if l.cfg.TTS == nil {
		return errors.New("voice/loop: TTS required")
	}

	segs, err := l.cfg.Listener.Start(ctx)
	if err != nil {
		return err
	}

	var wakeCh <-chan struct{}
	if l.cfg.Wake != nil {
		wakeCh = l.cfg.Wake.Events()
	}

	armed := wakeCh == nil

	var (
		turnCancel context.CancelFunc
		turnWG     sync.WaitGroup
	)
	cancelTurn := func() {
		if turnCancel != nil {
			turnCancel()
			turnCancel = nil
		}
		turnWG.Wait()
	}

	startTurn := func(text string) {
		tctx, tcancel := context.WithCancel(ctx)
		turnCancel = tcancel
		turnWG.Add(1)
		go func() {
			defer turnWG.Done()
			if l.cfg.IntentRouter != nil {
				handled, ierr := l.cfg.IntentRouter.Recognize(tctx, text)
				if ierr == nil && handled {
					return
				}
			}
			reply, rerr := l.cfg.Reasoner.Ask(tctx, text)
			if rerr != nil {
				if errors.Is(tctx.Err(), context.Canceled) {
					return
				}
				_ = l.cfg.TTS.Speak(tctx, "I couldn't reason that through, try again.")
				return
			}
			_ = l.cfg.TTS.Speak(tctx, reply)
		}()
	}

	for {
		select {
		case <-ctx.Done():
			cancelTurn()
			return nil
		case _, ok := <-wakeCh:
			if !ok {
				wakeCh = nil
				continue
			}
			armed = true
		case seg, ok := <-segs:
			if !ok {
				cancelTurn()
				return nil
			}
			if !seg.Final {
				continue
			}
			text := strings.TrimSpace(seg.Text)
			if text == "" {
				continue
			}
			if !armed {
				continue
			}
			if turnCancel != nil {
				cancelTurn()
			}
			startTurn(text)
			if wakeCh != nil {
				armed = false
			}
		}
	}
}

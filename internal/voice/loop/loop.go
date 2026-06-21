package loop

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/trilam/leah/internal/voice/listener"
)

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

// WakeSeam arms STT — required, never nil. An always-armed pipeline would
// transcribe every Final segment without operator gesture; attestation lives
// in session.Session, which signals WakeSeam after consent.
type WakeSeam interface {
	Events() <-chan struct{}
}

// IntentRouterSeam is the intent pre-filter. When Recognize returns ok=true
// the transcript is handed to the handler instead of the reasoner.
type IntentRouterSeam interface {
	Recognize(ctx context.Context, transcript string) (handled bool, err error)
}

// Config is the loop's seam bundle. Listener / Reasoner / TTS / Wake are
// required; IntentRouter and IntentToFirstAudio are optional.
type Config struct {
	Listener           listener.Listener
	Reasoner           ReasonerSeam
	TTS                TTSSeam
	Wake               WakeSeam
	IntentRouter       IntentRouterSeam
	IntentToFirstAudio *IntentToFirstAudioTracker
}

// Loop is the §4-arbitration goroutine: one select over wake events, listener
// finals, and ctx cancellation. Concurrent Run is undefined.
type Loop struct {
	cfg Config
}

// errReasonFallback duplicates session.Session's literal — a third caller
// (W15+ streaming reasoner) gates extraction to internal/voice/strings.
const errReasonFallback = "I couldn't reason that through, try again."

const errAnnounceTimeout = 1500 * time.Millisecond

// New returns a Loop ready for Run.
func New(cfg Config) *Loop { return &Loop{cfg: cfg} }

// Run blocks until ctx is cancelled or the listener channel closes.
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
	if l.cfg.Wake == nil {
		return errors.New("voice/loop: Wake required")
	}

	segs, err := l.cfg.Listener.Start(ctx)
	if err != nil {
		return err
	}

	wakeCh := l.cfg.Wake.Events()
	armed := false

	var (
		turnCancel context.CancelFunc
		turnDone   chan struct{}
	)
	cancelTurn := func() {
		if turnCancel == nil {
			return
		}
		turnCancel()
		// Wait for the turn goroutine OR outer ctx — never block the arbiter
		// when a reasoner ignores ctx.
		select {
		case <-turnDone:
		case <-ctx.Done():
		}
		turnCancel = nil
		turnDone = nil
	}

	startTurn := func(text string) {
		tctx, tcancel := context.WithCancel(ctx)
		done := make(chan struct{})
		turnCancel = tcancel
		turnDone = done
		go func() {
			defer close(done)
			// MarkIntentDone fires only when an IntentRouter exists — the A4
			// SLA measures intent-classifier-return → first audio, and the
			// no-router path has no classifier-return event to anchor to.
			if l.cfg.IntentRouter != nil {
				handled, ierr := l.cfg.IntentRouter.Recognize(tctx, text)
				l.cfg.IntentToFirstAudio.MarkIntentDone(time.Now())
				if ierr == nil && handled {
					return
				}
			}
			reply, rerr := l.cfg.Reasoner.Ask(tctx, text)
			if rerr != nil {
				if errors.Is(tctx.Err(), context.Canceled) {
					return
				}
				errCtx, errCancel := context.WithTimeout(tctx, errAnnounceTimeout)
				l.cfg.IntentToFirstAudio.MarkFirstAudio(time.Now(), "error")
				_ = l.cfg.TTS.Speak(errCtx, errReasonFallback)
				errCancel()
				return
			}
			l.cfg.IntentToFirstAudio.MarkFirstAudio(time.Now(), "ok")
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
				return errors.New("voice/loop: Wake channel closed")
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
			cancelTurn()
			startTurn(text)
			armed = false
		}
	}
}

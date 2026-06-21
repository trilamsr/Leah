// Package session implements the voice-loop §9 state machine + barge-in cancel. See docs/engineer/specs/2026-06-10-voice-comm.md.
package session

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/contracts"
	"github.com/trilam/leah/internal/voice"
	"github.com/trilam/leah/internal/voice/listener"
)

// ErrAttestationDenied terminates Session.Run when the operator's first
// post-prompt utterance is not the attestation phrase.
var ErrAttestationDenied = errors.New("voice/session: attestation denied")

// Reasoner mirrors the dispatcher's reply path. Kept local so session does
// not back-import dispatcher (which would invert the layering).
type Reasoner interface {
	Ask(ctx context.Context, prompt string) (string, error)
}

// StreamReasoner is the W109 streaming surface. When the configured Reason
// satisfies it, Session takes the sentence-boundary streaming path; otherwise
// it falls back to the original Ask → Speak flow.
type StreamReasoner interface {
	Reasoner
	AskStream(ctx context.Context, prompt string) (<-chan string, error)
}

// TTSSpeaker is the speak-with-cancel surface session needs. Matches
// voice.TTS but kept anonymous here so this package stays import-light.
type TTSSpeaker interface {
	Speak(ctx context.Context, text string) error
}

// state is the voice-loop state. Spec §9 enumerates the transition table.
type state int

const (
	stateArmedWaitingAttest state = iota
	stateArmed
	stateSpeaking
	stateRequiresWake
)

const attestPhrase = "yes leah"

// Session owns the §9 state machine. Fields mirror spec §3.1; consumers
// inject doubles (fakeListener, fakeTTS) in tests and the real backends in
// cmd/leah/voice.go.
type Session struct {
	Listen    listener.Listener
	Speak     TTSSpeaker
	Reason    Reasoner
	Audit     *audit.Logger
	Attest    contracts.Attestor
	IdleAfter time.Duration
	// Now is injected so idle-timeout tests stay deterministic; production
	// uses time.Now.
	Now func() time.Time
	// Metrics, when non-nil, receives per-turn + per-stage timing observations.
	// nil-safe: the timer methods drop silently on a nil receiver.
	Metrics *voice.TurnInstrumentation
}

// Run drives the state machine until ctx is cancelled, the operator denies
// attestation, or the listener channel closes. It is the single goroutine
// owning state transitions — barge-in is realized by cancelling the per-turn
// reply context from this loop, not by a second arbiter.
func (s *Session) Run(ctx context.Context) error {
	if s.IdleAfter <= 0 {
		s.IdleAfter = 60 * time.Second
	}
	if s.Now == nil {
		s.Now = time.Now
	}

	segs, err := s.Listen.Start(ctx)
	if err != nil {
		return err
	}

	// Attestation prompt is the first TTS — we speak it BEFORE entering the
	// loop so the first Final segment is unambiguously the reply.
	_ = s.speakControl(ctx, "Voice session starting. Confirm out loud: yes Leah.")

	st := stateArmedWaitingAttest
	var (
		replyCancel context.CancelFunc
		replyWG     sync.WaitGroup
		turnTimer   *voice.TurnTimer
		// turnTimerMu pins reads/writes between the loop goroutine (which
		// nils the pointer on barge-in/cleanup) and the reply goroutine
		// (which reads it across the reasoner→tts boundary).
		turnTimerMu sync.Mutex
	)

	loadTimer := func() *voice.TurnTimer {
		turnTimerMu.Lock()
		defer turnTimerMu.Unlock()
		return turnTimer
	}

	cancelReply := func(bargedIn bool) {
		if replyCancel != nil {
			if bargedIn {
				loadTimer().BargeIn()
			}
			// A8: clock starts at the cancel signal, stops when the reply
			// goroutine returns — i.e. TTS Speak has actually unwound.
			// Sampling only for bargedIn isolates the user-felt span from
			// outer-ctx shutdown spans which aren't barge-in.
			cancelStart := s.Now()
			replyCancel()
			replyCancel = nil
			replyWG.Wait()
			if bargedIn {
				s.Metrics.RecordBargeInCancel("completed", s.Now().Sub(cancelStart))
			}
			return
		}
		replyWG.Wait()
	}

	idleTimer := time.NewTimer(s.IdleAfter)
	defer idleTimer.Stop()
	resetIdle := func() {
		if !idleTimer.Stop() {
			select {
			case <-idleTimer.C:
			default:
			}
		}
		idleTimer.Reset(s.IdleAfter)
	}

	startTurn := func(prompt string) {
		rctx, rcancel := context.WithCancel(ctx)
		replyCancel = rcancel
		tt := s.Metrics.NewTurnTimer(s.Now())
		turnTimerMu.Lock()
		turnTimer = tt
		turnTimerMu.Unlock()
		replyWG.Add(1)
		go func() {
			defer replyWG.Done()
			tt.MarkReasonerAsk(s.Now())
			if sr, ok := s.Reason.(StreamReasoner); ok {
				s.runStreamingTurn(rctx, tt, sr, prompt)
				return
			}
			reply, rerr := s.Reason.Ask(rctx, prompt)
			if rerr != nil {
				if errors.Is(rctx.Err(), context.Canceled) {
					tt.MarkReasonerDone(s.Now(), "barged_in")
					s.recordTurn("interrupted")
					tt.Finish(s.Now(), "barged_in")
					return
				}
				tt.MarkReasonerDone(s.Now(), "error")
				_ = s.Speak.Speak(rctx, "I couldn't reason that through, try again.")
				s.recordTurn("reasoner_error")
				tt.Finish(s.Now(), "error")
				return
			}
			tt.MarkReasonerDone(s.Now(), "completed")
			tt.MarkTTSFirstByte(s.Now(), "completed")
			if err := s.Speak.Speak(rctx, reply); err != nil {
				if errors.Is(rctx.Err(), context.Canceled) {
					tt.MarkTTSDone(s.Now(), "barged_in")
					s.recordTurn("interrupted")
					tt.Finish(s.Now(), "barged_in")
					return
				}
				tt.MarkTTSDone(s.Now(), "error")
				s.recordTurn("tts_error")
				tt.Finish(s.Now(), "error")
				return
			}
			tt.MarkTTSDone(s.Now(), "completed")
			s.recordTurn("success")
			tt.Finish(s.Now(), "completed")
		}()
		st = stateSpeaking
		resetIdle()
	}

	for {
		select {
		case <-ctx.Done():
			cancelReply(false)
			return nil
		case seg, ok := <-segs:
			if !ok {
				cancelReply(false)
				return nil
			}
			if !seg.Final {
				continue
			}
			text := strings.TrimSpace(seg.Text)
			switch st {
			case stateArmedWaitingAttest:
				if !strings.EqualFold(text, attestPhrase) {
					s.Metrics.RecordWakeEvent("cancelled_phrase")
					s.recordTurn("attestation_denied")
					return ErrAttestationDenied
				}
				if s.Attest != nil {
					if err := s.Attest.Attest(ctx, "voice_session"); err != nil {
						s.Metrics.RecordWakeEvent("cancelled_phrase")
						s.recordTurn("attestation_denied")
						return ErrAttestationDenied
					}
				}
				s.Metrics.RecordWakeEvent("armed")
				st = stateArmed
				resetIdle()
			case stateSpeaking:
				// Barge-in: cancel in-flight reply ctx, then arm the next
				// turn on the same Final segment (spec §4 step 5–6).
				cancelReply(true)
				startTurn(text)
			case stateArmed, stateRequiresWake:
				startTurn(text)
			}
		case <-idleTimer.C:
			switch st {
			case stateArmed:
				s.Metrics.RecordWakeEvent("cancelled_silence")
				_ = s.speakControl(ctx, "Going to sleep.")
				st = stateRequiresWake
				resetIdle()
			case stateRequiresWake:
				cancelReply(false)
				return nil
			default:
				// Reply in flight — extend the idle window.
				resetIdle()
			}
		}
	}
}

// runStreamingTurn drives the W109 sentence-boundary path. Reasoner deltas
// fill a SentenceChunker; each emitted chunk is spoken serially via the same
// Speak surface (audio device serialization is the backend's job — see
// voice/tts.go withAudioDevice). First-token timing is captured for the
// reasoner_first_token V1 stage. Stream-error or speak-error unwinds the
// timer + audit row identically to the non-streaming path.
func (s *Session) runStreamingTurn(rctx context.Context, tt *voice.TurnTimer, sr StreamReasoner, prompt string) {
	deltas, err := sr.AskStream(rctx, prompt)
	if err != nil {
		if errors.Is(rctx.Err(), context.Canceled) {
			tt.MarkReasonerDone(s.Now(), "barged_in")
			s.recordTurn("interrupted")
			tt.Finish(s.Now(), "barged_in")
			return
		}
		tt.MarkReasonerDone(s.Now(), "error")
		_ = s.Speak.Speak(rctx, "I couldn't reason that through, try again.")
		s.recordTurn("reasoner_error")
		tt.Finish(s.Now(), "error")
		return
	}

	chunker := &SentenceChunker{}
	firstTokenSeen := false
	firstSpeakSeen := false

	speak := func(text string) bool {
		if text == "" {
			return true
		}
		if !firstSpeakSeen {
			tt.MarkTTSFirstByte(s.Now(), "completed")
			firstSpeakSeen = true
		}
		if err := s.Speak.Speak(rctx, text); err != nil {
			if errors.Is(rctx.Err(), context.Canceled) {
				tt.MarkTTSDone(s.Now(), "barged_in")
				s.recordTurn("interrupted")
				tt.Finish(s.Now(), "barged_in")
				return false
			}
			tt.MarkTTSDone(s.Now(), "error")
			s.recordTurn("tts_error")
			tt.Finish(s.Now(), "error")
			return false
		}
		return true
	}

	for d := range deltas {
		if !firstTokenSeen {
			tt.MarkReasonerFirstToken(s.Now())
			firstTokenSeen = true
		}
		if chunk, ok := chunker.Push(d); ok {
			if !speak(chunk) {
				// Drain on Speak failure where rctx is NOT cancelled
				// (e.g. tts_error) — AskStream's goroutine already exits
				// on ctx-cancel via its own select-send.
				for range deltas {
				}
				return
			}
		}
	}

	// Stream ended — flush trailing.
	if errors.Is(rctx.Err(), context.Canceled) {
		tt.MarkReasonerDone(s.Now(), "barged_in")
		s.recordTurn("interrupted")
		tt.Finish(s.Now(), "barged_in")
		return
	}
	tt.MarkReasonerDone(s.Now(), "completed")
	if tail := chunker.Flush(); tail != "" {
		if !speak(tail) {
			return
		}
	}
	if !firstSpeakSeen {
		// Empty reply — record TTS first-byte anyway so the stage histogram
		// doesn't silently drop turns.
		tt.MarkTTSFirstByte(s.Now(), "completed")
	}
	tt.MarkTTSDone(s.Now(), "completed")
	s.recordTurn("success")
	tt.Finish(s.Now(), "completed")
}

// speakControl speaks a session-control phrase (attestation prompt, idle
// warning). Not subject to barge-in cancel — only the outer ctx ends it.
func (s *Session) speakControl(ctx context.Context, text string) error {
	if s.Speak == nil {
		return nil
	}
	return s.Speak.Speak(ctx, text)
}

// recordTurn writes one audit row per turn (spec §6). Detail is the
// suppressed-transcript form per the default; toggle wiring lives in W14.
func (s *Session) recordTurn(outcome string) {
	if s.Audit == nil {
		return
	}
	_ = s.Audit.Append(audit.Entry{
		Kind:    "voice_turn",
		Outcome: outcome,
		Detail:  "[suppressed]",
	})
}

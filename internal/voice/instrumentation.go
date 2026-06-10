package voice

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/trilam/leah/internal/obs"
)

func Bind(c *ChainTTS, registry *obs.Registry) {
	if registry == nil || c == nil {
		return
	}
	cnt := registry.Counter("leah_voice_speak_total")
	c.OnSpeak = func(backend string) {
		cnt.Inc(map[string]string{"backend": backend})
	}
}

func EmitSpeak(registry *obs.Registry, backend string) {
	if registry == nil {
		return
	}
	registry.Counter("leah_voice_speak_total").Inc(map[string]string{"backend": backend})
}

type SelfChecker struct{ Chain *ChainTTS }

func (c *SelfChecker) SelfCheck(ctx context.Context) error {
	_ = ctx
	if c == nil || c.Chain == nil {
		return nil
	}
	if !c.Chain.Available() {
		return errors.New("voice: no backends available")
	}
	return nil
}

func backendName(t TTS) string {
	switch t.(type) {
	case *KokoroTTS:
		return "kokoro"
	case *OpenAITTS:
		return "openai"
	case *SayTTS:
		return "say"
	default:
		return "unknown"
	}
}

// voiceLatencyBuckets — voice-comm.md §5: bands span 50ms-5s so the
// wake→first-audio 1.5s target falls mid-distribution.
var voiceLatencyBuckets = []float64{0.05, 0.1, 0.2, 0.5, 1, 2, 5}

// TurnInstrumentation is the voice-turn metric facade. Path is the model-locality
// label ("local" | "cloud") on every turn + stage observation — V10 streaming
// changes cloud latency, not local, so baselines must be partitioned.
type TurnInstrumentation struct {
	turnSeconds  *obs.Histogram
	stageSeconds *obs.Histogram
	turnTotal    *obs.Counter
	bargeInTotal *obs.Counter
	wakeEvent    *obs.Counter
	path         string
}

// NewTurnInstrumentation registers the V1 voice-path series. nil-safe: a nil
// registry yields a nil receiver whose methods are no-ops, so production wiring
// can omit the registry without per-call guards.
func NewTurnInstrumentation(reg *obs.Registry, path string) *TurnInstrumentation {
	if reg == nil {
		return nil
	}
	if path == "" {
		path = "local"
	}
	return &TurnInstrumentation{
		turnSeconds:  reg.Histogram("leah_voice_turn_seconds", voiceLatencyBuckets),
		stageSeconds: reg.Histogram("leah_voice_stage_seconds", voiceLatencyBuckets),
		turnTotal:    reg.Counter("leah_voice_turn_total"),
		bargeInTotal: reg.Counter("leah_voice_barge_in_total"),
		wakeEvent:    reg.Counter("leah_voice_wake_event_total"),
		path:         path,
	}
}

// RecordTurn emits one turn-level observation. outcome ∈ {"completed",
// "barged_in", "error"}.
func (t *TurnInstrumentation) RecordTurn(outcome string, dur time.Duration) {
	if t == nil {
		return
	}
	t.turnSeconds.Observe(map[string]string{"outcome": outcome, "path": t.path}, dur.Seconds())
	t.turnTotal.Inc(map[string]string{"outcome": outcome})
}

// RecordStage emits one per-stage observation. stage ∈ {"wake", "stt",
// "reasoner", "tts_first_byte", "tts_total"}.
func (t *TurnInstrumentation) RecordStage(stage, outcome string, dur time.Duration) {
	if t == nil {
		return
	}
	t.stageSeconds.Observe(map[string]string{"stage": stage, "outcome": outcome}, dur.Seconds())
}

// RecordBargeIn flags the pipeline phase the operator interrupted. stage ∈
// {"stt", "reasoner", "tts"} — coarser than the stage histogram because
// barge-in is meaningful per-phase, not per-sub-stage.
func (t *TurnInstrumentation) RecordBargeIn(stage string) {
	if t == nil {
		return
	}
	t.bargeInTotal.Inc(map[string]string{"stage": stage})
}

// RecordWakeEvent counts wake-phrase outcomes. result ∈ {"armed",
// "cancelled_silence", "cancelled_phrase"}.
func (t *TurnInstrumentation) RecordWakeEvent(result string) {
	if t == nil {
		return
	}
	t.wakeEvent.Inc(map[string]string{"result": result})
}

// TurnTimer tracks per-turn timestamps so session.go calls one struct rather
// than threading six time.Time values through closures.
//
// All write methods (MarkReasonerAsk/Done, MarkTTS*, Finish) are owned by
// the per-turn reply goroutine — they are NOT goroutine-safe across writers.
// The barge-in surface (BargeIn) is the only method called from a different
// goroutine (the loop), and reads only the atomic stage handle so no race
// occurs with concurrent Mark* writes from the reply goroutine.
type TurnTimer struct {
	instr          *TurnInstrumentation
	turnStart      time.Time
	turnEnd        time.Time
	reasonerAskAt        time.Time
	reasonerFirstTokenAt time.Time
	reasonerDoneAt       time.Time
	ttsFirstByteAt       time.Time
	// stage is an atomic.Pointer because BargeIn reads it from the loop
	// goroutine while Mark* writes from the reply goroutine. Atomic swap is
	// cheaper than a mutex on a 1-byte-string-equivalent classification.
	stage atomic.Pointer[string]
}

// NewTurnTimer starts a turn at the Final-segment timestamp.
func (t *TurnInstrumentation) NewTurnTimer(finalAt time.Time) *TurnTimer {
	if t == nil {
		return nil
	}
	tt := &TurnTimer{instr: t, turnStart: finalAt}
	stt := "stt"
	tt.stage.Store(&stt)
	return tt
}

func (tt *TurnTimer) MarkReasonerAsk(at time.Time) {
	if tt == nil {
		return
	}
	tt.reasonerAskAt = at
	s := "reasoner"
	tt.stage.Store(&s)
}

// MarkReasonerFirstToken records the reasoner_first_token stage (W109/V10):
// reasoner-ask → first text delta. The stage histogram targets ≤300ms p95
// per the voice-frontier spec §2.5. No-op if MarkReasonerAsk hasn't fired
// or first-token has already been recorded on this turn.
func (tt *TurnTimer) MarkReasonerFirstToken(at time.Time) {
	if tt == nil || tt.reasonerAskAt.IsZero() || !tt.reasonerFirstTokenAt.IsZero() {
		return
	}
	tt.reasonerFirstTokenAt = at
	tt.instr.RecordStage("reasoner_first_token", "completed", at.Sub(tt.reasonerAskAt))
}

func (tt *TurnTimer) MarkReasonerDone(at time.Time, outcome string) {
	if tt == nil || tt.reasonerAskAt.IsZero() {
		return
	}
	tt.reasonerDoneAt = at
	tt.instr.RecordStage("reasoner", outcome, at.Sub(tt.reasonerAskAt))
}

// MarkTTSFirstByte records reasoner-done → speak-invoke as the
// tts_first_byte stage. This is the seam V10 will swap when true streaming TTS
// lands.
func (tt *TurnTimer) MarkTTSFirstByte(at time.Time, outcome string) {
	if tt == nil {
		return
	}
	tt.ttsFirstByteAt = at
	s := "tts"
	tt.stage.Store(&s)
	if !tt.reasonerDoneAt.IsZero() {
		tt.instr.RecordStage("tts_first_byte", outcome, at.Sub(tt.reasonerDoneAt))
	}
}

func (tt *TurnTimer) MarkTTSDone(at time.Time, outcome string) {
	if tt == nil || tt.ttsFirstByteAt.IsZero() {
		return
	}
	tt.instr.RecordStage("tts_total", outcome, at.Sub(tt.ttsFirstByteAt))
}

// Finish records the turn-level histogram + counter. Idempotent — only the
// first call wins so barge-in followed by reply-cleanup don't double-count.
func (tt *TurnTimer) Finish(at time.Time, outcome string) {
	if tt == nil || !tt.turnEnd.IsZero() {
		return
	}
	tt.turnEnd = at
	tt.instr.RecordTurn(outcome, at.Sub(tt.turnStart))
}

// BargeIn classifies the in-flight stage and increments the barge-in counter.
// Caller invokes this BEFORE cancelling the reply context so stage still
// reflects the cancelled phase.
func (tt *TurnTimer) BargeIn() {
	if tt == nil {
		return
	}
	stage := tt.stage.Load()
	if stage == nil {
		return
	}
	tt.instr.RecordBargeIn(*stage)
}

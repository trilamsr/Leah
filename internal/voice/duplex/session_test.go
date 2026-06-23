package duplex

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/trilam/leah/internal/tts"
	"github.com/trilam/leah/internal/voice/stt"
)

func TestSession_EndEmitsTerminal(t *testing.T) {
	s := NewSession(fakeSTT{}, fakeTTS{}, fakeAsk, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events, err := s.Start(ctx, DuplexOpts{MaxTurn: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(5 * time.Millisecond)
		s.End()
	}()
	deadline := time.After(500 * time.Millisecond)
	var sawTerminal bool
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				if !sawTerminal {
					t.Fatal("event channel closed without TTSEnd")
				}
				return
			}
			if ev.Kind == TTSEnd || ev.Kind == ErrorEvent {
				sawTerminal = true
			}
		case <-deadline:
			t.Fatal("session.End never produced terminal event")
		}
	}
}

func TestSession_StartRejectsNilDeps(t *testing.T) {
	s := NewSession(nil, fakeTTS{}, fakeAsk, nil)
	_, err := s.Start(context.Background(), DuplexOpts{MaxTurn: time.Second})
	if err == nil {
		t.Fatal("expected error for nil STT")
	}
}

func TestSession_InterruptDuringTTSEmitsBargeIn(t *testing.T) {
	s := NewSession(fakeSTT{}, fakeTTS{}, fakeAsk, nil).(*session)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events, err := s.Start(ctx, DuplexOpts{MaxTurn: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	s.arb.ttsStarted()
	s.Interrupt()
	deadline := time.After(150 * time.Millisecond)
	for {
		select {
		case ev := <-events:
			if ev.Kind == BargeIn {
				s.End()
				return
			}
		case <-deadline:
			t.Fatal("Interrupt during TTS did not emit BargeIn event")
		}
	}
}

func TestSession_TurnTimeoutEmitsTTSEnd(t *testing.T) {
	s := NewSession(fakeSTT{}, fakeTTS{}, fakeAsk, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events, err := s.Start(ctx, DuplexOpts{MaxTurn: 30 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case ev, ok := <-events:
		if !ok {
			return
		}
		if ev.Kind != TTSEnd && ev.Kind != ErrorEvent {
			for next := range events {
				if next.Kind == TTSEnd || next.Kind == ErrorEvent {
					return
				}
			}
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("MaxTurn deadline did not fire")
	}
}

type recordingCharger struct {
	buckets map[Bucket]int64
}

func (r *recordingCharger) Charge(b Bucket, n int64) error {
	if r.buckets == nil {
		r.buckets = map[Bucket]int64{}
	}
	r.buckets[b] += n
	return nil
}

func TestSession_ChargerInjectionOptional(t *testing.T) {
	rc := &recordingCharger{}
	s := NewSession(fakeSTT{}, fakeTTS{}, fakeAsk, rc)
	if s == nil {
		t.Fatal("nil session with charger")
	}
}

type fakeSTT struct{}

func (fakeSTT) Stream(_ context.Context, _ <-chan stt.AudioFrame) (<-chan stt.Partial, error) {
	c := make(chan stt.Partial)
	close(c)
	return c, nil
}
func (fakeSTT) Info() stt.ProviderInfo { return stt.ProviderInfo{Name: "fake"} }

type fakeTTS struct{}
type fakeStream struct{ r io.Reader }

func (f fakeStream) Read(p []byte) (int, error) { return f.r.Read(p) }
func (fakeStream) Close() error                 { return nil }
func (fakeStream) MIME() string                 { return "audio/mpeg" }

func (fakeTTS) Name() string { return "fake" }
func (fakeTTS) Speak(_ context.Context, _ string, _ string) (tts.AudioStream, error) {
	return fakeStream{r: strings.NewReader("")}, nil
}
func (fakeTTS) PreWarm(_ context.Context) error { return nil }

func fakeAsk(_ context.Context, _ string) (<-chan string, error) {
	c := make(chan string)
	close(c)
	return c, nil
}

package listener_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/trilam/leah/internal/input/voice/listener"
)

// TestFakeListener_StartStopCycle: enqueued segments arrive in order before ctx cancel closes the channel.
func TestFakeListener_StartStopCycle(t *testing.T) {
	t.Parallel()
	fl := listener.NewFake()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := fl.Start(ctx)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	fl.Emit(listener.Segment{Text: "hello", Final: false})
	fl.Emit(listener.Segment{Text: "hello world", Final: false})
	fl.Emit(listener.Segment{Text: "hello world done", Final: true})

	got := make([]listener.Segment, 0, 3)
	for i := 0; i < 3; i++ {
		select {
		case s := <-ch:
			got = append(got, s)
		case <-time.After(time.Second):
			t.Fatalf("timeout waiting for segment %d", i)
		}
	}

	if got[0].Text != "hello" || got[0].Final {
		t.Errorf("segment[0] = %+v", got[0])
	}
	if got[2].Text != "hello world done" || !got[2].Final {
		t.Errorf("segment[2] = %+v", got[2])
	}

	cancel()
	select {
	case _, ok := <-ch:
		if ok {
			// drain any in-flight segment, then channel must close
			select {
			case _, ok2 := <-ch:
				if ok2 {
					t.Fatalf("channel still open after cancel")
				}
			case <-time.After(time.Second):
				t.Fatalf("timeout waiting for channel close after cancel")
			}
		}
	case <-time.After(time.Second):
		t.Fatalf("timeout waiting for channel close after cancel")
	}
}

// TestFakeListener_FinalTranscript: a Final=true segment carries the full transcript.
func TestFakeListener_FinalTranscript(t *testing.T) {
	t.Parallel()
	fl := listener.NewFake()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := fl.Start(ctx)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	start := time.Now()
	fl.Emit(listener.Segment{
		Text:       "yes leah",
		Final:      true,
		StartedAt:  start,
		FinishedAt: start.Add(400 * time.Millisecond),
	})

	select {
	case s := <-ch:
		if !s.Final {
			t.Errorf("expected Final=true, got %+v", s)
		}
		if s.Text != "yes leah" {
			t.Errorf("Text = %q, want %q", s.Text, "yes leah")
		}
		if s.FinishedAt.Sub(s.StartedAt) != 400*time.Millisecond {
			t.Errorf("duration = %v, want 400ms", s.FinishedAt.Sub(s.StartedAt))
		}
	case <-time.After(time.Second):
		t.Fatalf("timeout waiting for final segment")
	}
}

// TestFakeListener_ConcurrentSafe: 10 emitters race against one reader under -race.
func TestFakeListener_ConcurrentSafe(t *testing.T) {
	t.Parallel()
	fl := listener.NewFake()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := fl.Start(ctx)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	const emitters = 10
	const perEmitter = 20
	var wg sync.WaitGroup
	wg.Add(emitters)
	for i := 0; i < emitters; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < perEmitter; j++ {
				fl.Emit(listener.Segment{Text: "x", Final: j == perEmitter-1})
			}
		}()
	}

	want := emitters * perEmitter
	got := 0
	deadline := time.After(5 * time.Second)
	for got < want {
		select {
		case <-ch:
			got++
		case <-deadline:
			t.Fatalf("received %d/%d segments before deadline", got, want)
		}
	}
	wg.Wait()
}

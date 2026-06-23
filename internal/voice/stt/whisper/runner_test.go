package whisper

import (
	"context"
	"testing"
	"time"

	"github.com/trilam/leah/internal/voice/stt"
)

func TestRunner_InfoReportsLocal(t *testing.T) {
	r, err := NewRunner(t.TempDir())
	if err == nil {
		defer func() { _ = r.Close() }()
		info := r.Info()
		if !info.IsLocal {
			t.Fatal("Whisper runner must report IsLocal=true")
		}
		if info.ModelID != "whisper-large-v3" {
			t.Fatalf("ModelID: want whisper-large-v3, got %q", info.ModelID)
		}
	}
}

func TestRunner_StreamCancelsOnContextDone(t *testing.T) {
	r, err := NewRunner(t.TempDir())
	if err != nil {
		t.Skip("ONNX model not available in test env; runtime-gated test")
	}
	defer func() { _ = r.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	audio := make(chan stt.AudioFrame)
	partials, err := r.Stream(ctx, audio)
	if err != nil {
		t.Fatal(err)
	}
	<-ctx.Done()
	for range partials {
	}
}

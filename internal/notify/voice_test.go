package notify

import (
	"context"
	"strings"
	"testing"
)

type fakeTTS struct {
	spoken []string
}

func (f *fakeTTS) Speak(_ context.Context, text string) error {
	f.spoken = append(f.spoken, text)
	return nil
}

// TestVoiceNotifyFormatsTitleBody asserts "<title>: <body>" reaches TTS.
func TestVoiceNotifyFormatsTitleBody(t *testing.T) {
	f := &fakeTTS{}
	v := &VoiceNotify{TTS: f}
	if err := v.Notify(context.Background(), "Leah", "PR #1112 merged"); err != nil {
		t.Fatalf("notify: %v", err)
	}
	if len(f.spoken) != 1 {
		t.Fatalf("want 1 utterance, got %d", len(f.spoken))
	}
	if !strings.Contains(f.spoken[0], "Leah: PR #1112 merged") {
		t.Errorf("formatted: %q", f.spoken[0])
	}
}

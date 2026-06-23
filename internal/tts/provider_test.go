package tts_test

import (
	"context"
	"io"
	"testing"

	"github.com/trilam/leah/internal/tts"
)

type stubProvider struct{ name string }

func (s *stubProvider) Name() string                    { return s.name }
func (s *stubProvider) PreWarm(_ context.Context) error { return nil }
func (s *stubProvider) Speak(_ context.Context, _, _ string) (tts.AudioStream, error) {
	return &stubStream{}, nil
}

type stubStream struct{}

func (s *stubStream) Read(p []byte) (int, error) { return 0, io.EOF }
func (s *stubStream) Close() error               { return nil }
func (s *stubStream) MIME() string               { return "audio/mpeg" }

// Provider contract: Name + Speak return a drainable AudioStream with MIME.
func TestProvider_StubSpeak(t *testing.T) {
	p := &stubProvider{name: "stub"}
	if p.Name() != "stub" {
		t.Fatalf("name: %q", p.Name())
	}
	stream, err := p.Speak(context.Background(), "hello", tts.DefaultVoice)
	if err != nil {
		t.Fatalf("speak: %v", err)
	}
	defer func() { _ = stream.Close() }()
	if stream.MIME() != "audio/mpeg" {
		t.Fatalf("mime: %q", stream.MIME())
	}
}

// DefaultVoice pins the §2.7 voice-canon id.
func TestDefaultVoice_Canon(t *testing.T) {
	if tts.DefaultVoice != "ava-alto-145wpm" {
		t.Fatalf("DefaultVoice drifted: %q", tts.DefaultVoice)
	}
}

// RouteCloud is the zero value — unset routing defaults to cloud.
func TestRoute_ZeroValueIsCloud(t *testing.T) {
	var r tts.Route
	if r != tts.RouteCloud {
		t.Fatalf("zero Route must be RouteCloud, got %v", r)
	}
	if tts.RouteCloud == tts.RouteLocal {
		t.Fatalf("RouteCloud and RouteLocal must differ")
	}
}

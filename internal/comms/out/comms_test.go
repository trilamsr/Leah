package out

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/trilam/leah/internal/contracts"
	"github.com/trilam/leah/internal/voice"
)

type fakePoster struct {
	channel  string
	body     string
	audio    []byte
	postErr  error
	voiceErr error
}

func (f *fakePoster) PostMessage(_ context.Context, channelID, body string) error {
	f.channel, f.body = channelID, body
	return f.postErr
}

func (f *fakePoster) PostVoice(_ context.Context, channelID string, audio []byte) error {
	f.channel, f.audio = channelID, audio
	return f.voiceErr
}

type fakeSender struct {
	to      string
	body    string
	sendErr error
}

func (f *fakeSender) SendText(_ context.Context, to, body string) error {
	f.to, f.body = to, body
	return f.sendErr
}

type fakeSynth struct {
	audio []byte
	err   error
}

func (f *fakeSynth) Synthesize(_ context.Context, _ string) ([]byte, string, error) {
	return f.audio, "audio/ogg", f.err
}

func TestDiscordNotifyPostsFormattedText(t *testing.T) {
	p := &fakePoster{}
	d := &DiscordNotify{Poster: p, ChannelID: "C1"}
	if err := d.Notify(context.Background(), "Morning brief", "2 PRs merged"); err != nil {
		t.Fatalf("notify: %v", err)
	}
	if p.channel != "C1" {
		t.Errorf("channel: %q", p.channel)
	}
	if p.body != "Morning brief: 2 PRs merged" {
		t.Errorf("body: %q", p.body)
	}
}

func TestDiscordNotifyWrapsPosterError(t *testing.T) {
	p := &fakePoster{postErr: errors.New("rate limited")}
	d := &DiscordNotify{Poster: p, ChannelID: "C1"}
	err := d.Notify(context.Background(), "T", "B")
	if err == nil || !strings.Contains(err.Error(), "discord notify") {
		t.Errorf("want wrapped discord error, got %v", err)
	}
}

func TestDiscordNotifyVoiceWhenSynthSet(t *testing.T) {
	p := &fakePoster{}
	d := &DiscordNotify{Poster: p, ChannelID: "C1", Synth: &fakeSynth{audio: []byte("ogg")}}
	if err := d.Notify(context.Background(), "T", "B"); err != nil {
		t.Fatalf("notify: %v", err)
	}
	if string(p.audio) != "ogg" {
		t.Errorf("want voice audio posted, got %q", p.audio)
	}
}

func TestDiscordNotifyVoiceFallsBackToTextOnUnsupported(t *testing.T) {
	p := &fakePoster{}
	d := &DiscordNotify{Poster: p, ChannelID: "C1", Synth: &fakeSynth{err: voice.ErrSynthesizeUnsupported}}
	if err := d.Notify(context.Background(), "T", "B"); err != nil {
		t.Fatalf("notify: %v", err)
	}
	if p.body != "T: B" {
		t.Errorf("want text fallback, got body=%q audio=%q", p.body, p.audio)
	}
}

func TestWhatsAppNotifySendsFormattedText(t *testing.T) {
	s := &fakeSender{}
	w := &WhatsAppNotify{Sender: s, To: "+15551234"}
	if err := w.Notify(context.Background(), "Morning brief", "all clear"); err != nil {
		t.Fatalf("notify: %v", err)
	}
	if s.to != "+15551234" {
		t.Errorf("to: %q", s.to)
	}
	if s.body != "Morning brief: all clear" {
		t.Errorf("body: %q", s.body)
	}
}

func TestWhatsAppNotifyWrapsSenderError(t *testing.T) {
	s := &fakeSender{sendErr: errors.New("not on allowlist")}
	w := &WhatsAppNotify{Sender: s, To: "+1"}
	err := w.Notify(context.Background(), "T", "B")
	if err == nil || !strings.Contains(err.Error(), "whatsapp notify") {
		t.Errorf("want wrapped whatsapp error, got %v", err)
	}
}

var (
	_ contracts.Notifier = (*DiscordNotify)(nil)
	_ contracts.Notifier = (*WhatsAppNotify)(nil)
)

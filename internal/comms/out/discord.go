package out

import (
	"context"
	"errors"
	"fmt"

	"github.com/trilam/leah/internal/voice"
)

type DiscordPoster interface {
	PostMessage(ctx context.Context, channelID, body string) error
	PostVoice(ctx context.Context, channelID string, audio []byte) error
}

// DiscordNotify posts the brief to a channel; a non-nil Synth speaks it.
type DiscordNotify struct {
	Poster    DiscordPoster
	ChannelID string
	Synth     voice.Synthesizer
}

func (d *DiscordNotify) Notify(ctx context.Context, title, body string) error {
	text := fmt.Sprintf("%s: %s", title, body)
	if d.Synth != nil {
		audio, _, err := d.Synth.Synthesize(ctx, text)
		if err == nil {
			if vErr := d.Poster.PostVoice(ctx, d.ChannelID, audio); vErr != nil {
				return fmt.Errorf("discord notify: %w", vErr)
			}
			return nil
		}
		if !errors.Is(err, voice.ErrSynthesizeUnsupported) {
			return fmt.Errorf("discord notify: %w", err)
		}
	}
	if err := d.Poster.PostMessage(ctx, d.ChannelID, text); err != nil {
		return fmt.Errorf("discord notify: %w", err)
	}
	return nil
}

package notify

import (
	"context"
	"fmt"
)

type WhatsAppSender interface {
	SendText(ctx context.Context, to, body string) error
}

// WhatsAppNotify sends the brief as text; the adapter has no voice-out.
type WhatsAppNotify struct {
	Sender WhatsAppSender
	To     string
}

func (w *WhatsAppNotify) Notify(ctx context.Context, title, body string) error {
	if err := w.Sender.SendText(ctx, w.To, fmt.Sprintf("%s: %s", title, body)); err != nil {
		return fmt.Errorf("whatsapp notify: %w", err)
	}
	return nil
}

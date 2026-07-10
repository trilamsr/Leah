package reasoner

import (
	"context"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// VerifyKey performs a 1-token Anthropic Messages call with the supplied key
// and returns nil iff the key is accepted. Used by the daemon IPC verify-key
// path — the wizard hands a freshly-entered key here BEFORE persisting it.
// Constructs a transient SDK client so the daemon's process-wide
// AnthropicClient (bound to the previously-set ANTHROPIC_API_KEY) is not
// involved — otherwise the ping would silently validate the wrong key.
func VerifyKey(ctx context.Context, key string) error {
	if key == "" {
		return fmt.Errorf("verify key: empty key")
	}
	c := anthropic.NewClient(option.WithAPIKey(key))
	_, err := c.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(defaultModel),
		MaxTokens: 1,
		Messages:  []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock("1"))},
	})
	if err != nil {
		return fmt.Errorf("verify key: %w", err)
	}
	return nil
}

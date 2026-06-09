//go:build integration

package reasoner

import (
	"context"
	"os"
	"testing"
)

func TestAnthropicClientReturnsText(t *testing.T) {
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		t.Skip("ANTHROPIC_API_KEY not set")
	}
	c, err := NewAnthropicClient()
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	text, cost, err := c.Complete(context.Background(),
		"You are a calculator. Reply with just the number.",
		"What is 2+2?")
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if text == "" {
		t.Error("empty response")
	}
	if cost <= 0 {
		t.Errorf("cost should be > 0, got %v", cost)
	}
	t.Logf("response: %q  cost: $%.6f", text, cost)
}

//go:build integration

package reviewer

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestAnthropicSubagentReturnsCanonicalID(t *testing.T) {
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		t.Skip("ANTHROPIC_API_KEY not set")
	}
	sa, err := NewAnthropicSubagent()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	prompt := `You are a reviewer. Reply EXACTLY with:

Summary: ok

Reviewer-recommendation: APPROVE
Reviewer-agent-id: cavecrew-reviewer-test-2026-06-09
`
	resp, err := sa.Run(context.Background(), prompt, "trivial diff")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(resp, "cavecrew-reviewer-test-2026-06-09") {
		t.Errorf("response missing id: %q", resp)
	}
}

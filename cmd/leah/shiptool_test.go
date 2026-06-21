package main

import (
	"context"
	"testing"
)

func TestRunShipTool_ConfluenceNoWriteSurface(t *testing.T) {
	t.Setenv("LEAH_STATE_DIR", t.TempDir())
	if code := runShipTool(context.Background(), "confluence", "anything"); code != 2 {
		t.Fatalf("confluence write: exit=%d, want 2 (no write surface)", code)
	}
}

func TestNewToolWriter_RealRPCBindings(t *testing.T) {
	t.Setenv("LEAH_SHIP_JIRA_BASE_URL", "https://acme.atlassian.net")
	for _, tool := range []string{"slack", "jira", "notion", "linear", "teams"} {
		if _, err := newToolWriter(tool); err != nil {
			t.Errorf("newToolWriter(%q): %v", tool, err)
		}
	}
	if _, err := newToolWriter("confluence"); err == nil {
		t.Error("newToolWriter(confluence): want error, got nil")
	}
}

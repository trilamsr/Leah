package reasoner

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/trilam/leah/internal/budget"
	"github.com/trilam/leah/internal/obs"
)

type fakeClient struct {
	lastPrompt  string
	respText    string
	respCostUSD float64
	respErr     error
}

func (f *fakeClient) Complete(ctx context.Context, system, user string) (text string, costUSD float64, err error) {
	f.lastPrompt = system + "\n---\n" + user
	return f.respText, f.respCostUSD, f.respErr
}

func TestAskCallsClientWithSystemAndUser(t *testing.T) {
	c := &fakeClient{respText: "hello tri", respCostUSD: 0.012}
	b := &budget.Budget{Ceiling: 5.0}
	r := &Reasoner{Client: c, Budget: b, SystemPrompt: "you are leah"}

	out, err := r.Ask(context.Background(), "say hi")
	if err != nil {
		t.Fatalf("ask: %v", err)
	}
	if out != "hello tri" {
		t.Errorf("got %q", out)
	}
	if !strings.Contains(c.lastPrompt, "you are leah") {
		t.Errorf("missing system: %q", c.lastPrompt)
	}
	if !strings.Contains(c.lastPrompt, "say hi") {
		t.Errorf("missing user: %q", c.lastPrompt)
	}
	if b.Spent() != 0.012 {
		t.Errorf("budget not charged: %v", b.Spent())
	}
}

// TestAskEmitsObsLogOnSuccess asserts the reasoner emits the
// reasoner.call.complete INFO event with duration_ms + cost_dollars
// attrs (Wave2-K obs instrumentation contract).
func TestAskEmitsObsLogOnSuccess(t *testing.T) {
	var buf bytes.Buffer
	lg := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := obs.WithLogger(context.Background(), lg)

	c := &fakeClient{respText: "ok", respCostUSD: 0.005}
	r := &Reasoner{Client: c, Budget: &budget.Budget{Ceiling: 1.0}, SystemPrompt: "x"}
	if _, err := r.Ask(ctx, "do"); err != nil {
		t.Fatalf("ask: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, `"msg":"reasoner.call.complete"`) {
		t.Errorf("missing complete event: %s", out)
	}
	if !strings.Contains(out, `"duration_ms"`) {
		t.Errorf("missing duration_ms attr: %s", out)
	}
	if !strings.Contains(out, `"cost_dollars":0.005`) {
		t.Errorf("missing cost_dollars attr: %s", out)
	}
}

// TestAskPrependsPersonaPrefix asserts a non-empty PersonaPrefix is woven
// into the system prompt before SystemPrompt, separated by a blank line.
// Empty prefix produces the legacy behavior — verified by the existing
// TestAskCallsClientWithSystemAndUser test (PersonaPrefix unset).
func TestAskPrependsPersonaPrefix(t *testing.T) {
	c := &fakeClient{respText: "ok", respCostUSD: 0.001}
	r := &Reasoner{
		Client:        c,
		Budget:        &budget.Budget{Ceiling: 1.0},
		SystemPrompt:  "you are leah",
		PersonaPrefix: "Workspace: acme. Tone: formal.",
	}
	if _, err := r.Ask(context.Background(), "hi"); err != nil {
		t.Fatalf("ask: %v", err)
	}
	if !strings.Contains(c.lastPrompt, "Workspace: acme. Tone: formal.") {
		t.Errorf("persona prefix missing from prompt: %q", c.lastPrompt)
	}
	if !strings.Contains(c.lastPrompt, "you are leah") {
		t.Errorf("system prompt missing: %q", c.lastPrompt)
	}
	// PersonaPrefix must appear BEFORE the base system prompt so the
	// workspace framing dominates.
	pi := strings.Index(c.lastPrompt, "Workspace: acme")
	si := strings.Index(c.lastPrompt, "you are leah")
	if pi < 0 || si < 0 || pi >= si {
		t.Errorf("persona must precede system: persona@%d system@%d", pi, si)
	}
}

// TestAskEmptyPersonaPrefixUnchanged asserts an empty PersonaPrefix
// produces the legacy system-prompt-only behavior.
func TestAskEmptyPersonaPrefixUnchanged(t *testing.T) {
	c := &fakeClient{respText: "ok"}
	r := &Reasoner{Client: c, Budget: &budget.Budget{Ceiling: 1.0}, SystemPrompt: "base"}
	if _, err := r.Ask(context.Background(), "x"); err != nil {
		t.Fatalf("ask: %v", err)
	}
	// Sentinel: the assembled system field equals base — no extra leading
	// whitespace or newline that an unguarded join would introduce.
	if !strings.HasPrefix(c.lastPrompt, "base") {
		t.Errorf("empty persona must not alter system prompt prefix: %q", c.lastPrompt)
	}
}

func TestAskBlocksOnBudgetExceeded(t *testing.T) {
	c := &fakeClient{respText: "won't matter", respCostUSD: 10.0}
	b := &budget.Budget{Ceiling: 1.0}
	r := &Reasoner{Client: c, Budget: b, SystemPrompt: "x"}

	_, err := r.Ask(context.Background(), "anything")
	if err == nil {
		t.Fatalf("want budget exceeded error, got nil")
	}
	if !strings.Contains(err.Error(), "budget exceeded") {
		t.Errorf("unexpected error: %v", err)
	}
}

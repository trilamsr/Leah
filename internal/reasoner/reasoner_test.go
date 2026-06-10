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
	// Optional LLM-dim fields surfaced through CompleteResult (W94).
	respModel  string
	respIn     int
	respOut    int
	respEgress int
	respCache  bool
}

func (f *fakeClient) Complete(ctx context.Context, system, user string) (CompleteResult, error) {
	f.lastPrompt = system + "\n---\n" + user
	return CompleteResult{
		Text:         f.respText,
		CostUSD:      f.respCostUSD,
		Model:        f.respModel,
		InputTokens:  f.respIn,
		OutputTokens: f.respOut,
		EgressBytes:  f.respEgress,
		CacheHit:     f.respCache,
	}, f.respErr
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

// TestReasoner_WritesLLMDimFields asserts Ask captures the W94 LLM-dim
// data (model, prompt_sha, input/output tokens, latency, egress, cache
// hit) into LastCallInfo so dispatcher.Ask.Run can stamp it on the
// audit row.
func TestReasoner_WritesLLMDimFields(t *testing.T) {
	c := &fakeClient{
		respText:    "hi",
		respCostUSD: 0.01,
		respModel:   "claude-sonnet-4-6",
		respIn:      42,
		respOut:     7,
		respEgress:  256,
		respCache:   true,
	}
	r := &Reasoner{Client: c, Budget: &budget.Budget{Ceiling: 1.0}, SystemPrompt: "you are leah"}
	if _, err := r.Ask(context.Background(), "say hi"); err != nil {
		t.Fatalf("ask: %v", err)
	}
	info := r.LastCallInfo()
	if info.Model != "claude-sonnet-4-6" {
		t.Errorf("Model: got %q", info.Model)
	}
	if info.InputTokens != 42 {
		t.Errorf("InputTokens: got %d", info.InputTokens)
	}
	if info.OutputTokens != 7 {
		t.Errorf("OutputTokens: got %d", info.OutputTokens)
	}
	if info.EgressBytes != 256 {
		t.Errorf("EgressBytes: got %d", info.EgressBytes)
	}
	if !info.CacheHit {
		t.Errorf("CacheHit: got false")
	}
	if info.LatencyMS < 0 {
		t.Errorf("LatencyMS negative: %d", info.LatencyMS)
	}
	// PromptSHA = 16 hex chars derived from the assembled system prompt.
	wantSHA := PromptSHA("you are leah")
	if info.PromptSHA != wantSHA {
		t.Errorf("PromptSHA: got %q want %q", info.PromptSHA, wantSHA)
	}
	if len(info.PromptSHA) != 16 {
		t.Errorf("PromptSHA length %d, want 16", len(info.PromptSHA))
	}
}

// TestPromptSHA_Stable asserts PromptSHA(b) returns a stable 16-hex
// truncated SHA256 — same input bytes → same prefix forever (audit
// replay depends on it).
func TestPromptSHA_Stable(t *testing.T) {
	got := PromptSHA("you are leah")
	if len(got) != 16 {
		t.Fatalf("length %d, want 16", len(got))
	}
	again := PromptSHA("you are leah")
	if got != again {
		t.Errorf("non-deterministic: %q vs %q", got, again)
	}
	if PromptSHA("you are leah") == PromptSHA("you are different") {
		t.Errorf("SHA collision on distinct inputs")
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

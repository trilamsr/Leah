package reasoner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/trilam/leah/internal/obs"
)

// TestMeasurePromptTokens_AboveThreshold_ReturnsTrue: the 4-char/token
// approximation is the floor — text of 4*1024 ASCII chars must clear the
// 1024-token gate.
func TestMeasurePromptTokens_AboveThreshold_ReturnsTrue(t *testing.T) {
	big := strings.Repeat("a", 4*1024+8) // > 1024 tokens at 4 chars/tok
	tokens := MeasurePromptTokens(big)
	if tokens < CacheableThresholdTokens {
		t.Fatalf("want tokens >= %d, got %d", CacheableThresholdTokens, tokens)
	}
	if !ShouldCachePrompt(big) {
		t.Errorf("ShouldCachePrompt false on big prompt (tokens=%d)", tokens)
	}
}

// TestMeasurePromptTokens_BelowThreshold_NoCacheBlock: the production
// prompts/system.md is ~250 tokens and must NOT trip the cache gate —
// cache-write surcharge (25% input) would be pure overhead.
func TestMeasurePromptTokens_BelowThreshold_NoCacheBlock(t *testing.T) {
	small := "you are leah, a short prompt"
	tokens := MeasurePromptTokens(small)
	if tokens >= CacheableThresholdTokens {
		t.Fatalf("want tokens < %d, got %d", CacheableThresholdTokens, tokens)
	}
	if ShouldCachePrompt(small) {
		t.Errorf("ShouldCachePrompt true on small prompt (tokens=%d)", tokens)
	}
}

// TestAnthropicClient_AddsCacheControlBlock_OnLargePrompt: the system
// TextBlockParam's CacheControl.Type must be "ephemeral" when the prompt
// crosses the gate. We exercise this via a unit-level seam — the real SDK
// call is integration-only, so we assert the param-builder helper directly.
func TestAnthropicClient_AddsCacheControlBlock_OnLargePrompt(t *testing.T) {
	big := strings.Repeat("a", 4*1024+8)
	blk := buildSystemBlock(big)
	if string(blk.CacheControl.Type) != "ephemeral" {
		t.Errorf("want cache_control.type=ephemeral, got %q", blk.CacheControl.Type)
	}
	if blk.Text != big {
		t.Errorf("system text mutated")
	}
}

// TestAnthropicClient_OmitsCacheControlBlock_OnSmallPrompt: small prompts
// produce a TextBlockParam with the zero-value (omitzero) CacheControl
// so it does not serialize.
func TestAnthropicClient_OmitsCacheControlBlock_OnSmallPrompt(t *testing.T) {
	small := "tiny prompt"
	blk := buildSystemBlock(small)
	if string(blk.CacheControl.Type) != "" {
		t.Errorf("want zero CacheControl, got type=%q", blk.CacheControl.Type)
	}
}

// TestReasonerInstrumentation_CacheHitMetric: registering instrumentation
// against an obs.Registry wires the cache_hit counter such that miss/hit/
// disabled outcomes each increment the matching label, and the savings
// histogram receives a token observation on hit.
func TestReasonerInstrumentation_CacheHitMetric(t *testing.T) {
	reg := obs.NewRegistry()
	BindInstrumentation(reg)

	RecordCacheOutcome(reg, OutcomeHit, 1234)
	RecordCacheOutcome(reg, OutcomeMiss, 0)
	RecordCacheOutcome(reg, OutcomeDisabled, 0)

	counts, hists := readSnapshot(t, reg)

	for _, outcome := range []string{"hit", "miss", "disabled"} {
		key := "leah_reasoner_cache_hit_total|outcome=" + outcome
		if counts[key] != 1 {
			t.Errorf("counter %s=%d, want 1", key, counts[key])
		}
	}
	// Only the hit observed savings; the histogram series exists with one
	// observation (key has no labels → bare metric name).
	if hists["leah_reasoner_cache_savings_tokens"] != 1 {
		t.Errorf("savings histogram count=%d, want 1", hists["leah_reasoner_cache_savings_tokens"])
	}
}

// readSnapshot returns the flat counter values and histogram counts from
// the registry's JSON snapshot. Keeps this test self-contained — obstest
// only exposes union-of-keys, not values.
func readSnapshot(t *testing.T, r *obs.Registry) (map[string]int64, map[string]int64) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "m.json")
	if err := r.Snapshot(path); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var out struct {
		Counters   map[string]int64 `json:"counters"`
		Histograms map[string]struct {
			Count int64 `json:"count"`
		} `json:"histograms"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	hc := map[string]int64{}
	for k, v := range out.Histograms {
		hc[k] = v.Count
	}
	return out.Counters, hc
}

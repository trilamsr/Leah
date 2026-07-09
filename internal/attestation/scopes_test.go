package attestation

import (
	"os"
	"path/filepath"
	"testing"
)

// The cost_override scope MUST be registerable so Pool.Pick does not
// fail-closed on the override flow. Test loads a Pool with CostOverrideScope
// and asserts Pick returns a real question.
func TestAttestationPool_CostOverrideScopeRegistered(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "questions.txt")
	body := "Q1?\nQ2?\nQ3?\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	p, err := Load(path, CostOverrideScope)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	q, err := p.Pick(CostOverrideScope)
	if err != nil {
		t.Fatalf("Pick(%q): %v", CostOverrideScope, err)
	}
	if q == "" {
		t.Error("Pick returned empty question")
	}
}

// F3 §4.2 + §8: a forgotten scope registration blocks the act rather than
// bypassing it. Pool.Pick MUST surface a real question for both inbound
// scopes once registered — and the AllScopes slice MUST include them so the
// daemon's single Load(...) call covers the router's gate.
func TestAttestationPool_InboundScopesRegistered(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "questions.txt")
	if err := os.WriteFile(path, []byte("Q1?\nQ2?\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	p, err := Load(path, AllScopes()...)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, scope := range []string{ScopeInboundEnroll, ScopeInboundApply} {
		q, err := p.Pick(scope)
		if err != nil {
			t.Fatalf("Pick(%q): %v", scope, err)
		}
		if q == "" {
			t.Errorf("Pick(%q): empty question", scope)
		}
	}
	var sawEnroll, sawApply bool
	for _, s := range AllScopes() {
		if s == ScopeInboundEnroll {
			sawEnroll = true
		}
		if s == ScopeInboundApply {
			sawApply = true
		}
	}
	if !sawEnroll || !sawApply {
		t.Fatalf("AllScopes missing inbound scopes: enroll=%v apply=%v", sawEnroll, sawApply)
	}
}

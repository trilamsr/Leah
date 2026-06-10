package attestation

import (
	"os"
	"path/filepath"
	"testing"
)

// W94 spec §7.3: cost_override scope MUST be registerable so Pool.Pick
// does not fail-closed on the override flow. Test loads a Pool with
// CostOverrideScope and asserts Pick returns a real question.
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

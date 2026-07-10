package dispatcher

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/budget"
)

// Asserts selfbuild routes through the centralized attestation pool — the
// question rendered in the issue body MUST come from the configured file.
func TestSelfbuild_UsesAttestationPool(t *testing.T) {
	dir := t.TempDir()
	attestPath := filepath.Join(dir, "attestations.txt")
	const q = "Routed-via-attestation-pool: does this PR change a secret-touching surface?"
	if err := os.WriteFile(attestPath, []byte(q+"\n"), 0o600); err != nil {
		t.Fatalf("write attestation file: %v", err)
	}
	gh := &fakeGh{createURL: "https://github.com/trilamsr/Leah/issues/99"}
	sb := &SelfBuild{
		Reasoner:                 &fakeShipReasoner{resp: validSpec},
		GH:                       gh,
		Audit:                    &audit.Logger{Path: dir + "/audit.jsonl"},
		Budget:                   &budget.Budget{Ceiling: 5.0},
		Out:                      &bytes.Buffer{},
		TmpDir:                   dir,
		AttestationQuestionsPath: attestPath,
	}
	if err := sb.Run(context.Background(), "add --json flag"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(gh.createdBody, q) {
		t.Errorf("issue body missing pool-sourced question; body=%q", gh.createdBody)
	}
}

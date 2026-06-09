package dispatcher

import (
	"bytes"
	"context"
	"errors"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/budget"
)

// validSpec is a minimally well-formed Reasoner output that selfBuild treats as
// a real feature spec (NOT a clarify-abort). Must contain `## Title` to pass
// isClarifyResponse.
const validSpec = `## Title

[SELF-BUILD] add --json flag to leah status

## Motivation

Operator wants scriptable status output. Advances self-improvement loop §2.1.

## Files to create or modify

- internal/dispatcher/status.go
- internal/dispatcher/status_test.go

## Code shape

` + "```go\n" +
	`type Status struct { JSON bool }
` + "```\n" +
	`
## Acceptance criteria

- ` + "`leah status --json`" + ` emits valid JSON.

## Test plan

- TestStatusJSONFlag

## Deferred

- multi-format output (yaml, toml)

## Self-build context

(verbatim block)
`

func TestSelfBuildRefusesNonLeahRepo(t *testing.T) {
	dir := t.TempDir()
	sb := &SelfBuild{
		Reasoner:     &fakeShipReasoner{resp: validSpec},
		GH:           &fakeGh{},
		Audit:        &audit.Logger{Path: dir + "/audit.jsonl"},
		Budget:       &budget.Budget{Ceiling: 5.0},
		Out:          &bytes.Buffer{},
		TmpDir:       dir,
		RepoOverride: "trilam/regatta", // operator tried to retarget
	}
	err := sb.Run(context.Background(), "add a flag")
	if !errors.Is(err, ErrSelfBuildRepoLocked) {
		t.Fatalf("want ErrSelfBuildRepoLocked, got %v", err)
	}
	// Audit row recorded the rejection.
	data, _ := os.ReadFile(dir + "/audit.jsonl")
	if !strings.Contains(string(data), `"kind":"self-build"`) {
		t.Errorf("audit missing kind=self-build: %q", data)
	}
	if !strings.Contains(string(data), `"blast_radius":4`) {
		t.Errorf("audit missing blast_radius=4: %q", data)
	}
	if !strings.Contains(string(data), `"outcome":"rejected"`) {
		t.Errorf("audit missing outcome=rejected: %q", data)
	}
}

func TestSelfBuildAuditsBR4(t *testing.T) {
	dir := t.TempDir()
	gh := &fakeGh{createURL: "https://github.com/trilamsr/Leah/issues/42"}
	sb := &SelfBuild{
		Reasoner: &fakeShipReasoner{resp: validSpec},
		GH:       gh,
		Audit:    &audit.Logger{Path: dir + "/audit.jsonl"},
		Budget:   &budget.Budget{Ceiling: 5.0},
		Out:      &bytes.Buffer{},
		TmpDir:   dir,
	}
	if err := sb.Run(context.Background(), "add --json flag to leah status"); err != nil {
		t.Fatalf("run: %v", err)
	}
	if gh.createdRepo != SelfBuildRepo {
		t.Errorf("repo: got %q want %q", gh.createdRepo, SelfBuildRepo)
	}
	data, _ := os.ReadFile(dir + "/audit.jsonl")
	// Two audit rows: ship's BR=3 + self-build's BR=4. Self-build success row required.
	if !strings.Contains(string(data), `"kind":"self-build"`) {
		t.Errorf("audit missing kind=self-build: %q", data)
	}
	if !strings.Contains(string(data), `"blast_radius":4`) {
		t.Errorf("audit missing blast_radius=4: %q", data)
	}
	if !strings.Contains(string(data), `"outcome":"dispatched"`) {
		t.Errorf("audit missing outcome=dispatched: %q", data)
	}
}

func TestSelfBuildPrependsTitle(t *testing.T) {
	dir := t.TempDir()
	gh := &fakeGh{createURL: "https://github.com/trilamsr/Leah/issues/1"}
	sb := &SelfBuild{
		Reasoner: &fakeShipReasoner{resp: validSpec},
		GH:       gh,
		Audit:    &audit.Logger{Path: dir + "/audit.jsonl"},
		Budget:   &budget.Budget{Ceiling: 5.0},
		Out:      &bytes.Buffer{},
		TmpDir:   dir,
	}
	if err := sb.Run(context.Background(), "add --json flag to leah status"); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.HasPrefix(gh.createdTitle, SelfBuildTitlePrefix) {
		t.Errorf("title missing %q prefix: %q", SelfBuildTitlePrefix, gh.createdTitle)
	}
}

// TestSelfBuildIncludesRandomAttestation asserts that when an attestation
// file is configured, the dispatched issue body contains the attestation
// block + one of the questions from the file (operator-habituation defense,
// Wave1-E HIGH-2).
func TestSelfBuildIncludesRandomAttestation(t *testing.T) {
	dir := t.TempDir()
	attestPath := filepath.Join(dir, "attestations.txt")
	const q1 = "What's the biggest risk in this diff?"
	const q2 = "Which test case would catch a regression here?"
	if err := os.WriteFile(attestPath, []byte(q1+"\n"+q2+"\n"), 0o600); err != nil {
		t.Fatalf("write attestation file: %v", err)
	}
	gh := &fakeGh{createURL: "https://github.com/trilamsr/Leah/issues/7"}
	sb := &SelfBuild{
		Reasoner:                 &fakeShipReasoner{resp: validSpec},
		GH:                       gh,
		Audit:                    &audit.Logger{Path: dir + "/audit.jsonl"},
		Budget:                   &budget.Budget{Ceiling: 5.0},
		Out:                      &bytes.Buffer{},
		TmpDir:                   dir,
		AttestationQuestionsPath: attestPath,
		AttestationOperatorLogin: "tri-lamsr",
		// Deterministic seed: rand.Intn(2) returns 1 → q2 with this source.
		// Test only asserts SOME known question appears, so the assertion is
		// stable regardless of which one Intn picks.
		Rand: rand.New(rand.NewSource(1)),
	}
	if err := sb.Run(context.Background(), "add --json flag"); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(gh.createdBody, "## Operator merge attestation") {
		t.Errorf("body missing attestation header: %q", gh.createdBody)
	}
	if !strings.Contains(gh.createdBody, "tri-lamsr") {
		t.Errorf("body missing operator login: %q", gh.createdBody)
	}
	if !strings.Contains(gh.createdBody, "Attestation:") {
		t.Errorf("body missing Attestation: comment shape: %q", gh.createdBody)
	}
	if !strings.Contains(gh.createdBody, q1) && !strings.Contains(gh.createdBody, q2) {
		t.Errorf("body missing both candidate questions: %q", gh.createdBody)
	}
	// Audit row should record the attestation question.
	data, _ := os.ReadFile(dir + "/audit.jsonl")
	if !strings.Contains(string(data), "attestation_question=") {
		t.Errorf("audit missing attestation_question field: %q", data)
	}
}

// TestSelfBuildAttestationFailClosed asserts that a configured but empty /
// missing attestation file aborts the dispatch (fail-closed) — the gate
// only disables when AttestationQuestionsPath is left empty.
func TestSelfBuildAttestationFailClosed(t *testing.T) {
	dir := t.TempDir()
	emptyPath := filepath.Join(dir, "empty.txt")
	if err := os.WriteFile(emptyPath, []byte("# only comments\n\n"), 0o600); err != nil {
		t.Fatalf("write empty file: %v", err)
	}
	gh := &fakeGh{createURL: "should-not-be-called"}
	sb := &SelfBuild{
		Reasoner:                 &fakeShipReasoner{resp: validSpec},
		GH:                       gh,
		Audit:                    &audit.Logger{Path: dir + "/audit.jsonl"},
		Budget:                   &budget.Budget{Ceiling: 5.0},
		Out:                      &bytes.Buffer{},
		TmpDir:                   dir,
		AttestationQuestionsPath: emptyPath,
	}
	if err := sb.Run(context.Background(), "anything"); err == nil {
		t.Fatal("expected error when attestation file has no questions, got nil")
	}
	if gh.createdTitle != "" {
		t.Errorf("issue created despite attestation failure: %q", gh.createdTitle)
	}
}

func TestSelfBuildClarifyAbortFilesNoIssue(t *testing.T) {
	dir := t.TempDir()
	clarify := "## Clarifying questions\n\n- Which package?\n- What's the observable acceptance?\n"
	gh := &fakeGh{createURL: "should-not-be-called"}
	out := &bytes.Buffer{}
	sb := &SelfBuild{
		Reasoner: &fakeShipReasoner{resp: clarify},
		GH:       gh,
		Audit:    &audit.Logger{Path: dir + "/audit.jsonl"},
		Budget:   &budget.Budget{Ceiling: 5.0},
		Out:      out,
		TmpDir:   dir,
	}
	err := sb.Run(context.Background(), "make leah smarter")
	if !errors.Is(err, ErrSelfBuildClarify) {
		t.Fatalf("want ErrSelfBuildClarify, got %v", err)
	}
	if gh.createdTitle != "" {
		t.Errorf("issue created on clarify-abort: %q", gh.createdTitle)
	}
	if !strings.Contains(out.String(), "Clarifying questions") {
		t.Errorf("clarify output missing questions: %q", out.String())
	}
	data, _ := os.ReadFile(dir + "/audit.jsonl")
	if !strings.Contains(string(data), `"outcome":"clarify"`) {
		t.Errorf("audit missing outcome=clarify: %q", data)
	}
}

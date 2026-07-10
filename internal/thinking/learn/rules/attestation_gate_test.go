package rules

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/trilam/leah/internal/audit"
)

// TestAttestationGate_FlagsMissingAttestationComment asserts the gate
// reports a violation when a dispatched self-build PR is MERGED but no PR
// comment begins with the "Attestation:" sentinel from the operator-merge
// attestation block (H3 — Wave2-5 retro: footer existed but no enforcer).
func TestAttestationGate_FlagsMissingAttestationComment(t *testing.T) {
	dir := t.TempDir()
	auditPath := filepath.Join(dir, "audit.jsonl")
	now := func() time.Time { return time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC) }
	lg := &audit.Logger{Path: auditPath, Now: now}
	if err := lg.Append(audit.Entry{
		Kind:        "self-build",
		ArgsHash:    "abc12345",
		BlastRadius: 4,
		Outcome:     "dispatched",
		Detail:      "prompt_sha=deadbeef url=https://github.com/trilamsr/Leah/pull/42",
	}); err != nil {
		t.Fatalf("seed audit: %v", err)
	}

	gate := AttestationGate{
		PR: func(_ context.Context, repo string, prNum int) (PRView, error) {
			return PRView{
				State: "MERGED",
				Comments: []PRComment{
					{Body: "LGTM"},
					{Body: "ship it"},
				},
			}, nil
		},
	}
	vs, err := gate.Scan(context.Background(), auditPath)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(vs) != 1 {
		t.Fatalf("violations: got %d, want 1: %+v", len(vs), vs)
	}
	if vs[0].PRNumber != 42 || vs[0].Repo != "trilamsr/Leah" {
		t.Errorf("violation: got %+v", vs[0])
	}
}

// TestAttestationGate_AcceptsAttestationComment asserts a PR with a
// well-formed Attestation: comment does NOT appear in the violation list.
func TestAttestationGate_AcceptsAttestationComment(t *testing.T) {
	dir := t.TempDir()
	auditPath := filepath.Join(dir, "audit.jsonl")
	lg := &audit.Logger{Path: auditPath}
	if err := lg.Append(audit.Entry{
		Kind:        "self-build",
		ArgsHash:    "abc",
		BlastRadius: 4,
		Outcome:     "dispatched",
		Detail:      "prompt_sha=cafefeed url=https://github.com/trilamsr/Leah/pull/99",
	}); err != nil {
		t.Fatalf("seed audit: %v", err)
	}

	gate := AttestationGate{
		PR: func(_ context.Context, repo string, prNum int) (PRView, error) {
			return PRView{
				State: "MERGED",
				Comments: []PRComment{
					{Body: "Attestation: I reviewed the diff and ran the test locally."},
				},
			}, nil
		},
	}
	vs, err := gate.Scan(context.Background(), auditPath)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(vs) != 0 {
		t.Fatalf("expected no violations, got: %+v", vs)
	}
}

// TestAttestationGate_SkipsOpenPRs asserts non-MERGED state is ignored —
// the gate only flags merged PRs that lacked attestation.
func TestAttestationGate_SkipsOpenPRs(t *testing.T) {
	dir := t.TempDir()
	auditPath := filepath.Join(dir, "audit.jsonl")
	lg := &audit.Logger{Path: auditPath}
	if err := lg.Append(audit.Entry{
		Kind:        "self-build",
		Outcome:     "dispatched",
		Detail:      "url=https://github.com/trilamsr/Leah/pull/7",
		BlastRadius: 4,
	}); err != nil {
		t.Fatalf("seed audit: %v", err)
	}

	gate := AttestationGate{
		PR: func(_ context.Context, _ string, _ int) (PRView, error) {
			return PRView{State: "OPEN"}, nil
		},
	}
	vs, err := gate.Scan(context.Background(), auditPath)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(vs) != 0 {
		t.Errorf("expected no violations for OPEN PR, got: %+v", vs)
	}
}

// TestAttestationGate_AttestationMatchIsCaseAndPrefixStrict asserts the
// sentinel must appear at the start of a non-empty line, not mid-paragraph.
// (Defends against accidental matches in arbitrary PR discussion.)
func TestAttestationGate_AttestationMatchIsCaseAndPrefixStrict(t *testing.T) {
	if !startsWithAttestation("Attestation: signed.") {
		t.Error("exact prefix should match")
	}
	if !startsWithAttestation("  Attestation: with leading space.") {
		t.Error("leading whitespace tolerated")
	}
	if startsWithAttestation("see Attestation: in the body") {
		t.Error("mid-line mention must not match")
	}
	if startsWithAttestation("attestation: lowercase") {
		t.Error("lowercase sentinel must not match (operator-habituation defense)")
	}
}

// TestAttestationGate_IgnoresNonDispatchedRows asserts only outcome=dispatched
// self-build rows are scanned (rejected/clarify/failed never opened a PR).
func TestAttestationGate_IgnoresNonDispatchedRows(t *testing.T) {
	dir := t.TempDir()
	auditPath := filepath.Join(dir, "audit.jsonl")
	lg := &audit.Logger{Path: auditPath}
	for _, oc := range []string{"rejected", "failed", "clarify"} {
		if err := lg.Append(audit.Entry{
			Kind:    "self-build",
			Outcome: oc,
			Detail:  "url=https://github.com/trilamsr/Leah/pull/1",
		}); err != nil {
			t.Fatal(err)
		}
	}

	called := false
	gate := AttestationGate{
		PR: func(_ context.Context, _ string, _ int) (PRView, error) {
			called = true
			return PRView{State: "MERGED"}, nil
		},
	}
	vs, err := gate.Scan(context.Background(), auditPath)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if called {
		t.Error("PR probe should not be called for non-dispatched rows")
	}
	if len(vs) != 0 {
		t.Errorf("expected no violations, got: %+v", vs)
	}
}

// TestAttestationGate_HopsIssueToPR asserts the gate accepts an
// `/issues/N` URL (the shape SelfBuild logs today) and resolves it to
// the closing PR via the IssueToPR probe before checking attestation.
func TestAttestationGate_HopsIssueToPR(t *testing.T) {
	dir := t.TempDir()
	auditPath := filepath.Join(dir, "audit.jsonl")
	lg := &audit.Logger{Path: auditPath}
	if err := lg.Append(audit.Entry{
		Kind:        "self-build",
		Outcome:     "dispatched",
		BlastRadius: 4,
		Detail:      "prompt_sha=abc url=https://github.com/trilamsr/Leah/issues/100",
	}); err != nil {
		t.Fatal(err)
	}

	gate := AttestationGate{
		IssueToPR: func(_ context.Context, repo string, issueNum int) (int, error) {
			if repo != "trilamsr/Leah" || issueNum != 100 {
				t.Errorf("unexpected IssueToPR call: %s %d", repo, issueNum)
			}
			return 101, nil
		},
		PR: func(_ context.Context, _ string, prNum int) (PRView, error) {
			if prNum != 101 {
				t.Errorf("PR probe got prNum=%d, want 101", prNum)
			}
			return PRView{State: "MERGED"}, nil // no comments → violation
		},
	}
	vs, err := gate.Scan(context.Background(), auditPath)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(vs) != 1 || vs[0].PRNumber != 101 {
		t.Errorf("expected one violation for PR 101, got: %+v", vs)
	}
}

// TestAttestationGate_SkipsIssueWithNoLinkedPR asserts an in-flight
// self-build issue (issue exists, no closing PR yet) is silently
// skipped rather than reported.
func TestAttestationGate_SkipsIssueWithNoLinkedPR(t *testing.T) {
	dir := t.TempDir()
	auditPath := filepath.Join(dir, "audit.jsonl")
	lg := &audit.Logger{Path: auditPath}
	if err := lg.Append(audit.Entry{
		Kind:        "self-build",
		Outcome:     "dispatched",
		BlastRadius: 4,
		Detail:      "url=https://github.com/trilamsr/Leah/issues/200",
	}); err != nil {
		t.Fatal(err)
	}

	gate := AttestationGate{
		IssueToPR: func(_ context.Context, _ string, _ int) (int, error) { return 0, nil },
		PR: func(_ context.Context, _ string, _ int) (PRView, error) {
			t.Error("PR probe should not be called when issue has no closing PR")
			return PRView{}, nil
		},
	}
	vs, err := gate.Scan(context.Background(), auditPath)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(vs) != 0 {
		t.Errorf("expected no violations, got: %+v", vs)
	}
}

// TestAttestationGate_DedupesByPR asserts a self-build PR appearing in
// multiple audit rows (re-dispatch after retro etc) is reported once.
func TestAttestationGate_DedupesByPR(t *testing.T) {
	dir := t.TempDir()
	auditPath := filepath.Join(dir, "audit.jsonl")
	lg := &audit.Logger{Path: auditPath}
	for i := 0; i < 2; i++ {
		if err := lg.Append(audit.Entry{
			Kind:    "self-build",
			Outcome: "dispatched",
			Detail:  "url=https://github.com/trilamsr/Leah/pull/55",
		}); err != nil {
			t.Fatal(err)
		}
	}
	gate := AttestationGate{
		PR: func(_ context.Context, _ string, _ int) (PRView, error) {
			return PRView{State: "MERGED"}, nil
		},
	}
	vs, err := gate.Scan(context.Background(), auditPath)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(vs) != 1 {
		t.Errorf("dedup failed: got %d violations, want 1", len(vs))
	}
	// Ensure strings.Contains check still useful — avoid unused import.
	_ = strings.Contains
}

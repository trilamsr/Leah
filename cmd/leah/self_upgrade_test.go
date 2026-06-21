package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestSelfUpgrade_AttestationDenied_NoRun asserts the gate runs FIRST: a denied
// attestation aborts before the upgrade script is ever invoked, and the abort is
// audited with outcome=declined. An unattested self-mutation of the binary is the
// exact threat the gate guards — if the runner fired here, the gate is decorative.
func TestSelfUpgrade_AttestationDenied_NoRun(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)

	ran := false
	deps := &upgradeDeps{
		Attestor: fakeAttestor{err: errors.New("denied")},
		Run: func(context.Context) error {
			ran = true
			return nil
		},
	}

	var buf bytes.Buffer
	code := runSelfUpgrade(context.Background(), nil, &buf, deps)
	if code == 0 {
		t.Fatalf("expected non-zero exit on attestation denial")
	}
	if ran {
		t.Fatalf("upgrade runner fired despite attestation denial")
	}
	raw, err := os.ReadFile(filepath.Join(dir, "audit.jsonl"))
	if err != nil {
		t.Fatalf("read audit: %v", err)
	}
	if !bytes.Contains(raw, []byte(`"self_upgrade"`)) ||
		!bytes.Contains(raw, []byte(`"declined"`)) {
		t.Fatalf("audit row mismatch: %s", raw)
	}
}

// TestSelfUpgrade_AttestationPass_RunsAndAudits asserts a passing attestation
// invokes the upgrade runner and audits outcome=ok.
func TestSelfUpgrade_AttestationPass_RunsAndAudits(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)

	ran := false
	deps := &upgradeDeps{
		Attestor: fakeAttestor{},
		Run: func(context.Context) error {
			ran = true
			return nil
		},
	}

	var buf bytes.Buffer
	code := runSelfUpgrade(context.Background(), nil, &buf, deps)
	if code != 0 {
		t.Fatalf("expected exit 0 on pass, got %d (out=%s)", code, buf.String())
	}
	if !ran {
		t.Fatalf("upgrade runner did not fire on attestation pass")
	}
	raw, err := os.ReadFile(filepath.Join(dir, "audit.jsonl"))
	if err != nil {
		t.Fatalf("read audit: %v", err)
	}
	if !bytes.Contains(raw, []byte(`"self_upgrade"`)) ||
		!bytes.Contains(raw, []byte(`"ok"`)) {
		t.Fatalf("audit row mismatch: %s", raw)
	}
}

// TestSelfUpgrade_RunnerFails_AuditsFailed asserts a script failure surfaces a
// non-zero exit and an outcome=failed audit row (no silent swallow).
func TestSelfUpgrade_RunnerFails_AuditsFailed(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)

	deps := &upgradeDeps{
		Attestor: fakeAttestor{},
		Run:      func(context.Context) error { return errors.New("build failed") },
	}

	var buf bytes.Buffer
	code := runSelfUpgrade(context.Background(), nil, &buf, deps)
	if code == 0 {
		t.Fatalf("expected non-zero exit on runner failure")
	}
	raw, err := os.ReadFile(filepath.Join(dir, "audit.jsonl"))
	if err != nil {
		t.Fatalf("read audit: %v", err)
	}
	if !bytes.Contains(raw, []byte(`"self_upgrade"`)) ||
		!bytes.Contains(raw, []byte(`"failed"`)) {
		t.Fatalf("audit row mismatch: %s", raw)
	}
}

package plugin

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestNewSandbox_RejectsNonPositiveRSS(t *testing.T) {
	sb := NewSandbox(nil)
	bundle := mkFakeBundle(t)
	if _, err := sb.Spawn(context.Background(), bundle, nil, 0); !errors.Is(err, ErrRSSCapInvalid) {
		t.Fatalf("want ErrRSSCapInvalid, got %v", err)
	}
	if _, err := sb.Spawn(context.Background(), bundle, nil, -1); !errors.Is(err, ErrRSSCapInvalid) {
		t.Fatalf("want ErrRSSCapInvalid, got %v", err)
	}
}

func TestNewSandbox_PolicyAppliedOnSpawn(t *testing.T) {
	bundle := mkFakeBundle(t)
	pol := &recordPolicy{}
	sb := NewSandbox(pol)
	cmd, err := sb.Spawn(context.Background(), bundle, nil, 256)
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	_ = cmd.Wait()
	if !pol.called {
		t.Fatal("policy.ApplyRSSCap not invoked")
	}
	if pol.gotMB != 256 {
		t.Fatalf("policy got rssCapMB=%d, want 256", pol.gotMB)
	}
}

func TestNewSandbox_PolicyFailureKillsChild(t *testing.T) {
	bundle := mkFakeBundle(t)
	pol := &recordPolicy{err: errors.New("task policy refused")}
	sb := NewSandbox(pol)
	if _, err := sb.Spawn(context.Background(), bundle, nil, 256); err == nil {
		t.Fatal("expected spawn error when policy fails")
	}
}

func TestNewSandbox_NilPolicyOK(t *testing.T) {
	bundle := mkFakeBundle(t)
	sb := NewSandbox(nil)
	cmd, err := sb.Spawn(context.Background(), bundle, nil, 256)
	if err != nil {
		t.Fatalf("spawn nil policy: %v", err)
	}
	_ = cmd.Wait()
}

// mkFakeBundle builds the minimum bundle layout the sandbox dereferences.
func mkFakeBundle(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	contents := filepath.Join(dir, "Contents")
	if err := os.MkdirAll(contents, 0o755); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(contents, "binary")
	// trivial sh script exits immediately; smoke-tests the spawn path without depending on a real plugin.
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

type recordPolicy struct {
	called bool
	gotMB  int
	err    error
}

func (p *recordPolicy) ApplyRSSCap(pid, mb int) error {
	p.called = true
	p.gotMB = mb
	return p.err
}

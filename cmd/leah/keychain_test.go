package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/trilam/leah/internal/keychain"
)

// fakeSecurity is a test-only replacement for /usr/bin/security. Shell script
// mirrors internal/keychain/keychain_test.go's fake so the CLI path exercises
// the same subprocess contract.
func fakeSecurity(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	state := filepath.Join(dir, "state")
	if err := os.WriteFile(state, []byte("MISSING"), 0o600); err != nil {
		t.Fatalf("seed state: %v", err)
	}
	bin := filepath.Join(dir, "security")
	script := `#!/bin/sh
sub=$1; shift
STATE="` + state + `"
case "$sub" in
find-generic-password)
  v=$(cat "$STATE")
  if [ "$v" = "MISSING" ]; then
    echo "The specified item could not be found in the keychain." 1>&2
    exit 44
  fi
  printf '%s\n' "$v"
  exit 0 ;;
add-generic-password)
  while [ $# -gt 0 ]; do
    case "$1" in -w) shift; printf '%s' "$1" > "$STATE"; shift ;; *) shift ;; esac
  done
  exit 0 ;;
delete-generic-password)
  v=$(cat "$STATE")
  if [ "$v" = "MISSING" ]; then
    echo "The specified item could not be found in the keychain." 1>&2
    exit 44
  fi
  echo "MISSING" > "$STATE"
  exit 0 ;;
esac
exit 2
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake: %v", err)
	}
	return bin
}

func TestRunKeychain_SetAndGet(t *testing.T) {
	bin := fakeSecurity(t)
	defer keychain.SetSecurityBin(keychain.SetSecurityBin(bin))
	t.Setenv("ANTHROPIC_API_KEY", "") // ensure env doesn't shadow

	var stdout bytes.Buffer
	stdin := strings.NewReader("sk-ant-abc\n")
	if code := runKeychain(context.Background(), []string{"set", "anthropic"}, stdin, &stdout); code != 0 {
		t.Fatalf("set exit %d out=%s", code, stdout.String())
	}
	stdout.Reset()
	if code := runKeychain(context.Background(), []string{"get", "anthropic"}, strings.NewReader(""), &stdout); code != 0 {
		t.Fatalf("get exit %d", code)
	}
	if got := strings.TrimSpace(stdout.String()); got != "sk-ant-abc" {
		t.Fatalf("want sk-ant-abc, got %q", got)
	}
}

func TestRunKeychain_UnknownServiceIsError(t *testing.T) {
	var stdout bytes.Buffer
	code := runKeychain(context.Background(), []string{"set", "bogus"}, strings.NewReader("x\n"), &stdout)
	if code == 0 {
		t.Fatal("want non-zero exit for unknown service")
	}
}

func TestRunKeychain_EmptyStdinRejected(t *testing.T) {
	bin := fakeSecurity(t)
	defer keychain.SetSecurityBin(keychain.SetSecurityBin(bin))

	var stdout bytes.Buffer
	code := runKeychain(context.Background(), []string{"set", "anthropic"}, strings.NewReader("\n"), &stdout)
	if code == 0 {
		t.Fatal("want non-zero exit for empty secret")
	}
}

func TestRunKeychain_Delete(t *testing.T) {
	bin := fakeSecurity(t)
	defer keychain.SetSecurityBin(keychain.SetSecurityBin(bin))

	var stdout bytes.Buffer
	// set then delete
	if code := runKeychain(context.Background(), []string{"set", "elevenlabs"}, strings.NewReader("v\n"), &stdout); code != 0 {
		t.Fatalf("set exit %d", code)
	}
	stdout.Reset()
	if code := runKeychain(context.Background(), []string{"delete", "elevenlabs"}, strings.NewReader(""), &stdout); code != 0 {
		t.Fatalf("delete exit %d", code)
	}
}

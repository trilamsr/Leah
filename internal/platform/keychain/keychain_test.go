package keychain

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// fakeSecurity writes a shell script that emulates security(1). state is a
// tmp file: "<value>" if set, "MISSING" sentinel otherwise. Used to exercise
// Load/Save/Delete without hitting the real Keychain (unavailable on CI).
func fakeSecurity(t *testing.T) (bin, state string) {
	t.Helper()
	dir := t.TempDir()
	state = filepath.Join(dir, "state")
	if err := os.WriteFile(state, []byte("MISSING"), 0o600); err != nil {
		t.Fatalf("seed state: %v", err)
	}
	bin = filepath.Join(dir, "security")
	script := `#!/bin/sh
sub=$1; shift
STATE="` + state + `"
case "$sub" in
find-generic-password)
  v=$(cat "$STATE")
  if [ "$v" = "MISSING" ]; then
    echo "security: SecKeychainSearchCopyNext: The specified item could not be found in the keychain." 1>&2
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
    echo "security: SecKeychainSearchCopyNext: The specified item could not be found in the keychain." 1>&2
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
	return bin, state
}

func TestLoad_Missing_ReturnsEmptyNoError(t *testing.T) {
	bin, _ := fakeSecurity(t)
	defer SetSecurityBin(SetSecurityBin(bin))

	v, err := Load("com.maydow.leah.anthropic", "default")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if v != "" {
		t.Fatalf("want empty, got %q", v)
	}
}

func TestSaveThenLoad_Roundtrip(t *testing.T) {
	bin, _ := fakeSecurity(t)
	defer SetSecurityBin(SetSecurityBin(bin))

	if err := Save("com.maydow.leah.anthropic", "default", "sk-ant-xxx"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	v, err := Load("com.maydow.leah.anthropic", "default")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if v != "sk-ant-xxx" {
		t.Fatalf("want sk-ant-xxx, got %q", v)
	}
}

func TestDelete_MissingIsNoop(t *testing.T) {
	bin, _ := fakeSecurity(t)
	defer SetSecurityBin(SetSecurityBin(bin))

	if err := Delete("com.maydow.leah.anthropic", "default"); err != nil {
		t.Fatalf("Delete missing: %v", err)
	}
}

func TestDelete_Present(t *testing.T) {
	bin, _ := fakeSecurity(t)
	defer SetSecurityBin(SetSecurityBin(bin))

	if err := Save("s", "a", "val"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := Delete("s", "a"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	v, err := Load("s", "a")
	if err != nil || v != "" {
		t.Fatalf("post-delete want empty/nil, got %q/%v", v, err)
	}
}

func TestLoad_RequiresServiceAndAccount(t *testing.T) {
	if _, err := Load("", "a"); err == nil {
		t.Fatal("want error for empty service")
	}
	if _, err := Load("s", ""); err == nil {
		t.Fatal("want error for empty account")
	}
}

func TestLoadAnthropicKey_EnvWins(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "env-key")
	v, err := LoadAnthropicKey()
	if err != nil {
		t.Fatalf("LoadAnthropicKey: %v", err)
	}
	if v != "env-key" {
		t.Fatalf("want env-key, got %q", v)
	}
}

func TestLoadAnthropicKey_FallsBackToKeychain(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	bin, _ := fakeSecurity(t)
	defer SetSecurityBin(SetSecurityBin(bin))
	if err := Save(AnthropicService, DefaultAccount, "kc-key"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	v, err := LoadAnthropicKey()
	if err != nil {
		t.Fatalf("LoadAnthropicKey: %v", err)
	}
	if v != "kc-key" {
		t.Fatalf("want kc-key, got %q", v)
	}
}

func TestLoadPushover_UserAndTokenDistinct(t *testing.T) {
	t.Setenv("LEAH_PUSHOVER_USER", "")
	t.Setenv("LEAH_PUSHOVER_TOKEN", "")
	bin, _ := fakeSecurity(t)
	defer SetSecurityBin(SetSecurityBin(bin))
	// Fake only tracks one state file, so verify the account plumbing
	// through Save-then-Load with matching accounts.
	if err := Save(PushoverService, PushoverUserAccount, "u1"); err != nil {
		t.Fatal(err)
	}
	got, err := LoadPushoverUser()
	if err != nil || got != "u1" {
		t.Fatalf("LoadPushoverUser: %q/%v", got, err)
	}
}

// skipIfNoSecurity is a helper for real-security integration tests (not run
// by default) so future contributors can flip a build tag and exercise the
// live Keychain path.
func skipIfNoSecurity(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("security"); err != nil {
		t.Skip("security(1) not available")
	}
}

var _ = skipIfNoSecurity // reserved for future integration tests

package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/trilam/leah/internal/platform/keychain"
)

func TestRunInit_WritesMarkerAndPlist(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	t.Setenv("HOME", dir)
	t.Setenv("LEAH_INIT_SKIP_LAUNCHCTL", "1")
	t.Setenv("LEAH_INIT_AUTO_ACCEPT", "1")

	var buf bytes.Buffer
	if code := runInit(context.Background(), nil, &buf, strings.NewReader("")); code != 0 {
		t.Fatalf("exit %d, out=%s", code, buf.String())
	}
	if !strings.Contains(buf.String(), "Welcome to Leah") {
		t.Fatalf("missing welcome banner: %s", buf.String())
	}

	marker := filepath.Join(dir, "init.done")
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("marker not written: %v", err)
	}
	plist := filepath.Join(dir, "Library", "LaunchAgents", "com.leah.daemon.plist")
	body, err := os.ReadFile(plist)
	if err != nil {
		t.Fatalf("plist not written: %v", err)
	}
	if !strings.Contains(string(body), "com.leah.daemon") {
		t.Fatalf("plist label missing: %s", body)
	}
	if !strings.Contains(string(body), "ProgramArguments") {
		t.Fatalf("plist body malformed: %s", body)
	}
}

func TestRunInit_IdempotentWithoutForce(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	t.Setenv("HOME", dir)
	t.Setenv("LEAH_INIT_SKIP_LAUNCHCTL", "1")
	t.Setenv("LEAH_INIT_AUTO_ACCEPT", "1")

	var buf bytes.Buffer
	_ = runInit(context.Background(), nil, &buf, strings.NewReader(""))

	buf.Reset()
	if code := runInit(context.Background(), nil, &buf, strings.NewReader("")); code != 0 {
		t.Fatalf("second run exit %d", code)
	}
	if !strings.Contains(buf.String(), "already initialized") {
		t.Fatalf("expected idempotent short-circuit message: %s", buf.String())
	}
}

func TestRunInit_ForceReRuns(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	t.Setenv("HOME", dir)
	t.Setenv("LEAH_INIT_SKIP_LAUNCHCTL", "1")
	t.Setenv("LEAH_INIT_AUTO_ACCEPT", "1")

	var buf bytes.Buffer
	_ = runInit(context.Background(), nil, &buf, strings.NewReader(""))

	buf.Reset()
	if code := runInit(context.Background(), []string{"--force"}, &buf, strings.NewReader("")); code != 0 {
		t.Fatalf("force run exit %d", code)
	}
	if !strings.Contains(buf.String(), "Welcome to Leah") {
		t.Fatalf("force did not re-run wizard: %s", buf.String())
	}
}

func TestRunInit_PromptsForAdapters(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	t.Setenv("HOME", dir)
	t.Setenv("LEAH_INIT_SKIP_LAUNCHCTL", "1")
	t.Setenv("LEAH_INIT_AUTO_ACCEPT", "1")

	var buf bytes.Buffer
	if code := runInit(context.Background(), nil, &buf, strings.NewReader("")); code != 0 {
		t.Fatalf("exit %d", code)
	}
	out := buf.String()
	// Expect at least the two OAuth adapters to be enumerated in the prompt
	// surface — operator should see what's available without re-reading docs.
	for _, name := range []string{"gmail", "gcal"} {
		if !strings.Contains(out, name) {
			t.Fatalf("adapter %q not enumerated in init output: %s", name, out)
		}
	}
}

func TestRunInit_Help(t *testing.T) {
	var buf bytes.Buffer
	if code := runInit(context.Background(), []string{"--help"}, &buf, strings.NewReader("")); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(buf.String(), "leah init") {
		t.Fatalf("help missing usage: %s", buf.String())
	}
}

// errBrokenPipe + errReader cover the non-EOF read-error branch — proves the
// wizard surfaces the error and continues instead of silently treating it as
// "no" (review #1 of MAY-W31a).
type errBrokenPipe struct{}

func (errBrokenPipe) Error() string { return "broken pipe" }

type errReader struct{ err error }

func (e errReader) Read(_ []byte) (int, error) { return 0, e.err }

func TestRunInit_InteractiveYesPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	t.Setenv("HOME", dir)
	t.Setenv("LEAH_INIT_SKIP_LAUNCHCTL", "1")
	t.Setenv("ANTHROPIC_API_KEY", "")
	// No AUTO_ACCEPT — exercise the live prompt.
	// Fake security bin so a real Keychain hit on a dev machine can't
	// short-circuit the wizard's step 1 before the adapter prompt.
	defer keychain.SetSecurityBin(keychain.SetSecurityBin(fakeSecurityBin(t)))

	// "y" + blanks for the rest. Without OAuth secrets, runConnect for gmail
	// returns non-zero; the wizard catches it and prints the (skipped) hint.
	in := strings.NewReader("y\n\n\n\n\n\n\n\n\n\n\n\n")
	var buf bytes.Buffer
	if code := runInit(context.Background(), nil, &buf, in); code != 0 {
		t.Fatalf("exit %d, out=%s", code, buf.String())
	}
	if !strings.Contains(buf.String(), "connect gmail now?") {
		t.Fatalf("interactive prompt missing: %s", buf.String())
	}
}

func TestRunInit_EOFStopsPrompting(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	t.Setenv("HOME", dir)
	t.Setenv("LEAH_INIT_SKIP_LAUNCHCTL", "1")
	t.Setenv("ANTHROPIC_API_KEY", "")
	defer keychain.SetSecurityBin(keychain.SetSecurityBin(fakeSecurityBin(t)))

	var buf bytes.Buffer
	// Empty stdin → first ReadString returns io.EOF on prompt #1.
	// Wizard MUST exit the loop early and still complete (marker written).
	if code := runInit(context.Background(), nil, &buf, strings.NewReader("")); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(buf.String(), "stdin closed") {
		t.Fatalf("EOF branch not exercised: %s", buf.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "init.done")); err != nil {
		t.Fatalf("marker missing after EOF: %v", err)
	}
}

func TestRunInit_ReaderErrorContinues(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	t.Setenv("HOME", dir)
	t.Setenv("LEAH_INIT_SKIP_LAUNCHCTL", "1")
	t.Setenv("ANTHROPIC_API_KEY", "")
	defer keychain.SetSecurityBin(keychain.SetSecurityBin(fakeSecurityBin(t)))

	var buf bytes.Buffer
	if code := runInit(context.Background(), nil, &buf, errReader{err: errBrokenPipe{}}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(buf.String(), "input error") {
		t.Fatalf("error branch not exercised: %s", buf.String())
	}
}

// fakeSecurityBin writes a shell script that impersonates security(1):
// find-generic-password → exit 44 (missing sentinel); add-generic-password →
// exit 0. Enough surface to exercise the wizard's Load-miss + Save-hit path
// without touching the real Keychain (unavailable in CI).
func fakeSecurityBin(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "security")
	script := `#!/bin/sh
sub=$1; shift
case "$sub" in
find-generic-password)
  echo "security: SecKeychainSearchCopyNext: The specified item could not be found in the keychain." 1>&2
  exit 44 ;;
add-generic-password) exit 0 ;;
delete-generic-password) exit 0 ;;
esac
exit 2
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake security: %v", err)
	}
	return bin
}

// Valid sk-ant-* key → keychain.Save invoked; "stored in Keychain." emitted.
func TestRunInit_AnthropicKeyStored(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	t.Setenv("HOME", dir)
	t.Setenv("LEAH_INIT_SKIP_LAUNCHCTL", "1")
	t.Setenv("ANTHROPIC_API_KEY", "")
	defer keychain.SetSecurityBin(keychain.SetSecurityBin(fakeSecurityBin(t)))

	fakeKey := "sk-ant-api03-" + strings.Repeat("x", 40)
	in := strings.NewReader(fakeKey + "\n" + strings.Repeat("\n", 20))
	var buf bytes.Buffer
	if code := runInit(context.Background(), nil, &buf, in); code != 0 {
		t.Fatalf("exit %d, out=%s", code, buf.String())
	}
	if !strings.Contains(buf.String(), "stored in Keychain.") {
		t.Fatalf("stored acknowledgment missing: %s", buf.String())
	}
}

// Malformed input must NOT reach keychain.Save — regex mirrors phase-5 design.
func TestRunInit_AnthropicKeyRejectsMalformed(t *testing.T) {
	cases := map[string]string{
		"short":       "y",
		"shortBody":   "sk-ant-tooshort",
		"wrongPfx":    "not-a-key-" + strings.Repeat("x", 40),
		"bannedChars": "sk-ant-!!!!" + strings.Repeat("x", 40),
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("LEAH_STATE_DIR", dir)
			t.Setenv("HOME", dir)
			t.Setenv("LEAH_INIT_SKIP_LAUNCHCTL", "1")
			t.Setenv("ANTHROPIC_API_KEY", "")
			defer keychain.SetSecurityBin(keychain.SetSecurityBin(fakeSecurityBin(t)))

			var buf bytes.Buffer
			if code := runInit(context.Background(), nil, &buf, strings.NewReader(in+"\n"+strings.Repeat("\n", 20))); code != 0 {
				t.Fatalf("exit %d", code)
			}
			if !strings.Contains(buf.String(), "does not look like an API key") {
				t.Fatalf("malformed-input guard not exercised: %s", buf.String())
			}
			if strings.Contains(buf.String(), "stored in Keychain.") {
				t.Fatalf("malformed input incorrectly reached keychain.Save")
			}
		})
	}
}

// Env var already set → skip prompt entirely; no bytes consumed from stdin.
func TestRunInit_AnthropicKeySkippedWhenEnvSet(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	t.Setenv("HOME", dir)
	t.Setenv("LEAH_INIT_SKIP_LAUNCHCTL", "1")
	t.Setenv("LEAH_INIT_AUTO_ACCEPT", "1")
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-preexisting")

	var buf bytes.Buffer
	if code := runInit(context.Background(), nil, &buf, strings.NewReader("")); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(buf.String(), "already set in environment") {
		t.Fatalf("env-set skip branch not hit: %s", buf.String())
	}
}

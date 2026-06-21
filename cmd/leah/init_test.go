package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	// No AUTO_ACCEPT — exercise the live prompt.

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

	var buf bytes.Buffer
	if code := runInit(context.Background(), nil, &buf, errReader{err: errBrokenPipe{}}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(buf.String(), "input error") {
		t.Fatalf("error branch not exercised: %s", buf.String())
	}
}

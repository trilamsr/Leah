package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/trilam/leah/internal/adapters/launcher"
)

// fakeOpenExec captures `open` calls so the CLI test stays hermetic.
type fakeOpenExec struct {
	called   int
	lastArgs []string
	err      error
}

func (f *fakeOpenExec) Run(_ context.Context, _ string, args []string, _ []byte) ([]byte, error) {
	f.called++
	f.lastArgs = args
	return nil, f.err
}

func withFakeLauncher(t *testing.T, ex launcher.ExecRunner) func() {
	t.Helper()
	prev := openExecFactory
	openExecFactory = func() launcher.ExecRunner { return ex }
	return func() { openExecFactory = prev }
}

func TestOpenCLI_HappyPath_PrintsConfirmation(t *testing.T) {
	ex := &fakeOpenExec{}
	defer withFakeLauncher(t, ex)()
	var buf bytes.Buffer
	code := runOpen(context.Background(), []string{"netflix"}, &buf)
	if code != 0 {
		t.Fatalf("exit=%d, want 0; out=%q", code, buf.String())
	}
	if !strings.Contains(strings.ToLower(buf.String()), "netflix") {
		t.Fatalf("output missing target name: %q", buf.String())
	}
	if ex.called != 1 {
		t.Fatalf("exec called %d times, want 1", ex.called)
	}
}

func TestOpenCLI_UnknownTarget_ExitsNonzero(t *testing.T) {
	ex := &fakeOpenExec{}
	defer withFakeLauncher(t, ex)()
	var buf bytes.Buffer
	code := runOpen(context.Background(), []string{"nosuchapp"}, &buf)
	if code == 0 {
		t.Fatalf("unknown target: got exit 0, want non-zero")
	}
}

func TestOpenCLI_MissingArg_PrintsUsage(t *testing.T) {
	ex := &fakeOpenExec{}
	defer withFakeLauncher(t, ex)()
	var buf bytes.Buffer
	code := runOpen(context.Background(), nil, &buf)
	if code != 2 {
		t.Fatalf("missing arg: got exit %d, want 2", code)
	}
}

func TestOpenCLI_HelpFlag_PrintsUsage(t *testing.T) {
	var buf bytes.Buffer
	code := runOpen(context.Background(), []string{"--help"}, &buf)
	if code != 0 {
		t.Fatalf("--help: got exit %d, want 0", code)
	}
	if !strings.Contains(strings.ToLower(buf.String()), "usage") {
		t.Fatalf("--help missing usage: %q", buf.String())
	}
}

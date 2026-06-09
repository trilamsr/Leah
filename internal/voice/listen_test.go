package voice

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestListenHappyPath asserts Listen shells out to sox then whisper-cli and
// returns the trimmed contents of the .txt file whisper-cli wrote.
func TestListenHappyPath(t *testing.T) {
	dir := t.TempDir()
	wav := filepath.Join(dir, "leah.wav")
	txt := filepath.Join(dir, "leah")

	fe := newFakeExec()
	fe.lookupOK["sox"] = true
	fe.lookupOK["whisper-cli"] = true

	// Simulate sox writing the wav.
	fe.runOut["sox"] = nil
	soxWrote := false
	whisperWrote := false
	fe.runHook = func(name string, args []string) {
		switch name {
		case "sox":
			_ = os.WriteFile(wav, []byte("RIFFFAKE"), 0o600)
			soxWrote = true
		case "whisper-cli":
			_ = os.WriteFile(txt+".txt", []byte("  hello world  \n"), 0o600)
			whisperWrote = true
		}
	}

	got, err := Listen(context.Background(), fe, ListenConfig{
		WavPath: wav,
		TxtPath: txt,
	})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	if got != "hello world" {
		t.Errorf("transcript = %q, want %q", got, "hello world")
	}
	if !soxWrote || !whisperWrote {
		t.Errorf("hooks not fired: sox=%v whisper=%v", soxWrote, whisperWrote)
	}
	if len(fe.runs) != 2 {
		t.Fatalf("runs = %d, want 2", len(fe.runs))
	}
	if fe.runs[0].name != "sox" {
		t.Errorf("first call = %q, want sox", fe.runs[0].name)
	}
	if fe.runs[1].name != "whisper-cli" {
		t.Errorf("second call = %q, want whisper-cli", fe.runs[1].name)
	}
}

// TestListenSoxMissing returns an actionable install-hint error.
func TestListenSoxMissing(t *testing.T) {
	fe := newFakeExec()
	// sox absent; whisper-cli irrelevant since sox is probed first.
	_, err := Listen(context.Background(), fe, ListenConfig{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "sox") || !strings.Contains(err.Error(), "brew install sox") {
		t.Errorf("error missing install hint: %v", err)
	}
}

// TestListenWhisperMissing returns an actionable install-hint error.
func TestListenWhisperMissing(t *testing.T) {
	fe := newFakeExec()
	fe.lookupOK["sox"] = true
	_, err := Listen(context.Background(), fe, ListenConfig{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "whisper-cli") || !strings.Contains(err.Error(), "whisper-cpp") {
		t.Errorf("error missing install hint: %v", err)
	}
}

// TestListenSoxFailure surfaces the underlying error.
func TestListenSoxFailure(t *testing.T) {
	fe := newFakeExec()
	fe.lookupOK["sox"] = true
	fe.lookupOK["whisper-cli"] = true
	fe.runErr["sox"] = errors.New("audio device busy")

	_, err := Listen(context.Background(), fe, ListenConfig{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "sox record") || !strings.Contains(err.Error(), "audio device busy") {
		t.Errorf("error missing context: %v", err)
	}
}

// TestListenEmptyWav fails when sox returned success but produced no audio.
func TestListenEmptyWav(t *testing.T) {
	dir := t.TempDir()
	wav := filepath.Join(dir, "leah.wav")

	fe := newFakeExec()
	fe.lookupOK["sox"] = true
	fe.lookupOK["whisper-cli"] = true
	// sox "succeeds" but writes nothing.
	_, err := Listen(context.Background(), fe, ListenConfig{WavPath: wav})
	if err == nil {
		t.Fatal("expected empty-audio error")
	}
	if !strings.Contains(err.Error(), "no audio") {
		t.Errorf("error missing context: %v", err)
	}
}

// TestListenWhisperFailure surfaces the underlying error.
func TestListenWhisperFailure(t *testing.T) {
	dir := t.TempDir()
	wav := filepath.Join(dir, "leah.wav")

	fe := newFakeExec()
	fe.lookupOK["sox"] = true
	fe.lookupOK["whisper-cli"] = true
	fe.runErr["whisper-cli"] = errors.New("model file missing")
	fe.runHook = func(name string, args []string) {
		if name == "sox" {
			_ = os.WriteFile(wav, []byte("RIFFFAKE"), 0o600)
		}
	}
	_, err := Listen(context.Background(), fe, ListenConfig{WavPath: wav})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "whisper-cli") || !strings.Contains(err.Error(), "model file missing") {
		t.Errorf("error missing context: %v", err)
	}
}

// TestListenDurationFlag asserts the -trim arg is appended when Duration > 0.
func TestListenDurationFlag(t *testing.T) {
	dir := t.TempDir()
	wav := filepath.Join(dir, "leah.wav")
	txt := filepath.Join(dir, "leah")

	fe := newFakeExec()
	fe.lookupOK["sox"] = true
	fe.lookupOK["whisper-cli"] = true
	fe.runHook = func(name string, args []string) {
		switch name {
		case "sox":
			_ = os.WriteFile(wav, []byte("RIFFFAKE"), 0o600)
		case "whisper-cli":
			_ = os.WriteFile(txt+".txt", []byte("ok"), 0o600)
		}
	}
	if _, err := Listen(context.Background(), fe, ListenConfig{
		Duration: 30 * time.Second, WavPath: wav, TxtPath: txt,
	}); err != nil {
		t.Fatalf("listen: %v", err)
	}
	soxArgs := fe.runs[0].args
	foundTrim := false
	for i, a := range soxArgs {
		if a == "trim" && i+2 < len(soxArgs) && soxArgs[i+2] == "30.00" {
			foundTrim = true
		}
	}
	if !foundTrim {
		t.Errorf("sox args missing trim 0 30.00: %v", soxArgs)
	}
}

package voice

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestVoiceSource_ReadsMarkdownFilesFromDir(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "morning.md"), "shipped the source layer")
	writeFile(t, filepath.Join(dir, "evening.txt"), "tomorrow: wire CLI")

	s := New(Config{Dir: dir})
	facts, err := s.Candidates(context.Background(), time.Time{})
	if err != nil {
		t.Fatalf("Candidates: %v", err)
	}
	if len(facts) != 2 {
		t.Fatalf("facts = %d, want 2: %+v", len(facts), facts)
	}
	for _, f := range facts {
		if f.Slot != "voice" {
			t.Errorf("Slot = %q, want voice", f.Slot)
		}
		if f.Origin == "" || !strings.HasPrefix(f.Origin, dir) {
			t.Errorf("Origin not absolute under dir: %q", f.Origin)
		}
	}
}

func TestVoiceSource_IgnoresNonTextFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "real.md"), "keep me")
	writeFile(t, filepath.Join(dir, "clip.mp3"), "binary")
	writeFile(t, filepath.Join(dir, "clip.wav"), "binary")
	writeFile(t, filepath.Join(dir, ".DS_Store"), "junk")

	s := New(Config{Dir: dir})
	facts, err := s.Candidates(context.Background(), time.Time{})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(facts) != 1 {
		t.Fatalf("want 1 fact, got %d: %+v", len(facts), facts)
	}
}

func TestVoiceSource_EmptyDir_ReturnsEmpty(t *testing.T) {
	s := New(Config{Dir: t.TempDir()})
	facts, err := s.Candidates(context.Background(), time.Time{})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(facts) != 0 {
		t.Fatalf("want empty, got %+v", facts)
	}
}

func TestVoiceSource_DirMissing_ReturnsErr(t *testing.T) {
	s := New(Config{Dir: "/no/such/dir/xyzzy-leah-test"})
	_, err := s.Candidates(context.Background(), time.Time{})
	if err == nil {
		t.Fatal("want error, got nil")
	}
}

func TestVoiceSource_SkipsOversizeFiles(t *testing.T) {
	dir := t.TempDir()
	small := filepath.Join(dir, "small.md")
	big := filepath.Join(dir, "big.md")
	writeFile(t, small, "tiny note")
	// Write file larger than the per-file cap (default 1 MiB).
	big_body := strings.Repeat("x", maxFileBytes+1)
	writeFile(t, big, big_body)

	s := New(Config{Dir: dir})
	facts, err := s.Candidates(context.Background(), time.Time{})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(facts) != 1 {
		t.Fatalf("want 1 (oversize skipped), got %d: %+v", len(facts), facts)
	}
}

func TestVoiceSource_FiltersBySince(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "old.md")
	fresh := filepath.Join(dir, "fresh.md")
	writeFile(t, old, "stale")
	writeFile(t, fresh, "new")
	past := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(old, past, past); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	s := New(Config{Dir: dir})
	facts, err := s.Candidates(context.Background(), time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(facts) != 1 {
		t.Fatalf("want 1 fresh, got %d", len(facts))
	}
}

func TestVoiceSource_Name(t *testing.T) {
	s := New(Config{Dir: "/tmp"})
	if s.Name() != "voice" {
		t.Errorf("Name = %q, want voice", s.Name())
	}
}

package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/trilam/leah/internal/regattaclient"
)

// stubRegatta returns an empty agent list — keeps buildBriefTask off the
// real `regatta` binary in tests.
type stubRegatta struct{}

func (stubRegatta) List(_ context.Context) ([]regattaclient.Agent, error) {
	return nil, nil
}

// TestBriefTaskWritesFile asserts buildBriefTask writes the brief markdown
// to <sd>/briefs/YYYY-MM-DD.md when invoked. Voice/desktop side-effects are
// gated by LEAH_VOICE_ENABLED and stay off in this test — only the file
// surface is asserted (the unit-of-work that matters for the daily cron).
func TestBriefTaskWritesFile(t *testing.T) {
	sd := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", sd)
	t.Setenv("LEAH_VOICE_ENABLED", "0")

	out := &bytes.Buffer{}
	task := buildBriefTask(sd, stubRegatta{}, os.Stdout)
	_ = out

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	task(ctx)

	want := filepath.Join(sd, "briefs", time.Now().Format("2006-01-02")+".md")
	body, err := os.ReadFile(want)
	if err != nil {
		t.Fatalf("brief file not written at %s: %v", want, err)
	}
	if len(body) == 0 {
		t.Errorf("brief file is empty")
	}
	for _, want := range []string{"# leah brief", "## Yesterday", "## Cost"} {
		if !bytes.Contains(body, []byte(want)) {
			t.Errorf("brief missing %q section in:\n%s", want, body)
		}
	}
}

// TestBriefTaskIdempotentOverwriteSameDay asserts a second fire on the same
// calendar day overwrites the prior file rather than appending — critical
// for the daily cron when the daemon restarts after a partial write.
func TestBriefTaskIdempotentOverwriteSameDay(t *testing.T) {
	sd := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", sd)
	t.Setenv("LEAH_VOICE_ENABLED", "0")

	task := buildBriefTask(sd, stubRegatta{}, os.Stdout)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	task(ctx)
	path := filepath.Join(sd, "briefs", time.Now().Format("2006-01-02")+".md")
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("first fire: %v", err)
	}
	task(ctx)
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("second fire: %v", err)
	}
	if len(second) > 2*len(first) {
		t.Errorf("second fire appended instead of overwriting (first=%d second=%d)",
			len(first), len(second))
	}
}

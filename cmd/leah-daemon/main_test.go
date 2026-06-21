package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
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

	task := buildBriefTask(sd, stubRegatta{}, os.Stdout, nil)

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

// TestBriefOptsSilentWhenUnconfigured asserts no token files → no listers and no crash.
func TestBriefOptsSilentWhenUnconfigured(t *testing.T) {
	sd := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", sd)
	o := briefOpts(sd)
	if o.Gmail != nil || o.Gcal != nil {
		t.Errorf("unconfigured integrations must yield nil listers, got gmail=%v gcal=%v", o.Gmail, o.Gcal)
	}
}

// TestBriefTaskOmitsSectionsWhenUnconfigured asserts the daemon brief stays silent (no "unavailable") with no tokens.
func TestBriefTaskOmitsSectionsWhenUnconfigured(t *testing.T) {
	sd := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", sd)
	t.Setenv("LEAH_VOICE_ENABLED", "0")
	task := buildBriefTask(sd, stubRegatta{}, os.Stdout, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	task(ctx)
	body, err := os.ReadFile(filepath.Join(sd, "briefs", time.Now().Format("2006-01-02")+".md"))
	if err != nil {
		t.Fatalf("brief not written: %v", err)
	}
	if strings.Contains(string(body), "unavailable") {
		t.Errorf("unconfigured integrations leaked an 'unavailable' section:\n%s", body)
	}
}

// TestBriefNotifierFansToConfiguredChannels asserts pushover joins desktop+voice when its creds are set.
func TestBriefNotifierFansToConfiguredChannels(t *testing.T) {
	t.Setenv("LEAH_PUSHOVER_USER", "u")
	t.Setenv("LEAH_PUSHOVER_TOKEN", "tok")
	f := buildBriefNotifier(nil)
	if got := len(f.Notifiers); got != 3 {
		t.Errorf("pushover+desktop+voice = 3 channels, got %d", got)
	}
}

// TestBriefNotifierSkipsUnconfiguredChannels asserts pushover is dropped when its creds are absent.
func TestBriefNotifierSkipsUnconfiguredChannels(t *testing.T) {
	t.Setenv("LEAH_PUSHOVER_USER", "")
	t.Setenv("LEAH_PUSHOVER_TOKEN", "")
	f := buildBriefNotifier(nil)
	if got := len(f.Notifiers); got != 2 {
		t.Errorf("desktop+voice = 2 channels when pushover absent, got %d", got)
	}
}

// TestBriefTaskIdempotentOverwriteSameDay asserts a second fire on the same
// calendar day overwrites the prior file rather than appending — critical
// for the daily cron when the daemon restarts after a partial write.
func TestBriefTaskIdempotentOverwriteSameDay(t *testing.T) {
	sd := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", sd)
	t.Setenv("LEAH_VOICE_ENABLED", "0")

	task := buildBriefTask(sd, stubRegatta{}, os.Stdout, nil)
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
